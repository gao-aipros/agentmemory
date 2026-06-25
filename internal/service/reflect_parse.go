package service

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	insightBlockRe   = regexp.MustCompile(`<insight\s+(.*?)>(.*?)</insight>`)
	confidenceAttrRe = regexp.MustCompile(`confidence="([^"]*)"`)
	titleAttrRe      = regexp.MustCompile(`title="([^"]*)"`)
)

// ParsedInsight represents a single parsed insight from a reflect response.
type ParsedInsight struct {
	Confidence float64
	Title      string
	Content    string
}

// ParseReflectResponse extracts structured ParsedInsight values from an
// XML-formatted LLM reflect response, using a two-pass regex approach.
func ParseReflectResponse(response string) []ParsedInsight {
	blocks := insightBlockRe.FindAllStringSubmatch(response, -1)
	if len(blocks) == 0 {
		return nil
	}

	insights := make([]ParsedInsight, 0, len(blocks))
	for _, m := range blocks {
		attrStr := m[1]
		content := strings.TrimSpace(m[2])
		if content == "" {
			continue
		}

		titleMatch := titleAttrRe.FindStringSubmatch(attrStr)
		if len(titleMatch) < 2 {
			continue
		}
		title := strings.TrimSpace(titleMatch[1])
		if title == "" || len(title) > 60 {
			continue
		}

		confMatch := confidenceAttrRe.FindStringSubmatch(attrStr)
		if len(confMatch) < 2 {
			continue
		}
		confidence, err := strconv.ParseFloat(confMatch[1], 64)
		if err != nil || confidence < 0.0 || confidence > 1.0 {
			continue
		}

		insights = append(insights, ParsedInsight{
			Confidence: confidence,
			Title:      title,
			Content:    content,
		})
	}
	return insights
}
