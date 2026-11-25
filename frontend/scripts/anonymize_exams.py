#!/usr/bin/env python3
"""Offline anonymizer for RefratIA exam documents (prototype, step 1 of the vision-extraction experiment).

  python3 frontend/scripts/anonymize_exams.py [patient-folder ...]   # default: every folder in frontend/data

For each frontend/data/<patient>/ source PDF/JPEG/PNG it writes a raster-only redacted PDF to
frontend/data/<patient>/anonymized/<TYPE>_<EYE>.pdf. Before/after previews, the status report and the
manual-correction file live in frontend/data/_anon-review/ (never inside anonymized/).

Detection is local only: pdftoppm renders pages, the tesseract CLI does OCR, simple rules find identifiers
(labelled fields such as Nome/Sobrenome/Nasc./ID/CPF, patient-name tokens, birth dates, CPF-like numbers).
Pages are rebuilt from the redacted pixels, so no original text layer, metadata or embedded file survives.

Manual correction: edit frontend/data/_anon-review/<patient>/overrides.json and re-run:
  "add":  {"<page>": [[x0, y0, x1, y1], ...]}   fractions of page width/height (draw them in index.html)
  "drop": {"<page>": [box index, ...]}           auto boxes to un-redact (indices are shown in the preview)
  "approved": true                                clears NEEDS_REVIEW after you have checked the preview
"""
import html, json, os, re, shutil, subprocess, sys, tempfile, unicodedata
from concurrent.futures import ProcessPoolExecutor
from pathlib import Path

from PIL import Image, ImageDraw, ImageOps, ImageStat

DATA = Path(__file__).resolve().parents[1] / "data"
REVIEW = DATA / "_anon-review"
OUT_DIR = "anonymized"
DPI = 300
EXTS = {".pdf", ".jpg", ".jpeg", ".png"}

NAME_LABELS = {"SOBRENOME", "NOME", "NAME", "PACIENTE", "PATIENT", "LASTNAME", "FIRSTNAME"}
ID_LABELS = {"ID", "CID", "NO", "RG", "CPF", "CPFCNPJ", "MATRICULA", "CODIGO", "PRONTUARIO",
             "NASC", "NASCIMENTO", "DOB", "TELEFONE", "TELEFONES"}
BIRTH_LABELS = {"NASC", "NASCIMENTO", "DOB"}
STOP_WORDS = {"OLHO", "EYE", "HORA", "DATA", "DATADEEXAME", "EXAME", "SEXO", "IDADE", "TIME", "MODE"}
NOT_NAMES = {"DE", "DA", "DO", "DOS", "DAS", "AO", "OD", "OE", "OS", "PENTACAM", "BIO", "BIOMETRIA", "SRK", "SRKT",
             "MICROSCOPIA", "ESPECULAR", "AUTO", "REFRACAO", "EYESUITE", "CORNEA", "RETINA", "TOPO", "HCV", "PDF",
             "JPEG", "JPG", "PNG", "HCOLHOS", "OLHOS", "HC"}
# ponytail: layouts whose identifiers are all printed fields; anything else (photos of receipts, scans with
# handwriting/signatures that OCR cannot read) is always NEEDS_REVIEW. Add a type here once its output has been checked.
KNOWN_LAYOUTS = {"PENTACAM", "BIOMETRIA", "MICROSCOPIA", "TOPOGRAFIA", "CORNEA", "RETINA"}
CLINICAL_WORDS = {"ESQUERDO", "DIREITO", "RIGHT", "LEFT"}
DOC_TYPES = [("PENTACAM", "PENTACAM"), ("BIO", "BIOMETRIA"), ("BIOMETRIA", "BIOMETRIA"), ("EYESUITE", "BIOMETRIA"),
             ("MICROSCOPIA", "MICROSCOPIA"), ("AUTO", "AUTO_REFRACAO"), ("TOPO", "TOPOGRAFIA"),
             ("CORNEA", "CORNEA"), ("RETINA", "RETINA")]


