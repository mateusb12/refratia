package ocr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func RunTesseractTSV(
	ctx context.Context,
	png []byte,
) (string, error) {
	input, err := os.CreateTemp("", "refratia-ocr-*.png")
	if err != nil {
		return "", err
	}

	path := input.Name()
	defer os.Remove(path)

	if _, err := input.Write(png); err != nil {
		input.Close()
		return "", err
	}

	if err := input.Close(); err != nil {
		return "", err
	}

	command := exec.CommandContext(
		ctx,
		"tesseract",
		path,
		"stdout",
		"tsv",
	)

	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok &&
			len(exitErr.Stderr) > 0 {

			return "", fmt.Errorf(
				"tesseract: %s",
				strings.TrimSpace(string(exitErr.Stderr)),
			)
		}

		return "", fmt.Errorf("tesseract: %w", err)
	}

	return string(output), nil
}
