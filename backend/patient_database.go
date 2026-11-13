package main

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"refratia/backend/database"
	patientfeature "refratia/backend/features/patient"
)

var errPatientIdentity = errors.New("patient identity is incomplete or conflicting")

// patientIdentity validates the identity of an analysis. It touches no store,
// so it can run before any Garage write.
func patientIdentity(analysis map[string]any) (identityKey, fullName, birthDate string, err error) {
	if analysis == nil {
		return "", "", "", errors.New("patient analysis is missing")
	}
	identityKey, identifiable := patientfeature.Identity(analysis)
	if !identifiable {
		return "", "", "", errPatientIdentity
	}
	patientData, ok := analysis["patient"].(map[string]any)
	if !ok {
		return "", "", "", errPatientIdentity
	}
	_ = patientData
	fullName, _ = patientfeature.ResolveName(analysis)
	fullName = strings.TrimSpace(fullName)
	birthDate, ok = patientfeature.CanonicalBirthDate(analysis)
	if fullName == "" || !ok {
		return "", "", "", errPatientIdentity
	}
	return identityKey, fullName, birthDate, nil
}

// lockPatient returns the patient row locked FOR UPDATE inside tx, creating it
// first when needed. INSERT ... ON CONFLICT DO NOTHING + SELECT FOR UPDATE makes
// concurrent confirms of the same patient serialize on the row (the loser waits
// for the winner's commit) instead of racing on a find-then-create.
func lockPatient(tx *gorm.DB, identityKey, fullName, birthDate string) (*database.Patient, error) {
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "identity_key"}},
		DoNothing: true,
	}).Create(&database.Patient{IdentityKey: identityKey, FullName: fullName, BirthDate: birthDate}).Error; err != nil {
		return nil, fmt.Errorf("create patient: %w", err)
	}
	var patient database.Patient
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("identity_key = ?", identityKey).First(&patient).Error; err != nil {
		return nil, fmt.Errorf("lock patient: %w", err)
	}
	return &patient, nil
}

// savePatient updates the display fields and links the case (only when the row
// has none yet: one case per patient).
func savePatient(tx *gorm.DB, patient *database.Patient, fullName, birthDate, caseID string) error {
	patient.FullName, patient.BirthDate = fullName, birthDate
	if patient.CaseID == "" {
		patient.CaseID = caseID
	}
	return tx.Save(patient).Error
}

// upsertPatientFromAnalysis is the standalone (manual exam) path: validate, lock
// and update the patient inside tx.
func upsertPatientFromAnalysis(tx *gorm.DB, analysis map[string]any, caseID string) (*database.Patient, error) {
	identityKey, fullName, birthDate, err := patientIdentity(analysis)
	if err != nil {
		return nil, err
	}
	patient, err := lockPatient(tx, identityKey, fullName, birthDate)
	if err != nil {
		return nil, err
	}
	if err := savePatient(tx, patient, fullName, birthDate, caseID); err != nil {
		return nil, fmt.Errorf("update patient: %w", err)
	}
	return patient, nil
}

// patientCaseFromSQL returns the case linked to the identity, "" when none.
func patientCaseFromSQL(db *gorm.DB, identityKey string) (string, error) {
	var patient database.Patient
	err := db.Where("identity_key = ?", identityKey).First(&patient).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return patient.CaseID, err
}

// similarPatientsByBirthDate lists other patients with the same birth date and
// a different name key (possible OCR typo or a namesake): flagged, never merged.
func similarPatientsByBirthDate(db *gorm.DB, identityKey, birthDate string) ([]database.Patient, error) {
	var patients []database.Patient
	err := db.Where("birth_date = ? AND identity_key <> ?", birthDate, identityKey).Limit(5).Find(&patients).Error
	return patients, err
}
