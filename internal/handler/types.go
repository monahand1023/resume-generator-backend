package handler

// CustomizeRequest is the JSON body accepted by POST /api/customize-resume.
type CustomizeRequest struct {
	Resume   string `json:"resume"`   // Base64-encoded resume file
	JobURL   string `json:"jobUrl"`   // URL of the job posting to scrape
	FileName string `json:"fileName"` // Original file name (used for type detection)
}

// CustomizeResponse is returned on success.
//
// Each text field uses a line-oriented marker format understood by the
// frontend renderer:
//
// resume     — NAME:, CONTACT:, SECTION:, SUMMARY_TEXT:, COMPANY:, TITLE:,
//              BULLET:, EDUCATION:, SKILL_CATEGORY:
// coverLetter — HEADER:, ADDRESS:, DATE:, EMPLOYER:, SUBJECT:,
//              BODY_PARAGRAPH:, CLOSING:
// changes    — METRICS:, CHANGE:, BEFORE:, AFTER:
type CustomizeResponse struct {
	Resume      string   `json:"resume"`
	CoverLetter string   `json:"coverLetter"`
	Changes     string   `json:"changes"`
	Metadata    Metadata `json:"metadata"`
}

// Metadata carries applicant and role information extracted from the inputs.
type Metadata struct {
	Name     string `json:"name"`
	Company  string `json:"company"`
	Position string `json:"position"`
}

// ErrorResponse wraps a human-readable error message for HTTP error responses.
type ErrorResponse struct {
	Error string `json:"error"`
}
