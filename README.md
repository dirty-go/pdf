# dirty-go/pdf

A collection of zero-dependency Go packages for PDF inspection, threat detection, and font analysis. All packages operate on raw bytes — no external C libraries, no cgo, no runtime PDF renderer required.

```
github.com/dirty-go/pdf
├── cmapdetector   — detect CJK CMap encoding issues before they crash your PDF library
├── scriptdetector — detect embedded scripts and malicious PDF constructs
└── extractor      — parse XRef tables, extract objects, analyze font embedding
```

**Go 1.23+. No external dependencies.**

---

## Installation

```bash
go get github.com/dirty-go/pdf
```

---

## Package: `cmapdetector`

Detects PDF files that reference CMap resources unavailable in common PDF processing libraries (iTextPDF, pdfbox, pdfcpu). This prevents the classic runtime crash:

```
com.itextpdf.io.font.cmap.UniGB-UTF16-H was not found
Could not find a CMap: missing CMap resource for UniJIS-UTF16-H
```

Two complementary strategies are combined:

| Method | Function | How it works |
|---|---|---|
| **Method 1** — Byte scan | `ScanBytes` / `ScanFile` / `ScanReader` | Searches raw PDF bytes for known CJK CMap name strings. Zero-parse, runs before any library touches the file. |
| **Method 2** — Runtime recovery | `WrapProcess` / `WrapProcessBytes` | Wraps your PDF processing function, catching panics and errors whose messages match known CMap error signatures. |

### Detecting a file on disk

```go
import cmapdetect "github.com/dirty-go/pdf/cmapdetector"

// Method 1 only (byte scan — no PDF library required)
result, err := cmapdetect.DetectFile("invoice.pdf", nil)
if err != nil {
    log.Fatal(err)
}
if result.HasProblematicCMap {
    fmt.Println("problematic CMaps:", result.FoundCMaps)
    fmt.Println("detected by:", result.DetectedBy) // "BytesScan"
}

// Methods 1 + 2 (byte scan + runtime recovery via your PDF library)
result, err = cmapdetect.DetectFile("invoice.pdf", func(path string) error {
    return myPDFLib.Open(path) // any function that processes a PDF
})
```

### Scanning raw bytes

```go
data, err := os.ReadFile("invoice.pdf")
if err != nil {
    log.Fatal(err)
}

result := cmapdetect.ScanBytes(data)
fmt.Println(result.HasProblematicCMap) // true/false
fmt.Println(result.FoundCMaps)         // e.g. ["UniGB-UTF16-H"]
fmt.Println(result.IsEncrypted)        // true if /Encrypt dict present

// Attach a file path to the result without re-reading the file
result = cmapdetect.DetectBytes("invoice.pdf", data)
```

### Scanning from an `io.Reader`

```go
f, _ := os.Open("invoice.pdf")
defer f.Close()

rawBytes, result, err := cmapdetect.ScanReader(f)
// rawBytes is available for Method 2 reuse without re-reading
```

### Method 2: wrapping your PDF library call

```go
// WrapProcess catches panics and CMap errors from fn.
// Pass the Method 1 result as `existing` to merge both methods' findings.
_, result, err := cmapdetect.ScanFile("invoice.pdf")

result, err = cmapdetect.WrapProcess("invoice.pdf", func(path string) error {
    return myPDFLib.Open(path)
}, result)

if err != nil {
    var r *cmapdetect.Result
    if errors.As(err, &r) {
        // CMap detection — not a hard failure
        fmt.Println("detected by:", r.DetectedBy) // "BytesScan+Runtime"
    } else {
        log.Fatal("unrelated error:", err)
    }
}
```

### HTTP multipart uploads

