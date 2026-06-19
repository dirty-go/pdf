// gen-samples generates all PDF test fixtures for the cmapdetector and
// scriptdetector packages. Run from the project root:
//
//	go run ./cmd/gen-samples/
//
// Output directories:
//
//	./sample/docs/              — project-root location (user-facing)
//	./cmapdetector/sample/docs/ — location expected by cmapdetector tests
//	./cmapdetector/sample/      — location expected by cmapdetector root fixtures
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// makePDF wraps body in a minimal PDF envelope.
func makePDF(version, body string) []byte {
	return fmt.Appendf(nil, "%%PDF-%s\n%s\n%%%%EOF\n", version, body)
}

// ── PDF bodies ────────────────────────────────────────────────────────────────

// bodyClean is a standard PDF 1.7 page with a Helvetica font. No CJK, no scripts.
// cmapdetector: HasProblematicCMap=false.
// scriptdetector: HasThreat=false.
const bodyClean = `1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
   /Resources << /Font << /F1 4 0 R >> >>
>>
endobj
4 0 obj
<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica
   /Encoding /WinAnsiEncoding
>>
endobj`

// bodyMalformed is a PDF using AdobeSongStd-Light with UniGB-UTF16-H encoding.
// This is the canonical case that causes iTextPDF to throw
// "com.itextpdf.io.font.cmap.UniGB-UTF16-H was not found".
// cmapdetector: HasProblematicCMap=true, FoundCMaps=[UniGB-UTF16-H], DetectedBy=BytesScan.
const bodyMalformed = `1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
   /Resources << /Font << /F1 4 0 R >> >>
>>
endobj
4 0 obj
<< /Type /Font /Subtype /Type0 /BaseFont /AdobeSongStd-Light
   /Encoding /UniGB-UTF16-H
   /ToUnicode 5 0 R
>>
endobj
5 0 obj
<< /Length 16 >>
stream
% CMap reference
endstream
endobj`

// bodySurat is a clean PDF 1.6 document. The extractor package does not support
// XRef streams (PDF 1.5+) but cmapdetector's byte scanner works regardless.
// cmapdetector: HasProblematicCMap=false.
const bodySurat = `1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842]
   /Resources << /Font << /F1 4 0 R >> >>
>>
endobj
4 0 obj
<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman
   /Encoding /MacRomanEncoding
>>
endobj`

// bodyInjected contains an inline /CMapName /Adobe-Identity-UCS — a custom
// CMap name that is NOT in knownProblematicCMaps. This is a false-positive
// guard: the detector must treat custom inline CMaps as safe.
// cmapdetector: HasProblematicCMap=false.
const bodyInjected = `1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>
endobj
4 0 obj
<< /Length 217 >>
stream
/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
endcmap
CMapResource defineresource pop
end
end
endstream
endobj`

// bodyForm is a JotForm-style PDF with embedded JavaScript form validation.
// cmapdetector: HasProblematicCMap=false — no CJK CMap names present.
// scriptdetector: ThreatJavaScript (MEDIUM) — /S /JavaScript + /JS key present.
const bodyForm = `1 0 obj
<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [5 0 R] >> >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>
endobj
4 0 obj
<< /Type /Action /S /JavaScript
   /JS (if(this.getField('email').value==''){app.alert('Email wajib diisi');})
>>
endobj
5 0 obj
<< /Type /Annot /Subtype /Widget /T (email) /AA << /V 4 0 R >> >>
endobj`

// bodySeverityLow contains only an /EmbeddedFile entry — an informational
// finding. This may be a benign PDF/A attachment; manual review is recommended.
// scriptdetector: ThreatEmbeddedFile (LOW).
// cmapdetector: HasProblematicCMap=false.
const bodySeverityLow = `1 0 obj
<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles 3 0 R >> >>
endobj
2 0 obj
<< /Type /Pages /Kids [] /Count 0 >>
endobj
3 0 obj
<< /Names [(readme.txt) 4 0 R] >>
endobj
4 0 obj
<< /Type /Filespec /F (readme.txt)
   /EF << /F 5 0 R >>
>>
endobj
5 0 obj
<< /Type /EmbeddedFile /Length 13 >>
stream
Hello, world!
endstream
endobj`

// bodySeverityMedium contains a Japanese CJK font (UniJIS-UTF16-H) and a
// JavaScript open-document action. Both detectors fire at MEDIUM severity.
// cmapdetector: HasProblematicCMap=true, FoundCMaps=[UniJIS-UTF16-H].
// scriptdetector: ThreatJavaScript (MEDIUM).
const bodySeverityMedium = `1 0 obj
<< /Type /Catalog /Pages 2 0 R /OpenAction 5 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
   /Resources << /Font << /F1 4 0 R >> >>
>>
endobj
4 0 obj
<< /Type /Font /Subtype /Type0 /BaseFont /KozMinPro-Regular
   /Encoding /UniJIS-UTF16-H
>>
endobj
5 0 obj
<< /Type /Action /S /JavaScript
   /JS (app.alert('Dokumen dibuka'))
>>
endobj`

