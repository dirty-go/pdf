package cmapdetector_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	cmapdetect "github.com/dirty-go/pdf/cmapdetector"
)

// ---- helpers ----------------------------------------------------------------

// makePDF builds a minimal (non-renderable) PDF byte slice that embeds the
// given CMap name inside a /ToUnicode stream reference, mimicking how a real
// CJK PDF references its CMap.
func makePDF(cmapName string) []byte {
	body := fmt.Sprintf(
		"%%PDF-1.4\n1 0 obj\n<</Type /Font /Encoding /%s>>\nendobj\n",
		cmapName,
	)
	return []byte(body)
}

// makePDFFile writes a synthetic PDF to a temp file and returns the path.
func makePDFFile(t *testing.T, cmapName string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(makePDF(cmapName)); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// makeCleanPDFFile writes a synthetic PDF with no problematic CMap references.
func makeCleanPDFFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "clean-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("%PDF-1.4\n1 0 obj\n<</Type /Font /Encoding /WinAnsiEncoding>>\nendobj\n")
	return f.Name()
}

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
		{"Identity-H"},
	}
	for _, tc := range cases {
		t.Run(tc.cmapName, func(t *testing.T) {
			data := makePDF(tc.cmapName)
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
	path := makePDFFile(t, "UniGB-UTF16-H")
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
	data := makePDF("UniCNS-UTF16-V")
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
	path := makePDFFile(t, "UniGB-UTF16-H")
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
	path := makePDFFile(t, "UniGB-UTF16-H")
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
	path := makeCleanPDFFile(t)
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

// ---- helper -----------------------------------------------------------------

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
