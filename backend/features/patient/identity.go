package patient

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

func Identity(analysis map[string]any) (string, bool) {
	patient, ok := analysis["patient"].(map[string]any)
	if !ok || patient == nil {
		return "", false
	}

	fullName, _ := ResolveName(analysis)
	fullName = NameKey(fullName)

	birthDate, birthOK := CanonicalBirthDate(analysis)

	// Não fazemos merge probabilístico.
	// Sem nome E nascimento confiáveis, é mais seguro criar outro registro
	// e deixar a divergência para revisão do que juntar dois pacientes.
	if fullName == "" || !birthOK {
		return "", false
	}

	return fullName + "|" + birthDate, true
}

// plausibleName: alphabetic tokens only (no digits/symbols), at least 2 tokens.
func plausibleName(name string) bool {
	tokens := strings.Fields(name)
	if len(tokens) < 2 {
		return false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !strings.ContainsRune(" .,'-", r) {
			return false
		}
	}
	return true
}

// ResolveName picks the identity name. patient.full_name wins when plausible;
// otherwise the plausible identity-entry name that most other readings agree
// with (ties: more tokens). No plausible name = unresolved: OCR garbage may
// never become an identity_key.
func ResolveName(analysis map[string]any) (string, bool) {
	patient, _ := analysis["patient"].(map[string]any)
	if name, _ := patient["full_name"].(string); plausibleName(name) {
		return strings.TrimSpace(name), true
	}
	entries, _ := analysis["verificacao_identidade"].([]any)
	names := make([]string, 0)
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		if name, _ := entry["nome_lido"].(string); plausibleName(name) {
			names = append(names, strings.TrimSpace(name))
		}
	}
	best, bestScore := "", -1
	for _, candidate := range names {
		score := 0
		for _, other := range names {
			if NamesCompatible(candidate, other) {
				score++
			}
		}
		score = score*100 + len(strings.Fields(candidate))
		if score > bestScore {
			best, bestScore = candidate, score
		}
	}
	return best, best != ""
}

// CanonicalBirthDate resolves the birth date. See ResolveBirthDate.
func CanonicalBirthDate(analysis map[string]any) (string, bool) {
	date, _, ok := ResolveBirthDate(analysis)
	return date, ok
}