// bodySeverityHigh contains a /Launch action (executes an external application)
// and a String.fromCharCode obfuscation pattern.
// Neither technique requires CJK fonts.
// scriptdetector: ThreatLaunchAction (HIGH) + ThreatObfuscation (HIGH).
// cmapdetector: HasProblematicCMap=false.
//
// NOTE: The /Launch target and String.fromCharCode content do not form a working
// exploit. Patterns are present for scanner detection only.
const bodySeverityHigh = `1 0 obj
<< /Type /Catalog /Pages 2 0 R /OpenAction 4 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>
endobj
4 0 obj
<< /Type /Action /S /Launch
   /Win << /F (calc.exe) /O (open) >>
>>
endobj
5 0 obj
<< /Type /Action /S /JavaScript
   /JS (var p = String.fromCharCode(99,97,108,99,46,101,120,101))
>>
endobj`

// bodySeverityCritical combines the most severe threat indicators from both
// detectors: eval(unescape()) obfuscation, ActiveXObject / cmd.exe shell
// execution, and a CJK CMap (UniGB-UTF16-H) that fails iTextPDF.
// scriptdetector: ThreatObfuscation (CRITICAL) + ThreatShellExecution (CRITICAL).
// cmapdetector: HasProblematicCMap=true, FoundCMaps=[UniGB-UTF16-H].
//
// NOTE: The eval/unescape payload decodes to the ASCII string
// "function noop(){}" — a no-op. No working shellcode is present.
// The ActiveXObject/cmd.exe strings are scanner patterns only.
const bodySeverityCritical = `1 0 obj
<< /Type /Catalog /Pages 2 0 R /OpenAction 5 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
   /Resources << /Font << /F1 4 0 R >> >>
>>
endobj
4 0 obj
<< /Type /Font /Subtype /Type0 /BaseFont /AdobeSongStd-Light
   /Encoding /UniGB-UTF16-H
>>
endobj
5 0 obj
<< /Type /Action /S /JavaScript
   /JS (eval(unescape('%66%75%6e%63%74%69%6f%6e%20%6e%6f%6f%70%28%29%7b%7d')))
>>
endobj
6 0 obj
<< /Type /Action /S /JavaScript
   /JS (var s = new ActiveXObject('WScript.Shell'); s.Run('cmd.exe /c echo test'))
>>
endobj`

// bodyCorrupted is a Western-fonts PDF whose XRef table is deliberately
// truncated to simulate structural corruption at the cross-reference level.
// The corruption has nothing to do with CMaps.
// cmapdetector: HasProblematicCMap=false, MethodNone.
const bodyCorrupted = "%PDF-1.4\n" +
	"1 0 obj\n<< /Type /Font /Subtype /TrueType /BaseFont /Arial\n" +
	"   /Encoding /WinAnsiEncoding >>\nendobj\n" +
	"xref\n0 1\n0000000000 65535 f \n" +
	"% XRef truncated — structural corruption for testing\n" +
	"trailer\n<< /Root 1 0 R /Size 1 >>\nstartxref\n9\n%%EOF\n"

// bodyKalibrator contains a custom inline /CMapName /A-B-C. The name /A-B-C
// is NOT in knownProblematicCMaps, so the detector must not flag this file.
// cmapdetector: HasProblematicCMap=false, MethodNone.
const bodyKalibrator = `1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>
endobj
4 0 obj
<< /Length 120 >>
stream
/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CMapName /A-B-C def
endcmap
CMapResource defineresource pop
end
end
endstream
endobj`

// ── Fixtures ──────────────────────────────────────────────────────────────────

type fixture struct {
	name    string
	data    []byte
	comment string // logged to stdout for documentation
}

