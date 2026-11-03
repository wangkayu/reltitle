package parser

import (
	"fmt"
	"strings"
	"time"

	"github.com/wangkayu/reltitle/pkg/model"
)

// dateSeparators are the single-character separators tokenize leaves between
// the parts of a calendar date.
var dateSeparators = map[string]bool{
	"-": true,
	".": true,
	"/": true,
	"_": true,
}

// extractDateAttrsAndRemoveFromTokens recognizes a release date and claims its
// tokens. Without this a bracketed date like [2020-04-14] loses only its year
// to year extraction, and the remaining 04-14 is read as an episode range.
// The year is still reported as a year attribute so year consumers are
// unaffected.
func extractDateAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if attrs == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Protected {
			continue
		}
		if text := strings.TrimSpace(tokens[i].Text); text != "" {
			if date, ok := parseCompactDate(text); ok {
				*attrs = append(*attrs, dateAttributes(date, i, i+1)...)
				claimDateTokens(tokens, i, i+1)
				continue
			}
		}
		if i+4 >= len(tokens) {
			continue
		}
		year := strings.TrimSpace(tokens[i].Text)
		month := strings.TrimSpace(tokens[i+2].Text)
		day := strings.TrimSpace(tokens[i+4].Text)
		if !dateSeparators[strings.TrimSpace(tokens[i+1].Text)] ||
			strings.TrimSpace(tokens[i+1].Text) != strings.TrimSpace(tokens[i+3].Text) {
			continue
		}
		date, ok := normalizeDate(year, month, day)
		if !ok {
			continue
		}
		*attrs = append(*attrs, dateAttributes(date, i, i+5)...)
		claimDateTokens(tokens, i, i+5)
		i += 4
	}
}

// claimDateTokens marks a date's tokens so no later stage reuses them.
// Removed keeps them out of the title; Protected stops episode/range scanning
// from reading a removed token as a range bound (the scanner deliberately
// reuses removed left bounds).
func claimDateTokens(tokens []model.Token, start, end int) {
	for i := start; i < end; i++ {
		tokens[i].Removed = true
		tokens[i].Protected = true
	}
}

func dateAttributes(date string, start, end int) []model.Attribute {
	return []model.Attribute{
		{Type: model.AttrDate, Normalized: date, TokenStart: start, TokenEnd: end, Confidence: 0.9, Source: model.AttrSourceDateToken},
		{Type: model.AttrYear, Normalized: date[:4], TokenStart: start, TokenEnd: start + 1, Confidence: 0.7, Source: model.AttrSourceYearToken},
	}
}

// parseCompactDate accepts the date shapes tokenize keeps in one token:
// 2020年4月14日, 2020年04月14日, and 20200414.
func parseCompactDate(text string) (string, bool) {
	if strings.Contains(text, "年") {
		rest, ok := strings.CutSuffix(text, "日")
		if !ok {
			return "", false
		}
		parts := strings.SplitN(rest, "年", 2)
		if len(parts) != 2 {
			return "", false
		}
		monthDay := strings.SplitN(parts[1], "月", 2)
		if len(monthDay) != 2 {
			return "", false
		}
		return normalizeDate(parts[0], monthDay[0], monthDay[1])
	}
	if len(text) == 8 && allDigits(text) {
		return normalizeDate(text[:4], text[4:6], text[6:])
	}
	return "", false
}

func normalizeDate(year, month, day string) (string, bool) {
	if !yearPattern.MatchString(year) {
		return "", false
	}
	m, ok := smallNumber(month)
	if !ok || m < 1 || m > 12 {
		return "", false
	}
	d, ok := smallNumber(day)
	if !ok || d < 1 || d > 31 {
		return "", false
	}
	t := time.Date(atoi(year), time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != atoi(year) || int(t.Month()) != m || t.Day() != d {
		return "", false
	}
	return fmt.Sprintf("%s-%02d-%02d", year, m, d), true
}

func smallNumber(s string) (int, bool) {
	if s == "" || len(s) > 2 || !allDigits(s) {
		return 0, false
	}
	return atoi(s), true
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