def norm(s):
    s = unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode().upper()
    return re.sub(r"[^A-Z0-9]", "", s)


def label_key(word):  # OCR confuses 1/I and 0/O in labels ("1D :" for "ID :")
    return norm(word).replace("1", "I").replace("0", "O")


def lev(a, b):
    prev = list(range(len(b) + 1))
    for i, ca in enumerate(a, 1):
        cur = [i]
        for j, cb in enumerate(b, 1):
            cur.append(min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + (ca != cb)))
        prev = cur
    return prev[-1]


def label_of(word):
    k = label_key(word)
    for lab in NAME_LABELS | ID_LABELS:
        if k == lab or (len(lab) >= 4 and len(k) >= 4 and k[0] == lab[0] and lev(k, lab) <= 1):
            return lab
    return None


def neutral_name(src, taken):
    toks = re.split(r"[^A-Z0-9]+", norm_keep_sep(src.stem))
    kind = next((v for k, v in DOC_TYPES if k in toks), "DOC")
    eye = next(({"OS": "OE"}.get(t, t) for t in toks if t in {"OD", "OE", "OS", "AO"}), None)
    base = kind + (f"_{eye}" if eye else "")
    name, n = base, 2
    while name in taken:
        name, n = f"{base}_{n}", n + 1
    taken.add(name)
    return name + ".pdf"


def norm_keep_sep(s):
    return unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode().upper()


# ---------------------------------------------------------------- OCR

def ocr(path):
    """Words of one page image: dicts with text, box (x0,y0,x1,y1 in page pixels), conf, row key."""
    im = Image.open(path).convert("L")
    if ImageStat.Stat(im).mean[0] < 110:  # light text on dark photos (retina/cornea overlays)
        im = ImageOps.invert(im)
    scale = max(1.0, 2400 / im.width)
    if scale > 1:
        im = im.resize((round(im.width * scale), round(im.height * scale)), Image.LANCZOS)
    words = []
    with tempfile.TemporaryDirectory() as td:
        p = Path(td) / "p.png"
        im.save(p)
        for psm in ("3", "11"):  # page layout + sparse text: headers often sit in tiny boxes
            tsv = subprocess.run(["tesseract", str(p), "-", "--psm", psm, "tsv"], capture_output=True, text=True,
                                 env={**os.environ, "OMP_THREAD_LIMIT": "1"}, timeout=600).stdout
            for line in tsv.splitlines()[1:]:
                f = line.split("\t")
                if len(f) < 12 or not f[11].strip():
                    continue
                x, y, w, h = (int(v) / scale for v in f[6:10])
                words.append({"text": f[11].strip(), "box": (x, y, x + w, y + h), "conf": float(f[10]),
                              "row": f"{psm}:{f[2]}:{f[3]}:{f[4]}"})
    return words


def same_row(a, b):
    ov = min(a[3], b[3]) - max(a[1], b[1])
    return ov >= 0.5 * min(a[3] - a[1], b[3] - b[1])


def label_values(words):
    """(label, label word, value words) for every identifying label found on the page."""
    out = []
    for i, w in enumerate(words):
        lab = label_of(w["text"])
        if not lab:
            continue
        lb, h = w["box"], w["box"][3] - w["box"][1]
        right = sorted((v for j, v in enumerate(words) if j != i and v["row"].split(":")[0] == w["row"].split(":")[0]
                        and same_row(lb, v["box"]) and v["box"][0] >= lb[2] - h * 0.3), key=lambda v: v["box"][0])
        vals, edge = [], lb[2]
        for v in right:
            # Pentacam value boxes sit in an aligned column well right of the label: allow a wide first gap
            k = label_key(v["text"])
            if v["box"][0] - edge > (10 if not vals else 3) * h or label_of(v["text"]) or \
                    any(k == s or (len(s) >= 4 and len(k) >= 3 and lev(k, s) <= 1) for s in STOP_WORDS):
                break
            if lab in BIRTH_LABELS and vals and not re.search(r"\d", v["text"]):
                break  # a birth date is digits; the next word is already the next field ("Olho: Direito")
            vals.append(v)
            edge = v["box"][2]
        # only field-like labels count: "Nome:", "Name :", "ID JOAO" (not "Nome da Aberração" column headers)
        nxt = vals[0]["text"] if vals else ""
        if not (re.search(r"[:;]$", w["text"]) or re.match(r"[:;]", nxt) or (lab == "ID" and re.fullmatch(r"[A-Za-z]{3,}", nxt))):
            continue
        out.append((lab, w, vals))
    return out


