package main

import (
	"context"
	"fmt"
	"strings"

	progress "refratia/backend/shared/progress"

	patientfeature "refratia/backend/features/patient"
	"refratia/backend/features/pentacam"
)

func tryExtractPentacamLocal(
	ctx context.Context,
	file uploadedFile,
	analysis map[string]any,
) bool {
	defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
	progress.Report(
		ctx,
		2,
		"pentacam",
		"Identificando lateralidade do Pentacam",
	)

	eye, err := pentacam.DetectEye(
		ctx,
		file.Data,
	)

	if err != nil {
		return false
	}

	eyeLabel := "olho direito"
	if eye == "OS" {
		eyeLabel = "olho esquerdo"
	}

	progress.Report(
		ctx,
		8,
		"pentacam",
		"Pentacam — "+eyeLabel,
	)

	exam, hints, err := pentacam.ExtractPDFWithHints(
		progress.WithRange(ctx, 8, 96),
		file.Data,
	)

	if err != nil {
		return false
	}

	if identity, identityErr := pentacam.ExtractIdentity(ctx, file.Data); identityErr == nil {
		mergeLocalPatientIdentity(
			analysis,
			file.Metadata.Filename,
			identity.FullName,
			identity.BirthDateRaw,
			identity.ExamDateRaw,
			"MDY", // the Pentacam prints MM/DD/YYYY
		)
	}

	mergeLocalPentacamExam(
		analysis,
		eye,
		file.Metadata.Filename,
		exam,
	)
	setFieldHints(analysis, "pentacam_corneal_tomography", eye, hints)

	progress.Report(
		ctx,
		100,
		"pentacam",
		"Pentacam concluído — "+eyeLabel,
	)

	return true
}

// mergeLocalPatientIdentity records one document's identity reading and then
// lets the patient feature derive patient.full_name / birth_date from ALL the
// evidence. Order matters: the raw reading (with the device's declared
// day/month order) is appended first and canonicalised afterwards, so
// patient.birth_date is ISO or absent, never the raw device string.
func mergeLocalPatientIdentity(
	analysis map[string]any,
	source,
	fullName,
	birthDateRaw,
	examDateRaw,
	dateOrder string,
) {
	patient, _ := analysis["patient"].(map[string]any)
	if patient == nil {
		patient = map[string]any{}
		analysis["patient"] = patient
	}

	if current, _ := patient["full_name"].(string); strings.TrimSpace(current) == "" {
		patient["full_name"] = fullName
	}

	verification, _ := analysis["verificacao_identidade"].([]any)
	known := false
	for _, raw := range verification {
		entry, _ := raw.(map[string]any)
		if fmt.Sprint(entry["source"]) == source {
			known = true
		}
	}

	if !known {
		entry := map[string]any{
			"source":          source,
			"nome_lido":       fullName,
			"nascimento_lido": birthDateRaw,
			"timestamp_lido":  examDateRaw,
			"confidence":      "deterministic_template",
			"method":          "local_ocr_tesseract",
		}
		if dateOrder != "" {
			entry["date_order"] = dateOrder
		}
		analysis["verificacao_identidade"] = append(verification, entry)
	}

	patientfeature.NormalizeIdentityFields(analysis)
	if _, ok := patientfeature.CanonicalBirthDate(analysis); !ok {
		delete(patient, "birth_date")
	}
}

func mergeLocalPentacamExam(
	analysis map[string]any,
	eye string,
	filename string,
	eyeExam map[string]any,
) {
	exams, _ := analysis["exams"].(map[string]any)
	if exams == nil {
		exams = map[string]any{}
		analysis["exams"] = exams
	}

	exam, _ := exams["pentacam_corneal_tomography"].(map[string]any)
	if exam == nil {
		exam = map[string]any{}
		exams["pentacam_corneal_tomography"] = exam
	}

	eyes, _ := exam["eyes"].(map[string]any)
	if eyes == nil {
		eyes = map[string]any{}
		exam["eyes"] = eyes
	}

	eyes[eye] = eyeExam

	sources, _ := exam["source"].([]any)
	for _, source := range sources {
		if fmt.Sprint(source) == filename {
			return
		}
	}

	exam["source"] = append(sources, filename)
}

var pentacamRequiredNumericPaths = [][]string{
	{"anterior_cornea", "k1_d"},
	{"anterior_cornea", "k2_d"},
	{"anterior_cornea", "km_d"},
	{"anterior_cornea", "astigmatism_d"},
	{"pachymetry", "thinnest_um"},
	{"belin_ambrosio", "d"},
	{"belin_ambrosio", "art_max"},
	{"topometric_indices_8mm", "isv"},
	{"topometric_indices_8mm", "iva"},
	{"topometric_indices_8mm", "iha"},
	{"topometric_indices_8mm", "ihd"},
	{"topometric_indices_8mm", "ki"},
	{"topometric_indices_8mm", "cki"},
	{"corneal_rings", "zernike", "5mm", "z31_coma"},
	{"anterior_segment", "internal_anterior_chamber_depth_mm"},
	{"cataract_preop", "total_corneal_z40_6mm_um"},
}

func pentacamEyeLocalGaps(
	eyeName string,
	eye map[string]any,
) []string {
	result := []string{}

	for _, path := range pentacamRequiredNumericPaths {
		if hasNumberAtAnyPath(eye, path) {
			continue
		}

		result = append(
			result,
			"pentacam."+eyeName+"."+strings.Join(path, "."),
		)
	}

	topometric, _ := eye["topometric_indices_8mm"].(map[string]any)
	tkc := ""

	if topometric != nil && topometric["tkc"] != nil {
		tkc = strings.TrimSpace(fmt.Sprint(topometric["tkc"]))
	}

	if tkc == "" {
		result = append(
			result,
			"pentacam."+eyeName+".topometric_indices_8mm.tkc",
		)
	}

	return result
}

func pentacamLocalGaps(analysis map[string]any) []string {
	exams, _ := analysis["exams"].(map[string]any)
	exam, _ := exams["pentacam_corneal_tomography"].(map[string]any)

	if exam == nil {
		return nil
	}

	eyes, _ := exam["eyes"].(map[string]any)
	if eyes == nil {
		return nil
	}

	result := []string{}

	for _, eyeName := range []string{"OD", "OS"} {
		eye, _ := eyes[eyeName].(map[string]any)
		if eye == nil {
			continue
		}

		result = append(
			result,
			pentacamEyeLocalGaps(eyeName, eye)...,
		)
	}

	return result
}

func pentacamLocalComplete(analysis map[string]any) bool {
	exams, _ := analysis["exams"].(map[string]any)
	exam, _ := exams["pentacam_corneal_tomography"].(map[string]any)

	if exam == nil {
		return false
	}

	eyes, _ := exam["eyes"].(map[string]any)
	if eyes == nil {
		return false
	}

	foundEye := false

	for _, eyeName := range []string{"OD", "OS"} {
		eye, _ := eyes[eyeName].(map[string]any)
		if eye == nil {
			continue
		}

		foundEye = true

		if len(pentacamEyeLocalGaps(eyeName, eye)) > 0 {
			return false
		}
	}

	return foundEye
}
