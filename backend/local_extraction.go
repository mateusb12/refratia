package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"refratia/backend/features/eyesuite"
	"refratia/backend/features/retinography"
	"refratia/backend/features/specularmicroscopy"

	progressutil "refratia/backend/shared/progress"

	patientfeature "refratia/backend/features/patient"
)

func localExamTypeFromFilename(
	filename string,
) string {
	parts := strings.Split(filename, "__")

	if len(parts) < 2 {
		return ""
	}

	return strings.ToUpper(
		strings.TrimSpace(parts[0]),
	)
}

func localExamEyeFromFilename(
	filename string,
) string {
	parts := strings.Split(filename, "__")

	if len(parts) < 2 {
		return ""
	}

	return strings.ToUpper(
		strings.TrimSpace(parts[1]),
	)
}

func emitLocalFileResult(
	ctx context.Context,
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
				ctx,
				filename,
				examType,
				eye,
				status,
				analysis,
			)

		if checkpointSaveError != nil {
			rememberFileCheckpointStorageError(
				ctx,
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

	progressutil.Emit(
		ctx,
		progressutil.Event{
			Type:     "file_result",
			Percent:  100,
			Stage:    "file_result",
			Message:  message,
			Filename: filename,
			Payload:  payload,
		},
	)
}

func tryExtractEyeSuiteLocal(
	ctx context.Context,
	file uploadedFile,
	analysis map[string]any,
) bool {
	bundle, err := eyesuite.ExtractPDF(
		ctx,
		file.Data,
	)

	if err != nil {
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
		patient := map[string]any{
			"full_name": bundle.Identity.FullName,
		}

		analysis["patient"] = patient

		analysis["verificacao_identidade"] =
			[]any{
				map[string]any{
					"source":          file.Metadata.Filename,
					"nome_lido":       bundle.Identity.FullName,
					"nascimento_lido": bundle.Identity.BirthDateRaw,
					"timestamp_lido":  bundle.Identity.TimestampRaw,
					"confidence":      "deterministic_template",
					"method":          "local_ocr_tesseract",
				},
			}

		if birthDate, ok :=
			patientfeature.CanonicalBirthDate(
				analysis,
			); ok {

			patient["birth_date"] =
				birthDate
		}
	}

	return true
}

func tryExtractRetinographyLocal(
	ctx context.Context,
	file uploadedFile,
	analysis map[string]any,
) (bool, error) {
	result, err := retinography.Extract(
		ctx,
		file.Data,
	)
	if err != nil {
		return false, err
	}

	if result.Eye != "OD" && result.Eye != "OS" {
		return false, fmt.Errorf(
			"retinografia: lateralidade OCR inválida: %q",
			result.Eye,
		)
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

			return false, fmt.Errorf(
				"retinografia: divergência de identidade entre imagens: %q != %q",
				existing,
				result.PatientID,
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

func tryExtractSpecularMicroscopyLocal(ctx context.Context, file uploadedFile, analysis map[string]any) (bool, error) {
	result, err := specularmicroscopy.Extract(ctx, file.Data)
	if err != nil {
		return false, err
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
	return true, nil
}

func emitLocalPartial(
	ctx context.Context,
	filename,
	message string,
	analysis map[string]any,
) {
	progressutil.Emit(
		ctx,
		progressutil.Event{
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
	ctx context.Context,
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
		fileCtx := progressutil.WithRange(
			ctx,
			index*100/total,
			(index+1)*100/total,
		)

		fileCtx = progressutil.TrackFileProgress(fileCtx)

		fileCtx = progressutil.WithFilename(
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

		examType :=
			file.Metadata.ExamType
		if examType == "" {
			examType = localExamTypeFromFilename(file.Metadata.Filename)
		}

		examEye :=
			file.Metadata.Eye
		if examEye == "" {
			examEye = localExamEyeFromFilename(file.Metadata.Filename)
		}

		progressutil.Report(
			fileCtx,
			2,
			"document",
			fmt.Sprintf(
				"Processando documento %d/%d",
				index+1,
				total,
			),
		)

		if examType == "RETINA" {
			progressutil.Report(
				fileCtx,
				8,
				"retinography_preprocess",
				"Extraindo retinografia",
			)

			ok, err := tryExtractRetinographyLocal(
				progressutil.WithRange(
					fileCtx,
					8,
					96,
				),
				file,
				analysis,
			)

			if ok {
				emitLocalPartial(
					fileCtx,
					file.Metadata.Filename,
					"Retinografia concluída",
					analysis,
				)

				emitLocalFileResult(
					fileCtx,
					file.Metadata.Filename,
					examType,
					examEye,
					"extracted",
					"Retinografia extraída por OCR local",
					analysis,
				)

				continue
			}

			message :=
				"retinografia: extração local falhou"

			if err != nil {
				message = err.Error()
			}

			progressutil.Report(
				fileCtx,
				96,
				"retinography_parse",
				"Retinografia não pôde ser extraída localmente",
			)

			emitLocalFileResult(
				fileCtx,
				file.Metadata.Filename,
				examType,
				examEye,
				"failed",
				message,
				nil,
			)

			continue
		}

		if examType == "MICROSCOPIA_ESPECULAR" {
			progressutil.Report(fileCtx, 8, "microscopy_preprocess", "Extraindo microscopia especular")
			ok, err := tryExtractSpecularMicroscopyLocal(progressutil.WithRange(fileCtx, 8, 96), file, analysis)
			if ok {
				status := "extracted"
				message :=
					"Microscopia especular extraída"
				partialMessage :=
					"Microscopia especular concluída"

				if specularMicroscopyFileNeedsFallback(
					analysis,
					file,
				) {
					status = "partial"
					message =
						"Microscopia especular parcialmente extraída"
					partialMessage =
						"Microscopia especular parcial"
				}

				emitLocalPartial(
					fileCtx,
					file.Metadata.Filename,
					partialMessage,
					analysis,
				)

				emitLocalFileResult(
					fileCtx,
					file.Metadata.Filename,
					examType,
					examEye,
					status,
					message,
					analysis,
				)

				continue
			}
			progressutil.Report(fileCtx, 96, "microscopy_parse", "Microscopia especular não pôde ser extraída localmente")
			emitLocalFileResult(fileCtx, file.Metadata.Filename, examType, examEye, "failed", err.Error(), nil)
			continue
		}

		// Arquivos de imagem já são identificados
		// pelo filename, mas ainda não possuem todos
		// os extratores locais implementados.
		if file.Metadata.ContentType != "application/pdf" {
			progressutil.Report(
				fileCtx,
				96,
				strings.ToLower(examType),
				"Arquivo identificado pelo nome",
			)

			emitLocalFileResult(
				fileCtx,
				file.Metadata.Filename,
				examType,
				examEye,
				"identified",
				"Identificado pelo filename; extrator clínico específico ainda não implementado",
				nil,
			)

			continue
		}

		switch examType {
		case "EYESUITE":
			progressutil.Report(
				fileCtx,
				8,
				"eyesuite",
				"Extraindo biometria EyeSuite",
			)

			if tryExtractEyeSuiteLocal(
				fileCtx,
				file,
				analysis,
			) {
				progressutil.Report(
					fileCtx,
					96,
					"eyesuite",
					"EyeSuite extraído localmente",
				)

				emitLocalPartial(
					fileCtx,
					file.Metadata.Filename,
					"EyeSuite concluído",
					analysis,
				)

				emitLocalFileResult(
					fileCtx,
					file.Metadata.Filename,
					"EYESUITE",
					examEye,
					"extracted",
					"EyeSuite extraído",
					analysis,
				)

				continue
			}

			progressutil.Report(
				fileCtx,
				96,
				"eyesuite",
				"EyeSuite não pôde ser extraído localmente",
			)

			emitLocalFileResult(
				fileCtx,
				file.Metadata.Filename,
				"EYESUITE",
				examEye,
				"failed",
				"Falha na extração EyeSuite",
				nil,
			)

			continue

		case "PENTACAM":
			progressutil.Report(
				fileCtx,
				8,
				"pentacam",
				"Extraindo Pentacam",
			)

			if tryExtractPentacamLocal(
				progressutil.WithRange(
					fileCtx,
					8,
					96,
				),
				file,
				analysis,
			) {
				emitLocalPartial(
					fileCtx,
					file.Metadata.Filename,
					"Pentacam concluído",
					analysis,
				)

				emitLocalFileResult(
					fileCtx,
					file.Metadata.Filename,
					"PENTACAM",
					examEye,
					"extracted",
					"Pentacam extraído",
					analysis,
				)

				continue
			}

			progressutil.Report(
				fileCtx,
				96,
				"pentacam",
				"Pentacam não pôde ser extraído localmente",
			)

			emitLocalFileResult(
				fileCtx,
				file.Metadata.Filename,
				"PENTACAM",
				examEye,
				"failed",
				"Falha na extração Pentacam",
				nil,
			)

			continue

		case "RETINA",
			"CORNEA",
			"MICROSCOPIA_ESPECULAR":

			progressutil.Report(
				fileCtx,
				100,
				strings.ToLower(examType),
				"Arquivo identificado; extrator local específico ainda não configurado",
			)

			continue
		}

		// Compatibilidade com arquivos antigos ainda
		// não padronizados: mantém o comportamento
		// heurístico anterior.
		progressutil.Report(
			fileCtx,
			8,
			"eyesuite",
			"Verificando biometria EyeSuite",
		)

		if tryExtractEyeSuiteLocal(
			fileCtx,
			file,
			analysis,
		) {
			emitLocalPartial(
				fileCtx,
				file.Metadata.Filename,
				"EyeSuite concluído",
				analysis,
			)

			continue
		}

		progressutil.Report(
			fileCtx,
			14,
			"pentacam",
			"Verificando Pentacam",
		)

		if tryExtractPentacamLocal(
			progressutil.WithRange(
				fileCtx,
				14,
				100,
			),
			file,
			analysis,
		) {
			emitLocalPartial(
				fileCtx,
				file.Metadata.Filename,
				"Pentacam concluído",
				analysis,
			)

			continue
		}

		progressutil.Report(
			fileCtx,
			100,
			"document",
			"Documento reservado para fallback",
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

	if specularMicroscopyLocalComplete(analysis) {
		resolved["specular_microscopy"] = true
	}

	if retinographyLocalComplete(analysis) {
		resolved["fundus_retinography"] = true
	}

	return resolved
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
			localExamTypeFromFilename(
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
			localExamEyeFromFilename(
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
