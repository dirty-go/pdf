package cmapdetector

import (
	"fmt"
	"os"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

// makePDF builds a minimal (non-renderable) PDF byte slice that embeds the
// given CMap name inside a /ToUnicode stream reference, mimicking how a real
// CJK PDF references its CMap.
func MakePDF(cmapName string) []byte {
	body := fmt.Sprintf(
		"%%PDF-1.4\n1 0 obj\n<</Type /Font /Encoding /%s>>\nendobj\n",
		cmapName,
	)
	return []byte(body)
}

// makePDFFile writes a synthetic PDF to a temp file and returns the path.
func MakePDFFile(t *testing.T, cmapName string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(MakePDF(cmapName)); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// makeCleanPDFFile writes a synthetic PDF with no problematic CMap references.
func MakeCleanPDFFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "clean-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("%PDF-1.4\n1 0 obj\n<</Type /Font /Encoding /WinAnsiEncoding>>\nendobj\n")
	return f.Name()
}
