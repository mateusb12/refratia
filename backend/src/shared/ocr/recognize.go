package ocr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// tsvHeader is the first line the tesseract CLI prints for `tsv` output.
const tsvHeader = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"

// Recognize returns exactly what
// `tesseract <png> stdout --psm <psm> [-c tessedit_char_whitelist=<whitelist>] [tsv]`
// prints. With cgo it runs on engines loaded once per process (see WarmUp);
// starting the CLI costs ~90 ms of model loading per call, which dominated
// the ~1,500 cell reads of a Pentacam report.
func Recognize(executionContext context.Context, pngData []byte, psm int, whitelist string, tsv bool) (string, error) {
	return recognize(executionContext, pngData, psm, whitelist, tsv)
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
	output, runError := exec.CommandContext(executionContext, "tesseract", arguments...).Output()
	if runError != nil {
		if exitError, isExitError := runError.(*exec.ExitError); isExitError && len(exitError.Stderr) > 0 {
			return "", fmt.Errorf("tesseract: %s", strings.TrimSpace(string(exitError.Stderr)))
		}
		return "", fmt.Errorf("tesseract: %w", runError)
	}
	return string(output), nil
}