# ---------------------------------------------------------------- patient-level identifiers

def date_variants(d, m, y):
    y = int(y) + (1900 if int(y) > 30 and int(y) < 100 else 2000 if int(y) < 100 else 0)
    d, m = int(d), int(m)
    out = set()
    for a, b in ((d, m), (m, d)):
        out |= {f"{a:02}{b:02}{y}", f"{a:02}{b:02}{y % 100:02}", f"{y}{b:02}{a:02}"}
    return out


def collect_identifiers(folder, docs):
    tokens = {t for t in norm_keep_sep(folder.name).replace("-", " ").split() if len(t) >= 3}
    births = set()
    for d in docs:
        tokens |= {t for t in re.split(r"[^A-Z]+", norm_keep_sep(d["src"].stem)) if len(t) >= 3}
        for words in d["ocr"]:
            for lab, _, vals in label_values(words):
                text = " ".join(v["text"] for v in vals)
                if lab in NAME_LABELS or lab == "ID":  # confident, purely alphabetic words only: box borders OCR as junk
                    tokens |= {norm(v["text"]) for v in vals if v["conf"] >= 70 and re.fullmatch(r"[A-Za-zÀ-ÿ]{4,},?", v["text"])}
                if lab in BIRTH_LABELS:
                    for m in re.finditer(r"(\d{1,2})[/.-](\d{1,2})[/.-](\d{2,4})", text):
                        births |= date_variants(*m.groups())
            rows = {}
            for w in words:
                rows.setdefault(w["row"], []).append(w["text"])
            for r in rows.values():  # EyeSuite header: "SURNAME, NAME, 15/02/1966"
                m = re.search(r"[A-Za-z]{3,},\s*[A-Za-z ]{3,},\s*(\d{2})/(\d{2})/(\d{4})", " ".join(r))
                if m:
                    births |= date_variants(*m.groups())
    tokens = {t for t in tokens if len(t) >= 3 and not t.isdigit() and t not in NOT_NAMES
              and not any(c in t or lev(c, t) <= 1 for c in CLINICAL_WORDS)}  # "JEsquerdo" from the Olho box
    return sorted(tokens), births


def is_name(word, tokens):
    w = re.sub(r"[^A-Z]", "", norm(word))
    if len(w) < 3:
        return False
    for t in tokens:
        if w == t or (len(t) >= 4 and t in w) or (len(w) >= 5 and len(t) >= 5 and (lev(w, t) <= 1 or t.startswith(w))):
            return True
    return False


def is_birth(word, births):
    digits = re.sub(r"\D", "", word)
    return len(digits) >= 6 and any(digits == b or (len(digits) >= 8 and b in digits)
                                    or (len(digits) >= 7 and b.startswith(digits)) for b in births)  # "05/12/196" cut by a narrow box


CPF = re.compile(r"\d{3}\.\d{3}\.\d{3}-\d{2}|\d{11}")  # used with fullmatch on a stripped word


