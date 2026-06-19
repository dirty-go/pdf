// Package scriptdetector detects embedded scripts, launch actions, and other
// malicious constructs in PDF files via raw byte pattern matching.
//
// All scan functions operate on the structurally visible portion of the file.
// PDF dictionary keys such as /S /JavaScript are never encrypted by the
// standard security handler, so structural threats are detectable without
// decryption. Pass WithPassword to proceed when the file is encrypted;
// encrypted string and stream content is still not decrypted.
//
// Detected threat categories and their default severity:
//
//   - ThreatJavaScript     MEDIUM   (/S /JavaScript, /JS key)
//   - ThreatLaunchAction   HIGH     (/Launch — executes an external application)
//   - ThreatEmbeddedFile   LOW      (/EmbeddedFile — may be a benign attachment)
//   - ThreatXFAForm        MEDIUM   (/XFA — XML Forms Architecture)
//   - ThreatObfuscation    HIGH / CRITICAL  (eval/unescape, String.fromCharCode)
//   - ThreatShellExecution HIGH / CRITICAL  (ActiveXObject, powershell, cmd.exe)
package scriptdetector

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

var pdfHeader = []byte("%PDF-")

// isEncrypted reports whether data contains a PDF /Encrypt dictionary entry.
// /Encrypt only appears as a trailer key, never inside compressed streams, so a
// raw-byte search is reliable without full parsing.
func isEncrypted(data []byte) bool {
	return bytes.Contains(data, []byte("/Encrypt"))
}

// applyOptions merges variadic ScanOptions into a scanConfig.
func applyOptions(opts []ScanOption) *scanConfig {
	cfg := &scanConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// ScanBytes inspects raw PDF bytes for known threat patterns.
// It does NOT parse the PDF structure; it searches the raw byte slice directly.
//
// Returns a Result with HasThreat=true if any pattern matches.
// One Finding is emitted per ThreatType; when multiple patterns match the same
// type, the one with the highest Severity is kept.
func ScanBytes(data []byte) *Result {
	result := &Result{IsEncrypted: isEncrypted(data)}
	lower := bytes.ToLower(data)

	// best keeps the highest-severity Finding seen for each ThreatType.
	best := make(map[ThreatType]Finding)

	for _, sig := range signatures {
		haystack := data
		pat := sig.pattern
		if sig.caseInsensitive {
			haystack = lower
			pat = bytes.ToLower(sig.pattern)
		}

		idx := bytes.Index(haystack, pat)
		if idx < 0 {
			continue
		}

		result.Threats |= sig.threat

		f := Finding{
			Threat:   sig.threat,
			Severity: sig.severity,
			Pattern:  sig.label,
			Snippet:  extractSnippet(data, idx, len(sig.pattern)),
		}
		if prev, ok := best[sig.threat]; !ok || sig.severity > prev.Severity {
			best[sig.threat] = f
		}
	}

	if len(best) > 0 {
		result.HasThreat = true
		for _, f := range best {
			result.Findings = append(result.Findings, f)
		}
		// Sort by severity descending; break ties by ThreatType value for determinism.
		sort.Slice(result.Findings, func(i, j int) bool {
			if result.Findings[i].Severity != result.Findings[j].Severity {
				return result.Findings[i].Severity > result.Findings[j].Severity
			}
			return result.Findings[i].Threat < result.Findings[j].Threat
		})
	}

	return result
}

// ScanReader reads all bytes from r and runs ScanBytes.
// Pass WithPassword to proceed when the PDF is encrypted; without it,
// ScanReader returns ErrEncryptedPDF for encrypted files.
func ScanReader(r io.Reader, opts ...ScanOption) ([]byte, *Result, error) {
	cfg := applyOptions(opts)

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("scriptdetect: reading content: %w", err)
	}

	if isEncrypted(data) && cfg.password == "" {
		return nil, nil, ErrEncryptedPDF
	}

	return data, ScanBytes(data), nil
}

// ScanFile opens the file at filePath, validates the PDF header, and runs ScanBytes.
// Pass WithPassword to proceed when the PDF is encrypted; without it,
// ScanFile returns ErrEncryptedPDF for encrypted files.
//
// When a password is supplied the scan still covers only structurally-visible
// keys (PDF name objects are not encrypted). Result.IsEncrypted will be true
// to signal that string/stream content was not decrypted.
func ScanFile(filePath string, opts ...ScanOption) ([]byte, *Result, error) {
	cfg := applyOptions(opts)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("scriptdetect: reading file %q: %w", filePath, err)
	}

	if !bytes.HasPrefix(data, pdfHeader) {
		return nil, nil, fmt.Errorf("scriptdetect: %w: %q", ErrNotPDF, filePath)
	}

	if isEncrypted(data) && cfg.password == "" {
		return nil, nil, fmt.Errorf("scriptdetect: %w: %q", ErrEncryptedPDF, filePath)
	}

	result := ScanBytes(data)
	result.FilePath = filePath
	return data, result, nil
}

// extractSnippet returns up to 40 bytes before and after the match as a
// printable ASCII string (non-printable bytes replaced with '.').
func extractSnippet(data []byte, offset, matchLen int) string {
	const ctx = 40
	start := max(0, offset-ctx)
	end := min(len(data), offset+matchLen+ctx)
	raw := data[start:end]

	var sb strings.Builder
	sb.Grow(len(raw))
	for _, c := range raw {
		if c >= 0x20 && c < 0x7f {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}
