package scriptdetector

// threatSignature pairs a byte pattern with the threat it represents.
type threatSignature struct {
	pattern         []byte
	caseInsensitive bool
	threat          ThreatType
	severity        Severity
	label           string // human-readable description shown in Finding.Pattern
}

// signatures is the master list of patterns scanned in ScanBytes.
// Order within a threat group matters only for tie-breaking: when two patterns
// hit the same ThreatType at the same severity, the first one is kept.
var signatures = []threatSignature{
	// ── JavaScript ──────────────────────────────────────────────────────────
	// /S /JavaScript is the PDF action type for JavaScript. Definitive — no
	// false positives possible here.
	{[]byte("/S /JavaScript"), false, ThreatJavaScript, SeverityMedium, "/S /JavaScript action type"},
	{[]byte("/S/JavaScript"), false, ThreatJavaScript, SeverityMedium, "/S/JavaScript action type"},
	// /JS is the dictionary key for JavaScript code (inline literal or stream ref).
	{[]byte("/JS "), false, ThreatJavaScript, SeverityMedium, "/JS content key"},
	{[]byte("/JS("), false, ThreatJavaScript, SeverityMedium, "/JS inline literal"},
	{[]byte("/JS\n"), false, ThreatJavaScript, SeverityMedium, "/JS stream reference"},
	{[]byte("/JS\r"), false, ThreatJavaScript, SeverityMedium, "/JS stream reference"},

	// ── Launch action ────────────────────────────────────────────────────────
	// /Launch executes an external application or document. There is no
	// legitimate reason for a user-submitted document to carry this action.
	{[]byte("/Launch"), false, ThreatLaunchAction, SeverityHigh, "/Launch action (executes external application)"},

	// ── Embedded files ───────────────────────────────────────────────────────
	// /EmbeddedFile may be benign (PDF/A attachments) but warrants inspection.
	{[]byte("/EmbeddedFile"), false, ThreatEmbeddedFile, SeverityLow, "/EmbeddedFile stream"},

	// ── XFA forms ────────────────────────────────────────────────────────────
	// XFA (XML Forms Architecture) supports ECMAScript and is a common malware
	// delivery mechanism in older PDF readers.
	{[]byte("/XFA"), false, ThreatXFAForm, SeverityMedium, "/XFA form (XML Forms Architecture)"},

	// ── JavaScript obfuscation ───────────────────────────────────────────────
	// eval(unescape( is the classic "shellcode in a string" pattern used to
	// hide exploits from static scanners.
	{[]byte("eval(unescape("), false, ThreatObfuscation, SeverityCritical, "eval(unescape() — shellcode obfuscation pattern"},
	// String.fromCharCode encodes strings as char-code arrays to bypass scanners.
	{[]byte("String.fromCharCode"), false, ThreatObfuscation, SeverityHigh, "String.fromCharCode — character-code encoding"},
	// unescape alone is a weaker indicator but still suspicious in a PDF.
	{[]byte("unescape("), false, ThreatObfuscation, SeverityHigh, "unescape() call"},

	// ── Shell / system execution ─────────────────────────────────────────────
	{[]byte("ActiveXObject"), true, ThreatShellExecution, SeverityCritical, "ActiveXObject — Windows COM automation"},
	{[]byte("WScript.Shell"), true, ThreatShellExecution, SeverityCritical, "WScript.Shell — Windows Script Host"},
	{[]byte("powershell"), true, ThreatShellExecution, SeverityCritical, "PowerShell execution"},
	{[]byte("cmd.exe"), true, ThreatShellExecution, SeverityCritical, "cmd.exe shell execution"},
	{[]byte("/bin/sh"), false, ThreatShellExecution, SeverityHigh, "/bin/sh shell execution"},
	{[]byte("/bin/bash"), false, ThreatShellExecution, SeverityHigh, "/bin/bash shell execution"},
}
