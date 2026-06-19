package scriptdetector_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	scriptdetect "github.com/dirty-go/pdf/scriptdetector"
)

// ── synthetic PDF builders ────────────────────────────────────────────────────

func makePDF(body string) []byte {
	return fmt.Appendf(nil, "%%PDF-1.7\n%s\n%%%%EOF\n", body)
}

func makeCleanPDF() []byte {
	return makePDF("1 0 obj\n<</Type /Page /MediaBox [0 0 612 792]>>\nendobj")
}

// ── ScanBytes: clean ──────────────────────────────────────────────────────────

func TestScanBytes_Clean(t *testing.T) {
	result := scriptdetect.ScanBytes(makeCleanPDF())
	if result.HasThreat {
		t.Errorf("want HasThreat=false, got findings: %v", result.Findings)
	}
	if result.Threats != scriptdetect.ThreatNone {
		t.Errorf("want ThreatNone, got %s", result.Threats)
	}
}

// ── ScanBytes: JavaScript ────────────────────────────────────────────────────

func TestScanBytes_JavaScriptAction(t *testing.T) {
	data := makePDF("1 0 obj\n<</S /JavaScript /JS (app.alert('hello')>>)\nendobj")
	result := scriptdetect.ScanBytes(data)

	if !result.HasThreat {
		t.Fatal("want HasThreat=true for /S /JavaScript")
	}
	if result.Threats&scriptdetect.ThreatJavaScript == 0 {
		t.Errorf("want ThreatJavaScript set, got %s", result.Threats)
	}
	if result.MaxSeverity() < scriptdetect.SeverityMedium {
		t.Errorf("want severity >= MEDIUM, got %s", result.MaxSeverity())
	}
}

func TestScanBytes_JSInlineKey(t *testing.T) {
	data := makePDF("1 0 obj\n<</Type /Action /S /JavaScript /JS (FormSubmit())>>\nendobj")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatJavaScript == 0 {
		t.Errorf("want ThreatJavaScript for /JS( key, got %s", result.Threats)
	}
}

// ── ScanBytes: Launch action ─────────────────────────────────────────────────

func TestScanBytes_LaunchAction(t *testing.T) {
	data := makePDF("1 0 obj\n<</Type /Action /S /Launch /Win << /F (malware.exe) >> >>\nendobj")
	result := scriptdetect.ScanBytes(data)

	if !result.HasThreat {
		t.Fatal("want HasThreat=true for /Launch")
	}
	if result.Threats&scriptdetect.ThreatLaunchAction == 0 {
		t.Errorf("want ThreatLaunchAction, got %s", result.Threats)
	}
	if result.MaxSeverity() < scriptdetect.SeverityHigh {
		t.Errorf("want severity >= HIGH for /Launch, got %s", result.MaxSeverity())
	}
}

// ── ScanBytes: obfuscation ────────────────────────────────────────────────────

func TestScanBytes_EvalUnescapeObfuscation(t *testing.T) {
	data := makePDF("/JS (eval(unescape('%75%72%6c')))")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatObfuscation == 0 {
		t.Errorf("want ThreatObfuscation for eval(unescape(, got %s", result.Threats)
	}
	if result.MaxSeverity() != scriptdetect.SeverityCritical {
		t.Errorf("want SeverityCritical for eval(unescape(, got %s", result.MaxSeverity())
	}
}

func TestScanBytes_StringFromCharCode(t *testing.T) {
	data := makePDF("/JS (var x = String.fromCharCode(104,101,108,108,111))")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatObfuscation == 0 {
		t.Errorf("want ThreatObfuscation for String.fromCharCode, got %s", result.Threats)
	}
	if result.MaxSeverity() < scriptdetect.SeverityHigh {
		t.Errorf("want severity >= HIGH, got %s", result.MaxSeverity())
	}
}

// ── ScanBytes: shell execution ────────────────────────────────────────────────

func TestScanBytes_ActiveXObject(t *testing.T) {
	// Case-insensitive: use mixed case to verify.
	data := makePDF("/JS (var s = new ActivexObject('WScript.Shell'))")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatShellExecution == 0 {
		t.Errorf("want ThreatShellExecution for ActiveXObject (case-insensitive), got %s", result.Threats)
	}
	if result.MaxSeverity() != scriptdetect.SeverityCritical {
		t.Errorf("want SeverityCritical, got %s", result.MaxSeverity())
	}
}

