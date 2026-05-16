package parser

import (
	"encoding/base64"
	"fmt"
	"log"
)

// ParseResumeFile decodes a base64-encoded file, detects its type, and
// returns the extracted plain text.
func ParseResumeFile(fileBase64 string, fileName string) (string, error) {
	fileBytes, err := base64.StdEncoding.DecodeString(fileBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	fileType := DetectFileType(fileBytes, fileName)
	log.Printf("Detected file type: %s for file: %s", fileType, fileName)

	switch fileType {
	case "pdf":
		return ParsePDFSimple(fileBytes)
	case "docx":
		return ParseDocxSimple(fileBytes)
	case "text":
		return string(fileBytes), nil
	default:
		text := string(fileBytes)
		if IsPlainText(text) {
			log.Println("Treating file as plain text fallback")
			return text, nil
		}
		return "", fmt.Errorf("unsupported file type. Please upload a PDF, Word document, or plain text file")
	}
}
