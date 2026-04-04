package cmapdetector

// knownProblematicCMaps is the list of CMap resource names known to be missing
// in many PDF libraries (e.g. iTextPDF, pdfbox) unless CJK font packs are installed.
//
// Sources:
//   - ISO 32000-1 (PDF 1.7 spec), Annex D: predefined CMaps
//   - Adobe CMap resources: https://github.com/adobe-type-tools/cmap-resources
var knownProblematicCMaps = []string{
	// --- Simplified Chinese (GB) ---
	"UniGB-UTF8-H",
	"UniGB-UTF8-V",
	"UniGB-UTF16-H",
	"UniGB-UTF16-V",
	"UniGB-UCS2-H",
	"UniGB-UCS2-V",
	"GB-EUC-H",
	"GB-EUC-V",
	"GBpc-EUC-H",
	"GBpc-EUC-V",
	"GBK-EUC-H",
	"GBK-EUC-V",
	"GBKp-EUC-H",
	"GBKp-EUC-V",
	"GBK2K-H",
	"GBK2K-V",
	"UniGB-UTF32-H",
	"UniGB-UTF32-V",

	// --- Traditional Chinese (CNS) ---
	"UniCNS-UTF8-H",
	"UniCNS-UTF8-V",
	"UniCNS-UTF16-H",
	"UniCNS-UTF16-V",
	"UniCNS-UCS2-H",
	"UniCNS-UCS2-V",
	"UniCNS-UTF32-H",
	"UniCNS-UTF32-V",
	"B5pc-H",
	"B5pc-V",
	"HKscs-B5-H",
	"HKscs-B5-V",
	"ETen-B5-H",
	"ETen-B5-V",
	"ETenms-B5-H",
	"ETenms-B5-V",
	"CNS-EUC-H",
	"CNS-EUC-V",

	// --- Japanese (JIS) ---
	"UniJIS-UTF8-H",
	"UniJIS-UTF8-V",
	"UniJIS-UTF16-H",
	"UniJIS-UTF16-V",
	"UniJIS-UCS2-H",
	"UniJIS-UCS2-V",
	"UniJIS-UCS2-HW-H",
	"UniJIS-UCS2-HW-V",
	"UniJIS2004-UTF8-H",
	"UniJIS2004-UTF8-V",
	"UniJIS2004-UTF16-H",
	"UniJIS2004-UTF16-V",
	"UniJIS2004-UTF32-H",
	"UniJIS2004-UTF32-V",
	"UniJISX02132004-UTF32-H",
	"UniJISX02132004-UTF32-V",
	"83pv-RKSJ-H",
	"90ms-RKSJ-H",
	"90ms-RKSJ-V",
	"90msp-RKSJ-H",
	"90msp-RKSJ-V",
	"90pv-RKSJ-H",
	"EUC-H",
	"EUC-V",

	// --- Korean (KS) ---
	"UniKS-UTF8-H",
	"UniKS-UTF8-V",
	"UniKS-UTF16-H",
	"UniKS-UTF16-V",
	"UniKS-UCS2-H",
	"UniKS-UCS2-V",
	"UniKS-UTF32-H",
	"UniKS-UTF32-V",
	"KSC-EUC-H",
	"KSC-EUC-V",
	"KSCms-UHC-H",
	"KSCms-UHC-V",
	"KSCms-UHC-HW-H",
	"KSCms-UHC-HW-V",
	"KSCpc-EUC-H",

	// --- Adobe identity CMaps (CIDFont) ---
	"Identity-H",
	"Identity-V",
}

// errorSignatures are substrings found in error messages or panic values
// produced by PDF libraries when a CMap resource cannot be resolved.
var errorSignatures = []string{
	// iTextPDF / iText7 (Java, called via subprocess or JNI)
	"com/itextpdf/io/font/cmap",
	"was not found",
	"CMap",

	// pdfbox
	"Could not find a CMap",
	"CMapParser",

	// pdfcpu
	"missing CMap",
	"unsupported CMap",

	// generic
	"cmap resource",
	"font cmap",
	"cmap not found",
	"CMapName",
}
