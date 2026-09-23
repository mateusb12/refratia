package intake

import (
	"encoding/hex"
	"strings"
	"time"
)

func ValidID(id string) bool {
	parts := strings.Split(strings.TrimPrefix(id, "intake-"), "-")
	if !strings.HasPrefix(id, "intake-") || len(parts) != 2 || len(parts[1]) != 8 {
		return false
	}
	_, timeErr := time.Parse("20060102T150405Z", parts[0])
	_, tokenErr := hex.DecodeString(parts[1])
	return timeErr == nil && tokenErr == nil
}
