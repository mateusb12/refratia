package ocr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"refratia/backend/src/shared/profiling"
)

// tsvHeader is the first line the tesseract CLI prints for `tsv` output.
const tsvHeader = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"

// A single OCR call must not keep an intake run alive forever. Full-page
// Pentacam OCR normally finishes well below this limit; a slower call should
// fail and let the extractor report a controlled error instead.
const recognizeTimeout = 30 * time.Second

// Recognize returns exactly what
// `tesseract <png> stdout --psm <psm> [-c tessedit_char_whitelist=<whitelist>] [tsv]`
// prints. With cgo it runs on engines loaded once per process (see WarmUp);
// starting the CLI costs ~90 ms of model loading per call, which dominated
// the ~1,500 cell reads of a Pentacam report.
func Recognize(executionContext context.Context, pngData []byte, psm int, whitelist string, tsv bool) (string, error) {
	ocrContext, cancel := context.WithTimeout(executionContext, recognizeTimeout)
	defer cancel()

	span := profiling.Start(executionContext, "ocr.recognize").Caller("/shared/ocr.", "/shared/document.").
		Set("backend", Backend).Set("psm", psm).Set("tsv", tsv).Set("whitelist", whitelist != "").Set("png_bytes", len(pngData))
	text, err := recognize(span.Into(ocrContext), pngData, psm, whitelist, tsv)
	span.End(err)
	return text, err
}

// recognizeWithCLI is the process-per-call path: the !cgo build uses it, and
// the cgo build's tests compare the resident engine against it.
func recognizeWithCLI(executionContext context.Context, pngData []byte, psm int, whitelist string, tsv bool) (string, error) {
	inputFile, createError := os.CreateTemp("", "refratia-ocr-*.png")
	if createError != nil {
		return "", createError
	}
	inputPath := inputFile.Name()
	defer os.Remove(inputPath)
	if _, writeError := inputFile.Write(pngData); writeError != nil {
		inputFile.Close()
		return "", writeError
	}
	if closeError := inputFile.Close(); closeError != nil {
		return "", closeError
	}

	arguments := []string{inputPath, "stdout", "--psm", strconv.Itoa(psm)}
	if whitelist != "" {
		arguments = append(arguments, "-c", "tessedit_char_whitelist="+whitelist)
	}
	if tsv {
		arguments = append(arguments, "tsv")
	}
	span := profiling.Start(executionContext, "proc.tesseract").Set("site", "ocr.cli")
	command := exec.CommandContext(executionContext, "tesseract", arguments...)
	output, runError := command.Output()
	span.EndCmd(command, runError)
	if runError != nil {
		if exitError, isExitError := runError.(*exec.ExitError); isExitError && len(exitError.Stderr) > 0 {
			return "", fmt.Errorf("tesseract: %s", strings.TrimSpace(string(exitError.Stderr)))
		}
		return "", fmt.Errorf("tesseract: %w", runError)
	}
	return string(output), nil
}
