package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/bedrockruntime"
	"github.com/ledongthuc/pdf"
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
		Region: aws.String("us-west-2"), // Keep your preferred region
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &NovaService{
		client:  bedrockruntime.New(sess),
		modelID: "us.amazon.nova-pro-v1:0", // Correct US inference profile ID
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

// File parsing functions
func parseResumeFile(fileBase64 string, fileName string) (string, error) {
	// Decode base64
	fileBytes, err := base64.StdEncoding.DecodeString(fileBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Simple file type detection
	fileType := detectFileType(fileBytes, fileName)

	log.Printf("Detected file type: %s for file: %s", fileType, fileName)

	switch fileType {
	case "pdf":
		return parsePDFSimple(fileBytes)
	case "docx":
		return parseDocxSimple(fileBytes)
	case "text":
		return string(fileBytes), nil
	default:
		// Fallback: try to parse as plain text if it looks like text
		text := string(fileBytes)
		if isPlainText(text) {
			log.Println("Treating file as plain text fallback")
			return text, nil
		}
		return "", fmt.Errorf("unsupported file type. Please upload a PDF, Word document, or plain text file")
	}
}

func detectFileType(fileBytes []byte, fileName string) string {
	if len(fileBytes) < 4 {
		return "unknown"
	}

	// Check PDF signature
	if fileBytes[0] == 0x25 && fileBytes[1] == 0x50 && fileBytes[2] == 0x44 && fileBytes[3] == 0x46 {
		return "pdf"
	}

	// Check ZIP signature (DOCX is a ZIP file)
	if fileBytes[0] == 0x50 && fileBytes[1] == 0x4B && (fileBytes[2] == 0x03 || fileBytes[2] == 0x05) {
		// Check if it's likely a DOCX by filename
		if strings.HasSuffix(strings.ToLower(fileName), ".docx") {
			return "docx"
		}
	}

	// Check if it looks like plain text
	if isPlainText(string(fileBytes)) {
		return "text"
	}

	return "unknown"
}

func parsePDFSimple(fileBytes []byte) (string, error) {
	reader, err := pdf.NewReader(strings.NewReader(string(fileBytes)), int64(len(fileBytes)))
	if err != nil {
		return "", fmt.Errorf("failed to create PDF reader: %w", err)
	}

	var textContent strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)

		// Check if page is valid
		if page.V.IsNull() {
			log.Printf("Warning: page %d is null, skipping", i)
			continue
		}

		// Create empty font map for GetPlainText
		fonts := make(map[string]*pdf.Font)
		text, err := page.GetPlainText(fonts)
		if err != nil {
			log.Printf("Warning: failed to extract text from page %d: %v", i, err)
			continue
		}

		textContent.WriteString(text)
		textContent.WriteString("\n")
	}

	result := textContent.String()
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("no text content found in PDF")
	}

	return result, nil
}

func parseDocxSimple(fileBytes []byte) (string, error) {
	// For now, return a helpful error message for DOCX files
	// We'll implement this properly once the core system is working
	return "", fmt.Errorf("Word document support is coming soon. Please convert your resume to PDF or save as plain text for now")
}

func isPlainText(text string) bool {
	// Simple heuristic to check if content is likely plain text
	if len(text) == 0 {
		return false
	}

	// Count printable characters
	printableCount := 0
	for _, r := range text {
		if r >= 32 && r <= 126 || r == '\n' || r == '\r' || r == '\t' {
			printableCount++
		}
	}

	// If more than 90% of characters are printable, consider it text
	return float64(printableCount)/float64(len(text)) > 0.9
}

// Web scraping function with fallback
func scrapeJobDescription(ctx context.Context, url string) (string, error) {
	log.Printf("Attempting to scrape job description from: %s", url)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request with proper headers to avoid basic bot detection
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers to mimic a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Cache-Control", "no-cache")

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch job description: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Extract text content from HTML
	text := extractTextFromHTML(string(body))

	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no text content found in the webpage")
	}

	// Clean up the extracted text
	cleanText := cleanJobDescription(text)

	log.Printf("Successfully scraped %d characters from job posting", len(cleanText))
	return cleanText, nil
}

// Simple HTML text extraction (basic implementation)
func extractTextFromHTML(html string) string {
	// Remove script and style tags
	scriptRegex := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	html = scriptRegex.ReplaceAllString(html, "")

	styleRegex := regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`)
	html = styleRegex.ReplaceAllString(html, "")

	// Remove HTML tags
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	text := tagRegex.ReplaceAllString(html, " ")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")

	// Clean up whitespace
	spaceRegex := regexp.MustCompile(`\s+`)
	text = spaceRegex.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// Clean and filter job description content
func cleanJobDescription(text string) string {
	lines := strings.Split(text, "\n")
	var cleanLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip common website navigation/footer content
		lower := strings.ToLower(line)
		if strings.Contains(lower, "cookie") ||
			strings.Contains(lower, "privacy policy") ||
			strings.Contains(lower, "terms of service") ||
			strings.Contains(lower, "sign in") ||
			strings.Contains(lower, "register") ||
			strings.Contains(lower, "follow us") ||
			len(line) < 10 {
			continue
		}

		cleanLines = append(cleanLines, line)
	}

	result := strings.Join(cleanLines, "\n")

	// Truncate if too long (keep first 8000 characters for AI processing)
	if len(result) > 8000 {
		result = result[:8000] + "\n\n[Job description truncated for processing]"
	}

	return result
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
	resumeText, err := parseResumeFile(req.Resume, req.FileName)
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
		// For Phase 1, we'll return an error, but in Phase 2 we could fallback to asking user to paste manually
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    headers,
			Body:       fmt.Sprintf(`{"error": "Failed to scrape job description: %s. Please ensure the URL is accessible and try again."}`, err.Error()),
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
