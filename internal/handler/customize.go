package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"resume-customizer/internal/ai"
	"resume-customizer/internal/parser"
	"resume-customizer/internal/scraper"
	"resume-customizer/internal/util"
)

// HandleCustomizeResume handles POST /api/customize-resume.
//
// The headers parameter that existed in the original code has been removed:
// the caller (handleRequest) unconditionally overwrote those headers with the
// CORS set anyway, making the parameter a no-op.  CORS headers are now applied
// at the routing layer in cmd/lambda/main.go.
func HandleCustomizeResume(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	errResp := func(statusCode int, msg string) (events.APIGatewayProxyResponse, error) {
		body, _ := json.Marshal(ErrorResponse{Error: msg})
		return events.APIGatewayProxyResponse{
			StatusCode: statusCode,
			Headers:    util.CORSHeaders,
			Body:       string(body),
		}, nil
	}

	var req CustomizeRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		log.Printf("Error parsing request: %v", err)
		return errResp(400, "Invalid request format")
	}

	if req.Resume == "" || req.JobURL == "" {
		return errResp(400, "Missing required fields: resume and jobUrl")
	}

	// Validate URL before anything else (SSRF protection)
	if err := scraper.ValidateJobURL(req.JobURL); err != nil {
		log.Printf("URL validation failed: %v", err)
		return errResp(400, fmt.Sprintf("Invalid job URL: %s", err.Error()))
	}

	novaService, err := ai.NewNovaService()
	if err != nil {
		log.Printf("Error initializing Nova service: %v", err)
		return errResp(500, "Failed to initialize AI service")
	}

	resumeText, err := parser.ParseResumeFile(req.Resume, req.FileName)
	if err != nil {
		log.Printf("Error parsing resume: %v", err)
		return errResp(400, fmt.Sprintf("Failed to parse resume: %s", err.Error()))
	}

	jobDescription, err := scraper.ScrapeJobDescription(ctx, req.JobURL)
	if err != nil {
		log.Printf("Error scraping job description: %v", err)
		return errResp(500, fmt.Sprintf(
			"Failed to scrape job description: %s. Please ensure the URL is accessible and try again.",
			err.Error()))
	}

	// Validate that we can extract job details before burning AI tokens.
	jobDetails, ok := util.ExtractJobDetails(jobDescription)
	if !ok {
		log.Printf("WARNING: could not extract job details from scraped content")
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
		log.Printf("Error generating resume: %v", resumeResult.err)
		return errResp(500, fmt.Sprintf("Failed to generate resume: %s", resumeResult.err.Error()))
	}
	if coverLetterResult.err != nil {
		log.Printf("Error generating cover letter: %v", coverLetterResult.err)
		return errResp(500, fmt.Sprintf("Failed to generate cover letter: %s", coverLetterResult.err.Error()))
	}
	if changesResult.err != nil {
		log.Printf("Error generating changes: %v", changesResult.err)
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
		log.Printf("Error marshaling response: %v", err)
		return errResp(500, "Failed to generate response")
	}

	log.Printf("Request completed successfully")
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    util.CORSHeaders,
		Body:       string(responseBody),
	}, nil
}