// ResolveBirthDate resolves the birth date from the raw per-document readings.
//
// The day/month order is a property of each document, never of the batch:
//  1. the document's own slash timestamp when conclusive (a part > 12);
//  2. otherwise a slash timestamp means a US-locale device (Pentacam): MDY;
//  3. otherwise the extractor already normalized the timestamp to ISO, which
//     is what BR-locale devices (EyeSuite) print as DMY: DMY;
//  4. no timestamp at all: only an unambiguous date (a part > 12) is accepted.
//
// Two documents may disagree only by a day/month swap. That is the protocol
// 0.B "data_nascimento_invertida": the reading of the slash-timestamp
// (Pentacam, reference) document wins and inverted=true. Any other
// disagreement is unresolved.
func ResolveBirthDate(analysis map[string]any) (date string, inverted bool, ok bool) {
	type reading struct {
		date      string
		reference bool
	}
	readings := make([]reading, 0)

	if entries, isList := analysis["verificacao_identidade"].([]any); isList {
		for _, rawEntry := range entries {
			entry, isMap := rawEntry.(map[string]any)
			if !isMap {
				continue
			}
			raw, _ := entry["nascimento_lido"].(string)
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			timestamp, _ := entry["timestamp_lido"].(string)
			var (
				order     string
				reference bool
				canonical string
				valid     bool
			)
			if declared, _ := entry["date_order"].(string); declared == "MDY" || declared == "DMY" || declared == "DMONY" {
				// Ordem declarada pelo extrator (conhecimento do aparelho): estrita.
				order, reference = declared, declared == "MDY"
				canonical, valid = canonicalizeDeclaredDate(raw, order)
			} else {
				order, reference = documentDateOrder(timestamp)
				canonical, valid = canonicalizeDocumentDate(raw, order)
			}
			if !valid {
				// Existe uma data bruta, mas ela é ambígua.
				// Não caímos para o "normalized" do modelo porque ele pode
				// ter invertido dia e mês.
				return "", false, false
			}
			readings = append(readings, reading{canonical, reference})
		}
	}

	if len(readings) > 0 {
		first := readings[0].date
		other := ""
		for _, r := range readings {
			if r.date != first {
				other = r.date
			}
		}
		if other == "" {
			return first, false, true
		}
		if swapDayMonth(first) != other {
			return "", false, false
		}
		for _, r := range readings {
			if r.date != first && r.date != other {
				return "", false, false
			}
		}
		refDate := ""
		for _, r := range readings {
			if r.reference {
				if refDate != "" && refDate != r.date {
					return "", false, false
				}
				refDate = r.date
			}
		}
		if refDate == "" {
			return "", false, false
		}
		return refDate, true, true
	}

	// Fallback para análises que não possuem verificacao_identidade.
	// Aceitamos somente ISO já válido.
	patient, isMap := analysis["patient"].(map[string]any)
	if !isMap {
		return "", false, false
	}
	if value, isStr := patient["birth_date"].(string); isStr {
		if canonical, valid := canonicalISODate(value); valid {
			return canonical, false, true
		}
	}
	if value, isStr := patient["birth_date_normalized"].(string); isStr {
		if canonical, valid := canonicalISODate(value); valid {
			return canonical, false, true
		}
	}
	if value, isMap := patient["birth_date"].(map[string]any); isMap {
		if normalized, isStr := value["normalized"].(string); isStr {
			if canonical, valid := canonicalISODate(normalized); valid {
				return canonical, false, true
			}
		}
	}
	return "", false, false
}

// documentDateOrder returns the day/month order of one document and whether
// it is a slash-timestamp (reference, Pentacam-style) document.
func documentDateOrder(timestamp string) (order string, reference bool) {
	timestamp = strings.TrimSpace(timestamp)
	if timestamp == "" {
		return "", false
	}
	if _, err := time.Parse("2006-01-02", strings.Fields(timestamp)[0]); err == nil {
		return "DMY", false
	}
	if hint := inferSlashDateOrder(timestamp); hint != "" {
		return hint, true
	}
	if strings.Contains(timestamp, "/") {
		return "MDY", true
	}
	return "", false
}

func swapDayMonth(iso string) string {
	parsed, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return ""
	}
	swapped := fmt.Sprintf("%04d-%02d-%02d", parsed.Year(), parsed.Day(), int(parsed.Month()))
	if canonical, ok := canonicalISODate(swapped); ok {
		return canonical
	}
	return ""
}

// Warning is a non-blocking identity finding shown to the reviewer.
type Warning map[string]any

// IdentityWarnings reports non-blocking identity findings:
//   - data_nascimento_invertida: devices disagree only by a day/month swap;
//   - nome_divergente: a document's name is not compatible with patient.full_name.
func IdentityWarnings(analysis map[string]any) []Warning {
	warnings := make([]Warning, 0)
	if date, inverted, ok := ResolveBirthDate(analysis); ok && inverted {
		warnings = append(warnings, Warning{
			"code":       "data_nascimento_invertida",
			"birth_date": date,
			"detail":     "documentos divergem apenas por inversão dia/mês; persistida a leitura do Pentacam (documento de referência)",
		})
	}
	patient, _ := analysis["patient"].(map[string]any)
	fullName, _ := patient["full_name"].(string)
	if strings.IndexFunc(fullName, func(r rune) bool {
		return !unicode.IsLetter(r) && !strings.ContainsRune(" .,'-", r)
	}) >= 0 {
		warnings = append(warnings, Warning{
			"code":   "nome_suspeito",
			"name":   fullName,
			"detail": "o nome contém dígitos ou símbolos: provável ruído de OCR; revisar antes de confirmar",
		})
	}
	entries, _ := analysis["verificacao_identidade"].([]any)
	for _, rawEntry := range entries {
		entry, _ := rawEntry.(map[string]any)
		name, _ := entry["nome_lido"].(string)
		if strings.TrimSpace(name) == "" || strings.TrimSpace(fullName) == "" || NamesCompatible(fullName, name) {
			continue
		}
		warnings = append(warnings, Warning{
			"code":   "nome_divergente",
			"source": entry["source"],
			"name":   name,
			"detail": "nome lido no documento não é compatível com o nome do paciente",
		})
	}
	return warnings
}

