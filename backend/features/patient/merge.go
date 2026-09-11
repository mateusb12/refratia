package patient

import (
	"encoding/json"
	"strings"
)

func Merge(existing, incoming map[string]any) map[string]any {
	if existing == nil {
		return cloneMap(incoming)
	}

	merged := cloneMap(existing)

	// Envelope / paciente / metadados:
	// valores novos não nulos prevalecem, mas campos ausentes no upload novo
	// continuam preservados.
	for key, incomingValue := range incoming {
		if key == "exams" || key == "source_files" {
			continue
		}

		if incomingValue == nil {
			continue
		}

		incomingMap, incomingIsMap := incomingValue.(map[string]any)
		existingMap, existingIsMap := merged[key].(map[string]any)

		if incomingIsMap && existingIsMap {
			mergeMapPreferIncoming(existingMap, incomingMap)
			continue
		}

		merged[key] = cloneValue(incomingValue)
	}

	// source_files representa a evidência clínica ATUAL.
	// Se chegaram novos arquivos do mesmo exame+olho, eles substituem
	// as referências antigas daquele mesmo grupo.
	merged["source_files"] = mergeSourceFiles(
		existing["source_files"],
		incoming["source_files"],
	)

	existingExams, _ := merged["exams"].(map[string]any)
	if existingExams == nil {
		existingExams = map[string]any{}
	}

	incomingExams, _ := incoming["exams"].(map[string]any)
	exactGroups := exactReuploadGroups(
		existing["source_files"],
		incoming["source_files"],
	)

	for examKey, incomingExam := range incomingExams {
		effectiveExam, shouldMerge := incomingExamForMerge(
			examKey,
			incomingExam,
			exactGroups,
		)
		if !shouldMerge {
			continue
		}

		existingExams[examKey] = mergeExamPayload(
			existingExams[examKey],
			effectiveExam,
		)
	}

	merged["exams"] = existingExams

	// Cases legados podem carregar aliases contraditórios produzidos
	// pelo modelo. Reaplicamos a identidade canônica ao resultado final.
	NormalizeIdentityFields(merged)

	// exam.source deve apontar somente para a evidência clínica atual,
	// em ordem determinística OD -> OS -> AO.
	rebuildExamSources(merged)

	return merged
}

func mergeExamPayload(existingRaw, incomingRaw any) any {
	incoming, incomingOK := incomingRaw.(map[string]any)
	if !incomingOK || incoming == nil {
		return cloneValue(incomingRaw)
	}

	existing, existingOK := existingRaw.(map[string]any)
	if !existingOK || existing == nil {
		return cloneMap(incoming)
	}

	incomingEyes, hasIncomingEyes := incoming["eyes"].(map[string]any)

	// Exames sem granularidade por olho são tratados como uma unidade.
	if !hasIncomingEyes {
		return cloneMap(incoming)
	}

	// Exames com eyes:
	// - olhos ausentes no upload novo permanecem;
	// - mesmo exame + mesmo olho é substituído integralmente.
	result := cloneMap(existing)

	for key, value := range incoming {
		if key == "eyes" || key == "source" {
			continue
		}
		result[key] = cloneValue(value)
	}

	// Fallback. rebuildExamSources substituirá isto quando houver
	// source_files devidamente classificados para este exame.
	if source, exists := incoming["source"]; exists {
		result["source"] = cloneValue(source)
	}

	targetEyes, _ := result["eyes"].(map[string]any)
	if targetEyes == nil {
		targetEyes = map[string]any{}
	}

	for eye, payload := range incomingEyes {
		targetEyes[eye] = cloneValue(payload)
	}

	result["eyes"] = targetEyes
	return result
}

func mergeMapPreferIncoming(target, incoming map[string]any) {
	for key, incomingValue := range incoming {
		if incomingValue == nil {
			continue
		}

		incomingMap, incomingIsMap := incomingValue.(map[string]any)
		targetMap, targetIsMap := target[key].(map[string]any)

		if incomingIsMap && targetIsMap {
			mergeMapPreferIncoming(targetMap, incomingMap)
			continue
		}

		target[key] = cloneValue(incomingValue)
	}
}

