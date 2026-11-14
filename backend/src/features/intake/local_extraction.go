package intake

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"refratia/backend/src/features/classification"
	"refratia/backend/src/features/eyesuite"
	"refratia/backend/src/features/refractometry"
	"refratia/backend/src/features/retinography"
	"refratia/backend/src/features/specularmicroscopy"

	progress "refratia/backend/src/shared/progress"
)

func emitLocalFileResult(
	executionContext context.Context,
	filename,
	examType,
	eye,
	status,
	message string,
	analysis map[string]any,
) {
	if status == "extracted" &&
		analysis != nil {
		checkpointSaveError :=
			saveCompletedFileCheckpointFromContext(
				executionContext,
				filename,
				examType,
				eye,
				status,
				analysis,
			)

		if checkpointSaveError != nil {
			rememberFileCheckpointStorageError(
				executionContext,
				checkpointSaveError,
			)

			status = "failed"
			message =
				"Extração concluída, mas o resultado não pôde ser salvo"
			analysis = nil
		}
	}

	payload := map[string]any{
		"filename": filename,
		"examType": examType,
		"eye":      eye,
		"status":   status,
		"message":  message,
	}

	if analysis != nil {
		payload["analysis"] = analysis
	}

	progress.Emit(
		executionContext,
		progress.Event{
			Type:     "file_result",
			Percent:  100,
			Stage:    "file_result",
			Message:  message,
			Filename: filename,
			Payload:  payload,
		},
	)
}

// errAnteriorSegmentPhoto marks a retina-camera image shot in "Mode Cornea".
var errAnteriorSegmentPhoto = errors.New("foto de segmento anterior")

func tryExtractEyeSuiteLocal(
	executionContext context.Context,
	file uploadedFile,
	analysis map[string]any,
) bool {
	defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
	bundle, errorValue := eyesuite.ExtractPDF(
		executionContext,
		file.Data,
	)

	if errorValue != nil {
		return false
	}

	exams, _ := analysis["exams"].(map[string]any)

	if exams == nil {
		exams = map[string]any{}
		analysis["exams"] = exams
	}

	bundle.Exam["source"] = []any{
		file.Metadata.Filename,
	}

	exams["iol_calculation"] =
		bundle.Exam

	if bundle.Identity != nil {
		// The EyeSuite prints DD/MM/YYYY.
		mergeLocalPatientIdentity(
			analysis,
			file.Metadata.Filename,
			bundle.Identity.FullName,
			bundle.Identity.BirthDateRaw,
			bundle.Identity.TimestampRaw,
			"DMY",
		)
	}

	return true
}

func tryExtractRetinographyLocal(
	executionContext context.Context,
	file uploadedFile,
	analysis map[string]any,
) (bool, error) {
	defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
	result, errorValue := retinography.Extract(
		executionContext,
		file.Data,
	)
	if errorValue != nil {
		return false, errorValue
	}

	if result.Mode == "Cornea" {
		return false, errAnteriorSegmentPhoto
	}

	if result.Eye != "OD" && result.Eye != "OS" {
		return false, errors.New("retinografia: lateralidade OCR inválida")
	}

	exams, _ := analysis["exams"].(map[string]any)
	if exams == nil {
		exams = map[string]any{}
		analysis["exams"] = exams
	}

	exam, _ := exams["fundus_retinography"].(map[string]any)
	if exam == nil {
		exam = map[string]any{}
		exams["fundus_retinography"] = exam
	}

	if existing, ok := exam["id"].(string); ok {
		existing = strings.TrimSpace(existing)

		if existing != "" &&
			!strings.EqualFold(
				existing,
				strings.TrimSpace(result.PatientID),
			) {

			return false, errors.New(
				"retinografia: divergência de identidade entre imagens",
			)
		}
	}

	exam["id"] = result.PatientID
	exam["device_or_mode"] = result.Mode

	eyes, _ := exam["eyes"].(map[string]any)
	if eyes == nil {
		eyes = map[string]any{}
		exam["eyes"] = eyes
	}

	resultEyes, _ := result.Exam["eyes"].(map[string]any)

	eyePayload, _ := resultEyes[result.Eye].(map[string]any)
	if eyePayload == nil {
		return false, fmt.Errorf(
			"retinografia: payload OCR do olho %s ausente",
			result.Eye,
		)
	}

	eyes[result.Eye] = eyePayload

	sources, _ := exam["source"].([]any)

	alreadyPresent := false

	for _, source := range sources {
		if fmt.Sprint(source) == file.Metadata.Filename {
			alreadyPresent = true
			break
		}
	}

	if !alreadyPresent {
		exam["source"] = append(
			sources,
			file.Metadata.Filename,
		)
	}

	patient, _ := analysis["patient"].(map[string]any)
	if patient == nil {
		patient = map[string]any{}
		analysis["patient"] = patient
	}

	currentName, _ := patient["full_name"].(string)

	if strings.TrimSpace(currentName) == "" {
		patient["full_name"] = result.PatientID
	}

	verification, _ := analysis["verificacao_identidade"].([]any)

	hasVerification := false

	for _, raw := range verification {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if fmt.Sprint(item["source"]) == file.Metadata.Filename {
			hasVerification = true
			break
		}
	}

	if !hasVerification {
		analysis["verificacao_identidade"] = append(
			verification,
			map[string]any{
				"source":         file.Metadata.Filename,
				"nome_lido":      result.PatientID,
				"timestamp_lido": result.ExamDateTime,
				"confidence":     "deterministic_template",
				"method":         "local_ocr_tesseract",
			},
		)
	}

	return true, nil
}

