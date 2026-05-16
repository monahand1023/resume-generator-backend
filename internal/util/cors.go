package util

// CORSHeaders returns the standard CORS headers used across all responses.
var CORSHeaders = map[string]string{
	"Content-Type":                 "application/json",
	"Access-Control-Allow-Origin":  "*",
	"Access-Control-Allow-Headers": "Content-Type,X-Amz-Date,Authorization,X-Api-Key",
	"Access-Control-Allow-Methods": "OPTIONS,POST,GET",
}
