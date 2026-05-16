package parser

import "testing"

func TestDetectFileType_pdf(t *testing.T) {
	// %PDF magic bytes
	fileBytes := []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E}
	if got := DetectFileType(fileBytes, "resume.pdf"); got != "pdf" {
		t.Errorf("expected 'pdf', got %q", got)
	}
}

func TestDetectFileType_docx(t *testing.T) {
	// PK\x03\x04 ZIP magic bytes with .docx extension
	fileBytes := []byte{0x50, 0x4B, 0x03, 0x04, 0x14, 0x00}
	if got := DetectFileType(fileBytes, "resume.docx"); got != "docx" {
		t.Errorf("expected 'docx', got %q", got)
	}
}

func TestDetectFileType_text(t *testing.T) {
	fileBytes := []byte("John Smith\nSoftware Engineer\nSan Francisco, CA\n")
	if got := DetectFileType(fileBytes, "resume.txt"); got != "text" {
		t.Errorf("expected 'text', got %q", got)
	}
}

func TestDetectFileType_unknown(t *testing.T) {
	// Binary-looking bytes with no recognisable magic
	fileBytes := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0x80, 0x81}
	if got := DetectFileType(fileBytes, "resume.xyz"); got != "unknown" {
		t.Errorf("expected 'unknown', got %q", got)
	}
}

func TestDetectFileType_too_short(t *testing.T) {
	if got := DetectFileType([]byte{0x25}, "short.pdf"); got != "unknown" {
		t.Errorf("expected 'unknown' for files shorter than 4 bytes, got %q", got)
	}
}

func TestIsPlainText_printable(t *testing.T) {
	text := "Hello, world! This is a plain text resume.\nWith multiple lines.\n"
	if !IsPlainText(text) {
		t.Error("expected IsPlainText to return true for ASCII text")
	}
}

func TestIsPlainText_empty(t *testing.T) {
	if IsPlainText("") {
		t.Error("expected IsPlainText to return false for empty string")
	}
}

func TestIsPlainText_binary(t *testing.T) {
	// Mostly non-printable bytes
	bin := string([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0x80, 0x00, 0x00, 0x00, 0x00})
	if IsPlainText(bin) {
		t.Error("expected IsPlainText to return false for binary data")
	}
}
