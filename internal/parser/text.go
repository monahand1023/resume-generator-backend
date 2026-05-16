package parser

import "strings"

// IsPlainText returns true when more than 90 % of the bytes in text are
// printable ASCII (including common whitespace characters).
func IsPlainText(text string) bool {
	if len(text) == 0 {
		return false
	}
	printableCount := 0
	for _, r := range text {
		if (r >= 32 && r <= 126) || r == '\n' || r == '\r' || r == '\t' {
			printableCount++
		}
	}
	return float64(printableCount)/float64(len(text)) > 0.9
}

// DetectFileType inspects the magic bytes of fileBytes and falls back to
// fileName extension / plain-text heuristic.
// Returns one of: "pdf", "docx", "text", "unknown".
func DetectFileType(fileBytes []byte, fileName string) string {
	if len(fileBytes) < 4 {
		return "unknown"
	}

	// PDF: %PDF
	if fileBytes[0] == 0x25 && fileBytes[1] == 0x50 && fileBytes[2] == 0x44 && fileBytes[3] == 0x46 {
		return "pdf"
	}

	// ZIP (DOCX is a ZIP file): PK\x03\x04 or PK\x05\x06
	if fileBytes[0] == 0x50 && fileBytes[1] == 0x4B && (fileBytes[2] == 0x03 || fileBytes[2] == 0x05) {
		if strings.HasSuffix(strings.ToLower(fileName), ".docx") {
			return "docx"
		}
	}

	if IsPlainText(string(fileBytes)) {
		return "text"
	}

	return "unknown"
}
