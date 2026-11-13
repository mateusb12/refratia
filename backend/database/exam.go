package database

import "time"

// Migrate creates/updates the schema. One exam row per (document, eye):
// the former UNIQUE(object_key) is replaced by UNIQUE(object_key, eye).
func (db *DB) Migrate() error {
	if err := db.Gorm.Exec(`DROP INDEX IF EXISTS idx_exams_object_key`).Error; err != nil {
		return err
	}
	if err := db.Gorm.AutoMigrate(&Patient{}, &Exam{}); err != nil {
		return err
	}
	return db.Gorm.Exec(`UPDATE exams SET exam_type = 'unclassified' WHERE exam_type = ''`).Error
}

// Exam stores searchable metadata and the pointer to the file in Garage.
// The exam contents never live in PostgreSQL.
type Exam struct {
	ID          uint   `gorm:"primaryKey"`
	PatientID   *uint  `gorm:"index"`
	CaseID      string `gorm:"not null;index"`
	PatientName string `gorm:"not null;index"`
	Filename    string `gorm:"not null"`
	ExamType    string `gorm:"not null;default:'unclassified';index"`
	Eye         string `gorm:"not null;default:'';index;uniqueIndex:idx_exams_object_eye,priority:2"`
	ContentType string `gorm:"not null"`
	Bucket      string `gorm:"not null"`
	ObjectKey   string `gorm:"not null;uniqueIndex:idx_exams_object_eye,priority:1"`
	Size        int64  `gorm:"not null"`
	Referenced  bool   `gorm:"not null;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Patient stores normalized identity metadata used to search and group cases.
// Detailed source evidence remains in the Garage analysis artifact.
type Patient struct {
	ID          uint   `gorm:"primaryKey"`
	IdentityKey string `gorm:"not null;uniqueIndex"`
	// CaseID is the Garage case of this patient (one case per patient).
	// Empty for legacy rows created before the column existed.
	CaseID    string `gorm:"not null;default:'';index"`
	FullName  string `gorm:"not null;index"`
	BirthDate string `gorm:"not null;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
