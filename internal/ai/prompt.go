package ai

import (
	"fmt"
	"time"
)

func getTodayDate() string {
	now := time.Now()
	months := []string{
		"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December",
	}
	return fmt.Sprintf("%s %d, %d", months[now.Month()-1], now.Day(), now.Year())
}

// CreatePrompts returns the user prompt and system prompt for the given
// contentType ("resume", "cover_letter", or "changes").
func CreatePrompts(resumeText, jobDescription, contentType string) (string, string) {
	todayDate := getTodayDate()

	systemPrompt := "You are an expert resume writer. Create professional, ATS-friendly content that matches job requirements. Be concise and focused."

	prompts := map[string]string{
		"resume": fmt.Sprintf(`Optimize this resume for the job posting. Use this EXACT format:

NAME: [Full Name]
CONTACT: [Email | Phone | LinkedIn | Location]
SECTION: [SECTION NAME]
SUMMARY_TEXT: [Brief professional summary]
COMPANY: [Company] | [Location] | [Dates]
TITLE: [Job Title]
BULLET: • [Achievement with metrics]
EDUCATION: [Degree] | [School] | [Year]
SKILL_CATEGORY: [Category]: [skills]

Keep all experiences. Only optimize wording for keywords and impact.

Resume:
%s

Job Requirements:
%s

Optimize for this role:`, resumeText, jobDescription),

		"cover_letter": fmt.Sprintf(`Write a cover letter using these markers:

HEADER: [Name]
ADDRESS: [Email | Phone | City, State]
DATE: %s
EMPLOYER: Hiring Manager
EMPLOYER: [Company]
SUBJECT: Re: [Position] Position

BODY_PARAGRAPH: [Opening - express interest]
BODY_PARAGRAPH: [Relevant experience matching job requirements]
BODY_PARAGRAPH: [Why this company/role]
BODY_PARAGRAPH: [Closing with next steps]

CLOSING: Sincerely,
CLOSING: [Name]

Resume: %s
Job: %s`, todayDate, resumeText, jobDescription),

		"changes": fmt.Sprintf(`Analyze resume optimization. Format:

METRICS: [Summary with numbers, e.g., "Added 5 keywords • Enhanced 8 bullets • Strengthened 3 sections"]

CHANGE: [Change title]
BEFORE: [Original text]
AFTER: [Optimized text]

CHANGE: [Change title]
BEFORE: [Original text]
AFTER: [Optimized text]

CHANGE: [Change title]
BEFORE: [Original text]
AFTER: [Optimized text]

Show 3 most impactful changes only.

Original: %s
Job: %s`, resumeText, jobDescription),
	}

	return prompts[contentType], systemPrompt
}