func TestScanBytes_PowerShell(t *testing.T) {
	data := makePDF("/JS (app.launchURL('powershell -enc abc123'))")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatShellExecution == 0 {
		t.Errorf("want ThreatShellExecution for powershell, got %s", result.Threats)
	}
}

func TestScanBytes_PowerShellCaseInsensitive(t *testing.T) {
	data := makePDF("/JS (PowerShell.exe -Command 'Get-Process')")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatShellExecution == 0 {
		t.Errorf("want ThreatShellExecution for PowerShell (uppercase), got %s", result.Threats)
	}
}

// ── ScanBytes: XFA / EmbeddedFile ────────────────────────────────────────────

func TestScanBytes_XFAForm(t *testing.T) {
	data := makePDF("1 0 obj\n<</AcroForm << /XFA 2 0 R >> >>\nendobj")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatXFAForm == 0 {
		t.Errorf("want ThreatXFAForm, got %s", result.Threats)
	}
}

func TestScanBytes_EmbeddedFile(t *testing.T) {
	data := makePDF("1 0 obj\n<</Type /Filespec /EmbeddedFile 2 0 R>>\nendobj")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatEmbeddedFile == 0 {
		t.Errorf("want ThreatEmbeddedFile, got %s", result.Threats)
	}
	if result.MaxSeverity() != scriptdetect.SeverityLow {
		t.Errorf("want SeverityLow for /EmbeddedFile, got %s", result.MaxSeverity())
	}
}

// ── ScanBytes: multiple threats ───────────────────────────────────────────────

func TestScanBytes_MultipleThreats(t *testing.T) {
	// Both /Launch and eval(unescape( present.
	data := makePDF("/S /Launch\n/JS (eval(unescape('%61%62%63')))")
	result := scriptdetect.ScanBytes(data)

	if result.Threats&scriptdetect.ThreatLaunchAction == 0 {
		t.Errorf("want ThreatLaunchAction")
	}
	if result.Threats&scriptdetect.ThreatObfuscation == 0 {
		t.Errorf("want ThreatObfuscation")
	}
	if result.Threats&scriptdetect.ThreatJavaScript == 0 {
		t.Errorf("want ThreatJavaScript")
	}
	// Findings should be sorted with Critical first.
	if len(result.Findings) == 0 {
		t.Fatal("expected findings")
	}
	if result.Findings[0].Severity != scriptdetect.SeverityCritical {
		t.Errorf("want first finding to be CRITICAL, got %s", result.Findings[0].Severity)
	}
}

// ── ScanBytes: obfuscation dedup (eval+unescape beats plain unescape) ─────────

func TestScanBytes_ObfuscationDedup_KeepsHighestSeverity(t *testing.T) {
	// File has both unescape( (HIGH) and eval(unescape( (CRITICAL).
	// The Finding for ThreatObfuscation must be CRITICAL.
	data := makePDF("/JS (unescape('%61') + eval(unescape('%62')))")
	result := scriptdetect.ScanBytes(data)

	for _, f := range result.Findings {
		if f.Threat == scriptdetect.ThreatObfuscation {
			if f.Severity != scriptdetect.SeverityCritical {
				t.Errorf("want SeverityCritical for ThreatObfuscation dedup, got %s", f.Severity)
			}
			return
		}
	}
	t.Error("ThreatObfuscation finding not present")
}

// ── ScanFile: not PDF ─────────────────────────────────────────────────────────

func TestScanFile_NotPDF_Returns_ErrNotPDF(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "notpdf-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("this is not a PDF at all")
	f.Close()

	_, _, err = scriptdetect.ScanFile(f.Name())
	if !errors.Is(err, scriptdetect.ErrNotPDF) {
		t.Errorf("want ErrNotPDF, got %v", err)
	}
}

func TestScanFile_MissingFile(t *testing.T) {
	_, _, err := scriptdetect.ScanFile(filepath.Join(t.TempDir(), "nonexistent.pdf"))
	if err == nil {
		t.Error("want error for missing file, got nil")
	}
}

// ── ScanReader ────────────────────────────────────────────────────────────────

