# cmapdetector/sample/docs

Per-document test fixtures for the `cmapdetector` package.

Each file in this directory targets a specific detection scenario: a clean
baseline, a CMap-bearing PDF, a false-positive guard, an invalid file, or a
severity-labelled research sample. Together they give `go test ./cmapdetector/`
full coverage of the byte-scanner and runtime-recovery code paths using
realistic PDF structures.

> Files are auto-generated. To regenerate, run from the project root:
> ```bash
> go run ./cmd/gen-samples/
> ```

---

## Directory Tree

```
cmapdetector/sample/docs/
├── README.md              ← this file
│
├── clean.pdf              [CLEAN]    PDF 1.7, Helvetica, WinAnsiEncoding
├── Surat.pdf              [CLEAN]    PDF 1.6, Times-Roman, MacRomanEncoding
├── injected.pdf           [CLEAN]    Adobe-Identity-UCS inline CMap (false-positive guard)
│
├── malformed.pdf          [MEDIUM]   UniGB-UTF16-H — triggers iTextPDF crash
│
├── form.pdf               [MEDIUM]   /S /JavaScript form validation (no CJK CMap)
│
├── severity_low.pdf       [LOW]      /EmbeddedFile attachment only
├── severity_medium.pdf    [MEDIUM]   UniJIS-UTF16-H CMap + JS open-action
├── severity_high.pdf      [HIGH]     /Launch + String.fromCharCode
├── severity_critical.pdf  [CRITICAL] eval/unescape + ActiveXObject + UniGB-UTF16-H
│
├── fileCorrupt.pdf        [INVALID]  No %PDF- header → ErrNotPDF
└── fake_pdf.pdf           [INVALID]  PNG magic bytes → ErrNotPDF
```

---

## File Descriptions

### Baseline / Clean

1. **`clean.pdf`**
   A minimal PDF 1.7 document using Helvetica with `WinAnsiEncoding`.
   Represents a standard Western document with no CJK fonts and no embedded
   scripts. Used as the primary clean baseline.

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = false`, `DetectedBy = None` |
   | `scriptdetector` | `HasThreat = false` |

   **Test:** `TestScanFile_Clean`

2. **`Surat.pdf`**
   A PDF 1.6 document using Times-Roman with `MacRomanEncoding`. Represents
   the era when XRef streams were introduced (PDF 1.5+). The `extractor`
   package cannot parse XRef streams, but `cmapdetector`'s byte scanner
   operates on raw bytes and is unaffected.

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = false`, `DetectedBy = None` |
   | `scriptdetector` | `HasThreat = false` |

   **Test:** `TestScanFile_Surat_Clean`

3. **`injected.pdf`**
   Contains an inline CMap stream that defines `/CMapName /Adobe-Identity-UCS`.
   This name is **not** in `knownProblematicCMaps` — it is a custom identifier,
   not a predefined ISO 32000-1 CJK CMap. The file is intentionally named
   "injected" to make it easy to find as a false-positive guard.

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = false`, `DetectedBy = None` |
   | `scriptdetector` | `HasThreat = false` |

   **Test:** `TestScanFile_Terinjeksi_NoFalsePositive`

---

### CMap Detection

4. **`malformed.pdf`** — `MEDIUM`
   `AdobeSongStd-Light` font with `/Encoding /UniGB-UTF16-H` (Simplified
   Chinese). This is the canonical case that causes iTextPDF to throw:

   ```
   com.itextpdf.io.font.cmap.UniGB-UTF16-H was not found
   ```

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = true`, `FoundCMaps = [UniGB-UTF16-H]`, `DetectedBy = BytesScan` |
   | `scriptdetector` | `HasThreat = false` |

   **Test:** `TestScanFile_MalformedPDF_DetectsCMap`

---

### Script Detection

