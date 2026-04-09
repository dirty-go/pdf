package extractor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// -----------------------------
// Low-level PDF structures
// -----------------------------

type XRefEntry struct {
	Offset int64
	Gen    int
	InUse  bool
}

type PDF struct {
	Data []byte
	XRef map[int]XRefEntry
}

func NewPDFFromFile(pdfFile string) *PDF {
	f, err := os.Open(pdfFile)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, _, err := NewReader(f)
	if err != nil {
		return nil
	}

	return NewPDF(data)
}

// NewReader initiate new reader from io.reader to byte data
func NewReader(r io.Reader) ([]byte, int, error) {
	var buff bytes.Buffer
	stream := io.TeeReader(r, &buff)
	buf := make([]byte, 1*1024*1024)
	dataSize := 0
	for {
		n, err := stream.Read(buf)
		dataSize += n
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err // propagate instead of panic
		}
	}

	return buff.Bytes(), dataSize, nil
}

func NewPDF(data []byte) *PDF {
	return &PDF{
		Data: data,
		XRef: make(map[int]XRefEntry),
	}
}

// -----------------------------
// XREF parsing (classic only)
// -----------------------------

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

	return p.parseXRefAt(int64(offset))
}

func (p *PDF) parseXRefAt(offset int64) error {
	r := bytes.NewReader(p.Data)
	_, _ = r.Seek(offset, io.SeekStart)

	buf := make([]byte, 4)
	if _, err := r.Read(buf); err != nil {
		return err
	}
	if string(buf) != "xref" {
		return errors.New("xref keyword not found (xref stream not supported)")
	}

	var start, count int
	for {
		_, err := fmt.Fscanf(r, "%d %d\n", &start, &count)
		if err != nil {
			break
		}

		for i := 0; i < count; i++ {
			line := make([]byte, 20)
			if _, err := r.Read(line); err != nil {
				continue
			}

			parts := strings.Fields(string(line))
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

var objRegex = regexp.MustCompile(`(?s)(\d+)\s+(\d+)\s+obj(.*?)endobj`)

func (p *PDF) GetObject(objNum int) (string, error) {
	entry, ok := p.XRef[objNum]
	if !ok || !entry.InUse {
		return "", errors.New("object not found in xref")
	}

	start := entry.Offset
	if start <= 0 || int(start) >= len(p.Data) {
		return "", errors.New("invalid object offset")
	}

	// slice from offset
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
	// matches: /Font 12 0 R OR /Font [12 0 R 15 0 R]
	var results []int

	// simple ref
	re := regexp.MustCompile(key + `\s+(\d+)\s+0\s+R`)
	matches := re.FindAllStringSubmatch(dict, -1)
	for _, m := range matches {
		id, _ := strconv.Atoi(m[1])
		results = append(results, id)
	}

	// array refs
	reArr := regexp.MustCompile(key + `\s*$begin:math:display$\(\.\*\?\)$end:math:display$`)
	arrMatches := reArr.FindAllStringSubmatch(dict, -1)
	for _, m := range arrMatches {
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
	re := regexp.MustCompile(key + `\s+(\d+)\s+0\s+R`)
	m := re.FindStringSubmatch(dict)
	if m == nil {
		return 0, false
	}
	id, _ := strconv.Atoi(m[1])
	return id, true
}

func findName(dict string, key string) string {
	re := regexp.MustCompile(key + `\s*/([A-Za-z0-9\-\+]+)`)
	m := re.FindStringSubmatch(dict)
	if m == nil {
		return ""
	}
	return m[1]
}

// -----------------------------
// Font analysis
// -----------------------------

type FontInfo struct {
	Name     string
	Subtype  string
	Embedded bool
}

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

		// 4. Font dictionary
		fontRefs := findRefs(res, "/Font")
		for _, fRef := range fontRefs {
			fontDict, err := p.GetObject(fRef)
			if err != nil {
				continue
			}

			name := findName(fontDict, "/BaseFont")
			subtype := findName(fontDict, "/Subtype")

			embedded := false

			// check FontDescriptor
			fdRef, ok := findSingleRef(fontDict, "/FontDescriptor")
			if ok {
				fd, err := p.GetObject(fdRef)
				if err == nil {
					if strings.Contains(fd, "/FontFile") ||
						strings.Contains(fd, "/FontFile2") ||
						strings.Contains(fd, "/FontFile3") {
						embedded = true
					}
				}
			}

			fonts = append(fonts, FontInfo{
				Name:     name,
				Subtype:  subtype,
				Embedded: embedded,
			})
		}
	}

	return fonts, nil
}

// -----------------------------
// Helpers: Catalog + Pages
// -----------------------------

func (p *PDF) findCatalog() int {
	for objNum := range p.XRef {
		obj, err := p.GetObject(objNum)
		if err != nil {
			continue
		}
		if strings.Contains(obj, "/Type /Catalog") {
			return objNum
		}
	}
	return 0
}

func (p *PDF) walkPages(root int) []int {
	var result []int

	var walk func(int)
	walk = func(objID int) {
		obj, err := p.GetObject(objID)
		if err != nil {
			return
		}

		if strings.Contains(obj, "/Type /Page") {
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
