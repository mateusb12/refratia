package main

// -profile: the server writes timing spans (src/shared/profiling) to
// <out>/timings.jsonl, the harness appends its own client-side spans to the
// same file (src "bench"), and summarizeProfile turns it into profile.md and
// profile.json. -profile-summary re-summarizes any timings.jsonl offline.
//
// Spans carry timings, counts and outcome classes only. Requests are labelled
// "p<n>/<step>" (n = position in the patient list), never the folder name.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type profSpan struct {
	TS      int64          `json:"ts_us"`
	Dur     int64          `json:"dur_us"`
	Src     string         `json:"src"`
	Run     string         `json:"run"`
	Req     string         `json:"req,omitempty"`
	Kind    string         `json:"kind,omitempty"`
	Label   string         `json:"label,omitempty"`
	Doc     *int           `json:"doc,omitempty"`
	Stage   string         `json:"stage"`
	Outcome string         `json:"outcome"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

type profiler struct {
	path, run string
	mu        sync.Mutex
}

// nil profiler (no -profile): every method is a no-op.
func (p *profiler) span(stage, label string, start time.Time, err error, attrs map[string]any) {
	if p == nil {
		return
	}
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	b, _ := json.Marshal(profSpan{TS: start.UnixMicro(), Dur: time.Since(start).Microseconds(), Src: "bench", Run: p.run,
		Label: label, Stage: stage, Outcome: outcome, Attrs: attrs})
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ferr := os.OpenFile(p.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if ferr != nil {
		return
	}
	f.Write(append(b, '\n'))
	f.Close()
}

// ---- summary ----

type interval struct{ a, b int64 }

// union returns the total length covered by the intervals, clipped to [lo, hi].
func union(iv []interval, lo, hi int64) int64 {
	sort.Slice(iv, func(i, j int) bool { return iv[i].a < iv[j].a })
	var total, curA, curB int64 = 0, -1, -1
	for _, x := range iv {
		a, b := max(x.a, lo), min(x.b, hi)
		if b <= a {
			continue
		}
		if curB < 0 || a > curB {
			if curB > curA {
				total += curB - curA
			}
			curA, curB = a, b
		} else if b > curB {
			curB = b
		}
	}
	if curB > curA {
		total += curB - curA
	}
	return total
}

func (s *profSpan) end() int64 { return s.TS + s.Dur }

func attrF(s *profSpan, k string) float64 {
	v, _ := s.Attrs[k].(float64)
	return v
}

func attrS(s *profSpan, k string) string {
	switch v := s.Attrs[k].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// leafStage groups spans into the stages the report ranks. Container spans
// (request, handler, phases, doc, classify, extract, pdf.inspect, parallel.map)
// return "" here: they are reported separately, never summed with leaves.
func leafStage(s *profSpan) string {
	st := s.Stage
	switch {
	case st == "ocr.recognize", strings.HasPrefix(st, "proc."), st == "pdf.render",
		strings.HasPrefix(st, "garage."), strings.HasPrefix(st, "sql."):
		return st
	case strings.HasPrefix(st, "preprocess."):
		return "preprocess (Go)"
	case strings.HasPrefix(st, "parse."), strings.HasPrefix(st, "validate."):
		return "parse/validate (Go)"
	}
	return ""
}

type stageAgg struct {
	Stage     string  `json:"stage"`
	Calls     int     `json:"calls"`
	Failed    int     `json:"failed"`
	SumMS     float64 `json:"summed_ms"`
	CoveredMS float64 `json:"covered_wall_ms"` // elapsed time with >=1 call running: the most removing it could save
	CPUms     float64 `json:"child_cpu_ms,omitempty"`
	FirstMS   float64 `json:"first_call_ms"`
	MedianMS  float64 `json:"median_ms"`
	Docs      string  `json:"triggered_by,omitempty"`
	iv        []interval
	durs      []float64
	types     map[string]int
}

type profileSummary struct {
	Run       string             `json:"run"`
	Info      map[string]any     `json:"startup_info"`
	Startup   map[string]float64 `json:"startup_ms"`
	Requests  []map[string]any   `json:"requests"`
	Docs      []map[string]any   `json:"documents"`
	Stages    []*stageAgg        `json:"fresh_analyze_stages"`
	Reingest  []*stageAgg        `json:"reingest_analyze_stages"`
	Callers   []map[string]any   `json:"ocr_callers"`
	Overlap   []map[string]any   `json:"overlap_by_request"`
	CallSites []*siteAgg         `json:"fresh_analyze_call_sites"`
	// re-ingest documents that reused a checkpoint; 0 of N means the second
	// analyze was a warm repeat, not a checkpoint-reuse measurement
	ReingestReused int      `json:"reingest_docs_reused"`
	ReingestDocs   int      `json:"reingest_docs"`
	Notes          []string `json:"notes"`
}

type siteAgg struct {
	Site       string  `json:"site"`
	Calls      int     `json:"calls"`
	SumMS      float64 `json:"summed_ms"`
	CoveredMS  float64 `json:"covered_wall_ms"`
	AloneMS    float64 `json:"only_call_in_flight_ms"` // no other call site (or second call of this one) running
	cov, alone int64
}

type tagged struct {
	a, b int64
	key  string
}

// sweep walks [lo, hi) in segments between span boundaries and calls fn with
// each segment's length and the number of spans of each key in flight.
func sweep(ev []tagged, lo, hi int64, fn func(dt int64, active map[string]int)) {
	type edge struct {
		t   int64
		d   int
		key string
	}
	var es []edge
	for _, e := range ev {
		a, b := max(e.a, lo), min(e.b, hi)
		if b > a {
			es = append(es, edge{a, 1, e.key}, edge{b, -1, e.key})
		}
	}
	sort.Slice(es, func(i, j int) bool { return es[i].t < es[j].t || es[i].t == es[j].t && es[i].d < es[j].d })
	active, t := map[string]int{}, lo
	for _, e := range es {
		if dt := e.t - t; dt > 0 {
			fn(dt, active)
		}
		t = e.t
		active[e.key] += e.d
	}
	if hi > t && hi != math.MaxInt64 {
		fn(hi-t, active)
	}
}

// overlapCat buckets leaf spans for the overlap timeline. ppm_to_png is left
// out because it runs inside pdf.render.
func overlapCat(s *profSpan) string {
	st := s.Stage
	switch {
	case st == "ocr.recognize", st == "proc.tesseract":
		return "ocr"
	case st == "pdf.render":
		return "render"
	case strings.HasPrefix(st, "preprocess.") && st != "preprocess.ppm_to_png":
		return "go_preprocess"
	case st == "proc.pdfinfo", st == "proc.pdftotext":
		return "pdf_inspect"
	case strings.HasPrefix(st, "garage."):
		return "garage"
	case strings.HasPrefix(st, "sql.") && st != "sql.transaction":
		return "sql"
	}
	return ""
}

// callSite names who triggered a leaf span: the parser helper for OCR, the
// wrapper and DPI for renders.
func callSite(s *profSpan) string {
	st := s.Stage
	switch {
	case st == "ocr.recognize":
		c := attrS(s, "caller")
		return "ocr:" + c[strings.LastIndex(c, "<")+1:]
	case st == "proc.tesseract":
		return "ocr:" + attrS(s, "site") + " (tesseract CLI)"
	case st == "pdf.render":
		c, _, _ := strings.Cut(attrS(s, "caller"), "<")
		return "render:" + c + "@" + attrS(s, "dpi") + "dpi"
	case strings.HasPrefix(st, "preprocess.") && st != "preprocess.ppm_to_png":
		return st
	case st == "proc.pdfinfo", st == "proc.pdftotext":
		return "pdfinfo+pdftotext"
	}
	return ""
}

// overlapAndCallSites answers "what was running while the request waited":
// per inspect/analyze/reingest request, how its wall time splits by which
// categories were in flight, and across fresh analyzes, how much wall time
// each call site covered and how much it was the only call in flight.
// Spans are attributed to a request by start time (one request at a time).
func overlapAndCallSites(spans []*profSpan, reqs []*profSpan) ([]map[string]any, []*siteAgg) {
	var rows []map[string]any
	sites := map[string]*siteAgg{}
	for _, r := range reqs {
		step := r.Label[strings.LastIndex(r.Label, "/")+1:]
		if step != "inspect" && step != "analyze" && step != "reingest" {
			continue
		}
		var cats, keyed []tagged
		for _, s := range spans {
			if s.Src != "server" || s.TS < r.TS || s.TS >= r.end() {
				continue
			}
			if c := overlapCat(s); c != "" {
				cats = append(cats, tagged{s.TS, s.end(), c})
			}
			if step == "analyze" {
				if k := callSite(s); k != "" {
					g := sites[k]
					if g == nil {
						g = &siteAgg{Site: k}
						sites[k] = g
					}
					g.Calls++
					g.SumMS += ms(s.Dur)
					keyed = append(keyed, tagged{s.TS, s.end(), k})
				}
			}
		}
		acc := map[string]int64{}
		var ocrConc int64
		sweep(cats, r.TS, r.end(), func(dt int64, act map[string]int) {
			var on []string
			for k, n := range act {
				if n > 0 {
					on = append(on, k)
				}
			}
			ocr := act["ocr"]
			switch {
			case len(on) == 0:
				acc["idle (no leaf span)"] += dt
			case ocr > 0 && len(on) == 1:
				acc["ocr only"] += dt
			case ocr > 0:
				acc["ocr + other"] += dt
			case len(on) == 1 && on[0] == "go_preprocess":
				acc["go preprocess only"] += dt
			case act["render"] > 0:
				acc["render, no ocr"] += dt
			default:
				acc["other, no ocr"] += dt
			}
			if ocr > 0 {
				acc["ocr_active"] += dt
				ocrConc += int64(ocr) * dt
			}
			if ocr == 1 {
				acc["ocr_exactly_one"] += dt
			}
		})
		row := map[string]any{"label": r.Label, "server_ms": ms(r.Dur)}
		for k, v := range acc {
			row[k] = ms(v)
		}
		if acc["ocr_active"] > 0 {
			row["avg_ocr_concurrency_when_active"] = math.Round(float64(ocrConc)/float64(acc["ocr_active"])*100) / 100
		}
		rows = append(rows, row)
		sweep(keyed, r.TS, r.end(), func(dt int64, act map[string]int) {
			var on []string
			for k, n := range act {
				if n > 0 {
					on = append(on, k)
				}
			}
			for _, k := range on {
				sites[k].cov += dt
			}
			if len(on) == 1 && act[on[0]] == 1 {
				sites[on[0]].alone += dt
			}
		})
	}
	var out []*siteAgg
	for _, g := range sites {
		g.SumMS, g.CoveredMS, g.AloneMS = math.Round(g.SumMS), math.Round(float64(g.cov)/1000), math.Round(float64(g.alone)/1000)
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CoveredMS > out[j].CoveredMS })
	return rows, out
}

func ms(us int64) float64 { return math.Round(float64(us)/10) / 100 }

func median(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	y := append([]float64{}, x...)
	sort.Float64s(y)
	return y[len(y)/2]
}

func summarizeProfile(path string) (*profileSummary, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var spans []*profSpan
	for _, line := range strings.Split(string(raw), "\n") {
		s := &profSpan{}
		if json.Unmarshal([]byte(line), s) == nil && s.Stage != "" {
			spans = append(spans, s)
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].TS < spans[j].TS })
	sum := &profileSummary{Info: map[string]any{}, Startup: map[string]float64{}}

	// requests and the client view of each, matched by label
	reqs := map[string]*profSpan{}
	var reqOrder []*profSpan
	client, buildBody := map[string]*profSpan{}, map[string]*profSpan{}
	var launch *profSpan
	for _, s := range spans {
		switch {
		case s.Src == "server" && s.Stage == "request":
			reqs[s.Req] = s
			reqOrder = append(reqOrder, s)
		case s.Src == "bench" && strings.HasPrefix(s.Stage, "client.") && s.Stage != "client.build_body":
			client[s.Label] = s
		case s.Src == "bench" && s.Stage == "client.build_body":
			buildBody[s.Label] = s
		case s.Src == "bench" && s.Stage == "bench.launch" && launch == nil:
			launch = s
		case s.Stage == "startup.info":
			sum.Info, sum.Run = s.Attrs, s.Run
		}
	}
	// ctx-less spans (preprocess, parse, SQL inside the confirm transaction)
	// belong to the request whose interval contains them: the harness sends
	// one request at a time, so containment is unambiguous.
	inReq := func(s *profSpan) *profSpan {
		if s.Req != "" {
			return reqs[s.Req]
		}
		i := sort.Search(len(reqOrder), func(i int) bool { return reqOrder[i].TS > s.TS }) - 1
		if i >= 0 && s.TS < reqOrder[i].end() {
			return reqOrder[i]
		}
		return nil
	}
	docs := map[string][]*profSpan{} // req -> doc spans
	for _, s := range spans {
		if s.Src == "server" && s.Stage == "doc" {
			docs[s.Req] = append(docs[s.Req], s)
		}
	}
	docOf := func(r *profSpan, s *profSpan) *profSpan {
		for _, d := range docs[r.Req] {
			if s.Doc != nil && d.Doc != nil && *s.Doc == *d.Doc {
				return d
			}
			if s.Doc == nil && s.TS >= d.TS && s.TS < d.end() {
				return d
			}
		}
		return nil
	}

	// startup
	var inits []float64
	for _, s := range spans {
		if strings.HasPrefix(s.Stage, "startup.") && s.Stage != "startup.info" {
			sum.Startup[strings.TrimPrefix(s.Stage, "startup.")+"_ms"] = ms(s.Dur)
			if s.Stage == "startup.db_open" && launch != nil {
				sum.Startup["exec_to_main_ms"] = ms(s.TS - launch.TS)
			}
		}
		if s.Stage == "ocr.engine_init" {
			inits = append(inits, ms(s.Dur))
		}
		if s.Src == "bench" && (s.Stage == "bench.build" || s.Stage == "bench.reset_infra") {
			sum.Startup[strings.TrimPrefix(s.Stage, "bench.")+"_ms (harness, not app)"] = ms(s.Dur)
		}
	}
	if launch != nil {
		sum.Startup["exec_to_healthy_ms"] = ms(launch.Dur)
	}
	if len(inits) > 0 {
		sum.Startup["ocr_engine_init_first_ms"] = inits[0]
		sum.Startup["ocr_engine_init_median_ms"] = median(inits)
	}

	// per-request aggregation
	fresh, reingest := map[string]*stageAgg{}, map[string]*stageAgg{}
	reqAggs := map[string]map[string]*stageAgg{}
	docAggs := map[*profSpan]map[string]*stageAgg{}
	callers := map[string]map[string]any{}
	add := func(m map[string]*stageAgg, key string, s *profSpan, a, b int64, docType string) {
		g := m[key]
		if g == nil {
			g = &stageAgg{Stage: key, types: map[string]int{}}
			m[key] = g
		}
		g.Calls++
		if s.Outcome != "ok" {
			g.Failed++
		}
		g.SumMS += ms(b - a)
		g.iv = append(g.iv, interval{a, b})
		g.durs = append(g.durs, ms(b-a))
		g.CPUms += attrF(s, "user_ms") + attrF(s, "sys_ms")
		if docType != "" {
			g.types[docType]++
		}
	}
	isFresh := func(r *profSpan) bool { return strings.HasSuffix(r.Label, "/analyze") }
	isReingest := func(r *profSpan) bool { return strings.HasSuffix(r.Label, "/reingest") }
	for _, s := range spans {
		if s.Src != "server" {
			continue
		}
		key := leafStage(s)
		if key == "" {
			continue
		}
		r := inReq(s)
		if r == nil {
			continue
		}
		docType := ""
		d := docOf(r, s)
		if d != nil {
			docType = attrS(d, "exam_type")
		}
		emit := func(key string, a, b int64) {
			if reqAggs[r.Req] == nil {
				reqAggs[r.Req] = map[string]*stageAgg{}
			}
			add(reqAggs[r.Req], key, s, a, b, docType)
			if isFresh(r) {
				add(fresh, key, s, a, b, docType)
			}
			if isReingest(r) {
				add(reingest, key, s, a, b, docType)
			}
			if d != nil {
				if docAggs[d] == nil {
					docAggs[d] = map[string]*stageAgg{}
				}
				add(docAggs[d], key, s, a, b, docType)
			}
		}
		if key == "ocr.recognize" {
			// split each call into engine wait and recognition, by time
			wait := int64(attrF(s, "wait_ms") * 1000)
			if wait > 0 {
				emit("ocr.engine_wait", s.TS, s.TS+wait)
			}
			emit("ocr.recognition", s.TS+wait, s.end())
			if isFresh(r) {
				c := attrS(s, "caller")
				if callers[c] == nil {
					callers[c] = map[string]any{"caller": c, "calls": 0, "summed_ms": 0.0, "types": map[string]int{}}
				}
				callers[c]["calls"] = callers[c]["calls"].(int) + 1
				callers[c]["summed_ms"] = callers[c]["summed_ms"].(float64) + ms(s.Dur)
				callers[c]["types"].(map[string]int)[docType]++
			}
			continue
		}
		emit(key, s.TS, s.end())
		if key != s.Stage { // grouped stages also get a per-helper row (overlaps the group row)
			emit(s.Stage, s.TS, s.end())
		}
	}
	finish := func(m map[string]*stageAgg, lo, hi int64) []*stageAgg {
		var out []*stageAgg
		for _, g := range m {
			g.CoveredMS = ms(union(g.iv, lo, hi))
			g.SumMS = math.Round(g.SumMS*100) / 100
			g.CPUms = math.Round(g.CPUms*100) / 100
			g.FirstMS, g.MedianMS = g.durs[0], median(g.durs)
			var ts []string
			for t, n := range g.types {
				ts = append(ts, fmt.Sprintf("%s×%d", orDash(t), n))
			}
			sort.Strings(ts)
			g.Docs = strings.Join(ts, " ")
			out = append(out, g)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].CoveredMS > out[j].CoveredMS })
		return out
	}
	sum.Stages = finish(fresh, 0, math.MaxInt64)
	sum.Reingest = finish(reingest, 0, math.MaxInt64)

	// leaf intervals per request/doc, for "unattributed" (Go glue and anything uninstrumented)
	leafIv := func(m map[string]*stageAgg) []interval {
		var iv []interval
		for _, g := range m { // engine waits count as accounted time, not as unattributed Go work
			iv = append(iv, g.iv...)
		}
		return iv
	}
	for _, r := range reqOrder {
		row := map[string]any{"label": orDash(r.Label), "kind": r.Kind, "server_ms": ms(r.Dur), "status": attrS(r, "status"), "files": attrS(r, "files")}
		if c := client[r.Label]; c != nil {
			row["client_ms"] = ms(c.Dur)
		}
		if c := buildBody[r.Label]; c != nil {
			row["client_build_body_ms"] = ms(c.Dur)
		}
		phases := map[string]float64{}
		for _, s := range spans {
			if s.Req == r.Req && (s.Stage == "handler" || strings.Count(s.Stage, ".") >= 1 && (strings.HasPrefix(s.Stage, "analyze.") || strings.HasPrefix(s.Stage, "confirm.")) || s.Stage == "upload.read") {
				phases[s.Stage] += ms(s.Dur)
			}
		}
		row["phases_ms"] = phases
		aggs := finish(reqAggs[r.Req], r.TS, r.end())
		top := map[string]any{}
		for _, g := range aggs {
			top[g.Stage] = map[string]any{"calls": g.Calls, "summed_ms": g.SumMS, "covered_ms": g.CoveredMS}
		}
		row["stages"] = top
		row["unattributed_ms"] = ms(r.Dur - union(leafIv(reqAggs[r.Req]), r.TS, r.end()))
		sum.Requests = append(sum.Requests, row)
		for _, d := range docs[r.Req] {
			drow := map[string]any{"label": orDash(r.Label), "doc": *d.Doc, "exam_type": orDash(attrS(d, "exam_type")), "reused": attrS(d, "reused"), "wall_ms": ms(d.Dur)}
			for _, s := range spans {
				if s.Req == r.Req && s.Doc != nil && *s.Doc == *d.Doc && (s.Stage == "classify" || s.Stage == "extract" || strings.HasPrefix(s.Stage, "checkpoint.")) {
					drow[s.Stage+"_ms"] = ms(s.Dur)
				}
			}
			dst := map[string]any{}
			for _, g := range finish(docAggs[d], d.TS, d.end()) {
				dst[g.Stage] = map[string]any{"calls": g.Calls, "summed_ms": g.SumMS, "covered_ms": g.CoveredMS}
			}
			drow["stages"] = dst
			drow["unattributed_ms"] = ms(d.Dur - union(leafIv(docAggs[d]), d.TS, d.end()))
			sum.Docs = append(sum.Docs, drow)
		}
	}
	for _, c := range callers {
		c["summed_ms"] = math.Round(c["summed_ms"].(float64))
		sum.Callers = append(sum.Callers, c)
	}
	sort.Slice(sum.Callers, func(i, j int) bool {
		return sum.Callers[i]["summed_ms"].(float64) > sum.Callers[j]["summed_ms"].(float64)
	})
	sum.Overlap, sum.CallSites = overlapAndCallSites(spans, reqOrder)
	for _, r := range reqOrder {
		if isReingest(r) {
			for _, d := range docs[r.Req] {
				sum.ReingestDocs++
				if attrS(d, "reused") == "true" {
					sum.ReingestReused++
				}
			}
		}
	}
	sum.Notes = []string{
		"covered_wall_ms = elapsed time during which at least one call of the stage was running; it bounds what removing the stage could save. summed_ms > covered_wall_ms means calls overlapped (parallel work).",
		"Nested stages overlap by design: proc.pdftoppm runs inside pdf.render; ocr.recognize is split into ocr.engine_wait + ocr.recognition; a preprocess helper can call another (summed preprocess counts both, covered does not). Never add stages together.",
		"unattributed_ms = request/document time with no leaf span running: Go glue, JSON, image work outside the instrumented helpers, HTTP I/O.",
		"Spans without a request tag (preprocess, parse, SQL on the confirm transaction handle) are attributed by time containment; the harness sends one request at a time.",
		"child_cpu_ms is user+sys CPU of child processes (Poppler, tesseract CLI). In-process libtesseract and Go work only have wall time.",
	}
	if sum.ReingestDocs > 0 && sum.ReingestReused == 0 {
		sum.Notes = append(sum.Notes, "No re-ingest document reused a checkpoint (doc attr reused=false everywhere), so the reingest rows measure a warm repeat of the full extraction in the same process, not checkpoint reuse.")
	}
	return sum, renderProfile(sum), nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func renderProfile(p *profileSummary) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	w("# Ingestion timing profile (run %s)\n\n", orDash(p.Run))
	w("OCR backend: **%v** (tesseract %v), GOMAXPROCS %v, NumCPU %v, OMP_THREAD_LIMIT=%v, %v\n\n",
		p.Info["ocr_backend"], p.Info["tesseract_version"], p.Info["gomaxprocs"], p.Info["num_cpu"], orDash(fmt.Sprint(p.Info["omp_thread_limit"])), p.Info["go_version"])
	w("## Startup (ms)\n\n| step | ms |\n|---|---:|\n")
	for _, k := range profKeys(p.Startup) {
		w("| %s | %.1f |\n", k, p.Startup[k])
	}
	w("\n## Requests\n\nclient = harness wall clock (includes building the multipart body, shown separately, and decoding the response); server = Audit wrapper (includes the intake_runs INSERT/UPDATE).\n\n")
	w("| label | kind | client ms | client body build ms | server ms | unattributed ms | top stages by covered wall (calls, summed→covered ms) |\n|---|---|---:|---:|---:|---:|---|\n")
	for _, r := range p.Requests {
		w("| %v | %v | %v | %v | %v | %v | %s |\n", r["label"], r["kind"], fmtNum(r["client_ms"]), fmtNum(r["client_build_body_ms"]), r["server_ms"], r["unattributed_ms"], topStages(r["stages"], 5))
	}
	w("\n### Handler phases (sequential, ms)\n\n| label | phases |\n|---|---|\n")
	for _, r := range p.Requests {
		ph := r["phases_ms"].(map[string]float64)
		var parts []string
		for _, k := range profKeys(ph) {
			if ph[k] >= 1 {
				parts = append(parts, fmt.Sprintf("%s %.0f", k, ph[k]))
			}
		}
		w("| %v | %s |\n", r["label"], strings.Join(parts, ", "))
	}
	w("\n## Documents (analyze)\n\n| label | doc | type | reused | wall ms | classify | extract | unattributed | top stages (calls, summed→covered ms) |\n|---|---:|---|---|---:|---:|---:|---:|---|\n")
	for _, d := range p.Docs {
		w("| %v | %v | %v | %v | %v | %v | %v | %v | %s |\n", d["label"], d["doc"], d["exam_type"], orDash(fmt.Sprint(d["reused"])), d["wall_ms"],
			fmtNum(d["classify_ms"]), fmtNum(d["extract_ms"]), d["unattributed_ms"], topStages(d["stages"], 4))
	}
	stageTable := func(title string, st []*stageAgg) {
		w("\n## %s\n\n| stage | calls | failed | summed ms | covered wall ms | parallelism | child CPU ms | first call ms | median ms | triggered by (doc type × calls) |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---|\n", title)
		for _, g := range st {
			par := 0.0
			if g.CoveredMS > 0 {
				par = g.SumMS / g.CoveredMS
			}
			w("| %s | %d | %d | %.0f | %.0f | %.2f | %.0f | %.2f | %.2f | %s |\n", g.Stage, g.Calls, g.Failed, g.SumMS, g.CoveredMS, par, g.CPUms, g.FirstMS, g.MedianMS, g.Docs)
		}
	}
	stageTable("Fresh analyze: stages across all patients", p.Stages)
	stageTable(reingestTitle(p), p.Reingest)
	overlapCols := []string{"ocr only", "ocr + other", "go preprocess only", "render, no ocr", "other, no ocr", "idle (no leaf span)", "ocr_active", "ocr_exactly_one", "avg_ocr_concurrency_when_active"}
	w("\n## What was in flight (per request, ms)\n\nEach request's wall time split once by which categories were running (the first six columns add up to server ms). ocr_active = at least one OCR call running; ocr_exactly_one = one call running, the rest of the engine pool idle.\n\n| label | server ms | %s |\n|---|---:|%s\n", strings.Join(overlapCols, " | "), strings.Repeat("---:|", len(overlapCols)))
	for _, r := range p.Overlap {
		w("| %v | %v |", r["label"], r["server_ms"])
		for _, c := range overlapCols {
			w(" %s |", fmtNum(r[c]))
		}
		w("\n")
	}
	w("\n## Call sites (fresh analyze, all patients)\n\ncovered = wall time with at least one call from this site running; only in flight = no other call site, and no second call from this one, was running, so that time delayed the request directly. Sites can overlap each other; never add rows.\n\n| site | calls | summed ms | covered wall ms | only in flight ms |\n|---|---:|---:|---:|---:|\n")
	for i, g := range p.CallSites {
		if i == 30 {
			break
		}
		w("| %s | %d | %.0f | %.0f | %.0f |\n", g.Site, g.Calls, g.SumMS, g.CoveredMS, g.AloneMS)
	}
	w("\n## OCR call sites by full caller (fresh analyze)\n\n| caller (wrapper<site) | calls | summed ms | doc types |\n|---|---:|---:|---|\n")
	for i, c := range p.Callers {
		if i == 25 {
			break
		}
		w("| %v | %v | %v | %v |\n", c["caller"], c["calls"], c["summed_ms"], c["types"])
	}
	w("\n## Reading this\n\n")
	for _, n := range p.Notes {
		w("- %s\n", n)
	}
	return b.String()
}

func reingestTitle(p *profileSummary) string {
	if p.ReingestDocs > 0 && p.ReingestReused == 0 {
		return fmt.Sprintf("Re-ingest analyze = WARM REPEAT (0 of %d documents reused a checkpoint): stages across all patients", p.ReingestDocs)
	}
	return fmt.Sprintf("Checkpoint re-ingest analyze (%d of %d documents reused a checkpoint): stages across all patients", p.ReingestReused, p.ReingestDocs)
}

func fmtNum(v any) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprint(v)
}

func topStages(v any, n int) string {
	m, _ := v.(map[string]any)
	type kv struct {
		k string
		c float64
		s string
	}
	var xs []kv
	for k, x := range m {
		e := x.(map[string]any)
		xs = append(xs, kv{k, e["covered_ms"].(float64), fmt.Sprintf("%s %v, %.0f→%.0f", k, e["calls"], e["summed_ms"], e["covered_ms"])})
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].c > xs[j].c })
	var out []string
	for i, x := range xs {
		if i == n {
			break
		}
		out = append(out, x.s)
	}
	return strings.Join(out, "; ")
}

func profKeys[V any](m map[string]V) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	sort.Strings(k)
	return k
}

func writeProfileSummary(timings string) error {
	sum, md, err := summarizeProfile(timings)
	if err != nil {
		return err
	}
	dir := filepath.Dir(timings)
	b, _ := json.MarshalIndent(sum, "", " ")
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), b, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.md"), []byte(md), 0o644); err != nil {
		return err
	}
	fmt.Printf("profile: %s, %s\n", filepath.Join(dir, "profile.md"), filepath.Join(dir, "profile.json"))
	return nil
}