// NameKey is the order-insensitive name identity: accents, case, punctuation,
// whitespace and word order ("SURNAME, NAME" vs "NAME SURNAME") are ignored.
func NameKey(name string) string {
	tokens := strings.Fields(normalizePatientName(name))
	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}

// NamesCompatible: every token of the shorter name appears in the longer one,
// either whole or as a one-letter initial (nome_parcial: "MARIA GORETE DE C").
func NamesCompatible(a, b string) bool {
	ta, tb := strings.Fields(normalizePatientName(a)), strings.Fields(normalizePatientName(b))
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	if len(ta) == 0 {
		return false
	}
	for _, t := range ta {
		found := false
		for _, u := range tb {
			if t == u || (len(t) == 1 && strings.HasPrefix(u, t)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
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

var monthNames = map[string]int{
	"JAN": 1, "FEV": 2, "FEB": 2, "MAR": 3, "ABR": 4, "APR": 4, "MAI": 5, "MAY": 5, "JUN": 6,
	"JUL": 7, "AGO": 8, "AUG": 8, "SET": 9, "SEP": 9, "OUT": 10, "OCT": 10, "NOV": 11, "DEZ": 12, "DEC": 12,
}

// canonicalizeDeclaredDate parses with the device's declared order, no guessing.
func canonicalizeDeclaredDate(value, order string) (string, bool) {
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
	if _, err := fmt.Sscanf(parts[2], "%d", &year); err != nil {
		return "", false
	}
	day, month := first, 0
	switch order {
	case "DMONY":
		m, ok := monthNames[strings.ToUpper(strings.TrimSpace(parts[1]))]
		if !ok {
			if _, err := fmt.Sscanf(parts[1], "%d", &second); err != nil {
				return "", false
			}
			m = second
		}
		month = m
	case "DMY":
		if _, err := fmt.Sscanf(parts[1], "%d", &second); err != nil {
			return "", false
		}
		month = second
	case "MDY":
		if _, err := fmt.Sscanf(parts[1], "%d", &second); err != nil {
			return "", false
		}
		month, day = first, second
	}
	return canonicalISODate(fmt.Sprintf("%04d-%02d-%02d", year, month, day))
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

	if name, ok := ResolveName(analysis); ok {
		patient["full_name"] = name
	}
	birthDate, ok := CanonicalBirthDate(analysis)
	if !ok {
		// birth_date é ISO ou ausente, nunca a string bruta.
		if raw, isStr := patient["birth_date"].(string); isStr {
			if _, iso := canonicalISODate(raw); !iso {
				delete(patient, "birth_date")
			}
		}
		return
	}
	{
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

// SameIdentity reports whether two (possibly partial) analyses are the same
// patient. Besides equal identity keys it accepts the protocol 0.B day/month
// inversion: same name key and birth dates that are each other's day/month
// swap (documents read in isolation, e.g. one EyeSuite and one Pentacam
// checkpoint, resolve to different dates for the same patient).
func SameIdentity(a, b map[string]any) bool {
	keyA, okA := Identity(a)
	keyB, okB := Identity(b)
	if !okA || !okB {
		return false
	}
	if keyA == keyB {
		return true
	}
	nameA, dateA, _ := strings.Cut(keyA, "|")
	nameB, dateB, _ := strings.Cut(keyB, "|")
	return nameA == nameB && swapDayMonth(dateA) == dateB
}
