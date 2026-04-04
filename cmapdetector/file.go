package cmapdetector

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
)

// ProcessBytesFunc is the in-memory equivalent of ProcessFunc.
// Use this variant when the PDF bytes are already loaded (e.g. from a
// multipart upload) and writing to a temp file is undesirable.
type ProcessBytesFunc func(filename string, data []byte) error

// ScanMultipartFile opens a *multipart.FileHeader, validates that it is a PDF,
// and runs Method 1 (raw byte scan).
//
// It returns the raw bytes so the caller can pass them to WrapProcessBytes
// for Method 2 without re-reading the upload.
func ScanMultipartFile(fh *multipart.FileHeader) ([]byte, *Result, error) {
	if fh == nil {
		return nil, nil, fmt.Errorf("cmapdetect: nil FileHeader")
	}

	f, err := fh.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("cmapdetect: opening upload %q: %w", fh.Filename, err)
	}
	defer f.Close()

	// Read entire upload into memory — required for byte scanning and so
	// subsequent PDF library calls don't need disk I/O.
	data, err := readWithSizeHint(f, fh.Size)
	if err != nil {
		return nil, nil, fmt.Errorf("cmapdetect: reading upload %q: %w", fh.Filename, err)
	}

	// Validate PDF magic bytes.
	if !bytes.HasPrefix(data, pdfHeader) {
		// Detect real MIME type for a friendlier error.
		mime := http.DetectContentType(data)
		return nil, nil, fmt.Errorf(
			"cmapdetect: %w: %q (detected content-type: %s)",
			ErrNotPDF, fh.Filename, mime,
		)
	}

	result := ScanBytes(data)
	result.FilePath = fh.Filename
	result.FileSize = fh.Size
	return data, result, nil
}

// WrapProcessBytes executes fn(filename, data) inside a panic-recovery wrapper
// and merges the outcome into existing (which may be nil).
//
// Behaviour mirrors WrapProcess: unrelated panics are re-panicked; unrelated
// errors are passed through unchanged.
func WrapProcessBytes(
	filename string,
	data []byte,
	fn ProcessBytesFunc,
	existing *Result,
) (result *Result, err error) {
	if existing == nil {
		existing = &Result{FilePath: filename}
	}
	result = existing

	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("%v", r)
			if isCMapMessage(msg) {
				result.HasProblematicCMap = true
				result.DetectedBy |= MethodRuntime
				result.RuntimeErr = fmt.Errorf("%w: panic: %s", ErrPDFPanic, msg)
				err = result
			} else {
				panic(r) // unrelated — let it propagate
			}
		}
	}()

	fnErr := fn(filename, data)
	if fnErr != nil {
		if isCMapMessage(fnErr.Error()) {
			result.HasProblematicCMap = true
			result.DetectedBy |= MethodRuntime
			result.RuntimeErr = fmt.Errorf("%w: %s", ErrMissingCMap, fnErr.Error())
			return result, result
		}
		return result, fnErr
	}

	return result, nil
}

// DetectMultipartFile is the primary entry point for upload-based detection.
// It combines Method 1 (byte scan) and, when fn is non-nil, Method 2
// (in-memory runtime recovery) against a *multipart.FileHeader.
//
//	// Method 1 only:
//	result, err := cmapdetect.DetectMultipartFile(fh, nil)
//
//	// Methods 1 + 2:
//	result, err := cmapdetect.DetectMultipartFile(fh, myPDFLib.ParseBytes)
func DetectMultipartFile(fh *multipart.FileHeader, fn ProcessBytesFunc) (*Result, error) {
	// Method 1 — byte scan
	data, result, err := ScanMultipartFile(fh)
	if err != nil {
		return nil, err
	}

	// Method 2 — runtime recovery
	if fn != nil {
		result, err = WrapProcessBytes(fh.Filename, data, fn, result)
		if err != nil {
			var r *Result
			if asResult(err, &r) {
				return r, nil // CMap detection is a Result, not a hard error
			}
			return result, err
		}
	}

	return result, nil
}

// DetectMultipartFiles runs DetectMultipartFile over every header in fhs and
// returns one BatchResult. Processing continues even when individual files
// fail — errors are captured per-file in BatchResult.Errors.
func DetectMultipartFiles(fhs []*multipart.FileHeader, fn ProcessBytesFunc) *BatchResult {
	batch := &BatchResult{
		Results: make([]*Result, 0, len(fhs)),
		Errors:  make([]FileError, 0),
	}

	for _, fh := range fhs {
		result, err := DetectMultipartFile(fh, fn)
		if err != nil {
			batch.Errors = append(batch.Errors, FileError{
				Filename: fh.Filename,
				Err:      err,
			})
			batch.ErrorCount++
			continue
		}
		batch.Results = append(batch.Results, result)
		batch.TotalCount++
		if result.HasProblematicCMap {
			batch.FlaggedCount++
		}
	}

	return batch
}

// BatchResult aggregates detection outcomes for multiple uploaded files.
type BatchResult struct {
	Results      []*Result
	Errors       []FileError
	TotalCount   int
	FlaggedCount int
	ErrorCount   int
}

// FileError pairs a filename with the error that occurred during its processing.
type FileError struct {
	Filename string
	Err      error
}

// readWithSizeHint reads all bytes from r, pre-allocating based on the hint
// when available to avoid unnecessary allocations.
func readWithSizeHint(r interface{ Read([]byte) (int, error) }, sizeHint int64) ([]byte, error) {
	if sizeHint > 0 {
		buf := make([]byte, 0, sizeHint)
		b := bytes.NewBuffer(buf)
		_, err := b.ReadFrom(wrappedReader{r})
		return b.Bytes(), err
	}
	var buf bytes.Buffer
	_, err := buf.ReadFrom(wrappedReader{r})
	return buf.Bytes(), err
}

type wrappedReader struct {
	r interface{ Read([]byte) (int, error) }
}

func (w wrappedReader) Read(p []byte) (int, error) { return w.r.Read(p) }
