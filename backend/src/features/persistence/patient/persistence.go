package patient

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"refratia/backend/src/shared/database"
)

var ErrIdentity = errors.New("patient identity is incomplete or conflicting")

// IdentityFromAnalysis validates the identity of an analysis. It touches no
// store, so it can run before any Garage write.
func IdentityFromAnalysis(analysis map[string]any) (identityKey, fullName, birthDate string, errorValue error) {
	if analysis == nil {
		return "", "", "", errors.New("patient analysis is missing")
	}
	identityKey, identifiable := Identity(analysis)
	if !identifiable {
		return "", "", "", ErrIdentity
	}
	if _, ok := analysis["patient"].(map[string]any); !ok {
		return "", "", "", ErrIdentity
	}
	fullName, _ = ResolveName(analysis)
	fullName = strings.TrimSpace(fullName)
	birthDate, ok := CanonicalBirthDate(analysis)
	if fullName == "" || !ok {
		return "", "", "", ErrIdentity
	}
	return identityKey, fullName, birthDate, nil
}

// Lock returns the patient row locked FOR UPDATE inside transaction, creating
// it first when needed. INSERT ... ON CONFLICT DO NOTHING + SELECT FOR UPDATE
// makes concurrent confirms of the same patient serialize on the row (the loser
// waits for the winner's commit) instead of racing on a find-then-create.
func Lock(transaction *gorm.DB, identityKey, fullName, birthDate string) (*database.Patient, error) {
	if createError := transaction.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "identity_key"}},
		DoNothing: true,
	}).Create(&database.Patient{IdentityKey: identityKey, FullName: fullName, BirthDate: birthDate}).Error; createError != nil {
		return nil, fmt.Errorf("create patient: %w", createError)
	}
	var patient database.Patient
	if lockError := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("identity_key = ?", identityKey).First(&patient).Error; lockError != nil {
		return nil, fmt.Errorf("lock patient: %w", lockError)
	}
	return &patient, nil
}

// Save updates the display fields and links the case (only when the row has
// none yet: one case per patient).
func Save(transaction *gorm.DB, patient *database.Patient, fullName, birthDate, caseID string) error {
	patient.FullName, patient.BirthDate = fullName, birthDate
	if patient.CaseID == "" {
		patient.CaseID = caseID
	}
	return transaction.Save(patient).Error
}

// UpsertFromAnalysis is the standalone (manual exam) path: validate, lock and
// update the patient inside transaction.
func UpsertFromAnalysis(transaction *gorm.DB, analysis map[string]any, caseID string) (*database.Patient, error) {
	identityKey, fullName, birthDate, identityError := IdentityFromAnalysis(analysis)
	if identityError != nil {
		return nil, identityError
	}
	patient, lockError := Lock(transaction, identityKey, fullName, birthDate)
	if lockError != nil {
		return nil, lockError
	}
	if saveError := Save(transaction, patient, fullName, birthDate, caseID); saveError != nil {
		return nil, fmt.Errorf("update patient: %w", saveError)
	}
	return patient, nil
}

// CaseFromSQL returns the case linked to the identity, "" when none.
func CaseFromSQL(sqlDatabase *gorm.DB, identityKey string) (string, error) {
	var patient database.Patient
	lookupError := sqlDatabase.Where("identity_key = ?", identityKey).First(&patient).Error
	if errors.Is(lookupError, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return patient.CaseID, lookupError
}

// SimilarByBirthDate lists other patients with the same birth date and a
// different name key (possible OCR typo or a namesake): flagged, never merged.
func SimilarByBirthDate(sqlDatabase *gorm.DB, identityKey, birthDate string) ([]database.Patient, error) {
	var patients []database.Patient
	findError := sqlDatabase.Where("birth_date = ? AND identity_key <> ?", birthDate, identityKey).Limit(5).Find(&patients).Error
	return patients, findError
}
