package parser

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ParsePDFSimple extracts plain text from a PDF supplied as raw bytes.
func ParsePDFSimple(fileBytes []byte) (string, error) {
	// bytes.NewReader is correct for binary PDF data — avoids an unnecessary
	// binary→string→reader round-trip that was present in the original code.
	reader, err := pdf.NewReader(bytes.NewReader(fileBytes), int64(len(fileBytes)))
	if err != nil {
		return "", fmt.Errorf("failed to create PDF reader: %w", err)
	}

	var textContent strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)

		if page.V.IsNull() {
			log.Printf("Warning: page %d is null, skipping", i)
			continue
		}

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