def page_boxes(words, tokens, births, size):
    """Auto redaction boxes (+ reasons the page needs review)."""
    boxes, review = [], []

    def add(b, h, why):
        pad = max(3, 0.2 * h)
        boxes.append({"box": [b[0] - pad, b[1] - pad, b[2] + pad, b[3] + pad], "why": why})

    labels = label_values(words)
    filled = [w["box"] for _, w, vals in labels if any(len(norm(v["text"])) >= 2 for v in vals)]
    for lab, w, vals in labels:
        h = w["box"][3] - w["box"][1]
        real = [v for v in vals if len(norm(v["text"])) >= 2]
        if not real and (len(lab) <= 3 or any(same_row(w["box"], f) and f[0] < w["box"][2] and w["box"][0] < f[2] for f in filled)):
            continue  # short labels need a value; the other OCR pass already read this field
        if real:
            x0 = min(v["box"][0] for v in vals); x1 = max(v["box"][2] for v in vals)
            y0 = min(v["box"][1] for v in vals + [w]); y1 = max(v["box"][3] for v in vals + [w])
            add((x0, y0, x1, y1), h, f"{lab} field")
        else:  # empty or unreadable field: blank a strip after the label anyway
            add((w["box"][2], w["box"][1], w["box"][2] + (25 if lab in NAME_LABELS else 12) * h, w["box"][3]), h,
                f"{lab} field (no OCR value)")
            if lab in NAME_LABELS:
                review.append(f"{lab} label without readable value (handwriting?)")
    for w in words:
        b, h = w["box"], w["box"][3] - w["box"][1]
        if is_name(w["text"], tokens):
            add(b, h, "name token")
        elif is_birth(w["text"], births):
            add(b, h, "birth date")
        elif CPF.fullmatch(w["text"].strip(".,;:|[]()")):
            add(b, h, "CPF-like number")
    W, H = size
    for bx in boxes:
        x0, y0, x1, y1 = bx["box"]
        bx["box"] = [max(0, x0), max(0, y0), min(W, x1), min(H, y1)]
    return boxes, review


# ---------------------------------------------------------------- pipeline

def load_pages(src, tmp):
    if src.suffix.lower() == ".pdf":
        subprocess.run(["pdftoppm", "-r", str(DPI), "-png", str(src), str(tmp / "p")], check=True)
        return sorted(tmp.glob("p-*.png")), DPI
    p = tmp / "p-1.png"
    Image.open(src).convert("RGB").save(p)
    return [p], 100  # ponytail: photos have no physical size; 100 px/in gives a sensible PDF page


def redact(page_path, boxes, out_path):
    im = Image.open(page_path).convert("RGB")
    d = ImageDraw.Draw(im)
    for b in boxes:
        d.rectangle(b, fill=(0, 0, 0))
    im.save(out_path)
    return out_path


def preview(before_path, after_path, boxes, dest, width=1600):
    im = Image.open(before_path).convert("RGB")
    s = width / im.width
    d = ImageDraw.Draw(im)
    for i, b in enumerate(boxes):
        d.rectangle(b["box"], outline=(255, 0, 0), width=max(3, int(3 / s)))
        d.text((b["box"][0], max(0, b["box"][1] - 14 / s)), str(i), fill=(255, 0, 0), font_size=int(14 / s))
    for img, name in ((im, "before"), (Image.open(after_path).convert("RGB"), "after")):
        img.resize((width, round(img.height * s)), Image.LANCZOS).save(dest / f"{name}.jpg", quality=90)


def write_pdf(pages, out, res):
    ims = [Image.open(p).convert("RGB") for p in pages]
    ims[0].save(out, "PDF", save_all=True, append_images=ims[1:], resolution=res, quality=95, subsampling=0,
                title="anonymized", author="", subject="", keywords="", creator="", producer="")


