package cmapdetector_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	cmapdetect "github.com/dirty-go/pdf/cmapdetector"
)

// ---- Method 1: ScanBytes ----------------------------------------------------

func TestScanBytes_DetectsKnownCMap(t *testing.T) {
	cases := []struct {
		cmapName string
	}{
		{"UniGB-UTF16-H"},
		{"UniGB-UTF16-V"},
		{"UniCNS-UTF16-H"},
		{"UniJIS-UTF16-H"},
		{"UniKS-UTF16-H"},
		{"GBK-EUC-H"},
		{"ETen-B5-H"},
		{"KSCms-UHC-H"},
		{"83pv-RKSJ-H"},
	}
	for _, tc := range cases {
		t.Run(tc.cmapName, func(t *testing.T) {
			data := cmapdetect.MakePDF(tc.cmapName)
			result := cmapdetect.ScanBytes(data)

			if !result.HasProblematicCMap {
				t.Errorf("expected HasProblematicCMap=true for %q", tc.cmapName)
			}
			if !contains(result.FoundCMaps, tc.cmapName) {
				t.Errorf("expected %q in FoundCMaps, got %v", tc.cmapName, result.FoundCMaps)
			}
			if result.DetectedBy&cmapdetect.MethodBytesScan == 0 {
				t.Errorf("expected MethodBytesScan to be set")
			}
		})
	}
}

