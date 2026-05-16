package main

import (
	"context"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"resume-customizer/internal/handler"
	"resume-customizer/internal/logger"
	"resume-customizer/internal/util"
)

func handleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	request = util.NormalizeRequest(request)

	logger.With(ctx).Info("request received",
		"path", request.Path,
		"resource", request.Resource,
		"method", request.HTTPMethod,
	)

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
		logger.With(ctx).Info("handling resume customization request")
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

	logger.With(ctx).Warn("unknown route", "method", method, "path", path)
	return events.APIGatewayProxyResponse{
		StatusCode: 404,
		Headers:    util.CORSHeaders,
		Body:       `{"error":"Not found"}`,
	}, nil
}

func init() {
	logger.Init("resume-customizer")
	// Emit a startup log so CloudWatch shows when the Lambda cold-starts.
	logger.Logger.Info("resume-customizer Lambda starting", "time", time.Now().Format(time.RFC3339))
}

func main() {
	lambda.Start(handleRequest)
}
