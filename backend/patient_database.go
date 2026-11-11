package main

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"refratia/backend/database"
	patientfeature "refratia/backend/features/patient"
)

func upsertPatientFromAnalysis(analysis map[string]any) (*database.Patient, error) {
	if appDatabase == nil || analysis == nil {
		return nil, nil
	}

	identityKey, identifiable := patientfeature.Identity(analysis)
	if !identifiable {
		return nil, nil
	}

	patientData, ok := analysis["patient"].(map[string]any)
	if !ok {
		return nil, nil
	}
	fullName, _ := patientData["full_name"].(string)
	fullName = strings.TrimSpace(fullName)
	birthDate, ok := patientfeature.CanonicalBirthDate(analysis)
	if fullName == "" || !ok {
		return nil, nil
	}

	var patient database.Patient
	lookupError := appDatabase.Gorm.Where("identity_key = ?", identityKey).First(&patient).Error
	switch {
	case lookupError == nil:
		if updateError := appDatabase.Gorm.Model(&patient).Updates(map[string]any{
			"full_name":  fullName,
			"birth_date": birthDate,
		}).Error; updateError != nil {
			return nil, fmt.Errorf("update patient: %w", updateError)
		}
		return &patient, nil
	case errors.Is(lookupError, gorm.ErrRecordNotFound):
		patient = database.Patient{
			IdentityKey: identityKey,
			FullName:    fullName,
			BirthDate:   birthDate,
		}
		if createError := appDatabase.Gorm.Create(&patient).Error; createError != nil {
			return nil, fmt.Errorf("create patient: %w", createError)
		}
		return &patient, nil
	default:
		return nil, fmt.Errorf("lookup patient: %w", lookupError)
	}
}
