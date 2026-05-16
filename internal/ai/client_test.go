package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/bedrockruntime"
)

// --------------------------------------------------------------------------
// Test doubles
// --------------------------------------------------------------------------

// mockInvoker is a test double for BedrockInvoker.
type mockInvoker struct {
	body []byte
	err  error
}

func (m *mockInvoker) InvokeModel(_ *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &bedrockruntime.InvokeModelOutput{Body: m.body}, nil
}

// captureModelInvoker records the ModelId passed to InvokeModel.
type captureModelInvoker struct {
	body          []byte
	capturedModel *string
}

func (c *captureModelInvoker) InvokeModel(in *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
	if in.ModelId != nil {
		*c.capturedModel = *in.ModelId
	}
	return &bedrockruntime.InvokeModelOutput{Body: c.body}, nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// buildResponseBody marshals a NovaResponse with the given text content.
func buildResponseBody(t *testing.T, text string) []byte {
	t.Helper()
	resp := NovaResponse{}
	resp.Output.Message.Content = []ContentMessage{{Text: text}}
	resp.Usage.InputTokens = 10
	resp.Usage.OutputTokens = 20
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("buildResponseBody: %v", err)
	}
	return b
}

// --------------------------------------------------------------------------
// Tests: GenerateContent
// --------------------------------------------------------------------------