def process_patient(folder, pool):
    review_dir = REVIEW / folder.name
    review_dir.mkdir(parents=True, exist_ok=True)
    ov_path = review_dir / "overrides.json"
    overrides = json.loads(ov_path.read_text()) if ov_path.exists() else {}
    out_dir = folder / OUT_DIR
    if out_dir.exists():
        shutil.rmtree(out_dir)  # generated output only; originals are never written
    out_dir.mkdir()
    srcs = sorted(p for p in folder.iterdir() if p.is_file() and p.suffix.lower() in EXTS)
    taken, docs = set(), []
    tmp_root = Path(tempfile.mkdtemp(prefix="anon-"))
    try:
        for i, src in enumerate(srcs):
            tmp = tmp_root / str(i); tmp.mkdir()
            pages, res = load_pages(src, tmp)
            docs.append({"src": src, "out": neutral_name(src, taken), "pages": pages, "res": res, "tmp": tmp})
        flat = [p for d in docs for p in d["pages"]]
        words = dict(zip(flat, pool.map(ocr, flat)))
        for d in docs:
            d["ocr"] = [words[p] for p in d["pages"]]
        tokens, births = collect_identifiers(folder, docs)

        jobs = []
        for d in docs:
            ov = overrides.setdefault(d["src"].name, {"approved": False, "add": {}, "drop": {}})
            d["reasons"], d["boxes"], d["redacted"] = [], [], []
            for n, (page, ws) in enumerate(zip(d["pages"], d["ocr"]), 1):
                size = Image.open(page).size
                boxes, why = page_boxes(ws, tokens, births, size)
                drop = set(ov.get("drop", {}).get(str(n), []))
                boxes = [b for k, b in enumerate(boxes) if k not in drop]
                for f in ov.get("add", {}).get(str(n), []):
                    boxes.append({"box": [f[0] * size[0], f[1] * size[1], f[2] * size[0], f[3] * size[1]], "why": "manual"})
                d["reasons"] += [f"p{n}: {r}" for r in why]
                d["boxes"].append(boxes)
                d["redacted"].append(d["tmp"] / f"r-{n}.png")
                jobs.append((page, [b["box"] for b in boxes], d["redacted"][-1]))
        list(pool.map(redact, *zip(*jobs)))

        # verification: OCR the redacted pixels again; anything still matching is covered and flagged
        flat_r = [p for d in docs for p in d["redacted"]]
        again = dict(zip(flat_r, pool.map(ocr, flat_r)))
        for d in docs:
            for n, rp in enumerate(d["redacted"], 1):
                left = [w for w in again[rp] if is_name(w["text"], tokens) or is_birth(w["text"], births) or CPF.fullmatch(w["text"].strip(".,;:|[]()"))]
                if left:
                    d["reasons"].append(f"p{n}: identifier still visible after first pass ({', '.join(sorted({w['text'] for w in left}))}) - covered, check it")
                    extra, _ = page_boxes(left, tokens, births, Image.open(rp).size)
                    d["boxes"][n - 1] += [dict(b, why=b["why"] + " (verify pass)") for b in extra]
                    redact(d["pages"][n - 1], [b["box"] for b in d["boxes"][n - 1]], rp)
            if Path(d["out"]).stem.split("_")[0] not in KNOWN_LAYOUTS:
                d["reasons"].append("free-form photo/scan: handwriting, signatures and stamps are invisible to OCR")
            if not any(d["boxes"]):
                d["reasons"].append("no identifier detected anywhere - OCR may have missed it")

        report = []
        for d in docs:
            out = out_dir / d["out"]
            write_pdf(d["redacted"], out, d["res"])
            leaked = subprocess.run(["pdftotext", str(out), "-"], capture_output=True, text=True).stdout.strip()
            if leaked:
                d["reasons"].append("output PDF has a text layer")
            raw = out.read_bytes().upper()
            if any(t.encode() in raw for t in tokens if len(t) >= 4):
                d["reasons"].append("a name token appears in the PDF bytes")
            approved = overrides[d["src"].name].get("approved", False)
            status = "APPROVED" if approved else ("NEEDS_REVIEW" if d["reasons"] else "OK")
            pv = review_dir / Path(d["out"]).stem
            if pv.exists():
                shutil.rmtree(pv)
            pv.mkdir()
            for n, (b, a, boxes) in enumerate(zip(d["pages"], d["redacted"], d["boxes"]), 1):
                pd = pv / f"p{n}"; pd.mkdir()
                preview(b, a, boxes, pd)
            report.append({"source": d["src"].name, "output": str(out.relative_to(DATA.parent.parent)),
                           "pages": len(d["pages"]), "status": status, "reasons": d["reasons"],
                           "boxes": [[{"i": k, "why": b["why"], "box": [round(v) for v in b["box"]]} for k, b in enumerate(p)]
                                     for p in d["boxes"]]})
        ov_path.write_text(json.dumps(overrides, indent=2, ensure_ascii=False))
        (review_dir / "report.json").write_text(json.dumps(report, indent=2, ensure_ascii=False))
        write_html(review_dir, folder.name, report)
        return report
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)


