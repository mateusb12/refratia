// Command benchmark: black-box regression benchmark for the RefratIA ingestion pipeline.
// Builds the backend, runs it against an ISOLATED Postgres DB + Garage bucket, drives
// analyze -> confirm like the frontend, and scores the result against the ground truth.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
)

const (
	benchDB     = "refratia_bench"
	benchBucket = "refratia-bench"
	casesPrefix = "refratia/cases/"
)

type Check struct {
	Group  string `json:"group"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Info   bool   `json:"info,omitempty"`          // informational: reported, never fails the run
	NE     bool   `json:"not_evaluable,omitempty"` // cannot be evaluated because of an upstream failure (counted once, at the root check)
	Kind   string `json:"kind"`                    // app_failure | infra_failure (meaningful when !OK)
	Detail string `json:"detail,omitempty"`
}

type FileRun struct {
	File    string `json:"file"`
	GTType  string `json:"gt_type"`
	GTEye   string `json:"gt_eye"`
	RecType string `json:"recognized_type"`
	RecEye  string `json:"recognized_eye"`
	Source  string `json:"classification_source"` // recognized | human-assisted (ground truth used)
	OK      bool   `json:"classification_ok"`
}

type PatientRun struct {
	Slug          string     `json:"slug"`
	Files         []FileRun  `json:"files"`
	AnalyzeStatus int        `json:"analyze_status"`
	AnalyzeSecs   float64    `json:"analyze_seconds"`
	Fields        []FieldRes `json:"fields"`
	Checks        []Check    `json:"checks"`
	Changes       []string   `json:"reingest_changes"`
	Unverified    []string   `json:"unverified"`
	Incomplete    string     `json:"incomplete,omitempty"` // set when the patient could not be fully evaluated (infra)
}

type Run struct {
	Started     string        `json:"started"`
	Command     string        `json:"command"`
	BackendDir  string        `json:"backend_dir"`
	Seconds     float64       `json:"seconds"`
	Patients    []*PatientRun `json:"patients"`
	Regressions []string      `json:"regressions"`
	Failures    []string      `json:"failures"`
	InfraFails  []string      `json:"infra_failures"`
	Complete    bool          `json:"complete"`
	Why         []string      `json:"incomplete_reasons,omitempty"`
	Requested   []string      `json:"requested_patients"`
	Compare     *Compare      `json:"compare,omitempty"`
	Note        string        `json:"note,omitempty"`
}

type env struct {
	vars              []string
	m                 map[string]string
	api               string
	out               string
	data              string
	gtDir             string
	spec              *Spec
	s3                *s3.Client
	pgURL             string
	httpc             *http.Client
	reForce           bool
	analyzeTO, stepTO time.Duration
	cmd               *exec.Cmd
	bin               string
	backendDir        string
}

func die(f string, a ...any) { fmt.Fprintf(os.Stderr, "benchmark: "+f+"\n", a...); os.Exit(2) }

func main() { os.Exit(realMain()) }

func realMain() (code int) {
	cwd, _ := os.Getwd()
	root := cwd
	if filepath.Base(cwd) == "backend" {
		root = filepath.Dir(cwd)
	}
	backendDir := flag.String("backend-dir", filepath.Join(root, "backend"), "backend source dir to build and run")
	patients := flag.String("patients", "", "comma-separated folder slugs (default: all)")
	dataDir := flag.String("data-dir", filepath.Join(root, "frontend", "data"), "corpus root")
	gtDir := flag.String("gt-dir", filepath.Join(root, "backend", "benchmark", "groundtruth"), "ground truth dir")
	outBase := flag.String("out", filepath.Join(root, "backend", "benchmark", "out"), "report output base dir")
	baseline := flag.String("baseline", filepath.Join(root, "backend", "benchmark", "last_pass.json"), "passing-field baseline")
	update := flag.Bool("update-baseline", false, "rewrite the baseline with the fields that pass now")
	envFile := flag.String("env-file", filepath.Join(root, "backend", "testinfra", "test.env"), "testinfra env file (never the repo .env)")
	reForce := flag.Bool("reingest-force", false, "re-ingest with full re-processing instead of checkpoints")
	resume := flag.String("resume", "", "resume this out/<ts> dir: patients already in patients/*.json are skipped")
	compare := flag.String("compare", "", "previous report.json (or run dir) to compare with (default: latest previous run)")
	rescore := flag.String("rescore", "", "re-score an earlier run dir offline from its raw/ files (no OCR, no backend)")
	analyzeTO := flag.Duration("analyze-timeout", 25*time.Minute, "per-patient analyze timeout")
	stepTO := flag.Duration("step-timeout", 5*time.Minute, "inspect/confirm timeout")
	flag.Parse()

	t0 := time.Now()
	ctx := context.Background()
	for _, p := range []*string{compare, rescore, resume} { // relative paths: as given, else relative to the repo root
		if *p != "" {
			if _, err := os.Stat(*p); err != nil {
				*p = filepath.Join(root, *p)
			}
		}
	}
	e := &env{data: *dataDir, gtDir: *gtDir, reForce: *reForce, httpc: &http.Client{}, analyzeTO: *analyzeTO, stepTO: *stepTO}
	e.spec = &Spec{}
	must(loadJSON(filepath.Join(e.gtDir, "_field_spec.json"), e.spec))
	slugs := []string{"creso-aversa", "daniela-nogueira", "francisco", "maria-gorete", "rolmey"}
	if *patients != "" {
		slugs = strings.Split(*patients, ",")
	}
	run := &Run{Started: t0.Format(time.RFC3339), BackendDir: *backendDir, Requested: slugs,
		Command: "npm run benchmark -- " + strings.Join(os.Args[1:], " ")}

	if *rescore != "" {
		e.out = filepath.Join(*outBase, t0.Format("20060102-150405")+"-rescore")
		must(os.MkdirAll(filepath.Join(e.out, "patients"), 0o755))
		run = e.rescoreRun(*rescore, run)
		cmp := *compare
		if cmp == "" {
			cmp = "-"
		}
		return conclude(run, e, *baseline, false, cmp, *outBase, t0)
	}

	// anything unexpected in setup is an infra failure, reported (INCOMPLETE) with exit code 2
	defer func() {
		if r := recover(); r != nil {
			run.InfraFails = append(run.InfraFails, fmt.Sprint("harness: ", r))
			code = conclude(run, e, *baseline, *update, *compare, *outBase, t0)
		}
	}()
	e.setupEnv(*envFile)
	if *resume != "" {
		e.out = *resume
		run.Patients = loadDone(e.out)
	} else {
		e.out = filepath.Join(*outBase, t0.Format("20060102-150405"))
	}
	must(os.MkdirAll(filepath.Join(e.out, "raw"), 0o755))
	must(os.MkdirAll(filepath.Join(e.out, "patients"), 0o755))
	defer e.stopBackend()

	// one run at a time: the bench DB and bucket are shared and wiped at start
	lock, err := os.OpenFile(filepath.Join(os.TempDir(), "refratia-benchmark.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	must(err)
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		die("another benchmark run is in progress (shared refratia_bench DB/bucket); wait for it to finish")
	}
	defer lock.Close()
	if *resume == "" || !e.dbExists(ctx) {
		e.resetInfra(ctx)
	}
	must(e.buildBackend(*backendDir))
	must(e.launch())

	done := map[string]bool{}
	for _, p := range run.Patients {
		done[p.Slug] = true
	}
	for _, slug := range slugs {
		if done[slug] {
			fmt.Printf("== %s (resumed, skipped)\n", slug)
			continue
		}
		fmt.Printf("== %s\n", slug)
		pr := e.safePatient(ctx, slug)
		run.Patients = append(run.Patients, pr)
		b, _ := json.MarshalIndent(pr, "", " ")
		os.WriteFile(filepath.Join(e.out, "patients", slug+".json"), b, 0o644)
		run.Seconds = time.Since(t0).Seconds()
		writeReports(run, e.out) // incremental: a crash never loses finished patients
	}
	return conclude(run, e, *baseline, *update, *compare, *outBase, t0)
}

func loadDone(out string) []*PatientRun {
	var res []*PatientRun
	files, _ := filepath.Glob(filepath.Join(out, "patients", "*.json"))
	for _, f := range files {
		p := &PatientRun{}
		if loadJSON(f, p) == nil && p.Incomplete == "" {
			res = append(res, p)
		}
	}
	return res
}

func (e *env) dbExists(ctx context.Context) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	c := e.pg(ctx, benchDB)
	c.Close(ctx)
	return true
}

// safePatient never lets one patient stop the run: panics become infra_failure checks.
func (e *env) safePatient(ctx context.Context, slug string) (pr *PatientRun) {
	pr = &PatientRun{Slug: slug}
	defer func() {
		for i := range pr.Checks {
			if pr.Checks[i].Kind == "" {
				pr.Checks[i].Kind = "app_failure"
			}
		}
	}()
	defer func() {
		if r := recover(); r != nil {
			pr.Checks = append(pr.Checks, Check{Group: "harness", Name: "panic", Kind: "infra_failure", Detail: fmt.Sprint(r)})
			pr.Incomplete = fmt.Sprint("harness panic: ", r)
		}
	}()
	if err := e.ensureBackend(); err != nil {
		pr.Checks = append(pr.Checks, Check{Group: "harness", Name: "backend_start", Kind: "infra_failure", Detail: err.Error()})
		pr.Incomplete = "backend could not be (re)started"
		return
	}
	e.runPatient(ctx, slug, pr)
	return
}

// must panics: callers inside a patient run recover it and record an infra_failure.
func must(err error) {
	if err != nil {
		panic(err)
	}
}

// ---- environment + safety ----

func (e *env) setupEnv(file string) {
	b, err := os.ReadFile(file)
	must(err)
	e.m = map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok && !strings.HasPrefix(k, "#") {
			e.m[k] = v
		}
	}
	e.m["POSTGRES_DB"] = benchDB
	e.m["DATABASE_URL"] = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		e.m["POSTGRES_USER"], e.m["POSTGRES_PASSWORD"], e.m["POSTGRES_HOST"], e.m["POSTGRES_PORT"], benchDB)
	e.m["BUCKET_NAME"] = benchBucket
	e.m["REFRATIA_NO_DOTENV"] = "1"
	e.m["OPENAI_API_KEY"] = "" // patient documents must never leave the machine
	// hard safety guard: only the isolated testinfra, only the bench db/bucket
	if e.m["POSTGRES_HOST"] != "127.0.0.1" || e.m["POSTGRES_PORT"] != "55432" ||
		!strings.Contains(e.m["GARAGE_S3_API_URL"], "127.0.0.1:53900") || e.m["POSTGRES_DB"] != benchDB {
		die("env file does not point at the isolated testinfra (127.0.0.1:55432 / :53900); refusing to run")
	}
	e.pgURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%%s?sslmode=disable",
		e.m["POSTGRES_USER"], e.m["POSTGRES_PASSWORD"], e.m["POSTGRES_HOST"], e.m["POSTGRES_PORT"])
	for _, k := range []string{"PATH", "HOME", "LANG", "TMPDIR", "TESSDATA_PREFIX", "OMP_THREAD_LIMIT"} {
		if v := os.Getenv(k); v != "" {
			e.vars = append(e.vars, k+"="+v)
		}
	}
	for k, v := range e.m {
		e.vars = append(e.vars, k+"="+v)
	}
	e.s3 = s3.New(s3.Options{
		Region:                     "garage",
		BaseEndpoint:               aws.String(e.m["GARAGE_S3_API_URL"]),
		UsePathStyle:               true,
		Credentials:                credentials.NewStaticCredentialsProvider(e.m["AWS_ACCESS_KEY_ID"], e.m["AWS_SECRET_ACCESS_KEY"], ""),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
}

func (e *env) pg(ctx context.Context, db string) *pgx.Conn {
	c, err := pgx.Connect(ctx, fmt.Sprintf(e.pgURL, db))
	must(err)
	return c
}

func (e *env) resetInfra(ctx context.Context) {
	admin := e.pg(ctx, e.m["POSTGRES_ADMIN_DB"])
	defer admin.Close(ctx)
	_, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+benchDB+" WITH (FORCE)")
	must(err)
	_, err = admin.Exec(ctx, "CREATE DATABASE "+benchDB)
	must(err)
	objs, err := e.listObjects(ctx, "")
	must(err)
	for k := range objs {
		_, err := e.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(benchBucket), Key: aws.String(k)})
		must(err)
	}
	fmt.Printf("reset: db %s recreated, bucket %s emptied (%d objects)\n", benchDB, benchBucket, len(objs))
}

func (e *env) listObjects(ctx context.Context, prefix string) (map[string]int64, error) {
	out := map[string]int64{}
	p := s3.NewListObjectsV2Paginator(e.s3, &s3.ListObjectsV2Input{Bucket: aws.String(benchBucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		pg, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range pg.Contents {
			out[*o.Key] = aws.ToInt64(o.Size)
		}
	}
	return out, nil
}

func (e *env) buildBackend(backendDir string) error {
	e.bin = filepath.Join(e.out, "backend.bin")
	e.backendDir = backendDir
	build := exec.Command("go", "build", "-o", e.bin, ".")
	build.Dir = backendDir
	if o, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("go build failed in %s: %s", backendDir, o)
	}
	return nil
}

func (e *env) healthy() bool {
	if e.api == "" {
		return false
	}
	c := http.Client{Timeout: 5 * time.Second}
	r, err := c.Get(e.api + "/health")
	if err != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == 200
}

func (e *env) stopBackend() {
	if e.cmd != nil && e.cmd.Process != nil {
		e.cmd.Process.Kill()
		e.cmd.Wait()
	}
	e.cmd = nil
}

// launch starts the built backend on a free port and waits for /health.
func (e *env) launch() error {
	e.stopBackend()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	logf, err := os.OpenFile(filepath.Join(e.out, "backend.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(e.bin)
	cmd.Dir = e.backendDir
	cmd.Env = append(append([]string{}, e.vars...), "PORT="+port)
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		return err
	}
	e.cmd = cmd
	e.api = "http://127.0.0.1:" + port
	for i := 0; i < 120; i++ {
		if e.healthy() {
			fmt.Printf("backend up on %s (built from %s)\n", e.api, e.backendDir)
			return nil
		}
		time.Sleep(time.Second)
	}
	e.stopBackend()
	return fmt.Errorf("backend did not become healthy; see %s/backend.log", e.out)
}

func (e *env) ensureBackend() error {
	if e.healthy() {
		return nil
	}
	fmt.Println("backend not healthy, restarting")
	return e.launch()
}

// ---- HTTP ----

func multipartBody(paths []string, cls any, process []string) (*bytes.Buffer, string) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for _, p := range paths {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename=%q`, filepath.Base(p)))
		ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(p)))
		if ct == "" {
			ct = "application/octet-stream"
		}
		h.Set("Content-Type", ct)
		pw, _ := w.CreatePart(h)
		b, _ := os.ReadFile(p)
		pw.Write(b)
	}
	if cls != nil {
		b, _ := json.Marshal(cls)
		w.WriteField("classifications", string(b))
	}
	if process != nil {
		b, _ := json.Marshal(process)
		w.WriteField("processFiles", string(b))
	}
	w.Close()
	return buf, w.FormDataContentType()
}