func tryExtractSpecularMicroscopyLocal(executionContext context.Context, file uploadedFile, analysis map[string]any) (bool, error) {
	defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
	result, errorValue := specularmicroscopy.Extract(executionContext, file.Data)
	if errorValue != nil {
		return false, errorValue
	}
	eyes, _ := result.Exam["eyes"].(map[string]any)
	if len(eyes) == 0 {
		return false, fmt.Errorf("microscopia especular: nenhum olho extraído")
	}
	result.Exam["source"] = []any{file.Metadata.Filename}
	exams, _ := analysis["exams"].(map[string]any)
	if exams == nil {
		exams = map[string]any{}
		analysis["exams"] = exams
	}
	exams["specular_microscopy"] = result.Exam
	if result.ExamDateTime != "" {
		verification, _ := analysis["verificacao_identidade"].([]any)
		verification = append(verification, map[string]any{
			"source":         file.Metadata.Filename,
			"timestamp_lido": result.ExamDateTime,
			"confidence":     "deterministic_template",
			"method":         "local_ocr_tesseract",
		})
		analysis["verificacao_identidade"] = verification
	}
	return true, nil
}

func tryExtractRefractometryLocal(
	executionContext context.Context,
	file uploadedFile,
	analysis map[string]any,
) bool {
	defer validateTouchedEyes(analysis, file, snapshotEyes(analysis))
	var exam map[string]any
	var errorValue error
	if file.Metadata.ContentType == "application/pdf" {
		exam, errorValue = refractometry.ExtractPDF(executionContext, file.Data)
	} else {
		exam, errorValue = refractometry.ExtractImage(executionContext, file.Data)
	}
	if errorValue != nil {
		return false
	}

	exam["source"] = []any{file.Metadata.Filename}
	exams, _ := analysis["exams"].(map[string]any)
	if exams == nil {
		exams = map[string]any{}
		analysis["exams"] = exams
	}
	exams["refractometry"] = exam
	if timestamp, ok := exam["exam_datetime"].(string); ok && timestamp != "" {
		verification, _ := analysis["verificacao_identidade"].([]any)
		verification = append(verification, map[string]any{
			"source":         file.Metadata.Filename,
			"timestamp_lido": timestamp,
			"confidence":     "deterministic_template",
			"method":         "local_ocr_tesseract",
		})
		analysis["verificacao_identidade"] = verification
	}
	return true
}

func emitLocalPartial(
	executionContext context.Context,
	filename,
	message string,
	analysis map[string]any,
) {
	progress.Emit(
		executionContext,
		progress.Event{
			Type:     "partial",
			Percent:  100,
			Stage:    "result",
			Message:  message,
			Filename: filename,
			Payload:  analysis,
		},
	)
}

