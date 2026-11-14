package exams

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"

	"gorm.io/gorm"
)

// Handler serves /api/exams, /api/exams/download, /api/exams/upload-url and
// /api/exams/confirm.
type Handler struct {
	Database *gorm.DB
}

// AllowedUploadTypes are the content types accepted for exam uploads.
var AllowedUploadTypes = map[string]bool{
	"application/pdf":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
	"image/jpeg": true,
	"image/png":  true,
}

func safeName(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, " ", "-")
	return randomToken() + "-" + name
}

func randomToken() string {
	var token [4]byte
	if _, readError := rand.Read(token[:]); readError != nil {
		return "file"
	}
	return hex.EncodeToString(token[:])
}
