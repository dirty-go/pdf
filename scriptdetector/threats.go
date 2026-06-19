package scriptdetector

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNotPDF is returned when the input does not begin with a valid PDF header.
	ErrNotPDF = errors.New("file is not a valid PDF")

	// ErrEncryptedPDF is returned when the PDF is encrypted and no password was
	// provided. Scanning encrypted content without decryption would silently miss
	// threats inside strings and streams.
	ErrEncryptedPDF = errors.New("PDF is encrypted")

	// ErrWrongPassword is returned when a password was provided but does not
	// authenticate against the PDF's encryption dictionary.
	ErrWrongPassword = errors.New("wrong password for encrypted PDF")
)

// ThreatType is a bitmask of detected threat categories.
type ThreatType uint16

const (
	ThreatNone           ThreatType = 0            // no threat detected
	ThreatJavaScript     ThreatType = 1 << iota    // Embedded JS actions (/JS, /S /JavaScript)
	ThreatLaunchAction                          // /Launch — triggers an external application
	ThreatEmbeddedFile                          // /EmbeddedFile — file embedded inside the PDF
	ThreatXFAForm                               // /XFA — XML Forms Architecture (scriptable forms)
	ThreatObfuscation                           // JS obfuscation: eval(unescape(, String.fromCharCode
	ThreatShellExecution                        // Shell keywords: ActiveXObject, powershell, cmd.exe
)

var threatNames = []struct {
	bit  ThreatType
	name string
}{
	{ThreatJavaScript, "JavaScript"},
	{ThreatLaunchAction, "LaunchAction"},
	{ThreatEmbeddedFile, "EmbeddedFile"},
	{ThreatXFAForm, "XFAForm"},
	{ThreatObfuscation, "Obfuscation"},
	{ThreatShellExecution, "ShellExecution"},
}

// String returns a human-readable representation of the threat type bitmask.
func (t ThreatType) String() string {
	if t == ThreatNone {
		return "None"
	}
	var parts []string
	for _, n := range threatNames {
		if t&n.bit != 0 {
			parts = append(parts, n.name)
		}
	}
	return strings.Join(parts, "+")
}

// MarshalJSON encodes the threat type bitmask as a JSON string.
func (t ThreatType) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `"%s"`, t), nil
}

// Severity describes how dangerous a finding is.
type Severity uint8

const (
	SeverityLow      Severity = iota + 1 // informational; may be legitimate
	SeverityMedium                       // suspicious; manual review recommended
	SeverityHigh                         // strong indicator of malicious intent
	SeverityCritical                     // block immediately
)

// String returns the severity level as an uppercase string (e.g. "HIGH").
func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "LOW"
	case SeverityMedium:
		return "MEDIUM"
	case SeverityHigh:
		return "HIGH"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON encodes the severity level as a JSON string.
func (s Severity) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `"%s"`, s), nil
}

// ScanOption configures the behaviour of scan functions.
type ScanOption func(*scanConfig)

type scanConfig struct {
	password string
}

// WithPassword provides a password for scanning encrypted PDFs.
// When a password is supplied, ScanFile / ScanReader will proceed with a
// partial scan (dictionary keys such as /S /JavaScript remain readable even
// in encrypted files) instead of returning ErrEncryptedPDF.
// The IsEncrypted flag on the Result is still set so callers know the scan
// may not have covered encrypted string / stream content.
func WithPassword(pw string) ScanOption {
	return func(c *scanConfig) { c.password = pw }
}

// Finding is a single detected threat with its context.
type Finding struct {
	Threat   ThreatType `json:"threat"`
	Severity Severity   `json:"severity"`
	Pattern  string     `json:"pattern"` // human-readable description of what matched
	Snippet  string     `json:"snippet"` // surrounding byte context (printable, truncated)
}

// Result holds the full outcome of a script/threat scan.
type Result struct {
	FilePath  string     `json:"file_path,omitempty"`
	HasThreat bool       `json:"has_threat"`
	Threats   ThreatType `json:"threats,omitempty"`
	// IsEncrypted is true when the PDF carries an /Encrypt entry. When set,
	// the scan covered only unencrypted structural keys (e.g. /S /JavaScript);
	// string and stream content was not readable without decryption.
	IsEncrypted bool `json:"is_encrypted,omitempty"`
	// Findings contains one entry per detected threat type, sorted by severity descending.
	Findings []Finding `json:"findings,omitempty"`
}

// MaxSeverity returns the highest severity found, or 0 if there are no findings.
func (r *Result) MaxSeverity() Severity {
	var max Severity
	for _, f := range r.Findings {
		if f.Severity > max {
			max = f.Severity
		}
	}
	return max
}

// Error implements the error interface so Result can be returned as an error value.
func (r *Result) Error() string {
	if !r.HasThreat {
		return ""
	}
	parts := make([]string, len(r.Findings))
	for i, f := range r.Findings {
		parts[i] = fmt.Sprintf("%s(%s)", f.Threat, f.Severity)
	}
	return fmt.Sprintf("suspicious content in %q: %s", r.FilePath, strings.Join(parts, ", "))
}
