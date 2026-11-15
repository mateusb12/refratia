package database

import "time"

// Intake run statuses.
const (
	RunRunning   = "running"
	RunDone      = "done"
	RunFailed    = "failed"
	RunCancelled = "cancelled"
)

// IntakeRun records one intake request (analyze, inspect or confirm): its
// status while it runs and, in Details, the audit document (timing, error,
// per-file details) plus, for a finished analyze, the result the browser
// shows. Only the lookup keys are columns.
type IntakeRun struct {
	ID        uint      `gorm:"primaryKey"`
	Kind      string    `gorm:"not null;index"`
	IntakeID  string    `gorm:"not null;default:'';index"`
	Status    string    `gorm:"not null;index"`
	Details   string    `gorm:"type:jsonb;not null"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
}

// migrateIntakeRuns moves the rows of the former processing_audits table into
// intake_runs, then fails runs a restart cut off (they can no longer finish).
func (db *DB) migrateIntakeRuns() error {
	if db.Gorm.Migrator().HasTable("processing_audits") {
		if err := db.Gorm.Exec(`
INSERT INTO intake_runs (kind, intake_id, status, details, created_at, updated_at)
SELECT kind, intake_id,
       CASE WHEN COALESCE((details->>'http_status')::int, 0) >= 400 OR details ? 'error' THEN 'failed' ELSE 'done' END,
       details, created_at, created_at
FROM processing_audits ORDER BY id`).Error; err != nil {
			return err
		}
		if err := db.Gorm.Migrator().DropTable("processing_audits"); err != nil {
			return err
		}
	}
	return db.Gorm.Exec(`
UPDATE intake_runs
SET status = 'failed', updated_at = now(),
    details = details || '{"error": "interrompido: o servidor reiniciou durante o processamento"}'
WHERE status = 'running'`).Error
}
