package cmapdetector

import "fmt"

// MakePDF builds a minimal (non-renderable) PDF byte slice that embeds the
// given CMap name inside a /ToUnicode stream reference, mimicking how a real
// CJK PDF references its CMap.
func MakePDF(cmapName string) []byte {
	body := fmt.Sprintf(
		"%%PDF-1.4\n1 0 obj\n<</Type /Font /Encoding /%s>>\nendobj\n",
		cmapName,
	)
	return []byte(body)
}
