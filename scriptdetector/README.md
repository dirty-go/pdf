# scriptdetector

Package `scriptdetector` detects embedded scripts, launch actions, and other
malicious constructs in PDF files via raw byte pattern matching.

All scan functions operate directly on the byte slice — no PDF parser, no
external C library, no cgo. PDF dictionary keys such as `/S /JavaScript` are
never encrypted by the standard security handler, so structural threats are
always visible without decryption.

---

## Threat Categories

| Constant | Severity | Triggers |
|---|---|---|
| `ThreatJavaScript` | MEDIUM | `/S /JavaScript`, `/JS `, `/JS(`, `/JS\n`, `/JS\r` |
| `ThreatLaunchAction` | HIGH | `/Launch` |
| `ThreatEmbeddedFile` | LOW | `/EmbeddedFile` |
| `ThreatXFAForm` | MEDIUM | `/XFA` |
| `ThreatObfuscation` | HIGH / CRITICAL | `String.fromCharCode`, `unescape(`, `eval(unescape(` |
| `ThreatShellExecution` | HIGH / CRITICAL | `ActiveXObject`, `WScript.Shell`, `powershell`, `cmd.exe`, `/bin/sh`, `/bin/bash` |

Multiple patterns can match the same `ThreatType` in one file. When they do,
only the highest-severity `Finding` is kept for that type. Findings are
returned sorted by severity descending.

---

## Installation

```bash
go get github.com/dirty-go/pdf/scriptdetector
```

No external dependencies. Requires Go 1.23+.

---

## Quick Start

```go
import scriptdetect "github.com/dirty-go/pdf/scriptdetector"

_, result, err := scriptdetect.ScanFile("document.pdf")
if err != nil {
    log.Fatal(err)
}
if result.HasThreat {
    fmt.Println("max severity:", result.MaxSeverity())
    for _, f := range result.Findings {
        fmt.Printf("[%s] %s\n  snippet: %s\n", f.Severity, f.Pattern, f.Snippet)
    }
}
```

---

## API Reference

### `ScanBytes`

Scans a byte slice directly. Never returns an error — use when bytes are
already loaded and you do not need encryption gating.

```go
data, _ := os.ReadFile("document.pdf")
result := scriptdetect.ScanBytes(data)

fmt.Println(result.HasThreat)    // true / false
fmt.Println(result.Threats)      // e.g. "JavaScript+Obfuscation"
fmt.Println(result.IsEncrypted)  // true when /Encrypt dict present
```

### `ScanFile`

Opens the file, validates the `%PDF-` header, checks for encryption, and
runs the byte scanner.

```go
// Without password — returns ErrEncryptedPDF for encrypted files
_, result, err := scriptdetect.ScanFile("document.pdf")

// With password — proceeds with a partial scan; IsEncrypted is still set
_, result, err = scriptdetect.ScanFile("protected.pdf",
    scriptdetect.WithPassword("secret"))

switch {
case errors.Is(err, scriptdetect.ErrNotPDF):
    log.Println("not a PDF")
case errors.Is(err, scriptdetect.ErrEncryptedPDF):
    log.Println("encrypted — provide a password")
}
```

### `ScanReader`

Reads all bytes from an `io.Reader` and scans them. Returns the raw bytes
alongside the result for reuse.

```go
rawBytes, result, err := scriptdetect.ScanReader(r)
rawBytes, result, err  = scriptdetect.ScanReader(r, scriptdetect.WithPassword("pw"))
```

### `WithPassword`

`ScanOption` that allows scanning encrypted PDFs. Without it, `ScanFile` and
`ScanReader` return `ErrEncryptedPDF` when `/Encrypt` is detected.

When a password is supplied the scan proceeds but covers only structurally
visible keys — string and stream content is not decrypted. `Result.IsEncrypted`
remains `true` to signal the partial coverage.

```go
_, result, err := scriptdetect.ScanFile("protected.pdf",
    scriptdetect.WithPassword("hunter2"))
fmt.Println(result.IsEncrypted) // true
```

---

## Types

### `Result`

