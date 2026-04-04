package cmapdetector

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors — use errors.Is() to check for these.
var (
	// ErrMissingCMap is returned when a PDF references a CMap resource that is
	// not available in the processing library. This typically affects PDFs with
	// CJK (Chinese, Japanese, Korean) font encodings.
	ErrMissingCMap = errors.New("missing CMap resource")

	// ErrNotPDF is returned when the file does not have a valid PDF header.
	ErrNotPDF = errors.New("file is not a valid PDF")

	// ErrPDFPanic is returned when the underlying PDF library panicked while
	// processing the file. The wrapped value contains the original panic value.
	ErrPDFPanic = errors.New("PDF library panicked")
)

// DetectionMethod indicates which detection strategy flagged the file.
type DetectionMethod uint8

const (
	MethodNone      DetectionMethod = 0
	MethodBytesScan DetectionMethod = 1 << iota // Method 1: raw byte scan
	MethodRuntime                               // Method 2: panic/error recovery
)

func (m DetectionMethod) String() string {
	var parts []string
	if m&MethodBytesScan != 0 {
		parts = append(parts, "BytesScan")
	}
	if m&MethodRuntime != 0 {
		parts = append(parts, "Runtime")
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, "+")
}

// Result holds the full outcome of a CMap detection run.
type Result struct {
	// FilePath is the path (or original filename for uploads) of the inspected PDF.
	FilePath string

	// FileSize is the size in bytes. Set when the source is a multipart upload;
	// zero when scanning a file by path.
	FileSize int64

	// HasProblematicCMap is true when at least one detection method flagged the file.
	HasProblematicCMap bool

	// FoundCMaps lists every CMap name discovered in the raw byte scan.
	// Empty when MethodBytesScan did not trigger.
	FoundCMaps []string

	// DetectedBy is a bitmask of which methods triggered.
	DetectedBy DetectionMethod

	// RuntimeErr is the error (or recovered panic) captured by Method 2.
	// Nil when Method 2 did not trigger.
	RuntimeErr error
}

// Error implements the error interface so a Result can be returned as an error.
func (r *Result) Error() string {
	if !r.HasProblematicCMap {
		return ""
	}
	return fmt.Sprintf(
		"problematic CMap(s) detected in %q via [%s]: %s",
		r.FilePath,
		r.DetectedBy,
		strings.Join(r.FoundCMaps, ", "),
	)
}

// Unwrap returns ErrMissingCMap so callers can use errors.Is(err, ErrMissingCMap).
func (r *Result) Unwrap() error {
	if r.HasProblematicCMap {
		return ErrMissingCMap
	}
	return nil
}