func TestScanBytes_CleanPDF(t *testing.T) {
	data := []byte("%PDF-1.4\n1 0 obj\n<</Type /Font /Encoding /WinAnsiEncoding>>\nendobj\n")
	result := cmapdetect.ScanBytes(data)

	if result.HasProblematicCMap {
		t.Errorf("expected HasProblematicCMap=false for clean PDF, got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("expected MethodNone, got %s", result.DetectedBy)
	}
}

func TestScanBytes_MultipleCMaps(t *testing.T) {
	data := []byte("%PDF-1.4\n/UniGB-UTF16-H /UniJIS-UTF16-H /KSCms-UHC-H\n")
	result := cmapdetect.ScanBytes(data)

	if len(result.FoundCMaps) < 3 {
		t.Errorf("expected at least 3 CMaps detected, got %v", result.FoundCMaps)
	}
}

// ---- Method 1: ScanFile -----------------------------------------------------

func TestScanFile_DetectsCMap(t *testing.T) {
	path := cmapdetect.MakePDFFile(t, "UniGB-UTF16-H")
	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasProblematicCMap {
		t.Error("expected HasProblematicCMap=true")
	}
	if result.FilePath != path {
		t.Errorf("FilePath mismatch: got %q want %q", result.FilePath, path)
	}
}

func TestScanFile_RejectsNonPDF(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "notapdf-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("this is not a PDF")
	f.Close()

	_, _, err = cmapdetect.ScanFile(f.Name())
	if !errors.Is(err, cmapdetect.ErrNotPDF) {
		t.Errorf("expected ErrNotPDF, got %v", err)
	}
}

func TestScanFile_MissingFile(t *testing.T) {
	_, _, err := cmapdetect.ScanFile(filepath.Join(t.TempDir(), "nonexistent.pdf"))
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// ---- Method 1: ScanReader ---------------------------------------------------

func TestScanReader_DetectsCMap(t *testing.T) {
	data := cmapdetect.MakePDF("UniCNS-UTF16-V")
	r := bytes.NewReader(data)

	readData, result, err := cmapdetect.ScanReader(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readData, data) {
		t.Error("returned bytes do not match input")
	}
	if !result.HasProblematicCMap {
		t.Error("expected HasProblematicCMap=true")
	}
}

// ---- Method 2: WrapProcess --------------------------------------------------

func TestWrapProcess_CatchesPanic(t *testing.T) {
	panicking := func(path string) error {
		panic("com/itextpdf/io/font/cmap/UniGB-UTF16-H was not found")
	}

	result, err := cmapdetect.WrapProcess("test.pdf", panicking, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !result.HasProblematicCMap {
		t.Error("expected HasProblematicCMap=true")
	}
	if result.DetectedBy&cmapdetect.MethodRuntime == 0 {
		t.Errorf("expected MethodRuntime to be set, got %s", result.DetectedBy)
	}
	if !errors.Is(err, cmapdetect.ErrMissingCMap) {
		t.Errorf("expected err to wrap ErrMissingCMap, got %T: %v", err, err)
	}
}

func TestWrapProcess_CatchesCMapError(t *testing.T) {
	returnsErr := func(path string) error {
		return fmt.Errorf("Could not find a CMap: missing CMap resource for UniJIS-UTF16-H")
	}

	result, err := cmapdetect.WrapProcess("test.pdf", returnsErr, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !result.HasProblematicCMap {
		t.Error("expected HasProblematicCMap=true")
	}
	if !errors.Is(err, cmapdetect.ErrMissingCMap) {
		t.Errorf("expected ErrMissingCMap, got %v", err)
	}
}

func TestWrapProcess_PassesThroughUnrelatedError(t *testing.T) {
	unrelated := fmt.Errorf("permission denied: /etc/shadow")
	returnsErr := func(path string) error { return unrelated }

	result, err := cmapdetect.WrapProcess("test.pdf", returnsErr, nil)
	if result.HasProblematicCMap {
		t.Error("expected HasProblematicCMap=false for unrelated error")
	}
	if !errors.Is(err, unrelated) {
		t.Errorf("expected unrelated error to be passed through, got %v", err)
	}
}

func TestWrapProcess_PassesThroughUnrelatedPanic(t *testing.T) {
	badPanic := func(path string) error {
		panic("index out of range [5] with length 3")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected unrelated panic to be re-panicked")
		}
	}()

	cmapdetect.WrapProcess("test.pdf", badPanic, nil) //nolint:errcheck
}

func TestWrapProcess_MergesWithExistingResult(t *testing.T) {
	// Simulate Method 1 already having found something
	existing := &cmapdetect.Result{
		FilePath:           "test.pdf",
		HasProblematicCMap: true,
		FoundCMaps:         []string{"UniGB-UTF16-H"},
		DetectedBy:         cmapdetect.MethodBytesScan,
	}

	returnsErr := func(path string) error {
		return fmt.Errorf("com/itextpdf/io/font/cmap/UniGB-UTF16-H was not found")
	}

	result, _ := cmapdetect.WrapProcess("test.pdf", returnsErr, existing)
	if result.DetectedBy&cmapdetect.MethodBytesScan == 0 {
		t.Error("expected MethodBytesScan to still be set after merge")
	}
	if result.DetectedBy&cmapdetect.MethodRuntime == 0 {
		t.Error("expected MethodRuntime to be added")
	}
}

// ---- Combined: DetectFile ---------------------------------------------------

func TestDetectFile_Method1Only(t *testing.T) {
	path := cmapdetect.MakePDFFile(t, "UniGB-UTF16-H")
	result, err := cmapdetect.DetectFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasProblematicCMap {
		t.Error("expected detection")
	}
	if result.DetectedBy != cmapdetect.MethodBytesScan {
		t.Errorf("expected only MethodBytesScan, got %s", result.DetectedBy)
	}
}

func TestDetectFile_BothMethods(t *testing.T) {
	path := cmapdetect.MakePDFFile(t, "UniGB-UTF16-H")
	fn := func(p string) error {
		return fmt.Errorf("com/itextpdf/io/font/cmap/UniGB-UTF16-H was not found")
	}
	result, err := cmapdetect.DetectFile(path, fn)
	if err != nil {
		// DetectFile suppresses CMap detection results from the error channel.
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasProblematicCMap {
		t.Error("expected detection")
	}
	want := cmapdetect.MethodBytesScan | cmapdetect.MethodRuntime
	if result.DetectedBy != want {
		t.Errorf("expected DetectedBy=%s, got %s", want, result.DetectedBy)
	}
}

func TestDetectFile_CleanPDF(t *testing.T) {
	path := cmapdetect.MakeCleanPDFFile(t)
	fn := func(p string) error { return nil }

	result, err := cmapdetect.DetectFile(path, fn)
	if err != nil {
		t.Fatal(err)
	}
	if result.HasProblematicCMap {
		t.Errorf("expected no detection for clean PDF, got %v", result.FoundCMaps)
	}
}

// ---- Result -----------------------------------------------------------------

func TestResult_ErrorInterface(t *testing.T) {
	r := &cmapdetect.Result{
		FilePath:           "x.pdf",
		HasProblematicCMap: true,
		FoundCMaps:         []string{"UniGB-UTF16-H"},
		DetectedBy:         cmapdetect.MethodBytesScan,
	}
	if !errors.Is(r, cmapdetect.ErrMissingCMap) {
		t.Error("Result should satisfy errors.Is(ErrMissingCMap)")
	}
	if r.Error() == "" {
		t.Error("Result.Error() should not be empty when HasProblematicCMap=true")
	}
}

func TestResult_NoError(t *testing.T) {
	r := &cmapdetect.Result{HasProblematicCMap: false}
	if errors.Is(r, cmapdetect.ErrMissingCMap) {
		t.Error("clean Result should not satisfy ErrMissingCMap")
	}
	if r.Error() != "" {
		t.Errorf("expected empty Error(), got %q", r.Error())
	}
}

func TestDetectionMethod_String(t *testing.T) {
	cases := []struct {
		m    cmapdetect.DetectionMethod
		want string
	}{
		{cmapdetect.MethodNone, "None"},
		{cmapdetect.MethodBytesScan, "BytesScan"},
		{cmapdetect.MethodRuntime, "Runtime"},
		{cmapdetect.MethodBytesScan | cmapdetect.MethodRuntime, "BytesScan+Runtime"},
	}
	for _, tc := range cases {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("DetectionMethod(%d).String() = %q, want %q", tc.m, got, tc.want)
		}
	}
}

// ---- Repository sample fixtures ---------------------------------------------
//
// All files live under ./sample/ and ./sample/docs/ and are generated by
// cmd/gen-samples. Tests use skipIfAbsent so they are skipped gracefully on
// any machine where the generator has not been run, but in this repository the
// fixtures are committed and will always be present.

const (
	sampleDir     = "./sample"
	sampleDirDocs = "./sample/docs"
)

// skipIfAbsent skips the test when path does not exist and returns path otherwise.
func skipIfAbsent(t *testing.T, path string) string {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sample file not found (run: go run ./cmd/gen-samples/): %s", path)
	}
	return path
}

func samplePath(t *testing.T, name string) string {
	t.Helper()
	return skipIfAbsent(t, filepath.Join(sampleDir, name))
}

func docPath(t *testing.T, name string) string {
	t.Helper()
	return skipIfAbsent(t, filepath.Join(sampleDirDocs, name))
}

// ── sample/ — structural edge cases ──────────────────────────────────────────

// TestScanFile_CorruptedSample verifies that a structurally-corrupted PDF
// (Western TrueType fonts, no CJK CMap names, truncated XRef table) does not
// produce a false positive. The corruption is at the XRef level — not
// CMap-related — so the detector must leave it for the caller to handle.
func TestScanFile_CorruptedSample(t *testing.T) {
	path := samplePath(t, "corrupted.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasProblematicCMap {
		t.Errorf("want HasProblematicCMap=false for structurally-corrupted PDF (Western fonts only), got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("want MethodNone, got %s", result.DetectedBy)
	}
}

// TestScanFile_KalibratorSample verifies that a PDF with a custom internal
// /CMapName (/A-B-C) does not trigger detection. Only predefined CJK CMaps
// from knownProblematicCMaps should be flagged.
func TestScanFile_KalibratorSample(t *testing.T) {
	path := samplePath(t, "Kalibrator.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasProblematicCMap {
		t.Errorf("want HasProblematicCMap=false for PDF with custom /CMapName /A-B-C, got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("want MethodNone, got %s", result.DetectedBy)
	}
}

// ── sample/docs/ — clean baselines ───────────────────────────────────────────

// TestScanFile_Clean verifies that a standard PDF 1.7 document using Helvetica
// with WinAnsiEncoding produces no false positive.
func TestScanFile_Clean(t *testing.T) {
	path := docPath(t, "clean.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasProblematicCMap {
		t.Errorf("want HasProblematicCMap=false for clean PDF, got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("want MethodNone, got %s", result.DetectedBy)
	}
}

// TestScanFile_Surat_Clean verifies that a PDF 1.6 document (Times-Roman,
// MacRomanEncoding) is handled without error. The extractor package does not
// support XRef streams (PDF 1.5+) but the byte scanner is unaffected.
func TestScanFile_Surat_Clean(t *testing.T) {
	path := docPath(t, "Surat.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasProblematicCMap {
		t.Errorf("want HasProblematicCMap=false for PDF 1.6 document, got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("want MethodNone, got %s", result.DetectedBy)
	}
}

// ── sample/docs/ — CMap detection ────────────────────────────────────────────

// TestScanFile_MalformedPDF_DetectsCMap verifies that a PDF using
// AdobeSongStd-Light with UniGB-UTF16-H encoding is flagged. This is the
// canonical case this package exists to catch:
// "com.itextpdf.io.font.cmap.UniGB-UTF16-H not found".
func TestScanFile_MalformedPDF_DetectsCMap(t *testing.T) {
	path := docPath(t, "malformed.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !result.HasProblematicCMap {
		t.Error("want HasProblematicCMap=true for PDF with UniGB-UTF16-H font encoding")
	}
	if !contains(result.FoundCMaps, "UniGB-UTF16-H") {
		t.Errorf("want UniGB-UTF16-H in FoundCMaps, got %v", result.FoundCMaps)
	}
	if result.DetectedBy&cmapdetect.MethodBytesScan == 0 {
		t.Errorf("want MethodBytesScan set, got %s", result.DetectedBy)
	}
}

// TestScanFile_SeverityMedium_DetectsCMap verifies that a PDF containing
// KozMinPro-Regular with UniJIS-UTF16-H (Japanese) encoding is flagged.
func TestScanFile_SeverityMedium_DetectsCMap(t *testing.T) {
	path := docPath(t, "severity_medium.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !result.HasProblematicCMap {
		t.Error("want HasProblematicCMap=true for PDF with UniJIS-UTF16-H font encoding")
	}
	if !contains(result.FoundCMaps, "UniJIS-UTF16-H") {
		t.Errorf("want UniJIS-UTF16-H in FoundCMaps, got %v", result.FoundCMaps)
	}
	if result.DetectedBy&cmapdetect.MethodBytesScan == 0 {
		t.Errorf("want MethodBytesScan set, got %s", result.DetectedBy)
	}
}

// TestScanFile_SeverityCritical_DetectsCMap verifies that a PDF combining
// obfuscated JavaScript (eval/unescape) with UniGB-UTF16-H is flagged by the
// CMap detector. The script content is outside this package's scope.
func TestScanFile_SeverityCritical_DetectsCMap(t *testing.T) {
	path := docPath(t, "severity_critical.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !result.HasProblematicCMap {
		t.Error("want HasProblematicCMap=true for PDF with UniGB-UTF16-H + obfuscated JS")
	}
	if !contains(result.FoundCMaps, "UniGB-UTF16-H") {
		t.Errorf("want UniGB-UTF16-H in FoundCMaps, got %v", result.FoundCMaps)
	}
	if result.DetectedBy&cmapdetect.MethodBytesScan == 0 {
		t.Errorf("want MethodBytesScan set, got %s", result.DetectedBy)
	}
}

// ── sample/docs/ — false-positive guards ─────────────────────────────────────

// TestScanFile_Terinjeksi_NoFalsePositive verifies that a PDF with an inline
// /CMapName /Adobe-Identity-UCS (not in knownProblematicCMaps) does not
// produce a false positive.
func TestScanFile_Terinjeksi_NoFalsePositive(t *testing.T) {
	path := docPath(t, "injected.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasProblematicCMap {
		t.Errorf("want HasProblematicCMap=false for PDF with inline Adobe-Identity-UCS CMap, got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("want MethodNone, got %s", result.DetectedBy)
	}
}

// TestScanFile_AnomalyForm_Clean verifies that a PDF with embedded JavaScript
// form validation does NOT trigger CMap detection. JavaScript detection is
// out of scope for this package — use scriptdetector for that.
func TestScanFile_AnomalyForm_Clean(t *testing.T) {
	path := docPath(t, "form.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasProblematicCMap {
		t.Errorf("want HasProblematicCMap=false for JS-bearing PDF (no CJK CMap), got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("want MethodNone, got %s", result.DetectedBy)
	}
}

// TestScanFile_SeverityLow_NoFalsePositive verifies that a PDF containing only
// an /EmbeddedFile entry (no CJK CMap) does not trigger CMap detection.
func TestScanFile_SeverityLow_NoFalsePositive(t *testing.T) {
	path := docPath(t, "severity_low.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasProblematicCMap {
		t.Errorf("want HasProblematicCMap=false for EmbeddedFile-only PDF, got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("want MethodNone, got %s", result.DetectedBy)
	}
}

// TestScanFile_SeverityHigh_NoFalsePositive verifies that a PDF with /Launch
// action and String.fromCharCode (no CJK CMap) does not trigger CMap detection.
func TestScanFile_SeverityHigh_NoFalsePositive(t *testing.T) {
	path := docPath(t, "severity_high.pdf")

	_, result, err := cmapdetect.ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if result.HasProblematicCMap {
		t.Errorf("want HasProblematicCMap=false for /Launch PDF (no CJK CMap), got CMaps: %v", result.FoundCMaps)
	}
	if result.DetectedBy != cmapdetect.MethodNone {
		t.Errorf("want MethodNone, got %s", result.DetectedBy)
	}
}

// ── sample/docs/ — invalid files ─────────────────────────────────────────────

// TestScanFile_FileCorrupt_NotPDF verifies that a high-entropy binary without
// a %PDF- header is rejected with ErrNotPDF and never scanned.
func TestScanFile_FileCorrupt_NotPDF(t *testing.T) {
	path := docPath(t, "fileCorrupt.pdf")

	_, _, err := cmapdetect.ScanFile(path)
	if !errors.Is(err, cmapdetect.ErrNotPDF) {
		t.Errorf("want ErrNotPDF for non-PDF binary, got %v", err)
	}
}

// TestScanFile_FakePDF_NotPDF verifies that a PNG image renamed to .pdf is
// rejected with ErrNotPDF (magic bytes \x89PNG, not %PDF-).
func TestScanFile_FakePDF_NotPDF(t *testing.T) {
	path := docPath(t, "fake_pdf.pdf")

	_, _, err := cmapdetect.ScanFile(path)
	if !errors.Is(err, cmapdetect.ErrNotPDF) {
		t.Errorf("want ErrNotPDF for PNG-disguised-as-PDF, got %v", err)
	}
}

// ---- helper -----------------------------------------------------------------

func contains(ss []string, s string) bool { return slices.Contains(ss, s) }