func TestScanReader_DetectsJS(t *testing.T) {
	data := makePDF("/S /JavaScript\n/JS (alert(1))")
	rawData, result, err := scriptdetect.ScanReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rawData, data) {
		t.Error("returned bytes do not match input")
	}
	if result.Threats&scriptdetect.ThreatJavaScript == 0 {
		t.Errorf("want ThreatJavaScript from reader, got %s", result.Threats)
	}
}

// ── Encrypted PDF ─────────────────────────────────────────────────────────────

func makeEncryptedPDF() []byte {
	// Minimal PDF with a /Encrypt trailer entry (simulates a standard-encrypted PDF).
	return makePDF("1 0 obj\n<</Type /Catalog>>\nendobj\ntrailer\n<</Root 1 0 R /Encrypt 2 0 R>>")
}

func TestScanFile_Encrypted_NoPassword_ReturnsErrEncryptedPDF(t *testing.T) {
	path := writeTemp(t, makeEncryptedPDF())
	_, _, err := scriptdetect.ScanFile(path)
	if !errors.Is(err, scriptdetect.ErrEncryptedPDF) {
		t.Errorf("want ErrEncryptedPDF without password, got %v", err)
	}
}

func TestScanFile_Encrypted_WithPassword_Proceeds(t *testing.T) {
	path := writeTemp(t, makeEncryptedPDF())
	_, result, err := scriptdetect.ScanFile(path, scriptdetect.WithPassword("secret"))
	if err != nil {
		t.Fatalf("want no error with password, got %v", err)
	}
	if !result.IsEncrypted {
		t.Error("want IsEncrypted=true when /Encrypt is present")
	}
}

func TestScanReader_Encrypted_NoPassword_ReturnsErrEncryptedPDF(t *testing.T) {
	_, _, err := scriptdetect.ScanReader(bytes.NewReader(makeEncryptedPDF()))
	if !errors.Is(err, scriptdetect.ErrEncryptedPDF) {
		t.Errorf("want ErrEncryptedPDF without password, got %v", err)
	}
}

func TestScanReader_Encrypted_WithPassword_Proceeds(t *testing.T) {
	_, result, err := scriptdetect.ScanReader(
		bytes.NewReader(makeEncryptedPDF()),
		scriptdetect.WithPassword("secret"),
	)
	if err != nil {
		t.Fatalf("want no error with password, got %v", err)
	}
	if !result.IsEncrypted {
		t.Error("want IsEncrypted=true")
	}
}

func TestScanBytes_Encrypted_SetsFlag(t *testing.T) {
	result := scriptdetect.ScanBytes(makeEncryptedPDF())
	if !result.IsEncrypted {
		t.Error("want IsEncrypted=true from ScanBytes when /Encrypt present")
	}
}

func TestScanBytes_NotEncrypted_FlagFalse(t *testing.T) {
	result := scriptdetect.ScanBytes(makeCleanPDF())
	if result.IsEncrypted {
		t.Error("want IsEncrypted=false for clean PDF")
	}
}

// ── Result helpers ────────────────────────────────────────────────────────────

func TestResult_MaxSeverity_Empty(t *testing.T) {
	r := &scriptdetect.Result{}
	if r.MaxSeverity() != 0 {
		t.Errorf("want 0 for empty result, got %d", r.MaxSeverity())
	}
}

func TestResult_Error_Clean(t *testing.T) {
	r := &scriptdetect.Result{HasThreat: false}
	if r.Error() != "" {
		t.Errorf("want empty Error() for clean result, got %q", r.Error())
	}
}

func TestResult_Error_WithThreat(t *testing.T) {
	data := makePDF("/S /JavaScript /JS (x())")
	_, result, _ := scriptdetect.ScanFile(writeTemp(t, data))
	if result.Error() == "" {
		t.Error("want non-empty Error() for result with threat")
	}
}

func TestThreatType_String(t *testing.T) {
	cases := []struct {
		t    scriptdetect.ThreatType
		want string
	}{
		{scriptdetect.ThreatNone, "None"},
		{scriptdetect.ThreatJavaScript, "JavaScript"},
		{scriptdetect.ThreatLaunchAction, "LaunchAction"},
		{scriptdetect.ThreatJavaScript | scriptdetect.ThreatLaunchAction, "JavaScript+LaunchAction"},
	}
	for _, tc := range cases {
		if got := tc.t.String(); got != tc.want {
			t.Errorf("ThreatType(%d).String() = %q, want %q", tc.t, got, tc.want)
		}
	}
}

