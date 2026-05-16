package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

// --------------------------------------------------------------------------
// Test doubles
// --------------------------------------------------------------------------

// mockGenerator is a stub ContentGenerator. Each call returns the next item
// from responses in order. If err is set every call returns it.
type mockGenerator struct {
	responses []string
	callIdx   int
	err       error
}

func (m *mockGenerator) GenerateContent(_ context.Context, _, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.callIdx >= len(m.responses) {
		return "fallback", nil
	}
	r := m.responses[m.callIdx]
	m.callIdx++
	return r, nil
}

// successGenerator returns distinct non-empty strings for each of the three
// concurrent GenerateContent calls (resume / cover_letter / changes).
func successGenerator() *mockGenerator {
	return &mockGenerator{
		responses: []string{"resume text", "cover letter text", "changes text"},
	}
}

// mockScraper builds a scraperFunc stub.
func mockScraper(result string, err error) scraperFunc {
	return func(_ context.Context, _ string) (string, error) {
		return result, err
	}
}

// noopValidator is a urlValidatorFunc that always succeeds — used in tests that
// should not perform real DNS resolution.
func noopValidator(_ string) error { return nil }

// rejectValidator is a urlValidatorFunc that always returns an error — used to
// exercise the URL-validation failure path.
func rejectValidator(_ string) error { return errors.New("blocked") }

// validJobDescription contains the keyword triggers that ExtractJobDetails
// needs (company + position), matching the heuristic in util/text.go.
const validJobDescription = `About Us: Acme Corp
Position: Senior Software Engineer
We are looking for talented engineers to join our team and build great things.`

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// apiRequest builds a minimal APIGatewayProxyRequest with the given JSON body.
func apiRequest(body string) events.APIGatewayProxyRequest {
	return events.APIGatewayProxyRequest{Body: body}
}

// plainTextResume returns a base64-encoded plain-text resume that passes
// DetectFileType and IsPlainText checks.
func plainTextResume() string {
	content := "Jane Doe\njane@example.com\nSoftware Engineer\nExperience\nSkills"
	return base64.StdEncoding.EncodeToString([]byte(content))
}

// buildBody marshals a CustomizeRequest to JSON and returns the string.
func buildBody(t *testing.T, resume, jobURL, fileName string) string {
	t.Helper()
	b, err := json.Marshal(CustomizeRequest{
		Resume:   resume,
		JobURL:   jobURL,
		FileName: fileName,
	})
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}
	return string(b)
}

// --------------------------------------------------------------------------
// Tests
// --------------------------------------------------------------------------

// TestHandleCustomizeResume_validRequest_returns200 exercises the happy path:
// plain-text resume, a no-op URL validator, a scraper stub that returns a
// parseable job description, and a generator that succeeds.
func TestHandleCustomizeResume_validRequest_returns200(t *testing.T) {
	body := buildBody(t, plainTextResume(), "https://jobs.example.com/engineer", "resume.txt")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper(validJobDescription, nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d — body: %s", resp.StatusCode, resp.Body)
	}

	var out CustomizeResponse
	if err := json.Unmarshal([]byte(resp.Body), &out); err != nil {
		t.Fatalf("response body is not valid CustomizeResponse JSON: %v", err)
	}
	if out.Resume == "" || out.CoverLetter == "" || out.Changes == "" {
		t.Error("expected all three fields (Resume, CoverLetter, Changes) to be non-empty")
	}
}