5. **`form.pdf`** — `MEDIUM`
   A JotForm-style PDF with `/S /JavaScript` and a `/JS` key containing form
   field validation logic. No CJK fonts are present. JavaScript detection is
   outside the scope of `cmapdetector`; this file verifies there is no
   false positive when a PDF carries scripts but no problematic CMap.

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = false`, `DetectedBy = None` |
   | `scriptdetector` | `HasThreat = true`, `Threats = JavaScript`, `MaxSeverity = MEDIUM` |

   **Test:** `TestScanFile_AnomalyForm_Clean`

---

### Severity-Labelled Fixtures

6. **`severity_low.pdf`** — `LOW`
   A PDF with a single `/EmbeddedFile` entry (`readme.txt`). An embedded file
   may be benign (PDF/A attachment) but warrants inspection. No CJK CMap.

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = false`, `DetectedBy = None` |
   | `scriptdetector` | `HasThreat = true`, `Threats = EmbeddedFile`, `MaxSeverity = LOW` |

   **Test:** `TestScanFile_SeverityLow_NoFalsePositive`

7. **`severity_medium.pdf`** — `MEDIUM`
   `KozMinPro-Regular` with `/Encoding /UniJIS-UTF16-H` (Japanese) combined
   with a `/S /JavaScript` open-document action. Both detectors fire.

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = true`, `FoundCMaps = [UniJIS-UTF16-H]`, `DetectedBy = BytesScan` |
   | `scriptdetector` | `HasThreat = true`, `Threats = JavaScript`, `MaxSeverity = MEDIUM` |

   **Test:** `TestScanFile_SeverityMedium_DetectsCMap`

8. **`severity_high.pdf`** — `HIGH`
   A `/Launch` action targeting `calc.exe` combined with a
   `String.fromCharCode` obfuscation pattern. No CJK CMap is present, so
   `cmapdetector` must not flag it.

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = false`, `DetectedBy = None` |
   | `scriptdetector` | `HasThreat = true`, `Threats = LaunchAction+Obfuscation`, `MaxSeverity = HIGH` |

   **Test:** `TestScanFile_SeverityHigh_NoFalsePositive`

9. **`severity_critical.pdf`** — `CRITICAL`
   Combines the most severe indicators across both detectors:

   - `eval(unescape('%66%75...'))`  — decodes to `"function noop(){}"` (no-op, no live shellcode)
   - `new ActiveXObject('WScript.Shell')` + `cmd.exe /c echo test` — detection patterns only
   - `AdobeSongStd-Light` with `/Encoding /UniGB-UTF16-H`

   | Detector | Result |
   |---|---|
   | `cmapdetector` | `HasProblematicCMap = true`, `FoundCMaps = [UniGB-UTF16-H]`, `DetectedBy = BytesScan` |
   | `scriptdetector` | `HasThreat = true`, `Threats = JavaScript+Obfuscation+ShellExecution`, `MaxSeverity = CRITICAL` |

   **Test:** `TestScanFile_SeverityCritical_DetectsCMap`

---

### Invalid / Non-PDF

10. **`fileCorrupt.pdf`**
    256 bytes of deterministic pseudo-random data (`byte = (i×37 + 13) % 256`,
    first byte forced to `0xFF`). Contains no `%PDF-` header. Simulates an
    encrypted or fully corrupted binary that has been given a `.pdf` extension.

    | Detector | Result |
    |---|---|
    | `cmapdetector` | returns `ErrNotPDF` |
    | `scriptdetector` | returns `ErrNotPDF` |

    **Test:** `TestScanFile_FileCorrupt_NotPDF`

11. **`fake_pdf.pdf`**
    Starts with the PNG magic signature (`\x89 P N G \r \n \x1A \n`) followed
    by a minimal IHDR chunk. Simulates a file-type spoofing attempt where an
    image is renamed to `.pdf`.

    | Detector | Result |
    |---|---|
    | `cmapdetector` | returns `ErrNotPDF` |
    | `scriptdetector` | returns `ErrNotPDF` |

    **Test:** `TestScanFile_FakePDF_NotPDF`

---

## Related

- [`../README.md`](../README.md) — `sample/` root: `corrupted.pdf` and `Kalibrator.pdf`
- [`../../../sample/docs/README.md`](../../../sample/docs/README.md) — project-root mirror of this directory
- [`../../../cmd/gen-samples/main.go`](../../../cmd/gen-samples/main.go) — generator source
- [`../../detector_test.go`](../../detector_test.go) — tests that consume these fixtures