func mergeSourceFiles(existingRaw, incomingRaw any) []any {
	existing := sourceFileSlice(existingRaw)
	incoming := sourceFileSlice(incomingRaw)

	// Um upload pode conter vários arquivos do mesmo exame+olho.
	// A substituição acontece por grupo inteiro, não arquivo por arquivo.
	replacementGroups := map[string]bool{}
	incomingHashes := map[string]map[string]bool{}

	for _, source := range incoming {
		group := sourceGroupKey(source)
		if group == "" {
			continue
		}

		replacementGroups[group] = true

		sha, _ := source["sha256"].(string)
		if sha != "" {
			if incomingHashes[group] == nil {
				incomingHashes[group] = map[string]bool{}
			}
			incomingHashes[group][sha] = true
		}
	}

	result := make([]any, 0, len(existing)+len(incoming))
	seen := map[string]bool{}

	appendSource := func(source map[string]any) {
		sha, _ := source["sha256"].(string)
		path, _ := source["path"].(string)

		dedupKey := ""
		if strings.TrimSpace(sha) != "" {
			dedupKey = "sha256:" + sha
		} else if strings.TrimSpace(path) != "" {
			dedupKey = "path:" + path
		}

		if dedupKey != "" && seen[dedupKey] {
			return
		}

		if dedupKey != "" {
			seen[dedupKey] = true
		}

		result = append(result, cloneValue(source))
	}

	for _, source := range existing {
		group := sourceGroupKey(source)

		if replacementGroups[group] {
			// Reenvio byte-a-byte do mesmo documento:
			// mantém a referência já persistida e descarta a cópia redundante.
			sha, _ := source["sha256"].(string)
			if sha != "" && incomingHashes[group][sha] {
				appendSource(source)
			}
			continue
		}

		appendSource(source)
	}

	for _, source := range incoming {
		appendSource(source)
	}

	return result
}

func exactReuploadGroups(existingRaw, incomingRaw any) map[string]bool {
	existing := sourceFileSlice(existingRaw)
	incoming := sourceFileSlice(incomingRaw)

	hashes := map[string]map[string]bool{}

	for _, source := range existing {
		group := sourceGroupKey(source)
		sha, _ := source["sha256"].(string)

		if group == "" || sha == "" {
			continue
		}

		if hashes[group] == nil {
			hashes[group] = map[string]bool{}
		}

		hashes[group][sha] = true
	}

	result := map[string]bool{}
	seenIncoming := map[string]bool{}

	for _, source := range incoming {
		group := sourceGroupKey(source)
		if group == "" {
			continue
		}

		if !seenIncoming[group] {
			result[group] = true
			seenIncoming[group] = true
		}

		sha, _ := source["sha256"].(string)
		if sha == "" || !hashes[group][sha] {
			result[group] = false
		}
	}

	return result
}

func incomingExamForMerge(
	examKey string,
	raw any,
	exactGroups map[string]bool,
) (any, bool) {
	exam, ok := raw.(map[string]any)
	if !ok || exam == nil {
		return raw, true
	}

	eyes, hasEyes := exam["eyes"].(map[string]any)
	if !hasEyes {
		if exactGroups[examKey+"|"] {
			return nil, false
		}
		return raw, true
	}

	filtered := cloneMap(exam)
	filteredEyes := map[string]any{}

	for eye, payload := range eyes {
		if exactGroups[examKey+"|"+eye] {
			continue
		}

		filteredEyes[eye] = cloneValue(payload)
	}

	if len(filteredEyes) == 0 {
		return nil, false
	}

	filtered["eyes"] = filteredEyes
	return filtered, true
}

func sourceFileSlice(raw any) []map[string]any {
	items, _ := raw.([]any)
	result := make([]map[string]any, 0, len(items))

	for _, rawSource := range items {
		source, ok := rawSource.(map[string]any)
		if ok && source != nil {
			result = append(result, source)
		}
	}

	return result
}