```go
// Single file from a multipart form field
func uploadHandler(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(32 << 20)
    fh := r.MultipartForm.File["pdf"][0]

    // Method 1 only
    result, err := cmapdetect.DetectMultipartFile(fh, nil)

    // Methods 1 + 2
    result, err = cmapdetect.DetectMultipartFile(fh, func(name string, data []byte) error {
        return myPDFLib.ParseBytes(name, data)
    })

    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    json.NewEncoder(w).Encode(result)
}

// Multiple files in one pass
func batchUploadHandler(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(128 << 20)
    fhs := r.MultipartForm.File["pdfs"]

    batch := cmapdetect.DetectMultipartFiles(fhs, nil)
    fmt.Printf("total: %d, flagged: %d, errors: %d\n",
        batch.TotalCount, batch.FlaggedCount, batch.ErrorCount)

    for _, result := range batch.Results {
        if result.HasProblematicCMap {
            fmt.Println(result.FilePath, result.FoundCMaps)
        }
    }
    for _, fe := range batch.Errors {
        fmt.Println("error:", fe.Filename, fe.Err)
    }
}
```

### Interpreting the `Result`

```go
type Result struct {
    FilePath           string          // path or upload filename
    FileSize           int64           // bytes (set for multipart uploads)
    IsEncrypted        bool            // /Encrypt dict present (does not block detection)
    HasProblematicCMap bool            // true when any method fires
    FoundCMaps         []string        // CMap names found by Method 1
    DetectedBy         DetectionMethod // MethodBytesScan | MethodRuntime bitmask
    RuntimeErr         error           // captured panic/error from Method 2
}

// Result implements error — use errors.As to distinguish CMap results from hard errors.
var r *cmapdetect.Result
if errors.As(err, &r) { /* CMap detected */ }

// errors.Is works via Unwrap
errors.Is(err, cmapdetect.ErrMissingCMap) // true when HasProblematicCMap
```

### Sentinel errors

| Error | When returned |
|---|---|
| `ErrMissingCMap` | A CMap resource could not be resolved |
| `ErrNotPDF` | File does not start with `%PDF-` |
| `ErrPDFPanic` | Underlying library panicked (wrapped by `RuntimeErr`) |
| `ErrEncryptedPDF` | File carries `/Encrypt` — provided for caller signalling |

### `DetectionMethod` bitmask

```go
result.DetectedBy == cmapdetect.MethodBytesScan               // Method 1 only
result.DetectedBy == cmapdetect.MethodRuntime                 // Method 2 only
result.DetectedBy == cmapdetect.MethodBytesScan | cmapdetect.MethodRuntime // both
result.DetectedBy.String() // "BytesScan", "Runtime", "BytesScan+Runtime", "None"
```

---

## Package: `scriptdetector`

Detects embedded scripts, launch actions, and other malicious constructs in PDF files via raw byte pattern matching. Works on unencrypted content without parsing the PDF structure.

### Threat categories

| Constant | Severity | Description |
|---|---|---|
| `ThreatJavaScript` | MEDIUM | `/S /JavaScript` or `/JS` key |
| `ThreatLaunchAction` | HIGH | `/Launch` — executes an external application |
| `ThreatEmbeddedFile` | LOW | `/EmbeddedFile` — may be a benign attachment |
| `ThreatXFAForm` | MEDIUM | `/XFA` — XML Forms Architecture (scriptable) |
| `ThreatObfuscation` | HIGH / CRITICAL | `eval(unescape(`, `String.fromCharCode`, `unescape(` |
| `ThreatShellExecution` | HIGH / CRITICAL | `ActiveXObject`, `WScript.Shell`, `powershell`, `cmd.exe`, `/bin/sh`, `/bin/bash` |

### Scanning a file

```go
import scriptdetect "github.com/dirty-go/pdf/scriptdetector"

_, result, err := scriptdetect.ScanFile("document.pdf")
if err != nil {
    // err == scriptdetect.ErrNotPDF      — not a PDF
    // err == scriptdetect.ErrEncryptedPDF — encrypted, no password given
    log.Fatal(err)
}

if result.HasThreat {
    fmt.Println("max severity:", result.MaxSeverity()) // LOW / MEDIUM / HIGH / CRITICAL
    for _, f := range result.Findings {
        fmt.Printf("[%s] %s — %q\n", f.Severity, f.Pattern, f.Snippet)
    }
}
```

### Scanning an encrypted PDF