func extractPatientLocal(
	executionContext context.Context,
	files []uploadedFile,
) map[string]any {
	exams := map[string]any{}

	analysis := map[string]any{
		"exams": exams,
	}

	total := len(files)

	if total == 0 {
		return analysis
	}

	for index, file := range files {
		fileCtx := progress.WithRange(
			executionContext,
			index*100/total,
			(index+1)*100/total,
		)

		fileCtx = progress.TrackFileProgress(fileCtx)

		fileCtx = progress.WithFilename(
			fileCtx,
			file.Metadata.Filename,
		)

		fileCtx = withCurrentCheckpointFile(
			fileCtx,
			file,
		)

		if checkpointStorageError :=
			currentFileCheckpointStorageError(
				fileCtx,
			); checkpointStorageError != nil {
			return analysis
		}

		reused,
			checkpointPrepareError :=
			prepareFileForIdempotentProcessing(
				fileCtx,
				file,
				analysis,
			)

		if checkpointPrepareError != nil {
			rememberFileCheckpointStorageError(
				fileCtx,
				checkpointPrepareError,
			)

			return analysis
		}

		if reused {
			continue
		}

		examType, examEye := resolveLocalClassification(fileCtx, file, analysis)

		progress.Report(
			fileCtx,
			2,
			"document",
			fmt.Sprintf(
				"Processando documento %d/%d",
				index+1,
				total,
			),
		)

		dispatchLocalFile(
			fileCtx,
			file,
			examType,
			examEye,
			analysis,
		)
	}

	return analysis
}

func localResolvedExamKeys(analysis map[string]any) map[string]bool {
	resolved := map[string]bool{}
	exams, _ := analysis["exams"].(map[string]any)

	if exam, _ := exams["iol_calculation"].(map[string]any); exam != nil && !iolNeedsRepair(exam) {
		resolved["iol_calculation"] = true
	}

	if pentacamLocalComplete(analysis) {
		resolved["pentacam_corneal_tomography"] = true
	}

	if refractometryLocalComplete(analysis) {
		resolved["refractometry"] = true
	}

	if specularMicroscopyLocalComplete(analysis) {
		resolved["specular_microscopy"] = true
	}

	if retinographyLocalComplete(analysis) {
		resolved["fundus_retinography"] = true
	}

	return resolved
}

func refractometryLocalComplete(analysis map[string]any) bool {
	exams, _ := analysis["exams"].(map[string]any)
	exam, _ := exams["refractometry"].(map[string]any)
	eyes, _ := exam["eyes"].(map[string]any)
	if len(eyes) == 0 {
		return false
	}

	for _, eyeName := range []string{"OD", "OS"} {
		eye, _ := eyes[eyeName].(map[string]any)
		if eye == nil {
			return false
		}
		for _, field := range []string{"sphere_d", "cylinder_d", "axis_deg"} {
			if !hasNumberAtAnyPath(eye, []string{field}) {
				return false
			}
		}
	}
	return true
}

func retinographyLocalComplete(
	analysis map[string]any,
) bool {
	exams, _ := analysis["exams"].(map[string]any)

	exam, _ := exams["fundus_retinography"].(map[string]any)
	if exam == nil {
		return false
	}

	id, _ := exam["id"].(string)
	id = strings.TrimSpace(id)

	if id == "" {
		return false
	}

	mode, _ := exam["device_or_mode"].(string)

	if !strings.EqualFold(
		strings.TrimSpace(mode),
		"Retina",
	) {
		return false
	}

	eyes, _ := exam["eyes"].(map[string]any)

	for _, eye := range []string{"OD", "OS"} {
		payload, _ := eyes[eye].(map[string]any)
		if payload == nil {
			return false
		}

		patientID, _ := payload["patient_id"].(string)

		if !strings.EqualFold(
			strings.TrimSpace(patientID),
			id,
		) {
			return false
		}

		payloadEye, _ := payload["eye"].(string)

		if !strings.EqualFold(
			strings.TrimSpace(payloadEye),
			eye,
		) {
			return false
		}

		examDateTime, _ :=
			payload["exam_datetime"].(string)

		if strings.TrimSpace(examDateTime) == "" {
			return false
		}

		eyeMode, _ := payload["mode"].(string)

		if !strings.EqualFold(
			strings.TrimSpace(eyeMode),
			"Retina",
		) {
			return false
		}
	}

	return true
}

