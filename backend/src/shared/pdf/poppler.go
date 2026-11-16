package pdf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"refratia/backend/src/shared/profiling"
)

const defaultRenderDPI = 180

// Inspect returns the number of pages and the document text layer.
//
// It uses Poppler's pdfinfo and pdftotext executables.
func Inspect(
	ctx context.Context,
	data []byte,
) (pages int, textLayer string, err error) {
	span := profiling.Start(ctx, "pdf.inspect").Caller("/shared/pdf.")
	defer func() { span.Set("pages", pages).End(err) }()

	pagesOutput, err := runCommand(
		ctx,
		"pdfinfo",
		data,
	)
	if err != nil {
		return 0, "", err
	}

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

	textLayer, err = runCommand(
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
) (png []byte, err error) {
	span := profiling.Start(ctx, "pdf.render").Caller("/shared/pdf.").Set("dpi", dpi).Set("pages", 1)
	defer func() { span.End(err) }()

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
		"-r",
		strconv.Itoa(dpi),
		inputPath,
		prefix,
	)

	procSpan := profiling.Start(ctx, "proc.pdftoppm").Set("dpi", dpi).Set("pages", 1)
	output, err := command.CombinedOutput()
	procSpan.EndCmd(command, err)
	if err != nil {
		return nil, fmt.Errorf(
			"pdftoppm: %s",
			strings.TrimSpace(string(output)),
		)
	}

	// Raw PPM, re-encoded in Go: see pngFromPPM.
	ppmData, err := os.ReadFile(
		prefix + ".ppm",
	)
	if err != nil {
		return nil, fmt.Errorf(
			"página %d: %w",
			page,
			err,
		)
	}

	return timedPNGFromPPM(ctx, ppmData)
}

// RenderAllPagesAtDPI renders pages 1..pages with a single pdftoppm call.
func RenderAllPagesAtDPI(
	ctx context.Context,
	data []byte,
	pages,
	dpi int,
) ([][]byte, error) {
	return renderPagesAtDPI(ctx, data, pages, dpi)
}

func renderPagesAtDPI(
	ctx context.Context,
	data []byte,
	pages,
	dpi int,
) (images [][]byte, err error) {
	span := profiling.Start(ctx, "pdf.render").Caller("/shared/pdf.").Set("dpi", dpi).Set("pages", pages)
	defer func() { span.End(err) }()

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
		"-r",
		strconv.Itoa(dpi),
		inputPath,
		prefix,
	)

	procSpan := profiling.Start(ctx, "proc.pdftoppm").Set("dpi", dpi).Set("pages", pages)
	output, err := command.CombinedOutput()
	procSpan.EndCmd(command, err)
	if err != nil {
		return nil, fmt.Errorf(
			"pdftoppm: %s",
			strings.TrimSpace(string(output)),
		)
	}

	images = make(
		[][]byte,
		0,
		pages,
	)

	for page := 1; page <= pages; page++ {
		// pdftoppm zero-pads the page number to the width of the page count
		// (page-01.ppm for a 12-page PDF). Raw PPM, re-encoded in Go: see
		// pngFromPPM.
		path := fmt.Sprintf(
			"%s-%0*d.ppm",
			prefix,
			len(strconv.Itoa(pages)),
			page,
		)

		ppmData, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf(
				"página %d: %w",
				page,
				err,
			)
		}
		image, err := timedPNGFromPPM(ctx, ppmData)
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

	span := profiling.Start(ctx, "proc."+name)
	output, err := command.Output()
	span.EndCmd(command, err)
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

func timedPNGFromPPM(ctx context.Context, ppmData []byte) ([]byte, error) {
	span := profiling.Start(ctx, "preprocess.ppm_to_png").Set("ppm_bytes", len(ppmData))
	png, err := pngFromPPM(ppmData)
	span.End(err)
	return png, err
}