```go
// Without a password: ScanFile returns ErrEncryptedPDF.
// With a password: proceeds with a partial scan. Dictionary keys
// such as /S /JavaScript are readable even in encrypted files;
// string and stream content is not decrypted.
_, result, err := scriptdetect.ScanFile("protected.pdf",
    scriptdetect.WithPassword("secret"))

fmt.Println(result.IsEncrypted) // always true when /Encrypt is present
```

### Scanning raw bytes or a reader

```go
data, _ := os.ReadFile("document.pdf")
result := scriptdetect.ScanBytes(data) // never returns an error

// Reader variant (returns raw bytes for reuse)
rawBytes, result, err := scriptdetect.ScanReader(r)
rawBytes, result, err  = scriptdetect.ScanReader(r, scriptdetect.WithPassword("pw"))
```

### Interpreting `Result` and `Finding`

```go
type Result struct {
    FilePath    string
    HasThreat   bool
    Threats     ThreatType // bitmask of detected categories
    IsEncrypted bool
    Findings    []Finding  // one entry per ThreatType, sorted by Severity desc
}

type Finding struct {
    Threat   ThreatType
    Severity Severity
    Pattern  string // human-readable description of what matched
    Snippet  string // ±40 bytes of context around the match
}

// Checking specific threats
if result.Threats&scriptdetect.ThreatJavaScript != 0 { /* JS present */ }
if result.Threats&scriptdetect.ThreatShellExecution != 0 { /* shell cmd present */ }

// Severity comparison
if result.MaxSeverity() >= scriptdetect.SeverityHigh {
    // block the upload
}
```

### Severity levels

```go
scriptdetect.SeverityLow      // informational; may be legitimate
scriptdetect.SeverityMedium   // suspicious; manual review recommended
scriptdetect.SeverityHigh     // strong indicator of malicious intent
scriptdetect.SeverityCritical // block immediately
```

### Sentinel errors

| Error | When returned |
|---|---|
| `ErrNotPDF` | File does not start with `%PDF-` |
| `ErrEncryptedPDF` | File is encrypted and no password was provided |
| `ErrWrongPassword` | Password provided but does not authenticate |

---

## Package: `extractor`

Low-level PDF parser — reads raw bytes, parses classic XRef tables, extracts object bodies, and walks the object graph to analyze font embedding.

> **Limitation:** only classic cross-reference tables are supported. XRef streams (PDF 1.5+, indicated by a `stream` keyword after `xref`) are not parsed.

### Loading a PDF

```go
import "github.com/dirty-go/pdf/extractor"

// From a file path
pdf, err := extractor.NewPDFFromFile("document.pdf")

// From a byte slice
data, _ := os.ReadFile("document.pdf")
pdf := extractor.NewPDF(data)

// From an io.Reader
data, err := extractor.ReadBytes(r)
pdf = extractor.NewPDF(data)
```

### Parsing the XRef table

`ParseXRef` must be called before `GetObject` or `AnalyzeFonts`.

```go
if err := pdf.ParseXRef(); err != nil {
    log.Fatal("xref parse failed:", err)
}
// pdf.XRef is now populated
// pdf.CatalogRef holds the /Root object number from the trailer
```

### Extracting an object

```go
body, err := pdf.GetObject(4) // returns the raw object body string
if err != nil {
    log.Println("object 4 not found:", err)
}
fmt.Println(body) // e.g. "<< /Type /Font /BaseFont /Helvetica ... >>"
```

### Analyzing font embedding

`AnalyzeFonts` walks Catalog → Pages → Resources → Font dictionaries, checking each font's `FontDescriptor` for `/FontFile`, `/FontFile2`, or `/FontFile3` to determine whether the font is embedded.

```go
fonts, err := pdf.AnalyzeFonts()
if err != nil {
    log.Fatal(err)
}

for _, f := range fonts {
    status := "not embedded"
    if f.Embedded {
        status = "embedded"
    }
    fmt.Printf("%-30s %-10s %s\n", f.Name, f.Subtype, status)
}
```

```
AdobeSongStd-Light             Type0      not embedded
Helvetica                      Type1      embedded
ArialMT                        TrueType   embedded
```