func specularMicroscopyLocalComplete(analysis map[string]any) bool {
	exams, _ := analysis["exams"].(map[string]any)
	exam, _ := exams["specular_microscopy"].(map[string]any)
	eyes, _ := exam["eyes"].(map[string]any)
	for _, eye := range []string{"OD", "OS"} {
		payload, _ := eyes[eye].(map[string]any)
		value, ok := payload["cell_density_cells_per_mm2"].(float64)
		if !ok || value <= 0 {
			return false
		}
	}
	return true
}

func localClaimedFiles(analysis map[string]any) map[string]bool {
	claimed := map[string]bool{}
	exams, _ := analysis["exams"].(map[string]any)

	for _, raw := range exams {
		exam, _ := raw.(map[string]any)
		sources, _ := exam["source"].([]any)
		for _, source := range sources {
			claimed[fmt.Sprint(source)] = true
		}
	}

	return claimed
}

func localPatientIdentityComplete(analysis map[string]any) bool {
	patient, _ := analysis["patient"].(map[string]any)
	name, _ := patient["full_name"].(string)
	birth, _ := patient["birth_date"].(string)

	return strings.TrimSpace(name) != "" && strings.TrimSpace(birth) != ""
}

func localIdentitySources(analysis map[string]any) map[string]bool {
	result := map[string]bool{}

	entries, _ := analysis["verificacao_identidade"].([]any)
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		source, _ := entry["source"].(string)
		if source != "" {
			result[source] = true
		}
	}

	return result
}

func specularMicroscopyFileExpectedEyes(
	file uploadedFile,
) []string {
	examType := file.Metadata.ExamType

	if examType == "" {
		examType =
			classification.ExamTypeFromFilename(
				file.Metadata.Filename,
			)
	}

	if examType != "MICROSCOPIA_ESPECULAR" {
		return nil
	}

	eye := strings.ToUpper(
		strings.TrimSpace(
			file.Metadata.Eye,
		),
	)

	if eye == "" {
		eye =
			classification.EyeFromFilename(
				file.Metadata.Filename,
			)
	}

	switch eye {
	case "OD":
		return []string{"OD"}

	case "OS":
		return []string{"OS"}

	case "AO":
		return []string{"OD", "OS"}

	default:
		// Compatibilidade conservadora com arquivos antigos:
		// se sabemos que é microscopia mas não sabemos a
		// lateralidade, não declaramos metade do exame como
		// completa silenciosamente.
		return []string{"OD", "OS"}
	}
}

func specularMicroscopyEyeComplete(
	analysis map[string]any,
	eye string,
) bool {
	exams, _ :=
		analysis["exams"].(map[string]any)

	exam, _ :=
		exams["specular_microscopy"].(map[string]any)

	eyes, _ :=
		exam["eyes"].(map[string]any)

	payload, _ :=
		eyes[eye].(map[string]any)

	value, ok :=
		payload["cell_density_cells_per_mm2"].(float64)

	return ok && value > 0
}

func specularMicroscopyLocalGaps(
	analysis map[string]any,
	files []uploadedFile,
) []string {
	expected := map[string]bool{}

	for _, file := range files {
		for _, eye := range specularMicroscopyFileExpectedEyes(file) {
			expected[eye] = true
		}
	}

	result := []string{}

	for _, eye := range []string{"OD", "OS"} {
		if !expected[eye] {
			continue
		}

		if specularMicroscopyEyeComplete(
			analysis,
			eye,
		) {
			continue
		}

		result =
			append(
				result,
				fmt.Sprintf(
					"specular_microscopy.eyes.%s.cell_density_cells_per_mm2",
					eye,
				),
			)
	}

	return result
}

