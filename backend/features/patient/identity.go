package patient

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

func Identity(analysis map[string]any) (string, bool) {
	patient, ok := analysis["patient"].(map[string]any)
	if !ok || patient == nil {
		return "", false
	}

	fullName, _ := patient["full_name"].(string)
	fullName = normalizePatientName(fullName)

	birthDate, birthOK := CanonicalBirthDate(analysis)

	// Não fazemos merge probabilístico.
	// Sem nome E nascimento confiáveis, é mais seguro criar outro registro
	// e deixar a divergência para revisão do que juntar dois pacientes.
	if fullName == "" || !birthOK {
		return "", false
	}

	return fullName + "|" + birthDate, true
}

func CanonicalBirthDate(analysis map[string]any) (string, bool) {
	// A evidência bruta do documento tem prioridade sobre datas
	// "normalized" produzidas pelo modelo.
	//
	// Exemplo real do Pentacam:
	//   nascimento_lido = 03/04/1980
	//   timestamp_lido  = 08/31/2026
	//
	// Como 30 não pode ser mês, o documento está em MM/DD/YYYY.
	// Portanto 03/04/1980 = 1980-03-04.

	rawBirthDates := make([]string, 0)
	dateOrder := ""

	if entries, ok := analysis["verificacao_identidade"].([]any); ok {
		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}

			if raw, ok := entry["nascimento_lido"].(string); ok && strings.TrimSpace(raw) != "" {
				rawBirthDates = append(rawBirthDates, strings.TrimSpace(raw))
			}

			if timestamp, ok := entry["timestamp_lido"].(string); ok {
				hint := inferSlashDateOrder(timestamp)

				if hint != "" {
					if dateOrder != "" && dateOrder != hint {
						return "", false
					}
					dateOrder = hint
				}
			}
		}
	}

	if len(rawBirthDates) > 0 {
		resolved := ""

		for _, raw := range rawBirthDates {
			canonical, ok := canonicalizeDocumentDate(raw, dateOrder)
			if !ok {
				// Existe uma data bruta, mas ela é ambígua.
				// Não caímos para o "normalized" do modelo porque ele pode
				// ter invertido dia e mês.
				return "", false
			}

			if resolved != "" && resolved != canonical {
				return "", false
			}

			resolved = canonical
		}

		if resolved != "" {
			return resolved, true
		}
	}

	// Fallback para análises que não possuem verificacao_identidade.
	// Aceitamos somente ISO já válido.
	patient, ok := analysis["patient"].(map[string]any)
	if !ok {
		return "", false
	}

	if value, ok := patient["birth_date"].(string); ok {
		if canonical, ok := canonicalISODate(value); ok {
			return canonical, true
		}
	}

	if value, ok := patient["birth_date_normalized"].(string); ok {
		if canonical, ok := canonicalISODate(value); ok {
			return canonical, true
		}
	}

	if value, ok := patient["birth_date"].(map[string]any); ok {
		if normalized, ok := value["normalized"].(string); ok {
			if canonical, ok := canonicalISODate(normalized); ok {
				return canonical, true
			}
		}
	}

	return "", false
}

func inferSlashDateOrder(value string) string {
	datePart := strings.Fields(strings.TrimSpace(value))
	if len(datePart) == 0 {
		return ""
	}

	parts := strings.Split(datePart[0], "/")
	if len(parts) != 3 {
		return ""
	}

	var first, second int
	if _, err := fmt.Sscanf(parts[0], "%d", &first); err != nil {
		return ""
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &second); err != nil {
		return ""
	}

	switch {
	case first > 12 && second <= 12:
		return "DMY"
	case second > 12 && first <= 12:
		return "MDY"
	default:
		return ""
	}
}

func canonicalizeDocumentDate(value, order string) (string, bool) {
	value = strings.TrimSpace(value)

	if canonical, ok := canonicalISODate(value); ok {
		return canonical, true
	}

	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return "", false
	}

	var first, second, year int
	if _, err := fmt.Sscanf(parts[0], "%d", &first); err != nil {
		return "", false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &second); err != nil {
		return "", false
	}
	if _, err := fmt.Sscanf(parts[2], "%d", &year); err != nil {
		return "", false
	}

	day := 0
	month := 0

	switch {
	case first > 12 && second <= 12:
		day = first
		month = second

	case second > 12 && first <= 12:
		month = first
		day = second

	case order == "DMY":
		day = first
		month = second

	case order == "MDY":
		month = first
		day = second

	default:
		return "", false
	}

	candidate := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	return canonicalISODate(candidate)
}

func canonicalISODate(value string) (string, bool) {
	value = strings.TrimSpace(value)

	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", false
	}

	return parsed.Format("2006-01-02"), true
}

func NormalizeIdentityFields(analysis map[string]any) {
	patient, ok := analysis["patient"].(map[string]any)
	if !ok || patient == nil {
		return
	}

	if birthDate, ok := CanonicalBirthDate(analysis); ok {
		patient["birth_date"] = birthDate

		// O modelo pode devolver aliases "normalized" incorretos.
		// Depois que a data canônica foi determinada pela evidência bruta,
		// removemos esses valores concorrentes para não persistir duas datas.
		delete(patient, "birth_date_normalized")
	}
}

func normalizePatientName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))

	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c",
	)
	value = replacer.Replace(value)

	var normalized strings.Builder
	lastWasSpace := false

	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
			lastWasSpace = false
			continue
		}

		if !lastWasSpace {
			normalized.WriteByte(' ')
			lastWasSpace = true
		}
	}

	return strings.Join(strings.Fields(normalized.String()), " ")
}