### `XRefEntry` and `FontInfo` types

```go
type XRefEntry struct {
    Offset int64  // byte offset of the object in the file
    Gen    int    // generation number
    InUse  bool   // false for free-list entries
}

type FontInfo struct {
    Name     string // /BaseFont value, e.g. "AdobeSongStd-Light"
    Subtype  string // Type0, Type1, TrueType, CIDFontType2, …
    Embedded bool   // true when FontDescriptor contains /FontFile*
}
```

---

## Sample Fixtures

The `sample/` directory contains ready-made test PDFs for both packages. Regenerate them at any time:

```bash
go run ./cmd/gen-samples/
```

### `sample/docs/` — categorized by severity

| File | cmapdetector | scriptdetector | Notes |
|---|---|---|---|
| `clean.pdf` | ✅ clean | ✅ clean | PDF 1.7, Helvetica/WinAnsiEncoding |
| `Surat.pdf` | ✅ clean | ✅ clean | PDF 1.6, Times-Roman/MacRoman |
| `injected.pdf` | ✅ clean | ✅ clean | Inline `Adobe-Identity-UCS` CMap — false-positive guard |
| `malformed.pdf` | ⚠️ MEDIUM | ✅ clean | `AdobeSongStd-Light` / `UniGB-UTF16-H` — triggers iTextPDF crash |
| `form.pdf` | ✅ clean | ⚠️ MEDIUM | `/S /JavaScript` form validation, no CJK CMap |
| `severity_low.pdf` | ✅ clean | 🔵 LOW | `/EmbeddedFile` attachment |
| `severity_medium.pdf` | ⚠️ MEDIUM | ⚠️ MEDIUM | `UniJIS-UTF16-H` CMap + JS open-action |
| `severity_high.pdf` | ✅ clean | 🔶 HIGH | `/Launch` action + `String.fromCharCode` |
| `severity_critical.pdf` | ⚠️ MEDIUM | 🔴 CRITICAL | `eval(unescape())` + `ActiveXObject`/`cmd.exe` + `UniGB-UTF16-H` |
| `fileCorrupt.pdf` | ❌ ErrNotPDF | ❌ ErrNotPDF | High-entropy binary, no `%PDF-` header |
| `fake_pdf.pdf` | ❌ ErrNotPDF | ❌ ErrNotPDF | PNG magic bytes disguised as PDF |

### `sample/` — structural edge cases

| File | cmapdetector | Notes |
|---|---|---|
| `corrupted.pdf` | ✅ clean | Western TrueType only; XRef table deliberately truncated |
| `Kalibrator.pdf` | ✅ clean | Custom `/CMapName /A-B-C` — not in the problematic list |

> **Note:** `severity_critical.pdf` contains `eval(unescape(...))` whose payload decodes to `"function noop(){}"` — a no-op string. The `ActiveXObject` / `cmd.exe` patterns are present as detection targets only; no working exploit code is included.

---

## Architecture

```
cmapdetector/
  detector.go     — ScanBytes, ScanFile, ScanReader, WrapProcess, DetectFile, DetectBytes
  file.go         — ScanMultipartFile, WrapProcessBytes, DetectMultipartFile, DetectMultipartFiles
  cmaps.go        — knownProblematicCMaps (ISO 32000-1 / Adobe CJK predefined CMaps)
  errors.go       — Result, DetectionMethod, sentinel errors
  helpers.go      — MakePDF test helper (exported for external test packages)

scriptdetector/
  detector.go     — ScanBytes, ScanReader, ScanFile, WithPassword
  threats.go      — ThreatType, Severity, Result, Finding, ScanOption
  patterns.go     — signatures (pattern → threat → severity mapping)

extractor/
  pdf.go          — PDF, XRefEntry, FontInfo, ParseXRef, GetObject, AnalyzeFonts

cmd/
  gen-samples/    — generates all test fixtures under sample/
```

The two detection packages share no code and have no cross-dependency. The `extractor` package is also independent. All three can be imported separately.

---

## License

Unlicense (public domain) — see [LICENSE](LICENSE).
