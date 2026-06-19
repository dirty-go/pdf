# cmapdetector

Package `cmapdetector` detects PDF files that reference CMap resources
unavailable in common PDF processing libraries (iTextPDF, pdfbox, pdfcpu).

The canonical failure this package exists to prevent:

```
com.itextpdf.io.font.cmap.UniGB-UTF16-H was not found
Could not find a CMap: missing CMap resource for UniJIS-UTF16-H
```

These errors occur when a PDF uses CJK (Chinese, Japanese, Korean) font
encodings that require predefined CMap files from the ISO 32000-1 or Adobe
CMap resource packs — files that are not bundled with most PDF libraries by
default.

---

## Detection Strategies

Two complementary methods are combined. Either can fire independently; both
can fire on the same file.

| | Method 1 — Byte Scan | Method 2 — Runtime Recovery |
|---|---|---|
| **How** | Searches raw PDF bytes for known CMap name strings | Wraps your PDF library call, catching panics and CMap errors |
| **Speed** | Fast — single pass over the byte slice | Depends on the wrapped library |
| **Dependency** | None — no PDF library required | Requires a caller-supplied `ProcessFunc` |
| **Entry points** | `ScanBytes`, `ScanFile`, `ScanReader`, `DetectBytes` | `WrapProcess`, `WrapProcessBytes` |
| **Bitmask flag** | `MethodBytesScan` | `MethodRuntime` |

---

## Installation

```bash
go get github.com/dirty-go/pdf/cmapdetector
```

No external dependencies. Requires Go 1.23+.

---

## Quick Start

```go
import cmapdetect "github.com/dirty-go/pdf/cmapdetector"

// Method 1 only — no PDF library needed
result, err := cmapdetect.DetectFile("invoice.pdf", nil)
if err != nil {
    log.Fatal(err)
}
if result.HasProblematicCMap {
    fmt.Println("unsupported CMaps:", result.FoundCMaps)
}
```

---

## API Reference

### Method 1 — Raw Byte Scan

#### `ScanBytes`

Scans an already-loaded byte slice. The fastest entry point — no file I/O.

```go
data, _ := os.ReadFile("invoice.pdf")
result := cmapdetect.ScanBytes(data)

fmt.Println(result.HasProblematicCMap) // true / false
fmt.Println(result.FoundCMaps)         // e.g. ["UniGB-UTF16-H", "UniGB-UTF8-H"]
fmt.Println(result.IsEncrypted)        // true when /Encrypt dict present
```

#### `ScanFile`

Opens the file, validates the `%PDF-` header, and runs the byte scanner.
Returns the raw bytes alongside the result so they can be forwarded to
Method 2 without re-reading the file.

```go
rawBytes, result, err := cmapdetect.ScanFile("invoice.pdf")
if errors.Is(err, cmapdetect.ErrNotPDF) {
    log.Println("not a PDF")
}
```

#### `ScanReader`

Reads all bytes from an `io.Reader` and scans them. Returns the raw bytes
for potential reuse.

```go
f, _ := os.Open("invoice.pdf")
defer f.Close()

rawBytes, result, err := cmapdetect.ScanReader(f)
```

#### `DetectBytes`

Method 1 only on a pre-loaded byte slice, with a file path attached to the
result for reference.

```go
result := cmapdetect.DetectBytes("invoice.pdf", data)
fmt.Println(result.FilePath) // "invoice.pdf"
```

---

### Method 2 — Runtime Recovery

#### `WrapProcess`

Wraps a file-based PDF processing function. Catches panics and errors whose
messages match known CMap error signatures; re-panics on unrelated panics;
passes through unrelated errors unchanged.

```go
// Typically called after ScanFile so both methods share one result.
_, result, err := cmapdetect.ScanFile("invoice.pdf")

result, err = cmapdetect.WrapProcess("invoice.pdf", func(path string) error {
    return myPDFLib.Open(path)
}, result)

if err != nil {
    var r *cmapdetect.Result
    if errors.As(err, &r) {
        // CMap detected — not a hard failure
        fmt.Println("detected by:", r.DetectedBy) // "BytesScan+Runtime"
    } else {
        log.Fatal("unrelated error:", err)
    }
}
```

#### `WrapProcessBytes`

In-memory equivalent of `WrapProcess`. Use when bytes are already loaded
(e.g. from a multipart upload) and writing to a temp file is undesirable.

