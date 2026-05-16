package handler

import (
	"time"

	"github.com/aws/aws-lambda-go/events"
	"resume-customizer/internal/util"
)

// HandleHealth responds to GET /health.
func HandleHealth() events.APIGatewayProxyResponse {
	body := `{"status":"ok","service":"resume-customizer","timestamp":"` +
		time.Now().Format(time.RFC3339) + `"}`
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    util.CORSHeaders,
		Body:       body,
	}
}
