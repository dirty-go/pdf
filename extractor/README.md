# extractor

Package `extractor` is a low-level, zero-dependency PDF parser.

It reads raw bytes, parses classic cross-reference tables, extracts object
bodies, and walks the object graph to report font embedding. No external C
library, no cgo, no PDF renderer required.

> **Limitation:** only classic XRef tables (PDF 1.0–1.4 style) are supported.
> XRef streams introduced in PDF 1.5 are not parsed — `ParseXRef` returns an
> error for files that use them.

---

## Installation

```bash
go get github.com/dirty-go/pdf/extractor
```

No external dependencies. Requires Go 1.23+.

---

## Quick Start

```go
import "github.com/dirty-go/pdf/extractor"

pdf, err := extractor.NewPDFFromFile("document.pdf")
if err != nil {
    log.Fatal(err)
}
if err := pdf.ParseXRef(); err != nil {
    log.Fatal(err)
}

fonts, err := pdf.AnalyzeFonts()
if err != nil {
    log.Fatal(err)
}
for _, f := range fonts {
    fmt.Printf("%-30s %-10s embedded=%v\n", f.Name, f.Subtype, f.Embedded)
}
```

---

## API Reference

### Loading a PDF

#### `NewPDFFromFile`

Opens a file by path and returns a `*PDF` ready for parsing.

```go
pdf, err := extractor.NewPDFFromFile("document.pdf")
```

#### `NewPDF`

Wraps an already-loaded byte slice. Use when bytes come from a network
request, a multipart upload, or another in-memory source.

```go
data, _ := os.ReadFile("document.pdf")
pdf := extractor.NewPDF(data)
```

#### `ReadBytes`

Reads all bytes from an `io.Reader` into memory.

```go
data, err := extractor.ReadBytes(r)
pdf := extractor.NewPDF(data)
```

#### `NewReader` *(deprecated)*

```go
// Deprecated: use ReadBytes instead.
data, n, err := extractor.NewReader(r)
```

---

### Parsing the XRef Table

`ParseXRef` must be called before `GetObject` or `AnalyzeFonts`.

```go
if err := pdf.ParseXRef(); err != nil {
    // ErrNotPDF, truncated file, or XRef stream (PDF 1.5+)
    log.Fatal(err)
}

// After ParseXRef:
fmt.Println(pdf.CatalogRef)   // object number of /Root (document catalog)
fmt.Println(len(pdf.XRef))    // number of objects in the cross-reference table
```

**What it does:**

1. Locates the `startxref` marker at the end of the file.
2. Seeks to the XRef table offset.
3. Parses every `N 0 obj` / `N 0 f` entry into `pdf.XRef`.
4. Reads the trailer dictionary to populate `pdf.CatalogRef` from `/Root`.

---

### Extracting Objects

#### `GetObject`

Returns the raw body string of any object by its object number. `ParseXRef`
must be called first.

```go
body, err := pdf.GetObject(4)
if err != nil {
    log.Println("object 4 not found:", err)
}
fmt.Println(body)
// << /Type /Font /Subtype /Type0 /BaseFont /AdobeSongStd-Light ... >>
```

---

### Font Analysis

#### `AnalyzeFonts`

Walks `Catalog → Pages → Resources → Font` dictionaries and checks each
font's `FontDescriptor` for `/FontFile`, `/FontFile2`, or `/FontFile3` keys
to determine whether the font data is embedded in the file.

`ParseXRef` must be called before `AnalyzeFonts`.

```go
fonts, err := pdf.AnalyzeFonts()
if err != nil {
    log.Fatal(err)
}

for _, f := range fonts {
    status := "NOT embedded"
    if f.Embedded {
        status = "embedded"
    }
    fmt.Printf("%-30s %-12s %s\n", f.Name, f.Subtype, status)
}
```

Example output:

```
AdobeSongStd-Light             Type0        NOT embedded
Helvetica                      Type1        embedded
ArialMT                        TrueType     embedded
```

A font that is not embedded relies on the rendering environment to supply the
font data at display time. CJK fonts (Type0 / CIDFont) are almost never
embedded, which is why PDF libraries fail to resolve their CMaps at runtime.

---

## Types

### `PDF`

```go
type PDF struct {
    Data       []byte
    XRef       map[int]XRefEntry
    CatalogRef int // object number of the document catalog (/Root)
}
```

### `XRefEntry`

Describes a single record in the cross-reference table.

```go
type XRefEntry struct {
    Offset int64 // byte offset of the object body in the file
    Gen    int   // generation number
    InUse  bool  // false for free-list entries (type "f")
}
```

### `FontInfo`

Returned by `AnalyzeFonts` for each font referenced by a page.

```go
type FontInfo struct {
    Name     string // /BaseFont value, e.g. "AdobeSongStd-Light"
    Subtype  string // /Subtype value, e.g. "Type0", "Type1", "TrueType"
    Embedded bool   // true when FontDescriptor contains /FontFile*
}
```

---

## Typical Workflow

```
NewPDFFromFile / NewPDF / ReadBytes
        │
        ▼
    ParseXRef          ← populates XRef + CatalogRef
        │
        ├──► GetObject(n)        ← extract any object body by number
        │
        └──► AnalyzeFonts()     ← walk page tree, report font embedding
```

---

## Running Tests

The `extractor` package has no test files of its own. Its behaviour is
exercised indirectly through `cmapdetector` tests that load real PDF fixtures.

```bash
go build ./extractor/
go vet ./extractor/
```

---

## Related Packages

| Package | Purpose |
|---|---|
| [`cmapdetector`](../cmapdetector/) | Detects missing CJK CMap resources using raw byte scan + runtime recovery |
| [`scriptdetector`](../scriptdetector/) | Detects embedded JavaScript, `/Launch`, XFA, and shell-execution patterns |
| [`cmd/gen-samples`](../cmd/gen-samples/) | Generates all test fixture PDFs |