```go
result, err = cmapdetect.WrapProcessBytes(
    "invoice.pdf", data,
    func(name string, b []byte) error {
        return myPDFLib.ParseBytes(name, b)
    },
    result, // merge with Method 1 result
)
```

---

### Combined Entry Points

#### `DetectFile`

Runs Method 1 and, when `fn` is non-nil, Method 2 against a file path.
The canonical single-call entry point for file-based workflows.

```go
// Method 1 only
result, err := cmapdetect.DetectFile("invoice.pdf", nil)

// Methods 1 + 2
result, err = cmapdetect.DetectFile("invoice.pdf", myPDFLib.Open)
if err != nil {
    log.Fatal(err)
}
if result.HasProblematicCMap {
    fmt.Printf("found via [%s]: %v\n", result.DetectedBy, result.FoundCMaps)
}
```

---

### HTTP Multipart Uploads

#### `ScanMultipartFile`

Validates and byte-scans a single `*multipart.FileHeader`. Returns raw bytes
for Method 2 reuse.

```go
func uploadHandler(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(32 << 20)
    fh := r.MultipartForm.File["pdf"][0]

    data, result, err := cmapdetect.ScanMultipartFile(fh)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    _ = data // forward to WrapProcessBytes if needed
    json.NewEncoder(w).Encode(result)
}
```

#### `DetectMultipartFile`

Combines Method 1 and, when `fn` is non-nil, Method 2 against a single
upload. The primary upload entry point.

```go
// Method 1 only
result, err := cmapdetect.DetectMultipartFile(fh, nil)

// Methods 1 + 2
result, err = cmapdetect.DetectMultipartFile(fh, func(name string, data []byte) error {
    return myPDFLib.ParseBytes(name, data)
})
```

#### `DetectMultipartFiles`

Runs `DetectMultipartFile` over a slice of file headers. Processing continues
even when individual files fail — errors are captured per-file in
`BatchResult.Errors`.

```go
fhs := r.MultipartForm.File["pdfs"]
batch := cmapdetect.DetectMultipartFiles(fhs, nil)

fmt.Printf("scanned: %d  flagged: %d  errors: %d\n",
    batch.TotalCount, batch.FlaggedCount, batch.ErrorCount)

for _, result := range batch.Results {
    if result.HasProblematicCMap {
        fmt.Println(result.FilePath, result.FoundCMaps)
    }
}
for _, fe := range batch.Errors {
    fmt.Fprintf(os.Stderr, "error: %s: %v\n", fe.Filename, fe.Err)
}
```

---

## Types

### `Result`

```go
type Result struct {
    FilePath           string          // path or upload filename
    FileSize           int64           // bytes (set for multipart uploads)
    IsEncrypted        bool            // /Encrypt dict present
    HasProblematicCMap bool            // true when any method fires
    FoundCMaps         []string        // CMap names found by Method 1
    DetectedBy         DetectionMethod // MethodBytesScan | MethodRuntime bitmask
    RuntimeErr         error           // panic/error captured by Method 2
}
```

`Result` implements the `error` interface. When `WrapProcess` or
`WrapProcessBytes` detects a CMap problem it returns the result as the error
value. Distinguish it from hard failures with `errors.As`:

```go
var r *cmapdetect.Result
if errors.As(err, &r) {
    // CMap detected — r is the full result
}

// Unwrap chain: errors.Is also works
errors.Is(err, cmapdetect.ErrMissingCMap) // true when HasProblematicCMap
```

### `DetectionMethod`

A bitmask that records which strategies fired.

```go
result.DetectedBy == cmapdetect.MethodNone                              // nothing fired
result.DetectedBy == cmapdetect.MethodBytesScan                        // Method 1 only
result.DetectedBy == cmapdetect.MethodRuntime                          // Method 2 only
result.DetectedBy == cmapdetect.MethodBytesScan | cmapdetect.MethodRuntime // both

result.DetectedBy.String() // "None" | "BytesScan" | "Runtime" | "BytesScan+Runtime"
```

### `BatchResult`

Returned by `DetectMultipartFiles`.

```go
type BatchResult struct {
    Results      []*Result   // one entry per successfully scanned file
    Errors       []FileError // one entry per file that could not be scanned
    TotalCount   int         // number of successfully scanned files
    FlaggedCount int         // number with HasProblematicCMap = true
    ErrorCount   int         // number that returned a hard error
}
```

---

## Errors

