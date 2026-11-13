package examdate

import "time"

// PlausibleBirth reports whether birth (ISO yyyy-mm-dd) is a believable birth
// date for a patient examined on exam (ISO): strictly before the exam and an
// age of 18..110 years at the exam. A birth date equal to the exam date (age 0)
// is the signature of a misread label, never a real value.
func PlausibleBirth(birthISO, examISO string) bool {
	birth, err := time.Parse("2006-01-02", birthISO)
	if err != nil {
		return false
	}
	exam, err := time.Parse("2006-01-02", examISO)
	if err != nil {
		return false
	}
	if !birth.Before(exam) {
		return false
	}
	age := exam.Year() - birth.Year()
	if exam.Before(time.Date(exam.Year(), birth.Month(), birth.Day(), 0, 0, 0, 0, time.UTC)) {
		age-- // birthday not reached yet this year
	}
	return age >= 18 && age <= 110
}

// FromSlashOrder converts "a/b/yyyy" to ISO with the device's declared order
// ("MDY" or "DMY"). It returns "" when the string is not a valid date in that
// order. The order is device knowledge, never inferred from the string.
func FromSlashOrder(value, order string) string {
	var layout string
	switch order {
	case "MDY":
		layout = "01/02/2006"
	case "DMY":
		layout = "02/01/2006"
	default:
		return ""
	}
	parsed, err := time.Parse(layout, value)
	if err != nil {
		return ""
	}
	return parsed.Format("2006-01-02")
}
