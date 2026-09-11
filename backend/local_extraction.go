package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"refratia/backend/features/eyesuite"

	progressutil "refratia/backend/shared/progress"

	patientfeature "refratia/backend/features/patient"
)

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

		progressutil.Report(
			fileCtx,
			2,
			"document",
			fmt.Sprintf(
				"Identificando documento %d/%d",
				index+1,
				total,
			),
		)

		if file.Metadata.ContentType != "application/pdf" {
			progressutil.Report(
				fileCtx,
				100,
				"document",
				"Documento reservado para etapa complementar",
			)
			continue
		}

		progressutil.Report(
			fileCtx,
			8,
			"eyesuite",
			"Verificando biometria EyeSuite",
		)

		if bundle, err :=
			eyesuite.ExtractPDF(
				fileCtx,
				file.Data,
			); err == nil {
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

			progressutil.Report(
				fileCtx,
				100,
				"eyesuite",
				"EyeSuite extraído localmente",
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

	return resolved
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

func localFallbackFiles(analysis map[string]any, files []uploadedFile) []uploadedFile {
	claimed := localClaimedFiles(analysis)
	identitySources := localIdentitySources(analysis)
	identityComplete := localPatientIdentityComplete(analysis)

	result := make([]uploadedFile, 0, len(files))
	for _, file := range files {
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
	gaps []string,
) string {
	resolved := localResolvedExamKeys(analysis)
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