```go
type Result struct {
    FilePath    string
    HasThreat   bool
    Threats     ThreatType // bitmask of detected categories
    IsEncrypted bool
    Findings    []Finding  // one entry per ThreatType, sorted by Severity desc
}
```

`Result` implements the `error` interface:

```go
func (r *Result) Error() string
func (r *Result) MaxSeverity() Severity
```

```go
// Severity gate example
if result.MaxSeverity() >= scriptdetect.SeverityHigh {
    http.Error(w, "file rejected", http.StatusUnprocessableEntity)
    return
}
```

### `Finding`

```go
type Finding struct {
    Threat   ThreatType // which category matched
    Severity Severity   // severity of this specific pattern
    Pattern  string     // human-readable description of what matched
    Snippet  string     // ±40 bytes of printable context around the match
}
```

### `ThreatType`

A bitmask — multiple threats can be set simultaneously.

```go
// Check individual threats
if result.Threats&scriptdetect.ThreatJavaScript != 0 { /* JS present */ }
if result.Threats&scriptdetect.ThreatShellExecution != 0 { /* shell cmd */ }

// String representation
result.Threats.String() // e.g. "JavaScript+Obfuscation+ShellExecution"
```

### `Severity`

```go
scriptdetect.SeverityLow      // 1 — informational; may be legitimate
scriptdetect.SeverityMedium   // 2 — suspicious; manual review recommended
scriptdetect.SeverityHigh     // 3 — strong indicator of malicious intent
scriptdetect.SeverityCritical // 4 — block immediately

severity.String() // "LOW" | "MEDIUM" | "HIGH" | "CRITICAL"
```

---

## Errors

| Sentinel | Returned when |
|---|---|
| `ErrNotPDF` | File does not start with `%PDF-` |
| `ErrEncryptedPDF` | File is encrypted and no password was provided |
| `ErrWrongPassword` | Password provided but does not authenticate |

```go
switch {
case errors.Is(err, scriptdetect.ErrNotPDF):
    // not a PDF file
case errors.Is(err, scriptdetect.ErrEncryptedPDF):
    // prompt user for password
case errors.Is(err, scriptdetect.ErrWrongPassword):
    // wrong password
}
```

---

## Detection Patterns

Patterns are defined in `patterns.go`. Each entry maps a byte sequence to a
`ThreatType` and `Severity`. Case-insensitive matching is opt-in per pattern
(e.g. `powershell`, `ActiveXObject`).

### JavaScript

| Pattern | Severity |
|---|---|
| `/S /JavaScript` | MEDIUM |
| `/S/JavaScript` | MEDIUM |
| `/JS ` | MEDIUM |
| `/JS(` | MEDIUM |

### Launch Action

| Pattern | Severity |
|---|---|
| `/Launch` | HIGH |

### Embedded File

| Pattern | Severity |
|---|---|
| `/EmbeddedFile` | LOW |

### XFA Form

| Pattern | Severity |
|---|---|
| `/XFA` | MEDIUM |

### Obfuscation

| Pattern | Severity |
|---|---|
| `eval(unescape(` | CRITICAL |
| `String.fromCharCode` | HIGH |
| `unescape(` | HIGH |

### Shell Execution *(case-insensitive)*

| Pattern | Severity |
|---|---|
| `ActiveXObject` | CRITICAL |
| `WScript.Shell` | CRITICAL |
| `powershell` | CRITICAL |
| `cmd.exe` | CRITICAL |
| `/bin/sh` | HIGH |
| `/bin/bash` | HIGH |

---

## Running Tests

```bash
# All tests
go test ./scriptdetector/

# Verbose
go test -v ./scriptdetector/

# Specific group
go test -v -run "TestScanBytes_" ./scriptdetector/
go test -v -run "TestScanFile_" ./scriptdetector/
```

---

## Related Packages

| Package | Purpose |
|---|---|
| [`cmapdetector`](../cmapdetector/) | Detects missing CJK CMap resources that crash iTextPDF / pdfbox |
| [`extractor`](../extractor/) | Low-level XRef parser and font-embedding analyser |
| [`cmd/gen-samples`](../cmd/gen-samples/) | Generates all test fixture PDFs |