func (e *env) post(ctx context.Context, url string, body io.Reader, ct, accept string) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
	req.Header.Set("Content-Type", ct)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return e.httpc.Do(req)
}

func (e *env) inspect(ctx context.Context, paths []string) (map[string][2]string, []byte, error) {
	buf, ct := multipartBody(paths, nil, nil)
	resp, err := e.post(ctx, e.api+"/api/intakes/inspect", buf, ct, "")
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r struct {
		Files []struct {
			Filename, SHA256, RecognizedExamType, RecognizedEye string
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, raw, err
	}
	m := map[string][2]string{}
	for _, f := range r.Files {
		m[f.Filename] = [2]string{f.RecognizedExamType, f.RecognizedEye}
	}
	return m, raw, nil
}

// analyze posts everything in ONE request like the frontend (stream=1 NDJSON).
func (e *env) analyze(ctx context.Context, paths []string, cls []map[string]string, process []string) (int, map[string]any, []byte, error) {
	buf, ct := multipartBody(paths, cls, process)
	q := "?stream=1"
	if e.reForce && len(process) == 0 {
		q += "&force=1"
	}
	resp, err := e.post(ctx, e.api+"/api/intakes/analyze"+q, buf, ct, "application/x-ndjson")
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	var payload json.RawMessage
	status, msg := resp.StatusCode, ""
	if strings.Contains(resp.Header.Get("Content-Type"), "ndjson") {
		dec := json.NewDecoder(resp.Body)
		for {
			var ev struct {
				Type    string          `json:"type"`
				Status  int             `json:"status"`
				Message string          `json:"message"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := dec.Decode(&ev); err != nil {
				break
			}
			switch ev.Type {
			case "result":
				payload = ev.Payload
			case "error":
				status, msg = ev.Status, ev.Message
			}
		}
	} else {
		payload, _ = io.ReadAll(resp.Body)
	}
	if payload == nil {
		return status, nil, nil, fmt.Errorf("analyze failed: %d %s", status, msg)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return status, nil, payload, err
	}
	return status, m, payload, nil
}

func (e *env) confirm(ctx context.Context, intakeID string) (int, map[string]any) {
	b, _ := json.Marshal(map[string]string{"intakeId": intakeID})
	resp, err := e.post(ctx, e.api+"/api/intakes/confirm", bytes.NewReader(b), "application/json", "")
	if err != nil {
		return 0, map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	var m map[string]any
	raw, _ := io.ReadAll(resp.Body)
	if json.Unmarshal(raw, &m) != nil {
		m = map[string]any{"error": string(raw)}
	}
	return resp.StatusCode, m
}

// ---- SQL / Garage snapshot ----

type Snap struct {
	Patients []map[string]any
	Exams    []map[string]any
	Objects  map[string]int64
}

func (e *env) rows(ctx context.Context, c *pgx.Conn, q string) []map[string]any {
	rs, err := c.Query(ctx, q)
	must(err)
	defer rs.Close()
	var out []map[string]any
	for rs.Next() {
		var raw []byte
		must(rs.Scan(&raw))
		m := map[string]any{}
		json.Unmarshal(raw, &m)
		out = append(out, m)
	}
	return out
}

func (e *env) snapshot(ctx context.Context) *Snap {
	c := e.pg(ctx, benchDB)
	defer c.Close(ctx)
	objs, err := e.listObjects(ctx, casesPrefix)
	must(err)
	return &Snap{
		Patients: e.rows(ctx, c, "SELECT to_jsonb(p) FROM patients p ORDER BY id"),
		Exams:    e.rows(ctx, c, "SELECT to_jsonb(x) FROM exams x ORDER BY id"),
		Objects:  objs,
	}
}

func sval(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func idSet(rows []map[string]any) map[string]bool {
	s := map[string]bool{}
	for _, r := range rows {
		s[sval(r, "id")] = true
	}
	return s
}

func newRows(pre, post []map[string]any) []map[string]any {
	old := idSet(pre)
	var out []map[string]any
	for _, r := range post {
		if !old[sval(r, "id")] {
			out = append(out, r)
		}
	}
	return out
}

func (s *Snap) sig() ([]string, []string, []string) {
	var p, x, o []string
	for _, r := range s.Patients {
		p = append(p, sval(r, "id"))
	}
	for _, r := range s.Exams {
		x = append(x, fmt.Sprintf("%s|%s|%s|%s", sval(r, "id"), sval(r, "exam_type"), sval(r, "eye"), sval(r, "object_key")))
	}
	for k := range s.Objects {
		o = append(o, k)
	}
	sort.Strings(p)
	sort.Strings(x)
	sort.Strings(o)
	return p, x, o
}

// diff returns human-readable differences between two snapshots (empty = identical).
func diffSnap(a, b *Snap) []string {
	var out []string
	ap, ax, ao := a.sig()
	bp, bx, bo := b.sig()
	d := func(label string, x, y []string) {
		inY, inX := map[string]bool{}, map[string]bool{}
		for _, s := range y {
			inY[s] = true
		}
		for _, s := range x {
			inX[s] = true
		}
		for _, s := range y {
			if !inX[s] {
				out = append(out, label+" added: "+s)
			}
		}
		for _, s := range x {
			if !inY[s] {
				out = append(out, label+" removed: "+s)
			}
		}
	}
	d("patient id", ap, bp)
	d("exam row", ax, bx)
	d("object", ao, bo)
	for k, sz := range a.Objects {
		if sz2, ok := b.Objects[k]; ok && sz2 != sz {
			out = append(out, fmt.Sprintf("object size changed: %s %d -> %d", k, sz, sz2))
		}
	}
	return out
}

// ---- per-patient flow ----

func fileSHA(p string) string {
	b, _ := os.ReadFile(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func withTO(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}

func (e *env) runPatient(ctx context.Context, slug string, pr *PatientRun) {
	add := func(group, name string, ok bool, detail string) {
		pr.Checks = append(pr.Checks, Check{Group: group, Name: name, OK: ok, Info: strings.HasPrefix(name, "info_"), Detail: detail})
	}
	ne := func(group, name, detail string) {
		pr.Checks = append(pr.Checks, Check{Group: group, Name: name, NE: true, Detail: detail})
	}
	infra := func(name, detail string) {
		pr.Checks = append(pr.Checks, Check{Group: "harness", Name: name, Kind: "infra_failure", Detail: detail})
		pr.Incomplete = name + ": " + detail
		fmt.Printf("   INFRA FAILURE %s: %s\n", name, detail)
	}
	g, err := loadGT(e.gtDir, slug)
	if err != nil {
		infra("ground_truth", err.Error())
		return
	}
	dir := filepath.Join(e.data, slug)
	ents, _ := os.ReadDir(dir)
	var paths, shas []string
	byName := map[string]string{}
	for _, en := range ents {
		if en.IsDir() || strings.HasSuffix(en.Name(), ".json") {
			continue
		}
		p := filepath.Join(dir, en.Name())
		paths = append(paths, p)
		shas = append(shas, fileSHA(p))
		byName[en.Name()] = fileSHA(p)
	}
	if len(paths) == 0 {
		infra("corpus_files", "no files in "+dir)
		return
	}

	// classification: what the UI would prefill (recognized); unrecognized files get the ground truth (a human would choose)
	ictx, cancel := withTO(ctx, e.stepTO)
	rec, rawInspect, err := e.inspect(ictx, paths)
	cancel()
	if err != nil {
		infra("inspect", err.Error())
		return
	}
	os.WriteFile(filepath.Join(e.out, "raw", slug+".inspect.json"), rawInspect, 0o644)
	var cls []map[string]string
	classOK := map[string]bool{}
	for _, f := range g.Files {
		fr := FileRun{File: f.File, GTType: f.ExamType, GTEye: f.Eye}
		r := rec[f.File]
		fr.RecType, fr.RecEye = r[0], r[1]
		wantKey := ""
		if k := e.spec.AppExamKey[f.ExamType]; k != nil {
			wantKey = *k
		}
		if r[0] != "" && r[1] != "" {
			fr.Source = "recognized"
			fr.OK = wantKey == "" || (r[0] == wantKey && r[1] == f.Eye)
			cls = append(cls, map[string]string{"sha256": byName[f.File], "examType": r[0], "eye": r[1]})
		} else {
			fr.Source = "human-assisted"
			fr.OK = true
			if wantKey != "" {
				cls = append(cls, map[string]string{"sha256": byName[f.File], "examType": wantKey, "eye": f.Eye})
			}
		}
		classOK[f.File] = fr.OK
		pr.Files = append(pr.Files, fr)
		pr.Checks = append(pr.Checks, Check{Group: "classification", Name: f.File, OK: fr.OK, Info: true,
			Detail: fmt.Sprintf("%s: recognized %s/%s, expected %s/%s", fr.Source, fr.RecType, fr.RecEye, wantKey, f.Eye)})
	}

	pre := e.snapshot(ctx)
	t := time.Now()
	actx, cancel := withTO(ctx, e.analyzeTO)
	status, payload, raw, err := e.analyze(actx, paths, cls, shas)
	cancel()
	pr.AnalyzeSecs = time.Since(t).Seconds()
	pr.AnalyzeStatus = status
	if raw != nil {
		os.WriteFile(filepath.Join(e.out, "raw", slug+".analyze.json"), raw, 0o644)
	}
	if status == 0 { // no HTTP answer: timeout, connection refused, backend died
		infra("analyze_no_response", fmt.Sprint(err))
		return
	}
	if err != nil || status != 200 {
		add("analyze", "analyze_http_200", false, fmt.Sprint(status, " ", err))
		pr.Fields = scorePatient(slug, g, e.spec, nil, classOK)
		fmt.Printf("   analyze FAILED (%d) after %.0fs\n", status, pr.AnalyzeSecs)
		return
	}
	fmt.Printf("   analyze ok in %.0fs\n", pr.AnalyzeSecs)
	analysis := payload["analysis"]
	pr.Fields = scorePatient(slug, g, e.spec, analysis, classOK)
	pr.Unverified = append(pr.Unverified, g.NeedsHuman...)
	pr.Checks = append(pr.Checks, identityChecks(g, analysis)...)

	// ---- persistence: always asserted on real SQL rows and Garage objects ----
	intakeID, _ := payload["intakeId"].(string)
	cctx, cancel := withTO(ctx, e.stepTO)
	cs, cbody := e.confirm(cctx, intakeID)
	cancel()
	if cs == 0 {
		infra("confirm_no_response", sval(cbody, "error"))
		return
	}
	b, _ := json.Marshal(cbody)
	os.WriteFile(filepath.Join(e.out, "raw", slug+".confirm1.json"), b, 0o644)
	add("persistence", "confirm_http_200", cs == 200, fmt.Sprintf("HTTP %d action=%s", cs, sval(cbody, "action")))
	post1 := e.snapshot(ctx)
	e.persistenceChecks(g, pre, post1, byName, sval(cbody, "caseId"), add, ne)

	cctx, cancel = withTO(ctx, e.stepTO)
	cs2, _ := e.confirm(cctx, intakeID)
	cancel()
	post2 := e.snapshot(ctx)
	d := diffSnap(post1, post2)
	add("idempotency", "confirm_retry", cs2 == 200 && !structural(d), fmt.Sprintf("HTTP %d; changes: %s", cs2, summarize(d)))

	// re-ingest: same folder again (checkpoints unless -reingest-force)
	var proc []string
	if e.reForce {
		proc = shas
	}
	actx, cancel = withTO(ctx, e.analyzeTO)
	st3, p3, raw3, err3 := e.analyze(actx, paths, cls, proc)
	cancel()
	if st3 == 0 {
		infra("reingest_analyze_no_response", fmt.Sprint(err3))
		return
	}
	if err3 != nil || st3 != 200 {
		add("idempotency", "reingest_no_duplicates", false, fmt.Sprintf("analyze %d %v", st3, err3))
		return
	}
	os.WriteFile(filepath.Join(e.out, "raw", slug+".reingest.analyze.json"), raw3, 0o644)
	id3, _ := p3["intakeId"].(string)
	cctx, cancel = withTO(ctx, e.stepTO)
	cs3, cb3 := e.confirm(cctx, id3)
	cancel()
	post3 := e.snapshot(ctx)
	d3 := diffSnap(post2, post3)
	pr.Changes = d3
	add("idempotency", "reingest_no_duplicates", cs3 == 200 && !structural(d3),
		fmt.Sprintf("HTTP %d action=%s; changes: %s", cs3, sval(cb3, "action"), summarize(d3)))
}

// structural: a duplicate = added patient row, exam row, case prefix or object. Content changes of the compiled marker/receipt and removals are recorded as info only.
func structural(d []string) bool {
	for _, x := range d {
		if strings.HasPrefix(x, "patient id added") || strings.HasPrefix(x, "exam row added") || strings.HasPrefix(x, "object added") {
			return true // new patient, new exam row, new case prefix or new document object = duplicate
		}
	}
	return false
}

func tokens(s string) []string {
	n := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return ' '
	}, normStr(s))
	t := strings.Fields(n)
	sort.Strings(t)
	return t
}

// nameCmp compares token multisets (case, accents, punctuation and order insensitive).
func nameCmp(got, want string) (bool, string) {
	a, b := tokens(got), tokens(want)
	if strings.Join(a, " ") == strings.Join(b, " ") {
		if normStr(got) == normStr(want) {
			return true, "match"
		}
		return true, "match (word order differs)"
	}
	return false, "mismatch"
}

func summarize(d []string) string {
	if len(d) == 0 {
		return "none"
	}
	if len(d) > 6 {
		return fmt.Sprintf("%d changes, e.g. %s", len(d), strings.Join(d[:6], "; "))
	}
	return strings.Join(d, "; ")
}

func identityChecks(g *GT, analysis any) []Check {
	var out []Check
	add := func(name string, ok bool, detail string) {
		out = append(out, Check{Group: "identity", Name: name, OK: ok, Kind: "app_failure", Detail: detail})
	}
	nm, _ := walk(analysis, []string{"patient", "full_name"})
	bd, _ := walk(analysis, []string{"patient", "birth_date"})
	ns, _ := nm.(string)
	bs, _ := bd.(string)
	wantName, _ := g.Patient.FullName.Value.(string)
	nok, nd := nameCmp(ns, wantName)
	add("name", nok, nd)
	if g.Patient.BirthDate.Verification != "agent_visual" {
		add("birth_date", true, "unverified (needs_human in ground truth); value not scored")
	} else {
		want, _ := g.Patient.BirthDate.Value.(string)
		ok := normDate(bs) == want
		add("birth_date", ok, map[bool]string{true: "match", false: "mismatch"}[ok])
	}
	v := ageViolations(analysis)
	add("age_never_zero_or_from_missing_birthdate", len(v) == 0, strings.Join(v, "; "))
	for _, c := range g.IdentityChecks {
		if c.Method == "data_nascimento_invertida" {
			b, _ := json.Marshal(analysis)
			add("warning_data_nascimento_invertida", strings.Contains(string(b), "data_nascimento_invertida"), "required by ground truth (C2)")
			break
		}
	}
	return out
}

func (e *env) persistenceChecks(g *GT, pre, post *Snap, shaByName map[string]string, confirmCase string, add func(string, string, bool, string), ne func(string, string, string)) {
	np := newRows(pre.Patients, post.Patients)
	nx := newRows(pre.Exams, post.Exams)
	add("persistence", "exactly_one_new_patient_row", len(np) == 1, fmt.Sprintf("%d new patient rows", len(np)))
	patientID, caseID := "", confirmCase
	if len(np) == 1 {
		patientID = sval(np[0], "id")
		if caseID == "" {
			caseID = sval(np[0], "case_id")
		}
		wantName, _ := g.Patient.FullName.Value.(string)
		ok, d := nameCmp(sval(np[0], "full_name"), wantName)
		add("persistence", "patient_row_name", ok, d)
		if g.Patient.BirthDate.Verification == "agent_visual" {
			want, _ := g.Patient.BirthDate.Value.(string)
			ok := normDate(sval(np[0], "birth_date")) == want
			add("persistence", "patient_row_birth_date", ok, map[bool]string{true: "match", false: "mismatch"}[ok])
		}
	}
	add("persistence", "exam_row_count", len(nx) == g.Expected.Count, fmt.Sprintf("%d rows, expected %d", len(nx), g.Expected.Count))

	// rows vs expected (exam_type, eye, source file); source derived from sha prefix or original name in key/filename
	srcOf := func(r map[string]any) string {
		hay := strings.ToLower(sval(r, "object_key") + " " + sval(r, "filename"))
		for name, sha := range shaByName {
			base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
			if strings.Contains(hay, sha[:8]) || strings.Contains(hay, base) {
				return name
			}
		}
		return ""
	}
	used := make([]bool, len(g.Expected.Records))
	var problems []string
	unknownSrc := 0
	for _, r := range nx {
		src := srcOf(r)
		if src == "" {
			unknownSrc++
		}
		found := false
		for i, rec := range g.Expected.Records {
			if !used[i] && rec.ExamType == sval(r, "exam_type") && rec.Eye == sval(r, "eye") && (src == "" || src == rec.SourceFile) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			problems = append(problems, fmt.Sprintf("unexpected row type=%q eye=%q src=%q", sval(r, "exam_type"), sval(r, "eye"), src))
		}
	}
	for i, rec := range g.Expected.Records {
		if !used[i] {
			problems = append(problems, fmt.Sprintf("missing row %s/%s from %s", rec.ExamType, rec.Eye, rec.SourceFile))
		}
	}
	detail := summarize(problems)
	if unknownSrc > 0 {
		detail += fmt.Sprintf(" (source file not derivable for %d rows; matched on type+eye only)", unknownSrc)
	}
	if len(nx) == 0 {
		ne("persistence", "exam_rows_match_expected", "no exam rows; see exam_row_count")
	} else {
		add("persistence", "exam_rows_match_expected", len(problems) == 0, detail)
	}

	// relationships
	cases := map[string]bool{}
	badFK := 0
	for _, r := range nx {
		if sval(r, "patient_id") != patientID || patientID == "" {
			badFK++
		}
		cid := sval(r, "case_id")
		if cid == "" {
			if k := strings.TrimPrefix(sval(r, "object_key"), casesPrefix); k != sval(r, "object_key") {
				cid = strings.SplitN(k, "/", 2)[0]
			}
		}
		cases[cid] = true
	}
	ids := keys(cases)
	if len(nx) == 0 {
		ne("persistence", "exam_patient_id_matches_patient", "no exam rows; see exam_row_count")
		ne("persistence", "case_id_consistent", "no exam rows; see exam_row_count")
	} else {
		add("persistence", "exam_patient_id_matches_patient", badFK == 0, fmt.Sprintf("%d rows with wrong/null patient_id", badFK))
		if len(ids) == 1 && ids[0] != "" {
			add("persistence", "case_id_consistent", caseID == "" || caseID == ids[0], fmt.Sprintf("exam rows case %v; confirm/patient case consistent=%v", len(ids), caseID == "" || caseID == ids[0]))
		} else {
			add("persistence", "case_id_consistent", false, fmt.Sprintf("%d distinct/empty case ids among exam rows", len(ids)))
		}
	}

	// objects in Garage
	missing := 0
	for _, r := range nx {
		k := sval(r, "object_key")
		out, err := e.s3.HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: aws.String(benchBucket), Key: aws.String(k)})
		if err != nil {
			missing++
			continue
		}
		if sz, ok := r["size"].(float64); ok && sz > 0 && int64(sz) != aws.ToInt64(out.ContentLength) {
			missing++
		}
	}
	if len(nx) == 0 {
		ne("persistence", "exam_objects_exist_with_size", "no exam rows; see exam_row_count")
	} else {
		add("persistence", "exam_objects_exist_with_size", missing == 0, fmt.Sprintf("%d rows whose object is missing or has a different size", missing))
	}
	if caseID != "" {
		_, err := e.s3.HeadObject(context.Background(), &s3.HeadObjectInput{Bucket: aws.String(benchBucket), Key: aws.String(casesPrefix + caseID + "/paciente_compilado.json")})
		add("persistence", "marker_exists", err == nil, "case "+map[bool]string{true: "found", false: "marker object missing"}[err == nil])
	} else {
		add("persistence", "marker_exists", false, "no caseId in the confirm response and no case_id on the patient row")
	}
	// uncommitted garage garbage: objects under the cases prefix that no exam row or marker references
	if caseID != "" {
		refd := map[string]bool{casesPrefix + caseID + "/paciente_compilado.json": true}
		for _, r := range post.Exams {
			refd[sval(r, "object_key")] = true
		}
		extra := 0
		for k := range post.Objects {
			if strings.HasPrefix(k, casesPrefix+caseID+"/") && !refd[k] {
				extra++
			}
		}
		add("persistence", "info_unreferenced_case_objects", extra == 0, fmt.Sprintf("%d objects in the case folder not referenced by an exam row (superseded or garbage)", extra))
	}
}

func keys(m map[string]bool) []string {
	var o []string
	for k := range m {
		o = append(o, k)
	}
	sort.Strings(o)
	return o
}
