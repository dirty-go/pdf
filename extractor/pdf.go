// Package extractor is a low-level, zero-dependency PDF parser.
//
// It reads raw bytes, parses classic cross-reference tables, and walks the
// object graph to extract dictionary bodies and analyze font embedding.
//
// Limitation: only classic XRef tables (PDF 1.0–1.4 style) are supported.
// XRef streams introduced in PDF 1.5 are not parsed; ParseXRef returns an
// error for files that use them.
package extractor

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Package-level compiled regexes avoid per-call regexp.MustCompile overhead.
var (
	objRegex      = regexp.MustCompile(`(?s)(\d+)\s+(\d+)\s+obj(.*?)endobj`)
	reTrailerRoot = regexp.MustCompile(`/Root\s+(\d+)\s+0\s+R`)
	reTypePage    = regexp.MustCompile(`/Type\s*/Page[\s>/]`)
	reTypeCatalog = regexp.MustCompile(`/Type\s*/Catalog[\s>/]`)
	reAnyRef      = regexp.MustCompile(`(\d+)\s+0\s+R`)
)

// reCache stores key-parameterised regexes compiled at most once per pattern.
var reCache sync.Map

func cachedRegexp(pattern string) *regexp.Regexp {
	if v, ok := reCache.Load(pattern); ok {
		return v.(*regexp.Regexp)
	}
	re := regexp.MustCompile(pattern)
	actual, _ := reCache.LoadOrStore(pattern, re)
	return actual.(*regexp.Regexp)
}

// -----------------------------
// Low-level PDF structures
// -----------------------------

// XRefEntry describes a single record in a PDF cross-reference table.
type XRefEntry struct {
	Offset int64 // byte offset of the object body within the file
	Gen    int   // generation number
	InUse  bool  // false for free-list entries (type "f")
}

// PDF holds the raw bytes and parsed cross-reference index of a PDF document.
// Call ParseXRef to populate XRef before using GetObject or AnalyzeFonts.
type PDF struct {
	Data       []byte
	XRef       map[int]XRefEntry
	CatalogRef int // object number of the document catalog (/Root from trailer)
}

