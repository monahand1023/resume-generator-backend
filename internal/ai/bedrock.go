package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/bedrockruntime"
	"resume-customizer/internal/logger"
)

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

	output, err := s.client.InvokeModel(&bedrockruntime.InvokeModelInput{
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
