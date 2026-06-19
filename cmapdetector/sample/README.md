# cmapdetector/sample

Test fixture root for the `cmapdetector` package.

This directory and its subdirectory (`docs/`) contain PDF files that are
loaded directly by `go test ./cmapdetector/` during the **real-world sample**
test section. The fixtures cover every code path in the byte scanner and the
runtime-recovery wrapper that synthetic in-memory PDFs cannot exercise.

---

## How Fixtures Are Used

The test file (`detector_test.go`) resolves all paths relative to the package
directory. When `go test` runs inside `cmapdetector/`, the working directory is
`cmapdetector/`, so the fixtures are found at:

```
cmapdetector/sample/           ← sampleDir  ("./sample")
cmapdetector/sample/docs/      ← sampleDirDocs  ("./sample/docs")
```

Tests that reference a missing file are **skipped automatically** — they never
fail on a clean checkout before the generator is run.

### Regenerate

All fixtures are produced by a single command run from the **project root**:

```bash
go run ./cmd/gen-samples/
```

The generator writes identical copies to both `sample/` (project root) and
`cmapdetector/sample/` so that the package tests always find their fixtures
without requiring a symlink or environment variable.

---

## Directory Tree

```
cmapdetector/sample/
├── README.md            ← this file
├── corrupted.pdf        — XRef-level corruption, Western fonts only
├── Kalibrator.pdf       — custom /CMapName /A-B-C (not in problematic list)
└── docs/
    ├── README.md        ← per-file descriptions
    ├── clean.pdf
    ├── Surat.pdf
    ├── injected.pdf
    ├── malformed.pdf
    ├── form.pdf
    ├── severity_low.pdf
    ├── severity_medium.pdf
    ├── severity_high.pdf
    ├── severity_critical.pdf
    ├── fileCorrupt.pdf
    └── fake_pdf.pdf
```

---

## Files in This Directory

### `corrupted.pdf`

A PDF 1.4 document with an `Arial` (TrueType) font using `WinAnsiEncoding`.
The XRef table is deliberately truncated to simulate structural corruption at
the cross-reference level.

**Why it exists:** verifies that XRef-level corruption, which has nothing to do
with CMap resources, does not cause a false positive.

**Expected result:**

```
HasProblematicCMap = false
DetectedBy         = None
```

**Test:** `TestScanFile_CorruptedSample`

---

### `Kalibrator.pdf`

A PDF 1.4 document that defines an inline CMap stream with
`/CMapName /A-B-C`. The name `/A-B-C` is a custom identifier that is **not**
present in `knownProblematicCMaps`.

**Why it exists:** verifies that only predefined CJK CMap names from the
ISO 32000-1 / Adobe list are flagged, and that arbitrary custom `/CMapName`
entries inside embedded CMap streams are ignored.

**Expected result:**

```
HasProblematicCMap = false
DetectedBy         = None
```

**Test:** `TestScanFile_KalibratorSample`

---

## Related

- [`docs/README.md`](docs/README.md) — descriptions for all fixtures in `docs/`
- [`../../sample/docs/README.md`](../../sample/docs/README.md) — project-root mirror
- [`../../cmd/gen-samples/main.go`](../../cmd/gen-samples/main.go) — generator source
- [`../detector_test.go`](../detector_test.go) — tests that load these fixtures
