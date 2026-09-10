package pdf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultRenderDPI = 180

// Inspect returns the number of pages and the document text layer.
//
// It uses Poppler's pdfinfo and pdftotext executables.
func Inspect(
	ctx context.Context,
	data []byte,
) (int, string, error) {
	pagesOutput, err := runCommand(
		ctx,
		"pdfinfo",
		data,
	)
	if err != nil {
		return 0, "", err
	}

	pages := 0

	for _, line := range strings.Split(
		pagesOutput,
		"\n",
	) {
		if strings.HasPrefix(line, "Pages:") {
			pages, _ = strconv.Atoi(
				strings.TrimSpace(
					strings.TrimPrefix(
						line,
						"Pages:",
					),
				),
			)
		}
	}

	if pages < 1 {
		return 0, "", fmt.Errorf(
			"número de páginas não identificado",
		)
	}

	textLayer, err := runCommand(
		ctx,
		"pdftotext",
		data,
	)
	if err != nil {
		return 0, "", err
	}

	return pages, textLayer, nil
}

// RenderPages renders every PDF page as PNG using the default DPI.
func RenderPages(
	ctx context.Context,
	data []byte,
	pages int,
) ([][]byte, error) {
	return renderPagesAtDPI(
		ctx,
		data,
		pages,
		defaultRenderDPI,
	)
}

// RenderPageAtDPI renders one PDF page as a PNG.
func RenderPageAtDPI(
	ctx context.Context,
	data []byte,
	page,
	dpi int,
) ([]byte, error) {
	input, err := os.CreateTemp(
		"",
		"refratia-page-*.pdf",
	)
	if err != nil {
		return nil, err
	}

	inputPath := input.Name()
	defer os.Remove(inputPath)

	if _, err := input.Write(data); err != nil {
		input.Close()
		return nil, err
	}

	if err := input.Close(); err != nil {
		return nil, err
	}

	directory, err := os.MkdirTemp(
		"",
		"refratia-page-png-",
	)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)

	prefix := filepath.Join(
		directory,
		"page",
	)

	command := exec.CommandContext(
		ctx,
		"pdftoppm",
		"-f",
		strconv.Itoa(page),
		"-singlefile",
		"-png",
		"-r",
		strconv.Itoa(dpi),
		inputPath,
		prefix,
	)

	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf(
			"pdftoppm: %s",
			strings.TrimSpace(string(output)),
		)
	}

	image, err := os.ReadFile(
		prefix + ".png",
	)
	if err != nil {
		return nil, fmt.Errorf(
			"página %d: %w",
			page,
			err,
		)
	}

	return image, nil
}

func renderPagesAtDPI(
	ctx context.Context,
	data []byte,
	pages,
	dpi int,
) ([][]byte, error) {
	input, err := os.CreateTemp(
		"",
		"refratia-pages-*.pdf",
	)
	if err != nil {
		return nil, err
	}

	inputPath := input.Name()
	defer os.Remove(inputPath)

	if _, err := input.Write(data); err != nil {
		input.Close()
		return nil, err
	}

	if err := input.Close(); err != nil {
		return nil, err
	}

	directory, err := os.MkdirTemp(
		"",
		"refratia-png-",
	)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)

	prefix := filepath.Join(
		directory,
		"page",
	)

	command := exec.CommandContext(
		ctx,
		"pdftoppm",
		"-png",
		"-r",
		strconv.Itoa(dpi),
		inputPath,
		prefix,
	)

	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf(
			"pdftoppm: %s",
			strings.TrimSpace(string(output)),
		)
	}

	images := make(
		[][]byte,
		0,
		pages,
	)

	for page := 1; page <= pages; page++ {
		path := fmt.Sprintf(
			"%s-%d.png",
			prefix,
			page,
		)

		image, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf(
				"página %d: %w",
				page,
				err,
			)
		}

		images = append(
			images,
			image,
		)
	}

	return images, nil
}

func runCommand(
	ctx context.Context,
	name string,
	data []byte,
) (string, error) {
	input, err := os.CreateTemp(
		"",
		"refratia-pdf-*.pdf",
	)
	if err != nil {
		return "", err
	}

	inputPath := input.Name()
	defer os.Remove(inputPath)

	if _, err := input.Write(data); err != nil {
		input.Close()
		return "", err
	}

	if err := input.Close(); err != nil {
		return "", err
	}

	args := []string{
		inputPath,
	}

	if name == "pdftotext" {
		args = []string{
			"-layout",
			inputPath,
			"-",
		}
	}

	command := exec.CommandContext(
		ctx,
		name,
		args...,
	)

	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok &&
			len(exitErr.Stderr) > 0 {

			return "", fmt.Errorf(
				"%s: %s",
				name,
				strings.TrimSpace(
					string(exitErr.Stderr),
				),
			)
		}

		return "", fmt.Errorf(
			"%s: %w",
			name,
			err,
		)
	}

	return string(output), nil
}
