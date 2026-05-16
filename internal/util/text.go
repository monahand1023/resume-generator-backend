package util

import (
	"log"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

// JobDetails holds extracted company and position from a job description.
type JobDetails struct {
	Company  string
	Position string
}

// normalizeRequest returns a copy of the request with the path standardized:
// trailing slashes removed, and API Gateway stage prefixes (prod/stage/dev) stripped.
func NormalizeRequest(req events.APIGatewayProxyRequest) events.APIGatewayProxyRequest {
	// Remove trailing slash
	req.Path = strings.TrimSuffix(req.Path, "/")

	// Strip stage prefix if present
	parts := strings.Split(req.Path, "/")
	if len(parts) > 1 && (parts[1] == "prod" || parts[1] == "stage" || parts[1] == "dev") {
		req.Path = "/" + strings.Join(parts[2:], "/")
	}

	log.Printf("Normalized Path: %s", req.Path)
	log.Printf("Method: %s", req.HTTPMethod)
	log.Printf("Path Parameters: %v", req.PathParameters)
	log.Printf("Query String Parameters: %v", req.QueryStringParameters)

	return req
}

// MatchesPath returns true when actualPath and expectedPath refer to the same
// route (comparison is performed after trailing-slash normalization; the caller
// can pass already-normalized paths — the function is idempotent).
func MatchesPath(actualPath, expectedPath string) bool {
	return strings.TrimSuffix(actualPath, "/") == strings.TrimSuffix(expectedPath, "/")
}

// ExtractNameFromResume attempts to find the applicant's full name in the first
// few lines of resume text.  Returns "Resume" if no plausible name is found.
func ExtractNameFromResume(resumeText string) string {
	lines := strings.Split(resumeText, "\n")
	for i, line := range lines {
		if i >= 5 {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "resume") ||
			strings.Contains(lower, "curriculum") ||
			strings.Contains(lower, "cv") {
			continue
		}
		words := strings.Fields(line)
		if len(words) >= 2 && len(words) <= 4 {
			isName := true
			for _, word := range words {
				if len(word) <= 1 ||
					word[0] != strings.ToUpper(word)[0] ||
					strings.ContainsAny(word, "0123456789@()") {
					isName = false
					break
				}
			}
			if isName {
				return line
			}
		}
	}
	return "Resume"
}

// ExtractJobDetails attempts to parse company and position from a job description.
// Returns (details, true) on success.  Returns (JobDetails{}, false) when either
// field cannot be determined — the caller should treat false as an extraction
// failure and respond accordingly rather than silently using sentinel strings.
func ExtractJobDetails(jobDescription string) (JobDetails, bool) {
	lines := strings.Split(jobDescription, "\n")
	company := ""
	position := ""

	for i, line := range lines {
		if i >= 15 {
			break
		}
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)

		if company == "" && (strings.Contains(lower, "company") ||
			strings.Contains(lower, "about us") ||
			strings.Contains(lower, "organization")) {
			parts := strings.Split(line, ":")
			company = strings.TrimSpace(parts[0])
		}

		if position == "" && (strings.Contains(lower, "position") ||
			strings.Contains(lower, "role") ||
			strings.Contains(lower, "job title") ||
			(strings.Contains(lower, "engineer") || strings.Contains(lower, "manager") || strings.Contains(lower, "developer")) &&
				len(line) < 80) {
			parts := strings.Split(line, ":")
			position = strings.TrimSpace(parts[0])
		}

		if company != "" && position != "" {
			break
		}
	}

	if company == "" || position == "" {
		return JobDetails{}, false
	}

	return JobDetails{Company: company, Position: position}, true
}
