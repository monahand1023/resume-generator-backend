package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"resume-customizer/internal/ai"
	"resume-customizer/internal/logger"
	"resume-customizer/internal/parser"
	"resume-customizer/internal/scraper"
	"resume-customizer/internal/util"
)

// ContentGenerator is the narrow interface satisfied by *ai.NovaService and
// any test double. It is the only AI method HandleCustomizeResume calls.
type ContentGenerator interface {
	GenerateContent(ctx context.Context, prompt, systemPrompt string) (string, error)
}

// scraperFunc matches the signature of scraper.ScrapeJobDescription so tests
// can supply a stub without network access.
type scraperFunc func(ctx context.Context, jobURL string) (string, error)

// urlValidatorFunc matches the signature of scraper.ValidateJobURL so tests
// can supply a no-op validator that does not perform DNS resolution.
type urlValidatorFunc func(rawURL string) error

// HandleCustomizeResume handles POST /api/customize-resume.
//
// The headers parameter that existed in the original code has been removed:
// the caller (handleRequest) unconditionally overwrote those headers with the
// CORS set anyway, making the parameter a no-op.  CORS headers are now applied
// at the routing layer in cmd/lambda/main.go.
func HandleCustomizeResume(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	novaService, err := ai.NewNovaService()
	if err != nil {
		logger.With(ctx).Error("failed to initialize Nova service", "error", err)
		body, _ := json.Marshal(ErrorResponse{Error: "Failed to initialize AI service"})
		return events.APIGatewayProxyResponse{
			StatusCode: 500,
			Headers:    util.CORSHeaders,
			Body:       string(body),
		}, nil
	}
	return handleCustomizeResume(ctx, request, novaService, scraper.ScrapeJobDescription, scraper.ValidateJobURL)
}

// handleCustomizeResume is the testable core of HandleCustomizeResume.
// Dependencies (AI generator, scraper, and URL validator) are injected so
// tests can stub them without network access.
func handleCustomizeResume(
	ctx context.Context,
	request events.APIGatewayProxyRequest,
	novaService ContentGenerator,
	scrape scraperFunc,
	validateURL urlValidatorFunc,
) (events.APIGatewayProxyResponse, error) {
	errResp := func(statusCode int, msg string) (events.APIGatewayProxyResponse, error) {
		body, _ := json.Marshal(ErrorResponse{Error: msg})
		return events.APIGatewayProxyResponse{
			StatusCode: statusCode,
			Headers:    util.CORSHeaders,
			Body:       string(body),
		}, nil
	}

	log := logger.With(ctx)

	var req CustomizeRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		log.Error("failed to parse request body", "error", err)
		return errResp(400, "Invalid request format")
	}

	log.Info("customize resume request", "job_url", req.JobURL, "file_name", req.FileName)

	if req.Resume == "" || req.JobURL == "" {
		return errResp(400, "Missing required fields: resume and jobUrl")
	}

	// Validate URL before anything else (SSRF protection)
	if err := validateURL(req.JobURL); err != nil {
		log.Error("URL validation failed", "job_url", req.JobURL, "error", err)
		return errResp(400, fmt.Sprintf("Invalid job URL: %s", err.Error()))
	}

	resumeText, err := parser.ParseResumeFile(req.Resume, req.FileName)
	if err != nil {
		log.Error("failed to parse resume", "file_name", req.FileName, "error", err)
		return errResp(400, fmt.Sprintf("Failed to parse resume: %s", err.Error()))
	}
	log.Info("resume parsed successfully", "file_name", req.FileName, "text_length", len(resumeText))

	jobDescription, err := scrape(ctx, req.JobURL)
	if err != nil {
		log.Error("failed to scrape job description", "job_url", req.JobURL, "error", err)
		return errResp(500, fmt.Sprintf(
			"Failed to scrape job description: %s. Please ensure the URL is accessible and try again.",
			err.Error()))
	}
	log.Info("job description scraped", "job_url", req.JobURL, "text_length", len(jobDescription))

	// Validate that we can extract job details before burning AI tokens.
	jobDetails, ok := util.ExtractJobDetails(jobDescription)
	if !ok {
		log.Warn("could not extract job details from scraped content", "job_url", req.JobURL)
		return errResp(422,
			"Could not extract job details from the provided content. "+
				"Please ensure the job description includes a company name and job title.")
	}

	// Generate resume, cover letter, and change summary concurrently.
	type result struct {
		content string
		err     error
	}

	resumeChan := make(chan result, 1)
	coverLetterChan := make(chan result, 1)
	changesChan := make(chan result, 1)

	go func() {
		prompt, systemPrompt := ai.CreatePrompts(resumeText, jobDescription, "resume")
		content, err := novaService.GenerateContent(ctx, prompt, systemPrompt)
		resumeChan <- result{content, err}
	}()

	go func() {
		prompt, systemPrompt := ai.CreatePrompts(resumeText, jobDescription, "cover_letter")
		content, err := novaService.GenerateContent(ctx, prompt, systemPrompt)
		coverLetterChan <- result{content, err}
	}()

	go func() {
		prompt, systemPrompt := ai.CreatePrompts(resumeText, jobDescription, "changes")
		content, err := novaService.GenerateContent(ctx, prompt, systemPrompt)
		changesChan <- result{content, err}
	}()

	resumeResult := <-resumeChan
	coverLetterResult := <-coverLetterChan
	changesResult := <-changesChan

	if resumeResult.err != nil {
		log.Error("failed to generate resume", "error", resumeResult.err)
		return errResp(500, fmt.Sprintf("Failed to generate resume: %s", resumeResult.err.Error()))
	}
	if coverLetterResult.err != nil {
		log.Error("failed to generate cover letter", "error", coverLetterResult.err)
		return errResp(500, fmt.Sprintf("Failed to generate cover letter: %s", coverLetterResult.err.Error()))
	}
	if changesResult.err != nil {
		log.Error("failed to generate changes", "error", changesResult.err)
		return errResp(500, fmt.Sprintf("Failed to generate changes: %s", changesResult.err.Error()))
	}

	name := util.ExtractNameFromResume(resumeText)

	response := CustomizeResponse{
		Resume:      resumeResult.content,
		CoverLetter: coverLetterResult.content,
		Changes:     changesResult.content,
		Metadata: Metadata{
			Name:     name,
			Company:  jobDetails.Company,
			Position: jobDetails.Position,
		},
	}

	responseBody, err := json.Marshal(response)
	if err != nil {
		log.Error("failed to marshal response", "error", err)
		return errResp(500, "Failed to generate response")
	}

	log.Info("request completed successfully",
		"job_url", req.JobURL,
		"company", jobDetails.Company,
		"position", jobDetails.Position,
	)
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    util.CORSHeaders,
		Body:       string(responseBody),
	}, nil
}