| Sentinel | Returned when |
|---|---|
| `ErrMissingCMap` | A CMap resource could not be resolved; also exposed via `Result.Unwrap` |
| `ErrNotPDF` | File does not start with `%PDF-` |
| `ErrPDFPanic` | The wrapped PDF library panicked (wrapped inside `Result.RuntimeErr`) |
| `ErrEncryptedPDF` | File carries an `/Encrypt` dictionary; provided for caller signalling |

```go
switch {
case errors.Is(err, cmapdetect.ErrNotPDF):
    // reject the file
case errors.Is(err, cmapdetect.ErrMissingCMap):
    // CMap issue — inspect the Result
case errors.Is(err, cmapdetect.ErrEncryptedPDF):
    // inform the user
}
```

---

## Supported CMap Names

The byte scanner matches against `knownProblematicCMaps` in `cmaps.go`,
which lists the predefined CJK CMaps from ISO 32000-1 Annex D and the Adobe
CMap resources repository.

| Script | Example names |
|---|---|
| Simplified Chinese (GB) | `UniGB-UTF8-H/V`, `UniGB-UTF16-H/V`, `GBK-EUC-H/V`, `GBK2K-H/V` |
| Traditional Chinese (CNS) | `UniCNS-UTF8-H/V`, `UniCNS-UTF16-H/V`, `ETen-B5-H/V`, `B5pc-H/V` |
| Japanese (JIS) | `UniJIS-UTF8-H/V`, `UniJIS-UTF16-H/V`, `90ms-RKSJ-H/V`, `EUC-H/V` |
| Korean (KS) | `UniKS-UTF8-H/V`, `UniKS-UTF16-H/V`, `KSCms-UHC-H/V`, `KSCpc-EUC-H` |

`Identity-H` and `Identity-V` are intentionally excluded — they are ISO
32000-1 mandatory CMaps supported by all compliant PDF libraries and their
inclusion would cause false positives on most modern PDFs.

---

## Test Helpers

The package exports three helpers for use in external test packages
(`package xxx_test`):

```go
// MakePDF returns a minimal PDF byte slice referencing cmapName.
data := cmapdetect.MakePDF("UniGB-UTF16-H")

// MakePDFFile writes the above to a temp file and returns the path.
path := cmapdetect.MakePDFFile(t, "UniGB-UTF16-H")

// MakeCleanPDFFile writes a PDF with WinAnsiEncoding (no problematic CMap).
path := cmapdetect.MakeCleanPDFFile(t)
```

---

## Running Tests

```bash
# All tests (synthetic + fixture-based)
go test ./cmapdetector/

# Verbose output
go test -v ./cmapdetector/

# Fixture-based tests only
go test -v -run "TestScanFile_" ./cmapdetector/

# Specific test
go test -run TestScanFile_MalformedPDF_DetectsCMap ./cmapdetector/
```

Fixture tests are skipped automatically when the sample files are absent.
To generate them:

```bash
go run ./cmd/gen-samples/
```

---

## Sample Fixtures

```
cmapdetector/sample/
├── corrupted.pdf        — truncated XRef, Western fonts — expect no detection
├── Kalibrator.pdf       — custom /CMapName /A-B-C — expect no detection
└── docs/
    ├── clean.pdf           [CLEAN]
    ├── Surat.pdf           [CLEAN]     PDF 1.6
    ├── injected.pdf        [CLEAN]     false-positive guard
    ├── malformed.pdf       [MEDIUM]    UniGB-UTF16-H
    ├── form.pdf            [MEDIUM]    JS only, no CMap
    ├── severity_low.pdf    [LOW]       /EmbeddedFile
    ├── severity_medium.pdf [MEDIUM]    UniJIS-UTF16-H + JS
    ├── severity_high.pdf   [HIGH]      /Launch + obfuscation
    ├── severity_critical.pdf [CRITICAL] eval/unescape + shell + UniGB-UTF16-H
    ├── fileCorrupt.pdf     [INVALID]   ErrNotPDF
    └── fake_pdf.pdf        [INVALID]   ErrNotPDF
```

See [`sample/README.md`](sample/README.md) and
[`sample/docs/README.md`](sample/docs/README.md) for per-file details.

---

## Related Packages

| Package | Purpose |
|---|---|
| [`scriptdetector`](../scriptdetector/) | Detects embedded JavaScript, `/Launch`, XFA forms, and shell-execution patterns |
| [`extractor`](../extractor/) | Low-level XRef parser and font-embedding analyser |
| [`cmd/gen-samples`](../cmd/gen-samples/) | Generates all test fixtures |
