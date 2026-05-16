package scraper

import (
	"strings"
	"testing"
)

func TestCleanJobDescription_removes_boilerplate(t *testing.T) {
	text := "Software Engineer\nCookies help us provide a better experience.\nWe are hiring a great engineer."
	result := CleanJobDescription(text)
	if strings.Contains(result, "Cookie") || strings.Contains(result, "cookie") {
		t.Error("cookie boilerplate should be removed")
	}
	if !strings.Contains(result, "engineer") {
		t.Error("relevant content should be preserved")
	}
}

func TestCleanJobDescription_truncates_at_max(t *testing.T) {
	// Build a string longer than defaultMaxJDLength
	long := strings.Repeat("This is a relevant job description sentence for testing purposes. ", 200)
	result := CleanJobDescription(long)
	if len(result) > defaultMaxJDLength {
		t.Errorf("result length %d exceeds maxLen %d", len(result), defaultMaxJDLength)
	}
}

func TestCleanJobDescription_skips_short_lines(t *testing.T) {
	text := "ok\nSoftware Engineer at Acme Corporation building distributed systems."
	result := CleanJobDescription(text)
	if strings.Contains(result, "\nok\n") || strings.HasPrefix(result, "ok\n") {
		t.Error("short lines (< 10 chars) should be filtered out")
	}
}

func TestExtractTextFromHTML_strips_tags(t *testing.T) {
	html := "<html><body><h1>Software Engineer</h1><p>Join our team.</p></body></html>"
	result := ExtractTextFromHTML(html)
	if strings.Contains(result, "<") || strings.Contains(result, ">") {
		t.Errorf("HTML tags should be stripped, got: %q", result)
	}
	if !strings.Contains(result, "Software Engineer") {
		t.Errorf("text content should be preserved, got: %q", result)
	}
}

func TestExtractTextFromHTML_removes_script(t *testing.T) {
	html := `<html><body><script>alert("xss")</script><p>Valid content.</p></body></html>`
	result := ExtractTextFromHTML(html)
	if strings.Contains(result, "alert") || strings.Contains(result, "xss") {
		t.Errorf("script content should be removed, got: %q", result)
	}
}

func TestExtractTextFromHTML_decodes_entities(t *testing.T) {
	html := "<p>AT&amp;T &lt;Telecom&gt; &quot;quoted&quot;</p>"
	result := ExtractTextFromHTML(html)
	if !strings.Contains(result, "AT&T") {
		t.Errorf("&amp; should be decoded to &, got: %q", result)
	}
	if !strings.Contains(result, "<Telecom>") {
		t.Errorf("&lt;&gt; should be decoded, got: %q", result)
	}
}
