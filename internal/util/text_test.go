package util

import "testing"

func TestExtractNameFromResume_found(t *testing.T) {
	resume := "John Smith\njohnsmith@example.com\nSoftware Engineer"
	name := ExtractNameFromResume(resume)
	if name != "John Smith" {
		t.Errorf("expected 'John Smith', got %q", name)
	}
}

func TestExtractNameFromResume_fallback(t *testing.T) {
	// No plausible name in the first 5 lines
	resume := "resume\nexperience\nskills\neducation\ncontact"
	name := ExtractNameFromResume(resume)
	if name != "Resume" {
		t.Errorf("expected fallback 'Resume', got %q", name)
	}
}

func TestExtractNameFromResume_skips_cv_header(t *testing.T) {
	resume := "Curriculum Vitae\nJane Doe\njane@example.com"
	name := ExtractNameFromResume(resume)
	if name != "Jane Doe" {
		t.Errorf("expected 'Jane Doe', got %q", name)
	}
}

func TestExtractJobDetails_success(t *testing.T) {
	// The heuristic looks for lines containing "company"/"about us"/"organization"
	// for the company field, and "position"/"role"/"engineer"/"manager"/"developer"
	// for the position field.
	jd := "About Us: Acme Corporation is a leading tech firm.\nPosition: Senior Software Engineer\nWe are looking for talent to join our team."
	details, ok := ExtractJobDetails(jd)
	if !ok {
		t.Fatal("expected extraction to succeed")
	}
	if details.Company == "" {
		t.Error("expected non-empty Company")
	}
	if details.Position == "" {
		t.Error("expected non-empty Position")
	}
}

func TestExtractJobDetails_failure_returns_false(t *testing.T) {
	// Content that has no company or position markers
	jd := "We offer great benefits.\nFree snacks.\nPing pong table.\nOpen floor plan."
	_, ok := ExtractJobDetails(jd)
	if ok {
		t.Error("expected extraction to fail (return false) for generic boilerplate text")
	}
}

func TestExtractJobDetails_no_sentinel_strings(t *testing.T) {
	// Verify the old sentinel values "Company" / "Position" are never returned
	jd := "We offer great benefits.\nFree snacks."
	details, ok := ExtractJobDetails(jd)
	if ok {
		t.Skip("extraction unexpectedly succeeded; sentinel check not needed")
	}
	if details.Company == "Company" || details.Position == "Position" {
		t.Error("sentinel strings 'Company'/'Position' must not be returned; caller should use the bool")
	}
}
