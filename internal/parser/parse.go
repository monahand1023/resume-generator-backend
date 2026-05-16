package parser

import (
	"context"
	"encoding/base64"
	"fmt"

	"resume-customizer/internal/logger"
)

// ParseResumeFile decodes a base64-encoded file, detects its type, and
// returns the extracted plain text.
func ParseResumeFile(fileBase64 string, fileName string) (string, error) {
	log := logger.With(context.Background())

	fileBytes, err := base64.StdEncoding.DecodeString(fileBase64)
	if err != nil {
		log.Error("failed to decode base64 resume", "file_name", fileName, "error", err)
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	fileType := DetectFileType(fileBytes, fileName)
	log.Info("detected resume file type", "file_type", fileType, "file_name", fileName, "file_size_bytes", len(fileBytes))

	switch fileType {
	case "pdf":
		text, err := ParsePDFSimple(fileBytes)
		if err != nil {
			log.Error("failed to parse PDF", "file_name", fileName, "error", err)
		}
		return text, err
	case "docx":
		text, err := ParseDocxSimple(fileBytes)
		if err != nil {
			log.Error("failed to parse DOCX", "file_name", fileName, "error", err)
		}
		return text, err
	case "text":
		return string(fileBytes), nil
	default:
		text := string(fileBytes)
		if IsPlainText(text) {
			log.Info("treating file as plain text fallback", "file_name", fileName)
			return text, nil
		}
		log.Error("unsupported file type", "file_type", fileType, "file_name", fileName)
		return "", fmt.Errorf("unsupported file type. Please upload a PDF, Word document, or plain text file")
	}
}