// TestHandleCustomizeResume_missingResume_returns400 verifies that an empty
// Resume field triggers a 400 before any network or AI work is done.
func TestHandleCustomizeResume_missingResume_returns400(t *testing.T) {
	body := buildBody(t, "", "https://jobs.example.com/engineer", "resume.txt")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper("", nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestHandleCustomizeResume_missingJobURL_returns400 verifies that an empty
// JobURL field triggers a 400.
func TestHandleCustomizeResume_missingJobURL_returns400(t *testing.T) {
	body := buildBody(t, plainTextResume(), "", "resume.txt")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper("", nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestHandleCustomizeResume_invalidJSON_returns400 verifies that a malformed
// request body results in a 400.
func TestHandleCustomizeResume_invalidJSON_returns400(t *testing.T) {
	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest("{not valid json"),
		successGenerator(),
		mockScraper("", nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestHandleCustomizeResume_invalidURLScheme_returns400 verifies that a failed
// URL validation (e.g. non-http/https scheme) is surfaced as a 400.
func TestHandleCustomizeResume_invalidURLScheme_returns400(t *testing.T) {
	body := buildBody(t, plainTextResume(), "file:///etc/passwd", "resume.txt")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper("", nil),
		rejectValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for blocked URL, got %d", resp.StatusCode)
	}
}

// TestHandleCustomizeResume_invalidFileType_returns400 checks that a base64
// payload whose magic bytes are unrecognised (not PDF, DOCX, or plain text)
// triggers a 400 from ParseResumeFile.
func TestHandleCustomizeResume_invalidFileType_returns400(t *testing.T) {
	// Encode raw binary that is neither PDF nor DOCX nor printable text.
	binaryGarbage := make([]byte, 16)
	for i := range binaryGarbage {
		binaryGarbage[i] = byte(i) // mix of non-printable bytes
	}
	encoded := base64.StdEncoding.EncodeToString(binaryGarbage)
	body := buildBody(t, encoded, "https://jobs.example.com/engineer", "garbage.bin")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper(validJobDescription, nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for unsupported file type, got %d — body: %s", resp.StatusCode, resp.Body)
	}
}

// TestHandleCustomizeResume_scraperError_returns500 verifies that a scraper
// failure (network error) is surfaced as a 500.
func TestHandleCustomizeResume_scraperError_returns500(t *testing.T) {
	body := buildBody(t, plainTextResume(), "https://jobs.example.com/engineer", "resume.txt")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper("", errors.New("connection refused")),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// TestHandleCustomizeResume_extractJobDetailsFails_returns422 verifies that
// when the scraped text does not contain identifiable company/position markers,
// the handler responds with 422.
func TestHandleCustomizeResume_extractJobDetailsFails_returns422(t *testing.T) {
	unparseableJD := "We offer great benefits and free snacks. Ping pong table available. Open floor plan."
	body := buildBody(t, plainTextResume(), "https://jobs.example.com/engineer", "resume.txt")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper(unparseableJD, nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 422 {
		t.Errorf("expected 422, got %d — body: %s", resp.StatusCode, resp.Body)
	}
}

// TestHandleCustomizeResume_aiError_returns500 verifies that when the AI
// generator returns an error, the handler responds with 500.
func TestHandleCustomizeResume_aiError_returns500(t *testing.T) {
	body := buildBody(t, plainTextResume(), "https://jobs.example.com/engineer", "resume.txt")
	failGen := &mockGenerator{err: errors.New("bedrock timeout")}

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		failGen,
		mockScraper(validJobDescription, nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

// TestHandleCustomizeResume_responseIncludesMetadata verifies that company and
// position metadata are populated in a successful response.
func TestHandleCustomizeResume_responseIncludesMetadata(t *testing.T) {
	body := buildBody(t, plainTextResume(), "https://jobs.example.com/engineer", "resume.txt")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper(validJobDescription, nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d — body: %s", resp.StatusCode, resp.Body)
	}

	var out CustomizeResponse
	if err := json.Unmarshal([]byte(resp.Body), &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if out.Metadata.Company == "" {
		t.Error("expected Metadata.Company to be non-empty")
	}
	if out.Metadata.Position == "" {
		t.Error("expected Metadata.Position to be non-empty")
	}
}

// TestHandleCustomizeResume_corsHeadersAlwaysPresent verifies that CORS headers
// are set even on error responses.
func TestHandleCustomizeResume_corsHeadersAlwaysPresent(t *testing.T) {
	body := buildBody(t, "", "", "")

	resp, err := handleCustomizeResume(
		context.Background(),
		apiRequest(body),
		successGenerator(),
		mockScraper("", nil),
		noopValidator,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Headers["Access-Control-Allow-Origin"] == "" {
		t.Error("expected Access-Control-Allow-Origin header to be set on error response")
	}
}