// TestGenerateContent_success verifies that a well-formed Bedrock response is
// parsed and the first content text is returned without error.
func TestGenerateContent_success(t *testing.T) {
	want := "optimized resume text"
	svc := NewNovaServiceWithClient(&mockInvoker{body: buildResponseBody(t, want)}, "test-model")

	got, err := svc.GenerateContent(context.Background(), "prompt", "system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestGenerateContent_bedrockError verifies that an error from the Bedrock
// client is propagated as a Go error.
func TestGenerateContent_bedrockError(t *testing.T) {
	svc := NewNovaServiceWithClient(&mockInvoker{err: errors.New("bedrock unavailable")}, "test-model")

	_, err := svc.GenerateContent(context.Background(), "prompt", "system")
	if err == nil {
		t.Fatal("expected an error but got nil")
	}
}

// TestGenerateContent_emptyContentList verifies that a response with an empty
// content array is handled gracefully and returns an error (not a panic).
func TestGenerateContent_emptyContentList(t *testing.T) {
	resp := NovaResponse{} // Content slice is empty.
	body, _ := json.Marshal(resp)

	svc := NewNovaServiceWithClient(&mockInvoker{body: body}, "test-model")

	_, err := svc.GenerateContent(context.Background(), "prompt", "system")
	if err == nil {
		t.Fatal("expected an error for empty content list")
	}
}

// TestGenerateContent_invalidJSON verifies that a malformed response body
// returns a parse error instead of panicking.
func TestGenerateContent_invalidJSON(t *testing.T) {
	svc := NewNovaServiceWithClient(&mockInvoker{body: []byte("{not valid json")}, "test-model")

	_, err := svc.GenerateContent(context.Background(), "prompt", "system")
	if err == nil {
		t.Fatal("expected an error for invalid JSON response body")
	}
}

// TestGenerateContent_emptyBody verifies that a completely empty response body
// is handled gracefully.
func TestGenerateContent_emptyBody(t *testing.T) {
	svc := NewNovaServiceWithClient(&mockInvoker{body: []byte{}}, "test-model")

	_, err := svc.GenerateContent(context.Background(), "prompt", "system")
	if err == nil {
		t.Fatal("expected an error for empty response body")
	}
}

// TestGenerateContent_multipleContentItems verifies that only the first content
// item is returned when the response contains multiple items.
func TestGenerateContent_multipleContentItems(t *testing.T) {
	resp := NovaResponse{}
	resp.Output.Message.Content = []ContentMessage{
		{Text: "first"},
		{Text: "second"},
	}
	body, _ := json.Marshal(resp)

	svc := NewNovaServiceWithClient(&mockInvoker{body: body}, "test-model")

	got, err := svc.GenerateContent(context.Background(), "prompt", "system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "first" {
		t.Errorf("expected 'first', got %q", got)
	}
}

// TestGenerateContent_requestPayload verifies that the correct model ID is
// forwarded to the Bedrock client.
func TestGenerateContent_requestPayload(t *testing.T) {
	const wantModel = "my-custom-model"
	var gotModel string
	svc := NewNovaServiceWithClient(&captureModelInvoker{
		body:          buildResponseBody(t, "ok"),
		capturedModel: &gotModel,
	}, wantModel)

	_, err := svc.GenerateContent(context.Background(), "user prompt", "sys prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotModel != wantModel {
		t.Errorf("model ID: got %q, want %q", gotModel, wantModel)
	}
}

// --------------------------------------------------------------------------
// Tests: CreatePrompts
// --------------------------------------------------------------------------

// TestCreatePrompts_resumeType verifies that CreatePrompts returns non-empty
// strings for the "resume" content type.
func TestCreatePrompts_resumeType(t *testing.T) {
	prompt, system := CreatePrompts("my resume", "job description", "resume")
	if prompt == "" {
		t.Error("expected non-empty prompt for resume type")
	}
	if system == "" {
		t.Error("expected non-empty system prompt for resume type")
	}
}

// TestCreatePrompts_coverLetterType verifies CreatePrompts for "cover_letter".
func TestCreatePrompts_coverLetterType(t *testing.T) {
	prompt, system := CreatePrompts("my resume", "job description", "cover_letter")
	if prompt == "" {
		t.Error("expected non-empty prompt for cover_letter type")
	}
	if system == "" {
		t.Error("expected non-empty system prompt for cover_letter type")
	}
}

// TestCreatePrompts_changesType verifies CreatePrompts for "changes".
func TestCreatePrompts_changesType(t *testing.T) {
	prompt, system := CreatePrompts("my resume", "job description", "changes")
	if prompt == "" {
		t.Error("expected non-empty prompt for changes type")
	}
	if system == "" {
		t.Error("expected non-empty system prompt for changes type")
	}
}

// TestCreatePrompts_unknownType verifies that an unknown content type returns
// an empty prompt (map miss) but still a valid system prompt.
func TestCreatePrompts_unknownType(t *testing.T) {
	prompt, system := CreatePrompts("resume", "jd", "unknown_type")
	if prompt != "" {
		t.Errorf("expected empty prompt for unknown type, got %q", prompt)
	}
	if system == "" {
		t.Error("system prompt should always be non-empty")
	}
}

// --------------------------------------------------------------------------
// Tests: invokeWithRetry
// --------------------------------------------------------------------------

// disableSleep replaces the package sleepFn with a no-op for the duration of
// the test and restores the original on cleanup.
func disableSleep(t *testing.T) {
	t.Helper()
	orig := sleepFn
	sleepFn = func(_ time.Duration) {}
	t.Cleanup(func() { sleepFn = orig })
}

// sequenceInvoker cycles through a slice of (body, error) pairs on each call.
type sequenceInvoker struct {
	calls    []mockCall
	callIdx  int
	callCount int
}

type mockCall struct {
	body []byte
	err  error
}

func (s *sequenceInvoker) InvokeModel(_ *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
	s.callCount++
	if s.callIdx >= len(s.calls) {
		// Repeat the last entry if we run past the end.
		s.callIdx = len(s.calls) - 1
	}
	c := s.calls[s.callIdx]
	s.callIdx++
	if c.err != nil {
		return nil, c.err
	}
	return &bedrockruntime.InvokeModelOutput{Body: c.body}, nil
}

// TestInvokeWithRetry_allAttemptsFailRetryable verifies that a retryable error
// is retried up to maxAttempts and the final error is returned wrapped with the
// attempt count.
func TestInvokeWithRetry_allAttemptsFailRetryable(t *testing.T) {
	disableSleep(t)
	throttle := errors.New("ThrottlingException: rate exceeded")
	invoker := &sequenceInvoker{
		calls: []mockCall{
			{err: throttle},
			{err: throttle},
			{err: throttle},
		},
	}

	_, err := invokeWithRetry(context.Background(), invoker, &bedrockruntime.InvokeModelInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if invoker.callCount != 3 {
		t.Errorf("expected 3 calls, got %d", invoker.callCount)
	}
	if !strings.Contains(err.Error(), "3 attempts") {
		t.Errorf("error should mention attempt count, got: %v", err)
	}
	if !errors.Is(err, throttle) {
		t.Errorf("expected underlying error to be wrapped, got: %v", err)
	}
}

// TestInvokeWithRetry_nonRetryableFailsImmediately verifies that a
// non-retryable error causes the function to return after exactly one attempt.
func TestInvokeWithRetry_nonRetryableFailsImmediately(t *testing.T) {
	disableSleep(t)
	authErr := errors.New("AccessDeniedException: not authorized")
	invoker := &sequenceInvoker{
		calls: []mockCall{
			{err: authErr},
		},
	}

	_, err := invokeWithRetry(context.Background(), invoker, &bedrockruntime.InvokeModelInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if invoker.callCount != 1 {
		t.Errorf("expected exactly 1 call for non-retryable error, got %d", invoker.callCount)
	}
	if !errors.Is(err, authErr) {
		t.Errorf("expected original error to be returned, got: %v", err)
	}
}

// TestInvokeWithRetry_successOnSecondAttempt verifies that a ThrottlingException
// on the first call is retried and the successful second response is returned.
func TestInvokeWithRetry_successOnSecondAttempt(t *testing.T) {
	disableSleep(t)
	want := "retry succeeded"
	resp := NovaResponse{}
	resp.Output.Message.Content = []ContentMessage{{Text: want}}
	body, _ := json.Marshal(resp)

	invoker := &sequenceInvoker{
		calls: []mockCall{
			{err: errors.New("ThrottlingException: slow down")},
			{body: body},
		},
	}

	// Call invokeWithRetry directly to verify the retry machinery.
	output, err := invokeWithRetry(context.Background(), invoker, &bedrockruntime.InvokeModelInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if invoker.callCount != 2 {
		t.Errorf("expected 2 calls, got %d", invoker.callCount)
	}
	var got NovaResponse
	if unmarshalErr := json.Unmarshal(output.Body, &got); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal response: %v", unmarshalErr)
	}
	if len(got.Output.Message.Content) == 0 || got.Output.Message.Content[0].Text != want {
		t.Errorf("expected response text %q, got %v", want, got.Output.Message.Content)
	}
}