func sourceExamName(source map[string]any) string {
	switch exam := source["exam"].(type) {
	case string:
		return strings.TrimSpace(exam)

	case []any:
		// O extrator real pode devolver:
		//   "exam": ["pentacam_corneal_tomography"]
		//
		// Só colapsamos automaticamente quando existe exatamente
		// um tipo de exame. Arquivos multi-exame ficam conservadores.
		if len(exam) != 1 {
			return ""
		}

		value, _ := exam[0].(string)
		return strings.TrimSpace(value)

	case []string:
		if len(exam) != 1 {
			return ""
		}

		return strings.TrimSpace(exam[0])
	}

	return ""
}

func sourceHasExam(source map[string]any, expected string) bool {
	switch exam := source["exam"].(type) {
	case string:
		return strings.TrimSpace(exam) == expected

	case []any:
		for _, raw := range exam {
			value, _ := raw.(string)
			if strings.TrimSpace(value) == expected {
				return true
			}
		}

	case []string:
		for _, value := range exam {
			if strings.TrimSpace(value) == expected {
				return true
			}
		}
	}

	return false
}

func sourceGroupKey(source map[string]any) string {
	exam := sourceExamName(source)
	eye, _ := source["eye"].(string)

	eye = strings.TrimSpace(eye)

	if exam == "" {
		return ""
	}

	return exam + "|" + eye
}

func rebuildExamSources(analysis map[string]any) {
	exams, _ := analysis["exams"].(map[string]any)
	if exams == nil {
		return
	}

	sources := sourceFileSlice(analysis["source_files"])

	for examKey, rawExam := range exams {
		exam, ok := rawExam.(map[string]any)
		if !ok || exam == nil {
			continue
		}

		current := make([]any, 0)
		seen := map[string]bool{}

		appendEye := func(expectedEye string) {
			for _, source := range sources {
				sourceEye, _ := source["eye"].(string)

				if !sourceHasExam(source, examKey) || sourceEye != expectedEye {
					continue
				}

				path, _ := source["path"].(string)
				if path == "" || seen[path] {
					continue
				}

				seen[path] = true
				current = append(current, path)
			}
		}

		// Essa ordem é importante porque o frontend hoje usa, por exemplo,
		// pentacam.source[0] para OD e pentacam.source[1] para OS.
		for _, eye := range []string{"OD", "OS", "AO", ""} {
			appendEye(eye)
		}

		// Preserva qualquer lateralidade futura/desconhecida sem misturá-la
		// com as posições conhecidas acima.
		for _, source := range sources {
			sourceEye, _ := source["eye"].(string)

			if !sourceHasExam(source, examKey) ||
				sourceEye == "OD" ||
				sourceEye == "OS" ||
				sourceEye == "AO" ||
				sourceEye == "" {
				continue
			}

			path, _ := source["path"].(string)
			if path == "" || seen[path] {
				continue
			}

			seen[path] = true
			current = append(current, path)
		}

		if len(current) > 0 {
			exam["source"] = current
		}
	}
}

func mergeStringLists(existingRaw, incomingRaw any) []any {
	result := make([]any, 0)
	seen := map[string]bool{}

	appendValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		result = append(result, value)
	}

	for _, raw := range []any{existingRaw, incomingRaw} {
		switch values := raw.(type) {
		case []any:
			for _, item := range values {
				if value, ok := item.(string); ok {
					appendValue(value)
				}
			}
		case []string:
			for _, value := range values {
				appendValue(value)
			}
		}
	}

	return result
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}

	var cloned map[string]any
	if json.Unmarshal(raw, &cloned) != nil || cloned == nil {
		return map[string]any{}
	}

	return cloned
}

func cloneValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}

	var cloned any
	if json.Unmarshal(raw, &cloned) != nil {
		return value
	}

	return cloned
}

func ReferencedSourcePaths(analysis map[string]any) map[string]bool {
	result := map[string]bool{}

	files, _ := analysis["source_files"].([]any)
	for _, raw := range files {
		source, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		path, _ := source["path"].(string)
		if path != "" {
			result[path] = true
		}
	}

	return result
}