// docsFixtures returns all fixtures that belong in sample/docs/.
func docsFixtures() []fixture {
	return []fixture{
		// ── Baseline / clean ─────────────────────────────────────────────────
		{
			"clean.pdf",
			makePDF("1.7", bodyClean),
			"CLEAN — PDF 1.7, Helvetica, WinAnsiEncoding. No threats.",
		},
		{
			"Surat.pdf",
			makePDF("1.6", bodySurat),
			"CLEAN — PDF 1.6 (XRef-stream era), Times-Roman, MacRoman. No threats.",
		},
		{
			"injected.pdf",
			makePDF("1.4", bodyInjected),
			"CLEAN — inline Adobe-Identity-UCS CMap (not in problematic list). False-positive guard.",
		},

		// ── CMap issue only ──────────────────────────────────────────────────
		{
			"malformed.pdf",
			makePDF("1.4", bodyMalformed),
			"MEDIUM (cmapdetector) — AdobeSongStd-Light / UniGB-UTF16-H. Triggers iTextPDF cmap error.",
		},

		// ── Script issue only ────────────────────────────────────────────────
		{
			"form.pdf",
			makePDF("1.7", bodyForm),
			"MEDIUM (scriptdetector) — /S /JavaScript form validation. No CJK CMap.",
		},

		// ── Severity-labelled fixtures ───────────────────────────────────────
		{
			"severity_low.pdf",
			makePDF("1.7", bodySeverityLow),
			"LOW (scriptdetector) — /EmbeddedFile attachment only.",
		},
		{
			"severity_medium.pdf",
			makePDF("1.7", bodySeverityMedium),
			"MEDIUM (both) — UniJIS-UTF16-H CMap + /S /JavaScript open action.",
		},
		{
			"severity_high.pdf",
			makePDF("1.7", bodySeverityHigh),
			"HIGH (scriptdetector) — /Launch action + String.fromCharCode obfuscation.",
		},
		{
			"severity_critical.pdf",
			makePDF("1.7", bodySeverityCritical),
			"CRITICAL (scriptdetector) + MEDIUM (cmapdetector) — eval(unescape) + ActiveXObject/cmd.exe + UniGB-UTF16-H.",
		},

		// ── Invalid / non-PDF ────────────────────────────────────────────────
		{
			"fileCorrupt.pdf",
			makeCorruptBinary(),
			"INVALID — high-entropy binary, no %PDF- header. Expect ErrNotPDF.",
		},
		{
			"fake_pdf.pdf",
			makePNGMagic(),
			"INVALID — PNG magic bytes disguised as PDF. Expect ErrNotPDF.",
		},
	}
}

// sampleFixtures returns fixtures that belong in sample/ (not sample/docs/).
func sampleFixtures() []fixture {
	return []fixture{
		{
			"corrupted.pdf",
			[]byte(bodyCorrupted),
			"CLEAN (cmapdetector) — Western TrueType, XRef-level corruption. No CJK.",
		},
		{
			"Kalibrator.pdf",
			makePDF("1.4", bodyKalibrator),
			"CLEAN (cmapdetector) — custom /CMapName /A-B-C (not in problematic list).",
		},
	}
}

// makeCorruptBinary returns a 256-byte deterministic pseudo-random buffer that
// does NOT start with %PDF- (first byte is 0xFF).
func makeCorruptBinary() []byte {
	buf := make([]byte, 256)
	for i := range buf {
		buf[i] = byte((i*37 + 13) % 256)
	}
	buf[0] = 0xFF // ensure no %PDF- header
	return buf
}

// makePNGMagic returns the first 32 bytes of a valid 1×1 PNG image.
func makePNGMagic() []byte {
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D,                           // IHDR chunk length
		0x49, 0x48, 0x44, 0x52,                           // "IHDR"
		0x00, 0x00, 0x00, 0x01,                           // width = 1
		0x00, 0x00, 0x00, 0x01,                           // height = 1
		0x08, 0x02,                                        // 8-bit RGB
		0x00, 0x00, 0x00,                                  // compression, filter, interlace
		0x90, 0x77, 0x53, 0xDE,                           // CRC32
	}
}

// ── Writer ────────────────────────────────────────────────────────────────────

func writeFixtures(dir string, fixtures []fixture) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	for _, f := range fixtures {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, f.data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Printf("  %-40s %4d bytes  %s\n", f.name, len(f.data), f.comment)
	}
	return nil
}

func main() {
	// Each entry is [docsDir, sampleDir] pair to write into.
	destinations := [][2]string{
		// project-root location (user-facing)
		{"sample/docs", "sample"},
		// cmapdetector package test location
		{"cmapdetector/sample/docs", "cmapdetector/sample"},
	}

	docs := docsFixtures()
	samples := sampleFixtures()

	for _, dest := range destinations {
		docsDir, sampleDir := dest[0], dest[1]

		fmt.Printf("\n── %s ──\n", docsDir)
		if err := writeFixtures(docsDir, docs); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("── %s ──\n", sampleDir)
		if err := writeFixtures(sampleDir, samples); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("\nTotal: %d docs fixtures × 2 destinations, %d sample fixtures × 2 destinations.\n",
		len(docs), len(samples))
}
