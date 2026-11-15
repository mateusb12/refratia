package database

import "time"

// ProcessingAudit records one intake request (analyze, inspect or confirm).
// Only the lookup keys are columns; everything else (timing, status, error,
// per-file details) lives in the Details JSON document.
type ProcessingAudit struct {
	ID        uint      `gorm:"primaryKey"`
	Kind      string    `gorm:"not null;index"`
	IntakeID  string    `gorm:"not null;default:'';index"`
	Details   string    `gorm:"type:jsonb;not null"`
	CreatedAt time.Time `gorm:"index"`
}