func specularMicroscopyFileNeedsFallback(
	analysis map[string]any,
	file uploadedFile,
) bool {
	expected :=
		specularMicroscopyFileExpectedEyes(
			file,
		)

	if len(expected) == 0 {
		return false
	}

	for _, eye := range expected {
		if !specularMicroscopyEyeComplete(
			analysis,
			eye,
		) {
			return true
		}
	}

	return false
}

func specularMicroscopyLocalCompleteForFiles(
	analysis map[string]any,
	files []uploadedFile,
) bool {
	hasSpecular := false

	for _, file := range files {
		expected :=
			specularMicroscopyFileExpectedEyes(
				file,
			)

		if len(expected) == 0 {
			continue
		}

		hasSpecular = true

		for _, eye := range expected {
			if !specularMicroscopyEyeComplete(
				analysis,
				eye,
			) {
				return false
			}
		}
	}

	return hasSpecular
}

func localResolvedExamKeysForFiles(
	analysis map[string]any,
	files []uploadedFile,
) map[string]bool {
	resolved :=
		localResolvedExamKeys(
			analysis,
		)

	hasSpecular := false

	for _, file := range files {
		if len(
			specularMicroscopyFileExpectedEyes(file),
		) > 0 {
			hasSpecular = true
			break
		}
	}

	if hasSpecular {
		delete(
			resolved,
			"specular_microscopy",
		)

		if specularMicroscopyLocalCompleteForFiles(
			analysis,
			files,
		) {
			resolved["specular_microscopy"] =
				true
		}
	}

	return resolved
}

func localFallbackFiles(analysis map[string]any, files []uploadedFile) []uploadedFile {
	claimed := localClaimedFiles(analysis)
	identitySources := localIdentitySources(analysis)
	identityComplete := localPatientIdentityComplete(analysis)

	result := make([]uploadedFile, 0, len(files))
	for _, file := range files {
		if specularMicroscopyFileNeedsFallback(
			analysis,
			file,
		) {
			result = append(
				result,
				file,
			)
			continue
		}

		if identityComplete &&
			claimed[file.Metadata.Filename] &&
			identitySources[file.Metadata.Filename] {
			continue
		}

		result = append(result, file)
	}

	return result
}

func collectLocalGaps(analysis map[string]any, files []uploadedFile) []string {
	gaps := []string{}
	seen := map[string]bool{}

	add := func(gap string) {
		if !seen[gap] {
			seen[gap] = true
			gaps = append(gaps, gap)
		}
	}

	if !localPatientIdentityComplete(analysis) {
		add("patient_identity")
	}

	for _, gap := range pentacamLocalGaps(analysis) {
		add(gap)
	}

	for _, gap := range specularMicroscopyLocalGaps(
		analysis,
		files,
	) {
		add(gap)
	}

	claimed := localClaimedFiles(analysis)
	identitySources := localIdentitySources(analysis)

	for _, file := range files {
		if !claimed[file.Metadata.Filename] {
			add("unresolved_file")
			continue
		}

		if !identitySources[file.Metadata.Filename] {
			add("document_identity")
		}
	}

	return gaps
}

func mergeFallbackAnalysis(local, fallback map[string]any) {
	var fallbackIdentity []any

	if entries, ok := fallback["verificacao_identidade"].([]any); ok {
		fallbackIdentity = entries
		delete(fallback, "verificacao_identidade")
	}

	mergeMissingValues(local, fallback)

	if len(fallbackIdentity) > 0 {
		current, _ := local["verificacao_identidade"].([]any)
		local["verificacao_identidade"] = append(current, fallbackIdentity...)
	}
}