// ReadBytes reads all bytes from r into memory.
// For file-path sources prefer NewPDFFromFile.
func ReadBytes(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// NewPDFFromFile opens pdfFile and returns a PDF ready for parsing.
func NewPDFFromFile(pdfFile string) (*PDF, error) {
	data, err := os.ReadFile(pdfFile)
	if err != nil {
		return nil, fmt.Errorf("pdf: open %q: %w", pdfFile, err)
	}
	return NewPDF(data), nil
}

// NewReader reads all bytes from r and returns them with the byte count.
//
// Deprecated: use ReadBytes.
func NewReader(r io.Reader) ([]byte, int, error) {
	data, err := io.ReadAll(r)
	return data, len(data), err
}

// NewPDF wraps data in a PDF ready for parsing. ParseXRef must be called
// before GetObject or AnalyzeFonts.
func NewPDF(data []byte) *PDF {
	return &PDF{
		Data: data,
		XRef: make(map[int]XRefEntry),
	}
}

// -----------------------------
// XREF parsing (classic only)
// -----------------------------

// ParseXRef locates the startxref marker, parses the classic cross-reference
// table into p.XRef, and populates p.CatalogRef from the trailer /Root entry.
// It returns an error for files that use XRef streams (PDF 1.5+).
func (p *PDF) ParseXRef() error {
	startxrefIdx := bytes.LastIndex(p.Data, []byte("startxref"))
	if startxrefIdx == -1 {
		return errors.New("startxref not found")
	}

	lines := strings.Split(string(p.Data[startxrefIdx:]), "\n")
	if len(lines) < 2 {
		return errors.New("invalid startxref")
	}

	offset, err := strconv.Atoi(strings.TrimSpace(lines[1]))
	if err != nil {
		return err
	}

	if err := p.parseXRefAt(int64(offset)); err != nil {
		return err
	}

	// Populate CatalogRef from the trailer's /Root entry — O(1) vs linear scan.
	trailerIdx := bytes.LastIndex(p.Data, []byte("trailer"))
	if trailerIdx != -1 {
		if m := reTrailerRoot.FindSubmatch(p.Data[trailerIdx:]); m != nil {
			if n, _ := strconv.Atoi(string(m[1])); n > 0 {
				p.CatalogRef = n
			}
		}
	}

	return nil
}

func (p *PDF) parseXRefAt(offset int64) error {
	scanner := bufio.NewScanner(bytes.NewReader(p.Data[offset:]))

	if !scanner.Scan() {
		return errors.New("xref: premature EOF")
	}
	if strings.TrimSpace(scanner.Text()) != "xref" {
		return errors.New("xref keyword not found (xref stream not supported)")
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "trailer" {
			break
		}
		if line == "" {
			continue
		}

		var start, count int
		if n, _ := fmt.Sscanf(line, "%d %d", &start, &count); n != 2 {
			continue
		}

		for i := 0; i < count && scanner.Scan(); i++ {
			parts := strings.Fields(scanner.Text())
			if len(parts) < 3 {
				continue
			}
			off, _ := strconv.ParseInt(parts[0], 10, 64)
			gen, _ := strconv.Atoi(parts[1])
			inUse := parts[2] == "n"
			p.XRef[start+i] = XRefEntry{
				Offset: off,
				Gen:    gen,
				InUse:  inUse,
			}
		}
	}

	return nil
}

// -----------------------------
// Object extraction
// -----------------------------

// GetObject returns the raw body string of the PDF object numbered objNum.
// ParseXRef must be called before GetObject.
func (p *PDF) GetObject(objNum int) (string, error) {
	entry, ok := p.XRef[objNum]
	if !ok || !entry.InUse {
		return "", errors.New("object not found in xref")
	}

	start := entry.Offset
	if start <= 0 || int(start) >= len(p.Data) {
		return "", errors.New("invalid object offset")
	}

	slice := p.Data[start:]
	m := objRegex.FindSubmatch(slice)
	if m == nil {
		return "", errors.New("object parse failed")
	}

	return string(m[3]), nil
}

// -----------------------------
// Dictionary helpers
// -----------------------------

func findRefs(dict string, key string) []int {
	var results []int

	// Simple indirect reference: /Key N 0 R
	re := cachedRegexp(key + `\s+(\d+)\s+0\s+R`)
	for _, m := range re.FindAllStringSubmatch(dict, -1) {
		id, _ := strconv.Atoi(m[1])
		results = append(results, id)
	}

	// Array of indirect references: /Key [N 0 R M 0 R ...]
	// [^\]]* stops at the closing bracket, avoiding runaway matches.
	reArr := cachedRegexp(key + `\s*\[([^\]]*)\]`)
	for _, m := range reArr.FindAllStringSubmatch(dict, -1) {
		items := strings.Fields(m[1])
		for i := 0; i < len(items)-2; i++ {
			if items[i+2] == "R" {
				id, _ := strconv.Atoi(items[i])
				results = append(results, id)
			}
		}
	}

	return results
}

func findSingleRef(dict string, key string) (int, bool) {
	re := cachedRegexp(key + `\s+(\d+)\s+0\s+R`)
	m := re.FindStringSubmatch(dict)
	if m == nil {
		return 0, false
	}
	id, _ := strconv.Atoi(m[1])
	return id, true
}

func findName(dict string, key string) string {
	re := cachedRegexp(key + `\s*/([A-Za-z0-9\-\+]+)`)
	m := re.FindStringSubmatch(dict)
	if m == nil {
		return ""
	}
	return m[1]
}

// -----------------------------
// Font analysis
// -----------------------------

// FontInfo describes a single font referenced by a page in the PDF.
type FontInfo struct {
	Name     string // /BaseFont value, e.g. "AdobeSongStd-Light"
	Subtype  string // /Subtype value, e.g. "Type0", "Type1", "TrueType"
	Embedded bool   // true when FontDescriptor contains a /FontFile, /FontFile2, or /FontFile3 entry
}

// AnalyzeFonts walks Catalog → Pages → Resources → Font dictionaries and
// reports whether each referenced font is embedded in the file.
// ParseXRef must be called before AnalyzeFonts.
func (p *PDF) AnalyzeFonts() ([]FontInfo, error) {
	// 1. Find Catalog (root)
	catalogObj := p.findCatalog()
	if catalogObj == 0 {
		return nil, errors.New("catalog not found")
	}

	catalog, _ := p.GetObject(catalogObj)

	// 2. Pages
	pagesObj, ok := findSingleRef(catalog, "/Pages")
	if !ok {
		return nil, errors.New("pages not found")
	}

	// 3. Walk pages recursively
	pageIDs := p.walkPages(pagesObj)

	var fonts []FontInfo

	for _, pid := range pageIDs {
		pageObj, _ := p.GetObject(pid)

		resObj, ok := findSingleRef(pageObj, "/Resources")
		if !ok {
			continue
		}

		res, _ := p.GetObject(resObj)

		// 4. Font dictionary container — each entry maps a name to a font object.
		fontRefs := findRefs(res, "/Font")
		for _, fRef := range fontRefs {
			fontContainer, err := p.GetObject(fRef)
			if err != nil {
				continue
			}
			// fontContainer is e.g. <</F1 29 0 R /F2 34 0 R ...>>
			// Walk every indirect ref inside it to reach the actual font objects.
			for _, m := range reAnyRef.FindAllStringSubmatch(fontContainer, -1) {
				fontObjNum, _ := strconv.Atoi(m[1])
				fontObj, err := p.GetObject(fontObjNum)
				if err != nil {
					continue
				}

				name := findName(fontObj, "/BaseFont")
				subtype := findName(fontObj, "/Subtype")
				if name == "" && subtype == "" {
					continue // skip non-font refs inside the container
				}

				embedded := false
				fdRef, ok := findSingleRef(fontObj, "/FontDescriptor")
				if ok {
					fd, err := p.GetObject(fdRef)
					if err == nil {
						embedded = strings.Contains(fd, "/FontFile") ||
							strings.Contains(fd, "/FontFile2") ||
							strings.Contains(fd, "/FontFile3")
					}
				}

				fonts = append(fonts, FontInfo{
					Name:     name,
					Subtype:  subtype,
					Embedded: embedded,
				})
			}
		}
	}

	return fonts, nil
}

// -----------------------------
// Helpers: Catalog + Pages
// -----------------------------

// findCatalog returns the object number of the document catalog.
// Uses CatalogRef populated during ParseXRef (O(1)); falls back to a linear
// scan for callers that skipped ParseXRef.
func (p *PDF) findCatalog() int {
	if p.CatalogRef > 0 {
		return p.CatalogRef
	}
	for objNum := range p.XRef {
		obj, err := p.GetObject(objNum)
		if err != nil {
			continue
		}
		if reTypeCatalog.MatchString(obj) {
			return objNum
		}
	}
	return 0
}

// walkPages recursively collects page object IDs from the page tree.
// visited guards against circular /Kids references in malformed PDFs.
func (p *PDF) walkPages(root int) []int {
	visited := make(map[int]bool)
	var result []int

	var walk func(int)
	walk = func(objID int) {
		if visited[objID] {
			return
		}
		visited[objID] = true

		obj, err := p.GetObject(objID)
		if err != nil {
			return
		}

		if reTypePage.MatchString(obj) {
			result = append(result, objID)
			return
		}

		kids := findRefs(obj, "/Kids")
		for _, k := range kids {
			walk(k)
		}
	}

	walk(root)
	return result
}
