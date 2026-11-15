//go:build cgo

package ocr

/*
#cgo pkg-config: tesseract lept
#include <stdlib.h>
#include <tesseract/capi.h>
#include <leptonica/allheaders.h>
*/
import "C"

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"unsafe"
)

// One engine per core, matching parallelwork's cap. A TessBaseAPI is not safe
// for concurrent use, so each call borrows one from the pool.
var (
	enginePool      chan *C.TessBaseAPI
	enginePoolOnce  sync.Once
	enginePoolError error
)

// WarmUp loads the tesseract engines (the English model, once per core). The
// server calls it at startup; otherwise the first Recognize does.
func WarmUp() error {
	enginePoolOnce.Do(func() {
		engineCount := runtime.GOMAXPROCS(0)
		enginePool = make(chan *C.TessBaseAPI, engineCount)
		language := C.CString("eng")
		defer C.free(unsafe.Pointer(language))
		for range engineCount {
			engine := C.TessBaseAPICreate()
			if C.TessBaseAPIInit3(engine, nil, language) != 0 {
				C.TessBaseAPIDelete(engine)
				enginePoolError = errors.New("tesseract: could not load the eng model")
				return
			}
			enginePool <- engine
		}
	})
	return enginePoolError
}

func recognize(executionContext context.Context, pngData []byte, psm int, whitelist string, tsv bool) (string, error) {
	if len(pngData) == 0 {
		return "", errors.New("tesseract: empty image")
	}
	if warmUpError := WarmUp(); warmUpError != nil {
		return "", warmUpError
	}

	var engine *C.TessBaseAPI
	select {
	case engine = <-enginePool:
	case <-executionContext.Done():
		return "", executionContext.Err()
	}
	defer func() { enginePool <- engine }()
	// ponytail: a running recognition is not interrupted on cancel (the CLI
	// was killed); cells take milliseconds, a full page a few seconds.

	pix := C.pixReadMem((*C.l_uint8)(unsafe.Pointer(&pngData[0])), C.size_t(len(pngData)))
	if pix == nil {
		return "", errors.New("tesseract: could not decode image")
	}
	defer C.pixDestroy(&pix)
	defer C.TessBaseAPIClear(engine)

	// Every call sets both settings, so nothing leaks from the previous user.
	variableName := C.CString("tessedit_char_whitelist")
	variableValue := C.CString(whitelist)
	C.TessBaseAPISetVariable(engine, variableName, variableValue)
	C.free(unsafe.Pointer(variableName))
	C.free(unsafe.Pointer(variableValue))
	C.TessBaseAPISetPageSegMode(engine, C.TessPageSegMode(psm))
	C.TessBaseAPISetImage2(engine, pix)

	var text *C.char
	if tsv {
		text = C.TessBaseAPIGetTsvText(engine, 0)
	} else {
		text = C.TessBaseAPIGetUTF8Text(engine)
	}
	if text == nil {
		return "", errors.New("tesseract: recognition failed")
	}
	defer C.TessDeleteText(text)
	if tsv {
		return tsvHeader + C.GoString(text), nil
	}
	return C.GoString(text), nil
}
