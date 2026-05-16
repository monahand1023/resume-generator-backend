package scraper

import (
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const defaultMaxJDLength = 8000

// Package-level compiled regexps — compiled once at startup, not on every call.
var (
	scriptTagRegex = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	styleTagRegex  = regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`)
	htmlTagRegex   = regexp.MustCompile(`<[^>]*>`)
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// ExtractTextFromHTML strips script/style blocks and HTML tags from html,
// decodes common HTML entities, and normalises whitespace.
func ExtractTextFromHTML(html string) string {
	html = scriptTagRegex.ReplaceAllString(html, "")
	html = styleTagRegex.ReplaceAllString(html, "")

	text := htmlTagRegex.ReplaceAllString(html, " ")

	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	text = whitespaceRegex.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// CleanJobDescription filters navigation/boilerplate lines and truncates the
// result to at most maxLen characters.  The default limit is defaultMaxJDLength
// but can be overridden via the JD_MAX_LENGTH environment variable.
func CleanJobDescription(text string) string {
	maxLen := defaultMaxJDLength
	if envMax := os.Getenv("JD_MAX_LENGTH"); envMax != "" {
		if n, err := strconv.Atoi(envMax); err == nil && n > 0 {
			maxLen = n
		}
	}

	lines := strings.Split(text, "\n")
	var cleanLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)
		if strings.Contains(lower, "cookie") ||
			strings.Contains(lower, "privacy policy") ||
			strings.Contains(lower, "terms of service") ||
			strings.Contains(lower, "sign in") ||
			strings.Contains(lower, "register") ||
			strings.Contains(lower, "follow us") ||
			strings.Contains(lower, "subscribe") ||
			strings.Contains(lower, "newsletter") ||
			strings.Contains(lower, "social media") ||
			len(line) < 10 {
			continue
		}

		cleanLines = append(cleanLines, line)
	}

	cleaned := strings.Join(cleanLines, "\n")

	if len(cleaned) > maxLen {
		log.Printf("WARNING: Job description truncated from %d to %d characters", len(cleaned), maxLen)
		cleaned = cleaned[:maxLen]
	}

	return cleaned
}