HTML_JS = """<script>
// drag on a 'before' image to get an overrides.json rectangle (fractions of the page)
document.querySelectorAll('img.b').forEach(img => {
  let s = null; const out = img.parentElement.querySelector('code');
  const pt = e => { const r = img.getBoundingClientRect(); return [(e.clientX - r.left) / r.width, (e.clientY - r.top) / r.height]; };
  img.addEventListener('mousedown', e => { e.preventDefault(); s = pt(e); });
  img.addEventListener('mouseup', e => { if (!s) return; const p = pt(e);
    out.textContent = JSON.stringify([Math.min(s[0],p[0]),Math.min(s[1],p[1]),Math.max(s[0],p[0]),Math.max(s[1],p[1])].map(v => +v.toFixed(4))); s = null; });
});
</script>"""


def write_html(review_dir, patient, report):
    e = html.escape
    parts = [f"<!doctype html><meta charset=utf-8><title>review {e(patient)}</title>",
             "<style>body{font:14px sans-serif;margin:1em}img{width:49%;cursor:crosshair}.NEEDS_REVIEW{color:#c00}"
             ".OK,.APPROVED{color:#080}section{border-top:2px solid #999;margin-top:2em}</style>",
             f"<h1>{e(patient)}</h1><p>Left: original with auto/manual boxes (red, numbered). Right: anonymized output. "
             "Drag on the left image to get a rectangle for <code>overrides.json</code>.</p>"]
    for r in report:
        stem = Path(r["output"]).stem
        parts.append(f"<section><h2>{e(stem)}.pdf <span class={r['status']}>{r['status']}</span></h2>"
                     f"<ul>{''.join(f'<li>{e(x)}</li>' for x in r['reasons'])}</ul>")
        for n, boxes in enumerate(r["boxes"], 1):
            why = ", ".join(f"{b['i']}:{b['why']}" for b in boxes)
            parts.append(f"<h3>page {n}</h3><small>{e(why)}</small><div><img class=b src='{stem}/p{n}/before.jpg'>"
                         f" <img src='{stem}/p{n}/after.jpg'><br>rect: <code></code></div>")
        parts.append("</section>")
    (review_dir / "index.html").write_text("\n".join(parts) + HTML_JS)


def main():
    names = sys.argv[1:] or sorted(p.name for p in DATA.iterdir() if p.is_dir())
    folders = [DATA / n for n in names if not n.startswith("_") and n != OUT_DIR and (DATA / n).is_dir()]
    summary = {}
    with ProcessPoolExecutor() as pool:
        for f in folders:
            rep = process_patient(f, pool)
            summary[f.name] = rep
            for r in rep:
                print(f"{r['status']:13} {f.name}/{OUT_DIR}/{Path(r['output']).name}  ({r['pages']} p)  {'; '.join(r['reasons'])}")
    allr = [r for rep in summary.values() for r in rep]
    print(f"\nsources={len(allr)} ok={sum(r['status'] == 'OK' for r in allr)} approved={sum(r['status'] == 'APPROVED' for r in allr)}"
          f" needs_review={sum(r['status'] == 'NEEDS_REVIEW' for r in allr)}  previews: {REVIEW}/<patient>/index.html")


if __name__ == "__main__":
    main()
