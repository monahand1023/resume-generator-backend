package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/bedrockruntime"
)

// Request/Response structures
type CustomizeRequest struct {
	Resume   string `json:"resume"` // Base64 encoded file
	JobURL   string `json:"jobUrl"`
	FileName string `json:"fileName"`
}

type CustomizeResponse struct {
	Resume      string   `json:"resume"`
	CoverLetter string   `json:"coverLetter"`
	Changes     string   `json:"changes"`
	Metadata    Metadata `json:"metadata"`
}

type Metadata struct {
	Name     string `json:"name"`
	Company  string `json:"company"`
	Position string `json:"position"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// Nova/Bedrock structures
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

// Nova AI Service
type NovaService struct {
	client  *bedrockruntime.BedrockRuntime
	modelID string
}

func NewNovaService() (*NovaService, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"), // Nova is available in us-west-2
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &NovaService{
		client:  bedrockruntime.New(sess),
		modelID: "us.amazon.nova-pro-v1:0", // Using Nova Pro
	}, nil
}

func (s *NovaService) GenerateContent(ctx context.Context, prompt, systemPrompt string) (string, error) {
	request := NovaRequest{
		Messages: []NovaMessage{
			{
				Role: "user",
				Content: []ContentMessage{
					{Text: prompt},
				},
			},
		},
		System: []SystemMessage{
			{Text: systemPrompt},
		},
		InferenceConfig: InferenceConfig{
			MaxTokens:   4000,
			Temperature: 0.3,
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
		return "", fmt.Errorf("error invoking Nova model: %w", err)
	}

	var response NovaResponse
	if err := json.Unmarshal(output.Body, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling Nova response: %w", err)
	}

	if len(response.Output.Message.Content) == 0 {
		return "", fmt.Errorf("no content in Nova response")
	}

	log.Printf("Nova usage: %d input + %d output = %d total tokens",
		response.Usage.InputTokens, response.Usage.OutputTokens,
		response.Usage.InputTokens+response.Usage.OutputTokens)

	return response.Output.Message.Content[0].Text, nil
}

// Utility functions
func getTodayDate() string {
	now := time.Now()
	months := []string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
	return fmt.Sprintf("%s %d, %d", months[now.Month()-1], now.Day(), now.Year())
}

func extractNameFromResume(resumeText string) string {
	lines := strings.Split(resumeText, "\n")
	for i, line := range lines {
		if i >= 5 {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(strings.ToLower(line), "resume") ||
			strings.Contains(strings.ToLower(line), "curriculum") ||
			strings.Contains(strings.ToLower(line), "cv") {
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

func extractJobDetails(jobDescription string) (string, string) {
	lines := strings.Split(jobDescription, "\n")
	company := ""
	position := ""

	for i, line := range lines {
		if i >= 20 {
			break
		}
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)

		if company == "" && (strings.Contains(lower, "company") ||
			strings.Contains(lower, "about us") ||
			strings.Contains(lower, "organization")) {
			parts := strings.Split(line, ":")
			if len(parts) > 0 {
				company = strings.TrimSpace(parts[0])
			}
		}

		if position == "" && (strings.Contains(lower, "position") ||
			strings.Contains(lower, "role") ||
			strings.Contains(lower, "job title") ||
			(strings.Contains(lower, "engineer") || strings.Contains(lower, "manager") || strings.Contains(lower, "developer")) &&
				len(line) < 80) {
			parts := strings.Split(line, ":")
			if len(parts) > 0 {
				position = strings.TrimSpace(parts[0])
			}
		}

		if company != "" && position != "" {
			break
		}
	}

	if company == "" {
		company = "Company"
	}
	if position == "" {
		position = "Position"
	}

	return company, position
}

func parseResumeFile(fileBase64 string) (string, error) {
	// Decode base64
	fileBytes, err := base64.StdEncoding.DecodeString(fileBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// For now, assume it's text content. In production, you'd want to:
	// 1. Detect file type from first few bytes
	// 2. Use appropriate parser (PDF: pdfcpu, Word: go-docx, etc.)
	// 3. Extract text content

	// Simple implementation - assume uploaded content is already text
	// This is a placeholder - implement proper file parsing based on your needs
	text := string(fileBytes)

	// If it looks like binary data, return an error
	if len(text) > 0 && text[0] == 0 {
		return "", fmt.Errorf("binary file parsing not yet implemented - please upload plain text for now")
	}

	return text, nil
}

func scrapeJobDescription(ctx context.Context, url string) (string, error) {
	// For now, return a placeholder
	// In production, you'd implement web scraping here
	log.Printf("Would scrape job description from: %s", url)
	return fmt.Sprintf("Job posting from %s\n\nThis is a placeholder job description. In production, this would contain the actual scraped content from the job posting URL.", url), nil
}

func createPrompts(resumeText, jobDescription, contentType string) (string, string) {
	todayDate := getTodayDate()

	systemPrompt := "You are an expert resume writer and career consultant. You help job seekers optimize their resumes and create compelling cover letters that align with specific job requirements. Always provide professional, ATS-friendly content."

	prompts := map[string]string{
		"resume": fmt.Sprintf(`Transform this resume for the job posting using this EXACT format. Each line must start with one of these markers:

NAME: [Full Name]
CONTACT: [Email | Phone | LinkedIn | Location]
SECTION: [SECTION NAME]
SUMMARY_TEXT: [Professional summary]
COMPANY: [Company Name] | [Location] | [Dates]
TITLE: [Job Title]
DESC: [Company description - only for non-major companies]
BULLET: • [Achievement/responsibility]
EDUCATION: [Degree] | [Institution] | [Location] | [Year]
SKILL_CATEGORY: [Category]: [skills]
SPACE (for visual breaks)

Keep ALL experiences and achievements. Only optimize wording and keywords to better match the job requirements.

Original Resume:
%s

Job Description:
%s

Transform the resume to better match this job while maintaining all original content:`, resumeText, jobDescription),

		"cover_letter": fmt.Sprintf(`Write a professional cover letter using these format markers:

HEADER: [Full Name]
ADDRESS: [Email | Phone | City, State]
DATE: %s
EMPLOYER: [Hiring Manager Name or "Hiring Manager"]
EMPLOYER: [Company Name]
EMPLOYER: [Company Address if known]
SUBJECT: Re: [Position Title] Position

BODY_PARAGRAPH: [Opening paragraph - express interest and how you learned about the position]
BODY_PARAGRAPH: [Second paragraph - highlight relevant experience and achievements from resume that match job requirements]
BODY_PARAGRAPH: [Third paragraph - explain why you're interested in this company/role specifically]
BODY_PARAGRAPH: [Closing paragraph - reiterate interest and mention next steps]

CLOSING: Sincerely,
CLOSING: [Your Name]

Resume: %s

Job Description: %s

Write a compelling cover letter:`, todayDate, resumeText, jobDescription),

		"changes": fmt.Sprintf(`Analyze the resume optimization and provide a structured summary of changes made.

Format your response EXACTLY like this:

METRICS: [High-level summary with specific numbers, e.g., "Added 8 job-relevant keywords • Strengthened 12 achievement statements • Enhanced 3 skill sections"]

CHANGE: [Brief title of first major change]
BEFORE: [Original text from resume]
AFTER: [Optimized text in new resume]

CHANGE: [Brief title of second major change]
BEFORE: [Original text from resume]
AFTER: [Optimized text in new resume]

CHANGE: [Brief title of third major change]
BEFORE: [Original text from resume]
AFTER: [Optimized text in new resume]

Only include the 3-5 most impactful changes. Focus on specific text improvements, not general observations.

Original Resume:
%s

Job Requirements:
%s

Provide structured analysis:`, resumeText, jobDescription),
	}

	return prompts[contentType], systemPrompt
}

// normalizeRequest pre-processes the request to standardize path and parameters
func normalizeRequest(request *events.APIGatewayProxyRequest) {
	// Clean up path - remove trailing slash and normalize
	if len(request.Path) > 1 && strings.HasSuffix(request.Path, "/") {
		request.Path = request.Path[:len(request.Path)-1]
	}

	// Strip stage prefix if present (for API Gateway stage name)
	parts := strings.Split(request.Path, "/")
	if len(parts) > 1 && (parts[1] == "prod" || parts[1] == "stage" || parts[1] == "dev") {
		// Remove the stage name from the path
		request.Path = "/" + strings.Join(parts[2:], "/")
	}

	// Debugging request info
	log.Printf("Normalized Path: %s", request.Path)
	log.Printf("Method: %s", request.HTTPMethod)
	log.Printf("Path Parameters: %v", request.PathParameters)
	log.Printf("Query String Parameters: %v", request.QueryStringParameters)
}

// Helper function to check if a path matches exactly (ignoring trailing slash)
func matchesPath(actualPath, expectedPath string) bool {
	// Remove trailing slash if present
	if len(actualPath) > 1 && actualPath[len(actualPath)-1] == '/' {
		actualPath = actualPath[:len(actualPath)-1]
	}
	if len(expectedPath) > 1 && expectedPath[len(expectedPath)-1] == '/' {
		expectedPath = expectedPath[:len(expectedPath)-1]
	}

	return actualPath == expectedPath
}

func createResponse(statusCode int, body string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type,X-Amz-Date,Authorization,X-Api-Key",
			"Access-Control-Allow-Methods": "OPTIONS,POST,GET",
		},
		Body: body,
	}
}

// Lambda handler
func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Normalize and sanitize request
	normalizeRequest(&request)

	// Log request details for debugging
	log.Printf("Request received: Path=%s, Resource=%s, Method=%s", request.Path, request.Resource, request.HTTPMethod)

	// Always add CORS headers for better browser compatibility
	corsHeaders := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Headers": "Content-Type,X-Amz-Date,Authorization,X-Api-Key",
		"Access-Control-Allow-Methods": "OPTIONS,POST,GET",
		"Content-Type":                 "application/json",
	}

	// Handle OPTIONS requests for CORS preflight
	if request.HTTPMethod == "OPTIONS" {
		return createResponse(200, "{}"), nil
	}

	// Extract path and HTTP method for routing
	path := request.Path
	httpMethod := request.HTTPMethod

	// Resume customization endpoint
	if matchesPath(path, "/api/customize-resume") && httpMethod == "POST" {
		log.Println("Handling resume customization request")
		resp, err := handleCustomizeResume(ctx, request, corsHeaders)

		// Ensure CORS headers are always present
		if resp.Headers == nil {
			resp.Headers = make(map[string]string)
		}
		for k, v := range corsHeaders {
			resp.Headers[k] = v
		}

		return resp, err
	}

	// Health check endpoint
	if matchesPath(path, "/health") && httpMethod == "GET" {
		return createResponse(200, `{"status":"ok","service":"resume-customizer","timestamp":"`+time.Now().Format(time.RFC3339)+`"}`), nil
	}

	// Return 404 for unknown routes
	log.Printf("Unknown route: %s %s", httpMethod, path)
	return createResponse(404, `{"error":"Not found"}`), nil
}

func handleCustomizeResume(ctx context.Context, request events.APIGatewayProxyRequest, headers map[string]string) (events.APIGatewayProxyResponse, error) {
	var req CustomizeRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		log.Printf("Error parsing request: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Headers:    headers,
			Body:       `{"error": "Invalid request format"}`,
		}, nil
	}

	// Validate required fields
	if req.Resume == "" || req.JobURL == "" {
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Headers:    headers,
			Body:       `{"error": "Missing required fields: resume and jobUrl"}`,
		}, nil
	}

	// Initialize Nova service
	novaService, err := NewNovaService()
	if err != nil {
		log.Printf("Error initializing Nova service: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    headers,
			Body:       `{"error": "Failed to initialize AI service"}`,
		}, nil
	}

	// Parse resume file
	resumeText, err := parseResumeFile(req.Resume)
	if err != nil {
		log.Printf("Error parsing resume: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: 400,
			Headers:    headers,
			Body:       fmt.Sprintf(`{"error": "Failed to parse resume: %s"}`, err.Error()),
		}, nil
	}

	// Scrape job description
	jobDescription, err := scrapeJobDescription(ctx, req.JobURL)
	if err != nil {
		log.Printf("Error scraping job description: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    headers,
			Body:       `{"error": "Failed to scrape job description"}`,
		}, nil
	}

	// Generate all three outputs concurrently
	type result struct {
		content string
		err     error
	}

	resumeChan := make(chan result, 1)
	coverLetterChan := make(chan result, 1)
	changesChan := make(chan result, 1)

	go func() {
		prompt, systemPrompt := createPrompts(resumeText, jobDescription, "resume")
		content, err := novaService.GenerateContent(ctx, prompt, systemPrompt)
		resumeChan <- result{content, err}
	}()

	go func() {
		prompt, systemPrompt := createPrompts(resumeText, jobDescription, "cover_letter")
		content, err := novaService.GenerateContent(ctx, prompt, systemPrompt)
		coverLetterChan <- result{content, err}
	}()

	go func() {
		prompt, systemPrompt := createPrompts(resumeText, jobDescription, "changes")
		content, err := novaService.GenerateContent(ctx, prompt, systemPrompt)
		changesChan <- result{content, err}
	}()

	// Collect results
	resumeResult := <-resumeChan
	coverLetterResult := <-coverLetterChan
	changesResult := <-changesChan

	// Check for errors
	if resumeResult.err != nil {
		log.Printf("Error generating resume: %v", resumeResult.err)
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    headers,
			Body:       fmt.Sprintf(`{"error": "Failed to generate resume: %s"}`, resumeResult.err.Error()),
		}, nil
	}

	if coverLetterResult.err != nil {
		log.Printf("Error generating cover letter: %v", coverLetterResult.err)
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    headers,
			Body:       fmt.Sprintf(`{"error": "Failed to generate cover letter: %s"}`, coverLetterResult.err.Error()),
		}, nil
	}

	if changesResult.err != nil {
		log.Printf("Error generating changes: %v", changesResult.err)
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    headers,
			Body:       fmt.Sprintf(`{"error": "Failed to generate changes: %s"}`, changesResult.err.Error()),
		}, nil
	}

	// Extract metadata
	name := extractNameFromResume(resumeText)
	company, position := extractJobDetails(jobDescription)

	response := CustomizeResponse{
		Resume:      resumeResult.content,
		CoverLetter: coverLetterResult.content,
		Changes:     changesResult.content,
		Metadata: Metadata{
			Name:     name,
			Company:  company,
			Position: position,
		},
	}

	responseBody, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    headers,
			Body:       `{"error": "Failed to generate response"}`,
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    headers,
		Body:       string(responseBody),
	}, nil
}

func main() {
	lambda.Start(handleRequest)
}
