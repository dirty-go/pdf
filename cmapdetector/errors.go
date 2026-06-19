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

	// ErrEncryptedPDF is returned when the file carries a PDF /Encrypt entry.
	// Note: CMap name detection still works on encrypted PDFs because PDF name
	// objects (e.g. /UniGB-UTF16-H) are not encrypted by the standard security
	// handler. This sentinel is provided for callers that need to signal the
	// encrypted state upstream.
	ErrEncryptedPDF = errors.New("PDF is encrypted")
)

// DetectionMethod indicates which detection strategy flagged the file.
type DetectionMethod uint8

const (
	MethodNone      DetectionMethod = 0            // no detection method fired
	MethodBytesScan DetectionMethod = 1 << iota    // Method 1: raw byte scan
	MethodRuntime                                  // Method 2: panic/error recovery
)

// String returns a human-readable representation of the detection method bitmask.
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

// MarshalJSON encodes the detection method bitmask as a JSON string.
func (m DetectionMethod) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `"%s"`, m), nil
}

// Result holds the full outcome of a CMap detection run.
type Result struct {
	// FilePath is the path (or original filename for uploads) of the inspected PDF.
	FilePath string `json:"file_path,omitempty"`

	// FileSize is the size in bytes. Set when the source is a multipart upload;
	// zero when scanning a file by path.
	FileSize int64 `json:"file_size,omitempty"`

	// IsEncrypted is true when the PDF carries a /Encrypt dictionary entry.
	// CMap detection is unaffected (name objects are not encrypted), but callers
	// may wish to surface this information to end users.
	IsEncrypted bool `json:"is_encrypted,omitempty"`

	// HasProblematicCMap is true when at least one detection method flagged the file.
	HasProblematicCMap bool `json:"has_problematic_cmap"`

	// FoundCMaps lists every CMap name discovered in the raw byte scan.
	// Empty when MethodBytesScan did not trigger.
	FoundCMaps []string `json:"found_cmaps,omitempty"`

	// DetectedBy is a bitmask of which methods triggered.
	DetectedBy DetectionMethod `json:"detected_by,omitempty"`

	// RuntimeErr is the error (or recovered panic) captured by Method 2.
	// Nil when Method 2 did not trigger.
	RuntimeErr error `json:"runtime_err,omitempty"`
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
