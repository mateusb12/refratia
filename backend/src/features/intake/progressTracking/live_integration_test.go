package progressTracking

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"refratia/backend/src/shared/database"
	progress "refratia/backend/src/shared/progress"
)

// fakeAnalyze streams like AnalyzeIntake: run ID, one progress event, then
// waits for release (or cancellation) before the result.
var testHandler Handler

func integrationSetup(t *testing.T) {
	t.Helper()
	if os.Getenv("REFRATIA_NO_DOTENV") == "" || !strings.HasSuffix(os.Getenv("BUCKET_NAME"), "-test") {
		t.Skip("integration test: source backend/testinfra/test.env first")
	}
	openedDatabase, openError := database.Open(context.Background())
	if openError != nil {
		t.Fatal(openError)
	}
	if migrateError := openedDatabase.Migrate(); migrateError != nil {
		t.Fatal(migrateError)
	}
	testHandler = Handler{Database: openedDatabase.Gorm}
}

func fakeAnalyze(release <-chan struct{}) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		stream, _ := NewStream(responseWriter)
		stream.Attach(From(request.Context()))
		stream.Emit(progress.Event{Type: "progress", Percent: 40, Message: "lendo"})
		select {
		case <-release:
			stream.WriteResult(map[string]any{"intakeId": "intake-x"})
		case <-request.Context().Done():
			stream.WriteError(http.StatusBadGateway, request.Context().Err().Error())
		}
	}
}

func startFakeRun(t *testing.T, release chan struct{}) (uint, chan struct{}) {
	t.Helper()
	clientContext, leave := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/intakes/analyze?stream=1", nil).WithContext(clientContext)
	recorder := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		testHandler.Audit("analyze", fakeAnalyze(release))(recorder, request)
		close(finished)
	}()
	var row database.IntakeRun
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		row = database.IntakeRun{} // First on a struct with an ID filters by that ID
		testHandler.Database.Where("kind = ?", "analyze").Order("id DESC").First(&row)
		if _, ok := liveRuns.Load(row.ID); ok && row.Status == database.RunRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not start")
		}
	}
	leave() // the browser tab closes
	return row.ID, finished
}

func streamEvents(t *testing.T, runID uint) []progress.Event {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/intakes/runs/"+strconv.Itoa(int(runID))+"/stream", nil)
	recorder := httptest.NewRecorder()
	testHandler.ServeRun(recorder, request)
	var events []progress.Event
	scanner := bufio.NewScanner(strings.NewReader(recorder.Body.String()))
	for scanner.Scan() {
		var event progress.Event
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			t.Fatalf("bad event line %q", scanner.Text())
		}
		events = append(events, event)
	}
	return events
}

func TestAnalyzeRunOutlivesTheBrowserAndReplays(t *testing.T) {
	integrationSetup(t)
	release := make(chan struct{})
	runID, finished := startFakeRun(t, release)

	// Reconnect while running: replays the run and progress, then follows.
	replayed := make(chan []progress.Event)
	go func() { replayed <- streamEvents(t, runID) }()
	time.Sleep(50 * time.Millisecond)
	close(release)
	<-finished
	events := <-replayed
	if len(events) != 3 || events[0].Type != "run" || events[1].Percent != 40 || events[2].Type != "result" {
		t.Fatalf("live replay: %+v", events)
	}

	// After it ended, the outcome comes from the table.
	var row database.IntakeRun
	testHandler.Database.First(&row, runID)
	if row.Status != database.RunDone {
		t.Fatalf("status %q, details %s", row.Status, row.Details)
	}
	events = streamEvents(t, runID)
	if len(events) != 1 || events[0].Type != "result" {
		t.Fatalf("stored replay: %+v", events)
	}
	if payload, _ := events[0].Payload.(map[string]any); payload["intakeId"] != "intake-x" {
		t.Fatalf("stored result: %+v", events[0].Payload)
	}
}

func TestAnalyzeRunCancel(t *testing.T) {
	integrationSetup(t)
	runID, finished := startFakeRun(t, make(chan struct{}))
	request := httptest.NewRequest(http.MethodPost, "/api/intakes/runs/"+strconv.Itoa(int(runID))+"/cancel", nil)
	testHandler.ServeRun(httptest.NewRecorder(), request)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not stop the run")
	}
	var row database.IntakeRun
	testHandler.Database.First(&row, runID)
	if row.Status != database.RunCancelled {
		t.Fatalf("status %q", row.Status)
	}
	if events := streamEvents(t, runID); len(events) != 1 || events[0].Type != "error" {
		t.Fatalf("cancelled replay: %+v", events)
	}
}

func TestMigrateMovesProcessingAudits(t *testing.T) {
	integrationSetup(t)
	gormDB := testHandler.Database
	if err := gormDB.Exec(`CREATE TABLE processing_audits (id bigserial PRIMARY KEY, kind text NOT NULL, intake_id text NOT NULL DEFAULT '', details jsonb NOT NULL, created_at timestamptz)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := gormDB.Exec(`INSERT INTO processing_audits (kind, intake_id, details, created_at) VALUES
		('analyze', 'intake-migrated-ok', '{"http_status": 200}', now()),
		('analyze', 'intake-migrated-bad', '{"http_status": 200, "error": "boom"}', now())`).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&database.DB{Gorm: gormDB}).Migrate(); err != nil {
		t.Fatal(err)
	}
	if gormDB.Migrator().HasTable("processing_audits") {
		t.Fatal("processing_audits still exists")
	}
	for intakeID, status := range map[string]string{"intake-migrated-ok": database.RunDone, "intake-migrated-bad": database.RunFailed} {
		var row database.IntakeRun
		if err := gormDB.Where("intake_id = ?", intakeID).Last(&row).Error; err != nil || row.Status != status {
			t.Fatalf("%s: %v status %q", intakeID, err, row.Status)
		}
	}
}