func extractionPromptForLocalGaps(
	analysis map[string]any,
	files []uploadedFile,
	gaps []string,
) string {
	resolved :=
		localResolvedExamKeysForFiles(
			analysis,
			files,
		)
	keys := make([]string, 0, len(resolved))

	for key := range resolved {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	resolvedText := "nenhum"
	if len(keys) > 0 {
		resolvedText = strings.Join(keys, ", ")
	}

	return extractionPrompt + fmt.Sprintf(`

MODO FALLBACK LOCAL-FIRST:

O backend determinístico já processou os arquivos antes desta chamada.

Exames completamente resolvidos localmente:
%s

Gaps exatos ainda conhecidos:
%s

REGRAS OBRIGATÓRIAS DO FALLBACK:
- preencha somente informações realmente ausentes;
- em exames parcialmente resolvidos, retorne somente os subcampos correspondentes aos gaps acima;
- não reinterprete, corrija ou substitua valores que já foram resolvidos localmente;
- para patient_identity/document_identity, extraia somente a identificação necessária;
- não invente valores para gaps ilegíveis;
- campos não solicitados podem ser omitidos ou permanecer null;
- preserve lateralidade OD/OS do documento.

Os valores locais são autoritativos e serão preservados pelo backend.`,
		resolvedText,
		strings.Join(gaps, ", "),
	)
}

func stripLocallyResolvedExams(
	analysis map[string]any,
	resolved map[string]bool,
) {
	exams, _ := analysis["exams"].(map[string]any)

	for key := range resolved {
		delete(exams, key)
	}

	// decodeAnalysis pode ter registrado um warning intermediário antes
	// de descobrirmos que esse exame já foi resolvido localmente.
	//
	// Ao remover o exame do fallback, removemos também warnings referentes
	// a ele para que extraction_notes descreva o estado final, não um estado
	// intermediário da pipeline.
	notes, _ := analysis["extraction_notes"].(map[string]any)
	if notes == nil {
		return
	}

	items, _ := notes["invalid_exams"].([]any)
	if len(items) == 0 {
		return
	}

	filtered := make([]any, 0, len(items))

	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}

		exam, _ := item["exam"].(string)

		if resolved[exam] {
			continue
		}

		filtered = append(filtered, raw)
	}

	if len(filtered) == 0 {
		delete(notes, "invalid_exams")
		return
	}

	notes["invalid_exams"] = filtered
}

// resolveLocalClassification returns the contract exam type and eye of a
// file. The declared classification (frontend or file name) is a hint; for
// PDFs the device title printed on the page decides, and a disagreement is
// recorded as a classification diagnostic.
func resolveLocalClassification(
	executionContext context.Context,
	file uploadedFile,
	analysis map[string]any,
) (string, string) {
	examType, _ := classification.ContractExamType(file.Metadata.ExamType)
	if examType == "" {
		examType = classification.ExamTypeFromFilename(file.Metadata.Filename)
	}

	eye := strings.ToUpper(strings.TrimSpace(file.Metadata.Eye))
	if eye == "" {
		eye = classification.EyeFromFilename(file.Metadata.Filename)
	}

	// Content decides when it recognises the device; otherwise the declared
	// type stands. Images are only inspected when nothing was declared, to
	// avoid an extra OCR pass over every photo.
	isPDF := file.Metadata.ContentType == "application/pdf"
	if isPDF || examType == "" {
		byContent, contentEye, _ := classification.ClassifyContentEye(
			executionContext,
			file.Metadata.ContentType,
			file.Data,
		)
		if eye == "" && contentEye != "" {
			eye = contentEye
		}
		if byContent != "" && byContent != examType {
			if examType != "" {
				recordClassificationConflict(analysis, file, byContent)
			}
			examType = byContent
		}
	}

	return examType, eye
}

func recordClassificationConflict(
	analysis map[string]any,
	file uploadedFile,
	byContent string,
) {
	diagnostics, _ := analysis["extraction_diagnostics"].([]any)
	analysis["extraction_diagnostics"] = append(diagnostics, map[string]any{
		"file_sha256": file.Metadata.SHA256,
		"exam_type":   byContent,
		"eye":         file.Metadata.Eye,
		"field":       "exam_type",
		"status":      "conflicting",
		"stage":       "classification",
		"strategy":    "device_title",
		"error":       "tipo informado diverge do conteúdo do documento",
	})
}