func TestSeverity_String(t *testing.T) {
	cases := []struct {
		s    scriptdetect.Severity
		want string
	}{
		{scriptdetect.SeverityLow, "LOW"},
		{scriptdetect.SeverityMedium, "MEDIUM"},
		{scriptdetect.SeverityHigh, "HIGH"},
		{scriptdetect.SeverityCritical, "CRITICAL"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Severity.String() = %q, want %q", got, tc.want)
		}
	}
}

// ── Real-world fixtures ───────────────────────────────────────────────────────

const sampleDirDocs = "/Users/OS7774/Documents/PDF"

func docPath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(sampleDirDocs, name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sample file not found, skipping: %s", path)
	}
	return path
}

// TestScanFile_AnomalyForm_DetectsJavaScript verifies that the JotForm PDF with
// active JavaScript (/S /JavaScript + /JS key) is flagged at SeverityMedium.
func TestScanFile_AnomalyForm_DetectsJavaScript(t *testing.T) {
	path := docPath(t, "pdf-file-anomaly-form.pdf")

	_, result, err := scriptdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !result.HasThreat {
		t.Error("want HasThreat=true for PDF with embedded JavaScript actions")
	}
	if result.Threats&scriptdetect.ThreatJavaScript == 0 {
		t.Errorf("want ThreatJavaScript, got %s", result.Threats)
	}
}

// TestScanFile_MalformedPDF_NoScriptThreat verifies that a CJK CMap PDF
// (UniGB-UTF16-H) produces no false positive in the script scanner. The CMap
// issue is cmapdetector's domain, not this package's.
func TestScanFile_MalformedPDF_NoScriptThreat(t *testing.T) {
	path := docPath(t, "malformed.pdf")

	_, result, err := scriptdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	// URI links in this file are benign hyperlinks, not threats.
	// Only check that no HIGH+ severity threats are present.
	if result.MaxSeverity() >= scriptdetect.SeverityHigh {
		t.Errorf("want no HIGH+ severity for CJK-CMap PDF, got findings: %v", result.Findings)
	}
}

// TestScanFile_FakePDF_NotPDF verifies that a PNG renamed to .pdf returns ErrNotPDF.
func TestScanFile_FakePDF_NotPDF(t *testing.T) {
	path := docPath(t, "fake pdf.pdf")

	_, _, err := scriptdetect.ScanFile(path)
	if !errors.Is(err, scriptdetect.ErrNotPDF) {
		t.Errorf("want ErrNotPDF for PNG-disguised-as-PDF, got %v", err)
	}
}

// TestScanFile_FileCorrupt01_NotPDF verifies that a high-entropy binary
// (likely encrypted, no PDF header) returns ErrNotPDF.
func TestScanFile_FileCorrupt01_NotPDF(t *testing.T) {
	path := docPath(t, "fileCorrupt-01.pdf")

	_, _, err := scriptdetect.ScanFile(path)
	if !errors.Is(err, scriptdetect.ErrNotPDF) {
		t.Errorf("want ErrNotPDF for encrypted binary, got %v", err)
	}
}

// TestScanFile_Terinjeksi_NoFalsePositive verifies that a synthetic "injected"
// PDF (inline Adobe-Identity-UCS CMap, no scripts) is not flagged.
func TestScanFile_Terinjeksi_NoFalsePositive(t *testing.T) {
	path := docPath(t, "pdf_terinjeksi.pdf")

	_, result, err := scriptdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.MaxSeverity() >= scriptdetect.SeverityMedium {
		t.Errorf("want no MEDIUM+ threats for synthetic test PDF, got %v", result.Findings)
	}
}

// TestScanFile_LOIANT1036_Clean verifies that a regular signed document is clean.
func TestScanFile_LOIANT1036_Clean(t *testing.T) {
	path := docPath(t, "LOIANT1036 Document Mar 26 2026.pdf")

	_, result, err := scriptdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasThreat {
		t.Errorf("want HasThreat=false for clean signed PDF, got %v", result.Findings)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// writeTemp writes data to a temp .pdf file and returns the path.
func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}
