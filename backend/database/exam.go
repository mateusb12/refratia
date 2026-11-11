package database

import "time"

// Exam stores searchable metadata and the pointer to the file in Garage.
// The exam contents never live in PostgreSQL.
type Exam struct {
	ID          uint   `gorm:"primaryKey"`
	CaseID      string `gorm:"not null;index"`
	PatientName string `gorm:"not null;index"`
	Filename    string `gorm:"not null"`
	ExamType    string `gorm:"index"`
	Eye         string `gorm:"index"`
	ContentType string `gorm:"not null"`
	Bucket      string `gorm:"not null"`
	ObjectKey   string `gorm:"not null;uniqueIndex"`
	Size        int64  `gorm:"not null"`
	Referenced  bool   `gorm:"not null;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