// dispatchLocalFile runs the extractor for the contract exam type and emits
// exactly one file_result for the file, whatever happens.
func dispatchLocalFile(
	executionContext context.Context,
	file uploadedFile,
	examType, eye string,
	analysis map[string]any,
) {
	filename := file.Metadata.Filename
	isPDF := file.Metadata.ContentType == "application/pdf"

	finish := func(status, message string, withAnalysis bool) {
		var payload map[string]any
		if withAnalysis {
			payload = analysis
		}
		emitLocalFileResult(executionContext, filename, examType, eye, status, message, payload)
	}
	failed := func(stage, message string) {
		progress.Report(executionContext, 96, stage, message)
		finish("failed", message, false)
	}
	partialThenDone := func(stage, partialMessage, message string) {
		progress.Report(executionContext, 96, stage, partialMessage)
		emitLocalPartial(executionContext, filename, partialMessage, analysis)
		finish("extracted", message, true)
	}

	switch examType {
	case "fundus_retinography":
		progress.Report(executionContext, 8, "retinography_preprocess", "Extraindo retinografia")
		ok, errorValue := tryExtractRetinographyLocal(progress.WithRange(executionContext, 8, 96), file, analysis)
		switch {
		case ok:
			emitLocalPartial(executionContext, filename, "Retinografia concluída", analysis)
			finish("extracted", "Retinografia extraída por OCR local", true)
		case errors.Is(errorValue, errAnteriorSegmentPhoto):
			// Same camera, "Mode Cornea": recognised, out of protocol.
			examType = "anterior_segment_photo"
			progress.Report(executionContext, 96, examType, "Foto de segmento anterior identificada")
			finish("identified", "Foto de segmento anterior (fora do protocolo): identificada, sem extração clínica", false)
		default:
			failed("retinography_parse", "Retinografia não pôde ser extraída localmente")
		}

	case "specular_microscopy":
		progress.Report(executionContext, 8, "microscopy_preprocess", "Extraindo microscopia especular")
		ok, _ := tryExtractSpecularMicroscopyLocal(progress.WithRange(executionContext, 8, 96), file, analysis)
		if !ok {
			failed("microscopy_parse", "Microscopia especular não pôde ser extraída localmente")
			return
		}
		status, message, partialMessage := "extracted", "Microscopia especular extraída", "Microscopia especular concluída"
		if specularMicroscopyFileNeedsFallback(analysis, file) {
			status, message, partialMessage = "partial", "Microscopia especular parcialmente extraída", "Microscopia especular parcial"
		}
		emitLocalPartial(executionContext, filename, partialMessage, analysis)
		finish(status, message, true)

	case "refractometry":
		progress.Report(executionContext, 8, "refractometry", "Extraindo refratometria")
		if tryExtractRefractometryLocal(progress.WithRange(executionContext, 8, 96), file, analysis) {
			partialThenDone("refractometry", "Refratometria concluída", "Refratometria extraída localmente")
		} else {
			failed("refractometry", "Refratometria não pôde ser extraída localmente")
		}

	case "iol_calculation":
		if !isPDF {
			failed("eyesuite", "EyeSuite exige PDF")
			return
		}
		progress.Report(executionContext, 8, "eyesuite", "Extraindo biometria EyeSuite")
		if tryExtractEyeSuiteLocal(executionContext, file, analysis) {
			partialThenDone("eyesuite", "EyeSuite concluído", "EyeSuite extraído")
		} else {
			failed("eyesuite", "EyeSuite não pôde ser extraído localmente")
		}

	case "pentacam_corneal_tomography":
		if !isPDF {
			failed("pentacam", "Pentacam exige PDF")
			return
		}
		progress.Report(executionContext, 8, "pentacam", "Extraindo Pentacam")
		if tryExtractPentacamLocal(progress.WithRange(executionContext, 8, 96), file, analysis) {
			partialThenDone("pentacam", "Pentacam concluído", "Pentacam extraído")
		} else {
			failed("pentacam", "Pentacam não pôde ser extraído localmente")
		}

	case "anterior_segment_photo", "corneal_topography_other", "oct_retina":
		// Recognised, out of the extraction protocol: kept for identity and
		// record counting only.
		progress.Report(executionContext, 96, examType, "Arquivo identificado; sem extrator clínico no protocolo")
		finish("identified", "Documento identificado (fora do protocolo de extração)", false)

	default:
		failed("classification", "Tipo de documento não reconhecido; classifique manualmente")
	}
}
