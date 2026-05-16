package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/bedrockruntime"
	"resume-customizer/internal/logger"
)

// rng is a package-level random source seeded once at startup, used only for
// jitter in invokeWithRetry. math/rand is appropriate here (not crypto/rand).
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// sleepFn is the function used to pause between retries. It is a package-level
// variable so tests can replace it with a no-op to avoid real delays.
var sleepFn = time.Sleep

// BedrockInvoker is the narrow interface satisfied by *bedrockruntime.BedrockRuntime
// and any test double. It is the only Bedrock method NovaService calls.
type BedrockInvoker interface {
	InvokeModel(input *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error)
}

// NovaRequest / NovaResponse mirror the Amazon Nova converse API shape.
type NovaRequest struct {
	Messages                     []NovaMessage   `json:"messages"`
	System                       []SystemMessage `json:"system"`
	InferenceConfig              InferenceConfig `json:"inferenceConfig"`
	AdditionalModelRequestFields json.RawMessage `json:"additionalModelRequestFields,omitempty"`
}

type NovaMessage struct {
	Role    string           `json:"role"`
	Content []ContentMessage `json:"content"`
}

type ContentMessage struct {
	Text string `json:"text"`
}

type SystemMessage struct {
	Text string `json:"text"`
}

type InferenceConfig struct {
	MaxTokens   int     `json:"maxTokens"`
	Temperature float32 `json:"temperature"`
	TopP        float32 `json:"topP,omitempty"`
	TopK        int     `json:"topK,omitempty"`
}

type NovaResponse struct {
	Output struct {
		Message struct {
			Content []ContentMessage `json:"content"`
		} `json:"message"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	} `json:"usage"`
}

// retryableMessages is the set of substrings that identify transient Bedrock
// errors worth retrying.
var retryableMessages = []string{
	"ThrottlingException",
	"ServiceUnavailableException",
	"RequestTimeout",
	"InternalServerException",
	"Too Many Requests",
}

// isRetryable returns true when err represents a transient Bedrock condition
// that is safe to retry.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, substr := range retryableMessages {
		if strings.Contains(msg, substr) {
			return true
		}
	}
	return false
}

// invokeWithRetry calls client.InvokeModel up to maxAttempts times (1 initial
// + 2 retries). It only retries on errors identified by isRetryable; all other
// errors are returned immediately. The delay between attempts starts at 1 s,
// doubles on each retry, and has jitter applied to avoid thundering-herd.
func invokeWithRetry(ctx context.Context, client BedrockInvoker, input *bedrockruntime.InvokeModelInput) (*bedrockruntime.InvokeModelOutput, error) {
	const maxAttempts = 3
	baseDelayMs := int64(1000) // 1 s

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		output, err := client.InvokeModel(input)
		if err == nil {
			return output, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}

		if attempt == maxAttempts {
			break
		}

		delay := baseDelayMs
		jitter := rng.Int63n(delay/2 + 1)
		actualDelay := delay + jitter

		logger.Logger.Warn("Bedrock call failed, retrying",
			"attempt", attempt,
			"delay_ms", actualDelay,
			"error", err,
		)

		sleepFn(time.Duration(actualDelay) * time.Millisecond)

		baseDelayMs *= 2 // double for next iteration
	}

	return nil, fmt.Errorf("Bedrock call failed after %d attempts: %w", maxAttempts, lastErr)
}

// NovaService wraps the Bedrock runtime client and provides a simple
// text-in / text-out interface.
type NovaService struct {
	client  BedrockInvoker
	modelID string
}

// NewNovaService creates a NovaService backed by the us-west-2 Bedrock endpoint.
func NewNovaService() (*NovaService, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &NovaService{
		client:  bedrockruntime.New(sess),
		modelID: "us.amazon.nova-lite-v1:0",
	}, nil
}

// NewNovaServiceWithClient creates a NovaService using the provided BedrockInvoker.
// This constructor is intended for testing.
func NewNovaServiceWithClient(client BedrockInvoker, modelID string) *NovaService {
	return &NovaService{client: client, modelID: modelID}
}

// GenerateContent sends prompt + systemPrompt to the Nova model and returns
// the first text response.
func (s *NovaService) GenerateContent(ctx context.Context, prompt, systemPrompt string) (string, error) {
	log := logger.With(ctx)
	log.Info("invoking Bedrock model", "model", s.modelID)
	start := time.Now()

	request := NovaRequest{
		Messages: []NovaMessage{
			{
				Role:    "user",
				Content: []ContentMessage{{Text: prompt}},
			},
		},
		System: []SystemMessage{{Text: systemPrompt}},
		InferenceConfig: InferenceConfig{
			MaxTokens:   2000,
			Temperature: 0.1,
			TopP:        0.9,
		},
	}

	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("error marshaling Nova request: %w", err)
	}

	output, err := invokeWithRetry(ctx, s.client, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(s.modelID),
		Body:        jsonBytes,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	})
	if err != nil {
		log.Error("Bedrock InvokeModel failed", "model", s.modelID, "error", err)
		return "", fmt.Errorf("error invoking Nova model: %w", err)
	}

	var response NovaResponse
	if err := json.Unmarshal(output.Body, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling Nova response: %w", err)
	}

	if len(response.Output.Message.Content) == 0 {
		return "", fmt.Errorf("no content in Nova response")
	}

	duration := time.Since(start)
	log.Info("Bedrock model invocation complete",
		"model", s.modelID,
		"duration_ms", duration.Milliseconds(),
		"input_tokens", response.Usage.InputTokens,
		"output_tokens", response.Usage.OutputTokens,
		"total_tokens", response.Usage.InputTokens+response.Usage.OutputTokens,
	)

	return response.Output.Message.Content[0].Text, nil
}
