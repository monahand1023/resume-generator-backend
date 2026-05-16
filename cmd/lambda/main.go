package main

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"resume-customizer/internal/handler"
	"resume-customizer/internal/util"
)

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	request = util.NormalizeRequest(request)

	log.Printf("Request received: Path=%s, Resource=%s, Method=%s",
		request.Path, request.Resource, request.HTTPMethod)

	// CORS preflight
	if request.HTTPMethod == "OPTIONS" {
		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Headers:    util.CORSHeaders,
			Body:       "{}",
		}, nil
	}

	path := request.Path
	method := request.HTTPMethod

	// POST /api/customize-resume
	if util.MatchesPath(path, "/api/customize-resume") && method == "POST" {
		log.Println("Handling resume customization request")
		resp, err := handler.HandleCustomizeResume(ctx, request)
		// Ensure CORS headers are always present, even on errors from the handler
		if resp.Headers == nil {
			resp.Headers = make(map[string]string)
		}
		for k, v := range util.CORSHeaders {
			resp.Headers[k] = v
		}
		return resp, err
	}

	// GET /health
	if util.MatchesPath(path, "/health") && method == "GET" {
		return handler.HandleHealth(), nil
	}

	log.Printf("Unknown route: %s %s", method, path)
	return events.APIGatewayProxyResponse{
		StatusCode: 404,
		Headers:    util.CORSHeaders,
		Body:       `{"error":"Not found"}`,
	}, nil
}

func init() {
	// Emit a startup log so CloudWatch shows when the Lambda cold-starts.
	log.Printf("resume-customizer Lambda starting at %s", time.Now().Format(time.RFC3339))
}

func main() {
	lambda.Start(handleRequest)
}
