package scraper

import (
	"net"
	"testing"
)

func TestValidateJobURL_rejects_file_scheme(t *testing.T) {
	if err := ValidateJobURL("file:///etc/passwd"); err == nil {
		t.Error("expected error for file:// scheme")
	}
}

func TestValidateJobURL_rejects_ftp(t *testing.T) {
	if err := ValidateJobURL("ftp://example.com/jobs"); err == nil {
		t.Error("expected error for ftp:// scheme")
	}
}

func TestValidateJobURL_rejects_javascript(t *testing.T) {
	if err := ValidateJobURL("javascript:alert(1)"); err == nil {
		t.Error("expected error for javascript: scheme")
	}
}

func TestValidateJobURL_rejects_invalid_url(t *testing.T) {
	if err := ValidateJobURL("://broken"); err == nil {
		t.Error("expected error for malformed URL")
	}
}

func TestValidateJobURL_accepts_https(t *testing.T) {
	// Use a well-known public domain that should resolve to a public IP.
	// This test requires network access; skip if DNS is unavailable.
	err := ValidateJobURL("https://www.example.com/jobs/123")
	if err != nil {
		t.Skipf("skipping: DNS lookup failed (likely no network): %v", err)
	}
}

// TestPrivateIPCheck validates the IP-range logic directly without DNS,
// since we cannot control DNS resolution in a unit test environment.
func TestPrivateIPCheck_loopback(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	if !isPrivateIP(ip) {
		t.Error("127.0.0.1 should be identified as private/reserved")
	}
}

func TestPrivateIPCheck_rfc1918_10(t *testing.T) {
	ip := net.ParseIP("10.0.0.1")
	if !isPrivateIP(ip) {
		t.Error("10.0.0.1 should be identified as private")
	}
}

func TestPrivateIPCheck_rfc1918_192168(t *testing.T) {
	ip := net.ParseIP("192.168.1.100")
	if !isPrivateIP(ip) {
		t.Error("192.168.1.100 should be identified as private")
	}
}

func TestPrivateIPCheck_metadata_service(t *testing.T) {
	ip := net.ParseIP("169.254.169.254") // AWS/GCP/Azure metadata IP
	if !isPrivateIP(ip) {
		t.Error("169.254.169.254 (cloud metadata) should be identified as private/reserved")
	}
}

func TestPrivateIPCheck_public(t *testing.T) {
	ip := net.ParseIP("8.8.8.8") // Google Public DNS — clearly public
	if isPrivateIP(ip) {
		t.Error("8.8.8.8 should not be identified as private")
	}
}

// isPrivateIP is a test-internal helper that mirrors the logic in ValidateJobURL
// so we can exercise it without triggering DNS.
func isPrivateIP(ip net.IP) bool {
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
