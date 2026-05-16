package scraper

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// jdCache holds recently-scraped job descriptions so repeat requests for the
// same URL do not incur an additional HTTP round-trip.
var jdCache = newLRUCache(100)

// privateRanges lists CIDR blocks that must never be reached via a
// user-supplied URL (SSRF protection).
var privateRanges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",  // link-local + cloud metadata (AWS, GCP, Azure)
	"127.0.0.0/8",     // loopback
	"0.0.0.0/8",
	"100.64.0.0/10",   // shared address space (RFC 6598)
	"fc00::/7",        // IPv6 unique local
	"fe80::/10",       // IPv6 link-local
	"::1/128",         // IPv6 loopback
}

// ValidateJobURL returns an error when rawURL is unsafe to fetch:
//   - scheme is not http or https
//   - hostname resolves to a private/reserved IP address
//
// DNS resolution is performed so that DNS-rebinding attacks using public-facing
// names that point to private IPs are also caught.
func ValidateJobURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https, got: %s", u.Scheme)
	}

	hostname := u.Hostname()
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return fmt.Errorf("cannot resolve hostname %s: %w", hostname, err)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		for _, cidr := range privateRanges {
			_, network, _ := net.ParseCIDR(cidr)
			if network.Contains(ip) {
				return fmt.Errorf("URL resolves to a private/reserved IP address")
			}
		}
	}
	return nil
}

// ScrapeJobDescription fetches the job posting at jobURL, validates the URL
// against the SSRF allowlist, and returns the cleaned plain-text content.
// Results are memoised in jdCache so repeated calls for the same URL skip the
// HTTP round-trip.
func ScrapeJobDescription(ctx context.Context, jobURL string) (string, error) {
	log.Printf("Attempting to scrape job description from: %s", jobURL)

	if err := ValidateJobURL(jobURL); err != nil {
		return "", fmt.Errorf("URL validation failed: %w", err)
	}

	// Cache hit — return immediately without making an HTTP request.
	if cached, ok := jdCache.get(jobURL); ok {
		log.Printf("Cache hit for %s (%d characters)", jobURL, len(cached))
		return cached, nil
	}

	// Custom HTTP client with redirect validation to prevent redirect-based
	// SSRF bypass (an attacker could redirect a "safe" URL to an internal one).
	scrapingClient := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(redirectReq *http.Request, via []*http.Request) error {
			if err := ValidateJobURL(redirectReq.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", jobURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := scrapingClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch job description: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	text := ExtractTextFromHTML(string(body))

	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no text content found in the webpage")
	}

	cleanText := CleanJobDescription(text)

	log.Printf("Successfully scraped %d characters from job posting", len(cleanText))

	// Populate cache so subsequent requests for this URL are served locally.
	jdCache.set(jobURL, cleanText)

	return cleanText, nil
}
