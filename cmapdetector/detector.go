// Package cmapdetector detects PDF files that reference CMap resources
// unavailable in common PDF processing libraries such as iTextPDF, pdfbox,
// and pdfcpu.
//
// Two complementary strategies are provided:
//
//   - Method 1 (byte scan): ScanBytes / ScanFile / ScanReader scan the raw PDF
//     bytes for known problematic CMap name strings from the ISO 32000-1 and
//     Adobe predefined CMap lists. Fast, zero-parse, and dependency-free.
//
//   - Method 2 (runtime recovery): WrapProcess / WrapProcessBytes wrap a
//     caller-supplied PDF processing function, catching panics and errors whose
//     messages match known CMap error signatures.
//
// Primary combined entry points: DetectFile (file path), DetectMultipartFile /
// DetectMultipartFiles (HTTP multipart uploads), and DetectBytes (in-memory
// byte slice).
package cmapdetector

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

// pdfHeader is the magic bytes that every valid PDF file starts with.
var pdfHeader = []byte("%PDF-")

// ---- Method 1: Raw byte scan ------------------------------------------------

// ScanBytes inspects raw PDF bytes for known problematic CMap name strings.
// It does NOT open or parse the PDF; it simply searches the byte slice.
//
// Returns a partial Result (DetectedBy may be MethodNone if nothing is found).
// The caller is responsible for setting FilePath.
func ScanBytes(data []byte) *Result {
	result := &Result{
		IsEncrypted: bytes.Contains(data, []byte("/Encrypt")),
	}

	for _, cmap := range knownProblematicCMaps {
		if bytes.Contains(data, []byte(cmap)) {
			result.HasProblematicCMap = true
			result.DetectedBy |= MethodBytesScan
			result.FoundCMaps = append(result.FoundCMaps, cmap)
		}
	}

	return result
}

// ScanReader inspects a PDF from an io.Reader.
// The entire content is read into memory so the byte scanner can operate on it.
// Returns the raw bytes alongside the Result so the caller can reuse them.
func ScanReader(r io.Reader) ([]byte, *Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("cmapdetect: reading content: %w", err)
	}
	return data, ScanBytes(data), nil
}

// ScanFile opens the file at filePath and runs the byte scanner (Method 1).
// Returns the raw bytes alongside the Result so Method 2 can reuse them.
func ScanFile(filePath string) ([]byte, *Result, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("cmapdetect: reading file %q: %w", filePath, err)
	}

	if !bytes.HasPrefix(data, pdfHeader) {
		return nil, nil, fmt.Errorf("cmapdetect: %w: %q", ErrNotPDF, filePath)
	}

	result := ScanBytes(data)
	result.FilePath = filePath
	return data, result, nil
}

// ---- Method 2: Runtime recovery ---------------------------------------------

// ProcessFunc is any function that processes a PDF and may return an error or
// panic when it encounters an unresolvable CMap. The string argument is the
// file path originally passed to WrapProcess / DetectFile.
type ProcessFunc func(filePath string) error

// WrapProcess executes fn(filePath) inside a panic-recovery wrapper.
// If fn panics or returns an error whose message matches a known CMap error
// signature, the Result is updated to reflect a runtime detection.
//
// The returned error is:
//   - nil                      — fn succeeded and no CMap issue was detected.
//   - *Result (HasProblematicCMap=true) — a CMap problem was detected.
//   - any other error           — fn failed for an unrelated reason.
func WrapProcess(filePath string, fn ProcessFunc, existing *Result) (result *Result, err error) {
	if existing == nil {
		existing = &Result{FilePath: filePath}
	}
	result = existing

	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("%v", r)
			if isCMapMessage(msg) {
				result.HasProblematicCMap = true
				result.DetectedBy |= MethodRuntime
				result.RuntimeErr = fmt.Errorf("%w: panic: %s", ErrPDFPanic, msg)
				err = result // satisfy the error return
			} else {
				// Re-panic for unrelated panics so the caller sees them.
				panic(r)
			}
		}
	}()

	fnErr := fn(filePath)
	if fnErr != nil {
		if isCMapMessage(fnErr.Error()) {
			result.HasProblematicCMap = true
			result.DetectedBy |= MethodRuntime
			result.RuntimeErr = fmt.Errorf("%w: %s", ErrMissingCMap, fnErr.Error())
			return result, result
		}
		return result, fnErr // unrelated error — pass through
	}

	return result, nil
}

// ---- Combined API -----------------------------------------------------------

// DetectFile runs both Method 1 (byte scan) and, if a ProcessFunc is supplied,
// Method 2 (runtime recovery) against the PDF at filePath.
//
// Pass nil for fn to run Method 1 only.
// Pass a real PDF processing function for fn to also run Method 2.
//
//	result, err := cmapdetect.DetectFile("input.pdf", nil)
//	result, err := cmapdetect.DetectFile("input.pdf", myPDFLib.Open)
func DetectFile(filePath string, fn ProcessFunc) (*Result, error) {
	// Method 1 — byte scan
	_, result, err := ScanFile(filePath)
	if err != nil {
		return nil, err
	}

	// Method 2 — runtime recovery (only when a process func is provided)
	if fn != nil {
		result, err = WrapProcess(filePath, fn, result)
		// If WrapProcess returned our Result as an error (CMap detected),
		// we treat it as a successful detection, not a hard error.
		if err != nil {
			var r *Result
			if asResult(err, &r) {
				return r, nil
			}
			return result, err
		}
	}

	return result, nil
}

// DetectBytes runs Method 1 only against an already-loaded byte slice.
// filePath is stored on the Result for reference only; the file is not opened.
func DetectBytes(filePath string, data []byte) *Result {
	r := ScanBytes(data)
	r.FilePath = filePath
	return r
}

// ---- Helpers ----------------------------------------------------------------

// isCMapMessage returns true if msg contains at least two independent CMap
// error signatures (to reduce false positives from single-word matches like
// "CMap" appearing in unrelated log output).
func isCMapMessage(msg string) bool {
	lower := strings.ToLower(msg)
	hits := 0
	for _, sig := range errorSignatures {
		if strings.Contains(lower, strings.ToLower(sig)) {
			hits++
			if hits >= 2 {
				return true
			}
		}
	}
	return false
}

// asResult checks whether err is a *Result and writes it to target.
func asResult(err error, target **Result) bool {
	r, ok := err.(*Result)
	if ok {
		*target = r
	}
	return ok
}
