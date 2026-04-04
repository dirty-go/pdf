# README

Package cmapdetect provides utilities for detecting PDF files that reference
CMap resources unavailable in common PDF processing libraries.

It combines two complementary strategies:

1. Raw byte scan (Method 1) — scans the raw PDF bytes for known CMap name
    strings. Fast, zero-dependency, runs before any library touches the file.

2. Runtime recovery (Method 2) — wraps a caller-supplied PDF processing
    function, catching panics and errors whose messages match known CMap error
    signatures. Works with any PDF library.

Typical usage:

```go
result, err := cmapdetect.DetectFile("input.pdf")
if err != nil {
    log.Fatal(err)
}
if result.HasProblematicCMap {
    fmt.Println("Skipping:", result.FoundCMaps)
}
```
