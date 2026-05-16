package util

import (
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestNormalizeRequest_strips_trailing_slash(t *testing.T) {
	req := events.APIGatewayProxyRequest{Path: "/api/customize-resume/"}
	out := NormalizeRequest(req)
	if out.Path != "/api/customize-resume" {
		t.Errorf("expected '/api/customize-resume', got %q", out.Path)
	}
}

func TestNormalizeRequest_no_change_without_slash(t *testing.T) {
	req := events.APIGatewayProxyRequest{Path: "/api/customize-resume"}
	out := NormalizeRequest(req)
	if out.Path != "/api/customize-resume" {
		t.Errorf("expected '/api/customize-resume', got %q", out.Path)
	}
}

func TestNormalizeRequest_strips_prod_stage_prefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/prod/api/customize-resume", "/api/customize-resume"},
		{"/stage/api/customize-resume", "/api/customize-resume"},
		{"/dev/api/customize-resume", "/api/customize-resume"},
	}
	for _, tt := range tests {
		req := events.APIGatewayProxyRequest{Path: tt.input}
		out := NormalizeRequest(req)
		if out.Path != tt.expected {
			t.Errorf("input %q: expected %q, got %q", tt.input, tt.expected, out.Path)
		}
	}
}

func TestNormalizeRequest_does_not_mutate_input(t *testing.T) {
	original := events.APIGatewayProxyRequest{Path: "/prod/api/customize-resume/"}
	_ = NormalizeRequest(original)
	if original.Path != "/prod/api/customize-resume/" {
		t.Error("NormalizeRequest must not mutate the input (should return a new value)")
	}
}

func TestMatchesPath_equal(t *testing.T) {
	if !MatchesPath("/api/customize-resume", "/api/customize-resume") {
		t.Error("identical paths should match")
	}
}

func TestMatchesPath_trailing_slash_ignored(t *testing.T) {
	if !MatchesPath("/api/customize-resume/", "/api/customize-resume") {
		t.Error("trailing slash should be ignored in comparison")
	}
}

func TestMatchesPath_different(t *testing.T) {
	if MatchesPath("/api/other", "/api/customize-resume") {
		t.Error("different paths should not match")
	}
}
