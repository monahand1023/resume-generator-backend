package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// ParseDocxSimple extracts plain text from a DOCX file supplied as raw bytes.
// DOCX files are ZIP archives; this function reads word/document.xml and walks
// the XML token stream to collect text from <w:t> elements, inserting newlines
// at paragraph boundaries (<w:p> end elements).
func ParseDocxSimple(fileBytes []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(fileBytes), int64(len(fileBytes)))
	if err != nil {
		return "", fmt.Errorf("failed to open DOCX: %w", err)
	}

	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("failed to read document.xml: %w", err)
		}
		defer rc.Close()

		var text strings.Builder
		decoder := xml.NewDecoder(rc)
		inText := false
		for {
			tok, err := decoder.Token()
			if err != nil {
				// io.EOF is the normal end-of-document signal
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "t" { // <w:t> elements contain text runs
					inText = true
				}
			case xml.EndElement:
				if t.Name.Local == "t" {
					inText = false
				}
				if t.Name.Local == "p" { // paragraph end
					text.WriteString("\n")
				}
			case xml.CharData:
				if inText {
					text.Write(t)
				}
			}
		}
		return text.String(), nil
	}
	return "", fmt.Errorf("word/document.xml not found in DOCX")
}
