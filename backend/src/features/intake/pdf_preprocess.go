package intake

import (
	"context"
	"fmt"
	"strings"

	pdfutil "refratia/backend/src/shared/pdf"
)

// preparedFile is deliberately kept compatible with uploadedFile. A PDF is
// replaced by either its text layer or one PNG per page before it reaches the
// vision model. The original filename is retained so source_files continue to
// point to the user's document.
type preparedFile struct {
	File      uploadedFile
	Page      int
	TextLayer string
}

func prepareExtractionFiles(executionContext context.Context, files []uploadedFile) ([]preparedFile, error) {
	prepared := make([]preparedFile, 0, len(files))
	for _, file := range files {
		if file.Metadata.ContentType != "application/pdf" {
			prepared = append(prepared, preparedFile{File: file})
			continue
		}

		pages, textLayer, errorValue := pdfutil.Inspect(executionContext, file.Data)
		if errorValue != nil {
			return nil, fmt.Errorf("não foi possível preparar %s: %w", file.Metadata.Filename, errorValue)
		}
		if strings.TrimSpace(textLayer) != "" {
			prepared = append(prepared, preparedFile{File: file, TextLayer: textLayer})
			continue
		}

		pageImages, errorValue := pdfutil.RenderPages(executionContext, file.Data, pages)
		if errorValue != nil {
			return nil, fmt.Errorf("não foi possível renderizar %s: %w", file.Metadata.Filename, errorValue)
		}
		for index, image := range pageImages {
			metadata := file.Metadata
			metadata.ContentType = "image/png"
			metadata.Size = int64(len(image))
			prepared = append(prepared, preparedFile{
				File: uploadedFile{Metadata: metadata, Data: image},
				Page: index + 1,
			})
		}
	}
	return prepared, nil
}

func preparedPromptLabel(file preparedFile) string {
	if file.Page > 0 {
		return fmt.Sprintf("Arquivo: %s — página %d", file.File.Metadata.Filename, file.Page)
	}
	return "Arquivo: " + file.File.Metadata.Filename
}
