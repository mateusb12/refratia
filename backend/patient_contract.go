package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

var officialExamKeys = map[string]bool{
	"fundus_retinography":         true,
	"iol_calculation":             true,
	"refractometry":               true,
	"oct_retina":                  true,
	"pentacam_corneal_tomography": true,
	"specular_microscopy":         true,
}

// patientJSONContract is the backend output contract. Exam payloads remain
// source-specific, while this envelope and its exam names are stable.
type patientJSONContract struct {
	SchemaVersion   string                     `json:"schema_version"`
	GeneratedOn     string                     `json:"generated_on"`
	Language        string                     `json:"language"`
	Patient         map[string]any             `json:"patient"`
	Facility        map[string]any             `json:"facility,omitempty"`
	Conventions     map[string]any             `json:"conventions,omitempty"`
	SourceFiles     []map[string]any           `json:"source_files"`
	Exams           map[string]json.RawMessage `json:"exams"`
	ExtractionNotes map[string]any             `json:"extraction_notes,omitempty"`
}

func validatePatientJSON(raw string) error {
	var contract patientJSONContract
	if err := json.Unmarshal([]byte(raw), &contract); err != nil {
		return errors.New("o serviço de extração não retornou um JSON válido")
	}
	if contract.Patient == nil {
		return errors.New("o JSON extraído não contém patient")
	}
	if contract.Exams == nil {
		return errors.New("o JSON extraído não contém exams")
	}
	for key, payload := range contract.Exams {
		if !officialExamKeys[key] {
			return fmt.Errorf("tipo de exame não previsto no contrato: %s", key)
		}
		var exam map[string]json.RawMessage
		if err := json.Unmarshal(payload, &exam); err != nil || exam == nil {
			return fmt.Errorf("exame inválido no contrato: %s", key)
		}
		var source []string
		if err := json.Unmarshal(exam["source"], &source); err != nil || source == nil {
			return fmt.Errorf("exame sem source no contrato: %s", key)
		}
	}
	return nil
}
