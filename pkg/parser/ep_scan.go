package parser

import (
	"regexp"
	"strconv"
	"strings"

	"reltitle/pkg/model"
)

// epScanSkipped reports whether token i must be skipped by episode/season/range
// scanning. Protected tokens belong to a known title span (title dictionary or
// structural guard) and are never episode signals; Removed tokens were already
// consumed by an earlier pass. Every scanning loop in this domain MUST use this
// guard so title digits are never reinterpreted as episodes.
func epScanSkipped(tokens []model.Token, i int) bool {
	return i < 0 || i >= len(tokens) || tokens[i].Protected || tokens[i].Removed
}

func detectSxxExx(tokens []model.Token) (season string, episode string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if epScanSkipped(tokens, i) {
			continue
		}
		m := sxxExxPattern.FindStringSubmatch(tokens[i].Text)
		if len(m) == 3 {
			season = strings.TrimLeft(m[1], "0")
			if season == "" {
				season = "0"
			}
			episode = strings.TrimLeft(m[2], "0")
			if episode == "" {
				episode = "0"
			}
			return season, episode, i
		}
	}
	return "", "", -1
}

func detectSeasonKeyword(tokens []model.Token) (season string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		text := strings.TrimSpace(tokens[i].Text)
		if text == "" {
			continue
		}
		if v := extractSeasonKeywordValue(text); v != "" {
			return v, i
		}
		if seasonWordLoosePattern.MatchString(text) {
			if i+1 < len(tokens) {
				next := strings.TrimSpace(tokens[i+1].Text)
				if v := parseSmallSeasonNumberToken(next); v != "" {
					return v, i
				}
			}
		}
		if m := seasonOrdinalPattern.FindStringSubmatch(text); len(m) == 2 {
			if i+1 < len(tokens) && seasonWordPattern.MatchString(strings.TrimSpace(tokens[i+1].Text)) {
				return strings.TrimLeft(m[1], "0"), i
			}
		}
	}
	return "", -1
}

func detectSeasonKeywordAny(tokens []model.Token) (season string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if v := extractSeasonKeywordValue(tokens[i].Text); v != "" {
			return v, i
		}
	}
	return "", -1
}

func extractSeasonKeywordValue(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if m := reExtractSeasonKeywordValue1.FindStringSubmatch(text); len(m) == 2 {
		if v := parseSmallSeasonNumberToken(m[1]); v != "" {
			return v
		}
	}
	if m := reExtractSeasonKeywordValue2.FindStringSubmatch(text); len(m) == 2 {
		return strings.TrimLeft(m[1], "0")
	}
	if m := reExtractSeasonKeywordValue3.FindStringSubmatch(text); len(m) == 2 {
		if v := parseSmallSeasonNumberToken(m[1]); v != "" {
			return v
		}
	}
	if m := reExtractSeasonKeywordValue4.FindStringSubmatch(text); len(m) == 2 {
		if v := parseSmallSeasonNumberToken(m[1]); v != "" {
			return v
		}
	}
	if m := seasonCJKPattern.FindStringSubmatch(text); len(m) == 2 {
		if v := parseSmallSeasonNumberToken(m[1]); v != "" {
			return v
		}
	}
	return ""
}

var episodeRangePattern = regexp.MustCompile(`(?i)^(\d{1,4})[-~](\d{1,4})$`)
var episodeRangeEmbeddedPattern = regexp.MustCompile(`(?i)(\d{1,4})[-~](\d{1,4})`)
var sxxExxEpisodeRangePattern = regexp.MustCompile(`(?i)^s\d{1,2}e(\d{1,4})[-~]e?(\d{1,4})$`)
var sxxExxLeftPattern = regexp.MustCompile(`(?i)^s(\d{1,2})e(\d{1,4})$`)
var episodeRangeLeftCJKPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})$`)
var episodeRangeRightCJKPattern = regexp.MustCompile(`^(\d{1,4})(?:集|话|話|回|章)?$`)
var epPrefixedDigitsPattern = regexp.MustCompile(`(?i)^e(?:p)?(\d{1,4})$`)
var episodeOfTotalPattern = regexp.MustCompile(`^(\d{1,4})$`)
var leadingDigitsPattern = regexp.MustCompile(`^(\d{1,4})`)

func isTitlePrefixedDigitRangeInBracket(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	if !isInBracket(tokens, idx) {
		return false
	}
	// Detect patterns like: [ 藤本树 17 - 26 ] where 17-26 is part of the title.
	if idx+2 >= len(tokens) {
		return false
	}
	if !episodeOfTotalPattern.MatchString(strings.TrimSpace(tokens[idx].Text)) {
		return false
	}
	if strings.TrimSpace(tokens[idx+1].Text) != "-" {
		return false
	}
	if !episodeOfTotalPattern.MatchString(strings.TrimSpace(tokens[idx+2].Text)) {
		return false
	}
	// If there is meaningful title text before the range within the same bracket block,
	// treat this range as title suffix, not episode_range.
	openIdx := -1
	for j := idx; j >= 0; j-- {
		if tokens[j].Text == "[" || tokens[j].Text == "【" {
			openIdx = j
			break
		}
	}
	if openIdx < 0 {
		return false
	}
	for j := idx - 1; j > openIdx; j-- {
		t := strings.TrimSpace(tokens[j].Text)
		if t == "" {
			continue
		}
		if t == "-" || t == "~" {
			continue
		}
		if episodeOfTotalPattern.MatchString(t) {
			continue
		}
		// Anything that is not pure digits/separators is treated as title prefix.
		// Do NOT skip Removed tokens here: a preceding title block (e.g. "[藤本樹 17-26]")
		// may already have been consumed as title/alias by an earlier pass, and its
		// tokens are exactly the title-text evidence this guard needs.
		return true
	}
	return false
}

func detectEpisodeRange(tokens []model.Token) (string, int) {
	// Pattern: S01E08-E09 / S01E08-09 / S01E08~E09
	for i := 0; i < len(tokens); i++ {
		text := strings.TrimSpace(tokens[i].Text)
		if m := sxxExxEpisodeRangePattern.FindStringSubmatch(text); len(m) == 3 {
			l := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if l == "" {
				l = "0"
			}
			r := strings.TrimLeft(strings.TrimSpace(m[2]), "0")
			if r == "" {
				r = "0"
			}
			if l != "0" && r != "0" {
				return l + "-" + r, i
			}
		}
	}
	for i := 0; i+2 < len(tokens); i++ {
		// The left SxxExx token may already be removed by season extraction.
		// Keep matching this range pattern as long as separator/right token remain visible.
		if tokens[i+1].Removed || tokens[i+2].Removed {
			continue
		}
		left := strings.TrimSpace(tokens[i].Text)
		sep := strings.TrimSpace(tokens[i+1].Text)
		right := strings.TrimSpace(tokens[i+2].Text)
		if sep != "-" && sep != "~" {
			continue
		}
		lm := sxxExxLeftPattern.FindStringSubmatch(left)
		if len(lm) != 3 {
			continue
		}
		if rm := epPrefixedDigitsPattern.FindStringSubmatch(strings.ToLower(right)); len(rm) == 2 {
			right = strings.TrimSpace(rm[1])
		}
		if !episodeOfTotalPattern.MatchString(right) {
			continue
		}
		l := strings.TrimLeft(strings.TrimSpace(lm[2]), "0")
		if l == "" {
			l = "0"
		}
		r := strings.TrimLeft(strings.TrimSpace(right), "0")
		if r == "" {
			r = "0"
		}
		if l != "0" && r != "0" {
			return l + "-" + r, i
		}
	}

	// Pattern: Episode 27-40 / EP 01-12
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if isInBracket(tokens, i) {
			continue
		}
		kw := strings.TrimSpace(tokens[i].Text)
		if !(strings.EqualFold(kw, "episode") || strings.EqualFold(kw, "ep")) {
			continue
		}
		if i+1 >= len(tokens) {
			continue
		}
		if m := episodeRangePattern.FindStringSubmatch(strings.TrimSpace(tokens[i+1].Text)); len(m) == 3 {
			l := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if l == "" {
				l = "0"
			}
			r := strings.TrimLeft(strings.TrimSpace(m[2]), "0")
			if r == "" {
				r = "0"
			}
			return l + "-" + r, i + 1
		}
		if m := episodeRangeEmbeddedPattern.FindStringSubmatch(strings.TrimSpace(tokens[i+1].Text)); len(m) == 3 {
			l := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if l == "" {
				l = "0"
			}
			r := strings.TrimLeft(strings.TrimSpace(m[2]), "0")
			if r == "" {
				r = "0"
			}
			return l + "-" + r, i + 1
		}
		if i+3 < len(tokens) {
			lraw := strings.TrimSpace(tokens[i+1].Text)
			sep := strings.TrimSpace(tokens[i+2].Text)
			rraw := strings.TrimSpace(tokens[i+3].Text)
			if episodeOfTotalPattern.MatchString(lraw) && (sep == "-" || sep == "~") {
				if !episodeOfTotalPattern.MatchString(rraw) {
					if mm := leadingDigitsPattern.FindStringSubmatch(rraw); len(mm) == 2 {
						rraw = strings.TrimSpace(mm[1])
					}
				}
			}
			if episodeOfTotalPattern.MatchString(lraw) && (sep == "-" || sep == "~") && episodeOfTotalPattern.MatchString(rraw) {
				l := strings.TrimLeft(lraw, "0")
				if l == "" {
					l = "0"
				}
				r := strings.TrimLeft(rraw, "0")
				if r == "" {
					r = "0"
				}
				if l != "0" && r != "0" {
					return l + "-" + r, i + 1
				}
			}
		}
	}

	// Pattern: "01 a 12" / "01 à 12" in plain text.
	// We normalize it into a single token "01-12" to avoid leaving connector + right bound as tail noise.
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i].Removed || tokens[i+1].Removed || tokens[i+2].Removed {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		lraw := strings.TrimSpace(tokens[i].Text)
		mid := strings.TrimSpace(tokens[i+1].Text)
		rraw := strings.TrimSpace(tokens[i+2].Text)
		if !episodeOfTotalPattern.MatchString(lraw) || !episodeOfTotalPattern.MatchString(rraw) {
			continue
		}
		if !isEpisodeRangeWordConnector(mid) {
			continue
		}
		l := strings.TrimLeft(lraw, "0")
		if l == "" {
			l = "0"
		}
		r := strings.TrimLeft(rraw, "0")
		if r == "" {
			r = "0"
		}
		if l != "0" && r != "0" {
			tokens[i].Text = l + "-" + r
			tokens[i+1].Removed = true
			tokens[i+2].Removed = true
			return l + "-" + r, i
		}
	}

	for i := 0; i+2 < len(tokens); i++ {
		left := strings.TrimSpace(tokens[i].Text)
		if left == "" {
			continue
		}
		m := epPrefixedDigitsPattern.FindStringSubmatch(left)
		if len(m) != 2 {
			continue
		}
		if strings.TrimSpace(tokens[i+1].Text) != "-" {
			continue
		}
		rawR := strings.TrimSpace(tokens[i+2].Text)
		if rm := epPrefixedDigitsPattern.FindStringSubmatch(strings.ToLower(rawR)); len(rm) == 2 {
			rawR = strings.TrimSpace(rm[1])
		}
		if !episodeOfTotalPattern.MatchString(rawR) {
			continue
		}
		l := strings.TrimLeft(m[1], "0")
		if l == "" {
			l = "0"
		}
		r := strings.TrimLeft(rawR, "0")
		if r == "" {
			r = "0"
		}
		if len(m[1]) == 4 && len(rawR) == 4 {
			if y1, err1 := strconv.Atoi(m[1]); err1 == nil {
				if y2, err2 := strconv.Atoi(rawR); err2 == nil {
					if y1 >= 1900 && y1 <= 2099 && y2 >= 1900 && y2 <= 2099 {
						continue
					}
				}
			}
		}
		return l + "-" + r, i
	}

	for i := 0; i < len(tokens); i++ {
		if tokens[i].Protected {
			// Tokens inside a known-title span (title_knowledge.yaml or structural
			// guard) are part of the title itself; never reinterpret their numbers
			// as an episode range.
			continue
		}
		if tokens[i].Removed {
			if i-1 >= 0 {
				prevRaw := strings.TrimSpace(tokens[i-1].Text)
				if seasonWordLoosePattern.MatchString(prevRaw) {
					continue
				}
			}
			// Single-episode extraction can consume the left bound before range detection
			// runs, e.g. "Prism Rondo - 01 ~ 20 ...". Preserve detection when the removed
			// token is still the left edge of an explicit numeric range.
			if i+2 >= len(tokens) {
				continue
			}
			leftRaw := strings.TrimSpace(tokens[i].Text)
			sep := strings.TrimSpace(tokens[i+1].Text)
			if hasPartIndexLeadBeforeToken(tokens, i) {
				continue
			}
			if !episodeOfTotalPattern.MatchString(leftRaw) {
				continue
			}
			if sep != "-" && sep != "~" && !isEpisodeRangeWordConnector(sep) {
				continue
			}
		}
		inBracket := isInBracket(tokens, i)
		inParen := isInParen(tokens, i)
		if m := episodeCountPackSuffixPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(m) == 3 {
			n := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if n == "" {
				n = "0"
			}
			if n != "0" {
				return "1-" + n, i
			}
		}
		if inBracket {
			if txt := bracketBlockInnerText(tokens, i); txt != "" {
				if isLikelyMetaBracketText(txt) && !isEpisodeLikeBracketText(txt) {
					continue
				}
				if m := reDetectEpisodeRange1.FindStringSubmatch(strings.TrimSpace(txt)); len(m) == 3 {
					l := strings.TrimLeft(m[1], "0")
					if l == "" {
						l = "0"
					}
					r := strings.TrimLeft(m[2], "0")
					if r == "" {
						r = "0"
					}
					if l != "0" && r != "0" {
						return l + "-" + r, i
					}
				}
			}
		}
		if inBracket {
			// Guard: avoid extracting episode_range from bracket content when the range is preceded
			// by title text (e.g. "[藤本树 17-26]").
			if isTitlePrefixedDigitRangeInBracket(tokens, i) {
				continue
			}
		}
		if inParen {
			// Guard: avoid extracting episode_range from paren content when the range is preceded
			// by title text (e.g. "(藤本树 17-26)").
			if isTitlePrefixedDigitRangeInParen(tokens, i) {
				continue
			}
		}
		if episodeRangePattern.MatchString(tokens[i].Text) {
			if hasPartIndexLeadBeforeToken(tokens, i) {
				continue
			}
			m := episodeRangePattern.FindStringSubmatch(tokens[i].Text)
			if len(m) == 3 {
				if len(m[1]) == 4 && len(m[2]) == 4 {
					if y1, err1 := strconv.Atoi(m[1]); err1 == nil {
						if y2, err2 := strconv.Atoi(m[2]); err2 == nil {
							if y1 >= 1900 && y1 <= 2099 && y2 >= 1900 && y2 <= 2099 {
								continue
							}
						}
					}
				}
			}
			return normalizeEpisodeRangeText(tokens[i].Text), i
		}
		if m := episodeRangeEmbeddedPattern.FindStringSubmatch(tokens[i].Text); len(m) == 3 {
			if hasPartIndexLeadBeforeToken(tokens, i) {
				continue
			}
			if len(m[1]) == 4 && len(m[2]) == 4 {
				if y1, err1 := strconv.Atoi(m[1]); err1 == nil {
					if y2, err2 := strconv.Atoi(m[2]); err2 == nil {
						if y1 >= 1900 && y1 <= 2099 && y2 >= 1900 && y2 <= 2099 {
							continue
						}
					}
				}
			}
			return normalizeEpisodeRangeText(m[1] + "-" + m[2]), i
		}
		if i+2 < len(tokens) {
			leftRaw := strings.TrimSpace(tokens[i].Text)
			sep := strings.TrimSpace(tokens[i+1].Text)
			rightRaw := strings.TrimSpace(tokens[i+2].Text)
			if sep == "-" || sep == "~" {
				leftHasMarker := strings.HasPrefix(leftRaw, "第")
				rightHasMarker := strings.HasSuffix(rightRaw, "集") || strings.HasSuffix(rightRaw, "话") || strings.HasSuffix(rightRaw, "話") || strings.HasSuffix(rightRaw, "回") || strings.HasSuffix(rightRaw, "章")
				if leftHasMarker || rightHasMarker {
					lraw := leftRaw
					if lm := episodeRangeLeftCJKPattern.FindStringSubmatch(lraw); len(lm) == 2 {
						lraw = strings.TrimSpace(lm[1])
					}
					rraw := rightRaw
					if rm := episodeRangeRightCJKPattern.FindStringSubmatch(rraw); len(rm) == 2 {
						rraw = strings.TrimSpace(rm[1])
					} else if mm := leadingDigitsPattern.FindStringSubmatch(rraw); len(mm) == 2 {
						rraw = strings.TrimSpace(mm[1])
					}
					if episodeOfTotalPattern.MatchString(lraw) && episodeOfTotalPattern.MatchString(rraw) {
						l := strings.TrimLeft(lraw, "0")
						if l == "" {
							l = "0"
						}
						r := strings.TrimLeft(rraw, "0")
						if r == "" {
							r = "0"
						}
						if l != "0" && r != "0" {
							if len(lraw) == 4 && len(rraw) == 4 {
								if y1, err1 := strconv.Atoi(lraw); err1 == nil {
									if y2, err2 := strconv.Atoi(rraw); err2 == nil {
										if y1 >= 1900 && y1 <= 2099 && y2 >= 1900 && y2 <= 2099 {
											continue
										}
									}
								}
							}
							return l + "-" + r, i
						}
					}
				}
			}
			sep = strings.TrimSpace(tokens[i+1].Text)
			if episodeOfTotalPattern.MatchString(strings.TrimSpace(tokens[i].Text)) && (sep == "-" || sep == "~" || isEpisodeRangeWordConnector(sep)) {
				if hasPartIndexLeadBeforeToken(tokens, i) {
					continue
				}
				rawR := strings.TrimSpace(tokens[i+2].Text)
				if episodeOfTotalPattern.MatchString(rawR) {
					// Plain text range like "... - 241-248 ..."
					// Keep a year guard to avoid interpreting year spans as episodes.
					if !inBracket && !inParen {
						leftRaw := strings.TrimSpace(tokens[i].Text)
						if len(leftRaw) == 4 && len(rawR) == 4 {
							if y1, err1 := strconv.Atoi(leftRaw); err1 == nil {
								if y2, err2 := strconv.Atoi(rawR); err2 == nil {
									if y1 >= 1900 && y1 <= 2099 && y2 >= 1900 && y2 <= 2099 {
										continue
									}
								}
							}
						}
						l := strings.TrimLeft(leftRaw, "0")
						if l == "" {
							l = "0"
						}
						r := strings.TrimLeft(rawR, "0")
						if r == "" {
							r = "0"
						}
						return l + "-" + r, i
					}
					if inBracket {
						if isTitlePrefixedDigitRangeInBracket(tokens, i) {
							continue
						}
						l := strings.TrimLeft(strings.TrimSpace(tokens[i].Text), "0")
						if l == "" {
							l = "0"
						}
						r := strings.TrimLeft(rawR, "0")
						if r == "" {
							r = "0"
						}
						if len(strings.TrimSpace(tokens[i].Text)) == 4 && len(rawR) == 4 {
							if y1, err1 := strconv.Atoi(strings.TrimSpace(tokens[i].Text)); err1 == nil {
								if y2, err2 := strconv.Atoi(rawR); err2 == nil {
									if y1 >= 1900 && y1 <= 2099 && y2 >= 1900 && y2 <= 2099 {
										continue
									}
								}
							}
						}
						return l + "-" + r, i
					}
					if inParen {
						if isTitlePrefixedDigitRangeInParen(tokens, i) {
							continue
						}
						l := strings.TrimLeft(strings.TrimSpace(tokens[i].Text), "0")
						if l == "" {
							l = "0"
						}
						r := strings.TrimLeft(rawR, "0")
						if r == "" {
							r = "0"
						}
						if len(strings.TrimSpace(tokens[i].Text)) == 4 && len(rawR) == 4 {
							if y1, err1 := strconv.Atoi(strings.TrimSpace(tokens[i].Text)); err1 == nil {
								if y2, err2 := strconv.Atoi(rawR); err2 == nil {
									if y1 >= 1900 && y1 <= 2099 && y2 >= 1900 && y2 <= 2099 {
										continue
									}
								}
							}
						}
						return l + "-" + r, i
					}
					continue
				}
				if m := leadingDigitsPattern.FindStringSubmatch(rawR); len(m) == 2 {
					l := strings.TrimLeft(strings.TrimSpace(tokens[i].Text), "0")
					if l == "" {
						l = "0"
					}
					r := strings.TrimLeft(m[1], "0")
					if r == "" {
						r = "0"
					}
					return l + "-" + r, i
				}
			}
		}
	}
	return "", -1
}

func isEpisodeRangeWordConnector(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a", "à":
		return true
	default:
		return false
	}
}

func normalizeEpisodeRangeText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	m := episodeRangeEmbeddedPattern.FindStringSubmatch(s)
	if len(m) != 3 {
		return s
	}
	a := strings.TrimLeft(m[1], "0")
	if a == "" {
		a = "0"
	}
	b := strings.TrimLeft(m[2], "0")
	if b == "" {
		b = "0"
	}
	return a + "-" + b
}

var groupHyphenSpacePattern = regexp.MustCompile(`([A-Za-z0-9])\s*-\s*([A-Za-z0-9])`)
var groupAmpSpacePattern = regexp.MustCompile(`\s*[&＆]\s*`)
var groupCommaSpacePattern = regexp.MustCompile(`\s*[,，]\s*`)

func normalizeGroupText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	s = groupHyphenSpacePattern.ReplaceAllString(s, `$1-$2`)
	// Normalize '&' / '＆' without surrounding spaces.
	s = groupAmpSpacePattern.ReplaceAllString(s, "&")
	// Normalize ',' / '，' without surrounding spaces.
	s = groupCommaSpacePattern.ReplaceAllString(s, ",")
	return s
}

func bracketBlockInnerText(tokens []model.Token, idx int) string {
	if idx < 0 || idx >= len(tokens) {
		return ""
	}
	start := -1
	depth := 0
	for i := 0; i <= idx; i++ {
		if tokens[i].Text == "[" || tokens[i].Text == "【" {
			if depth == 0 {
				start = i
			}
			depth++
		} else if tokens[i].Text == "]" || tokens[i].Text == "】" {
			if depth > 0 {
				depth--
				if depth == 0 {
					start = -1
				}
			}
		}
	}
	if start < 0 {
		return ""
	}
	end := -1
	depth = 0
	for j := start; j < len(tokens); j++ {
		if tokens[j].Text == "[" || tokens[j].Text == "【" {
			depth++
		} else if tokens[j].Text == "]" || tokens[j].Text == "】" {
			if depth > 0 {
				depth--
				if depth == 0 {
					end = j
					break
				}
			}
		}
	}
	if end < 0 {
		return ""
	}
	parts := make([]string, 0, end-start)
	for k := start + 1; k < end; k++ {
		if tokens[k].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[k].Text)
		if t == "" || t == "[" || t == "]" || t == "【" || t == "】" {
			continue
		}
		parts = append(parts, t)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func extractTitleFromSpan(tokens []model.Token, start int, end int) string {
	return extractTitleFromSpanAllowRemoved(tokens, start, end, false)
}

func extractTitleFromSpanAllowRemoved(tokens []model.Token, start int, end int, allowRemoved bool) string {
	if start < 0 {
		start = 0
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	if start >= end {
		return ""
	}
	parts := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		if tokens[i].Removed && !allowRemoved {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if t == "[" || t == "]" || t == "【" || t == "】" {
			continue
		}
		if t == "(" || t == ")" || t == "（" || t == "）" {
			continue
		}
		if t == "|" || t == "｜" {
			continue
		}
		parts = append(parts, t)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func detectTechPrefixedGroup(tokens []model.Token) (string, int) {
	tech := map[string]bool{
		"pcm":  true,
		"x264": true,
		"x265": true,
		"h264": true,
		"h265": true,
	}
	for i := 0; i+3 < len(tokens); i++ {
		if tokens[i].Removed || tokens[i+1].Removed || tokens[i+2].Removed {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(tokens[i].Text))
		if !tech[k] {
			continue
		}
		if strings.TrimSpace(tokens[i+1].Text) != "-" {
			continue
		}
		g := strings.TrimSpace(tokens[i+2].Text)
		if !isTailGroupToken(g) {
			continue
		}
		if !hasUpperASCII(g) {
			continue
		}
		if strings.TrimSpace(tokens[i+3].Text) != "-" {
			continue
		}
		return g, i + 2
	}
	return "", -1
}

func extractInlineStarGroup(tokens []model.Token) string {
	if len(tokens) == 0 {
		return ""
	}
	if tokens[0].Removed {
		return ""
	}
	validLeft := func(left string) bool {
		left = strings.TrimSpace(left)
		if left == "" {
			return false
		}
		if looksLikeLeadingTitleMeta(left) {
			return false
		}
		if len([]rune(left)) > 24 {
			return false
		}
		// must contain at least one non-ascii rune or '&'
		hasNonASCII := false
		for _, r := range left {
			if r == '&' {
				hasNonASCII = true
				break
			}
			if r > 127 {
				hasNonASCII = true
				break
			}
		}
		return hasNonASCII
	}

	// Case A: tokenizer split the delimiter into its own token.
	// Example: ["六四位元字幕組", "★", "..."].
	if len(tokens) >= 2 {
		sep := strings.TrimSpace(tokens[1].Text)
		if sep == "★" || sep == "☆" {
			left := strings.TrimSpace(tokens[0].Text)
			if validLeft(left) {
				hasRight := false
				maxLookahead := 16
				if maxLookahead > len(tokens) {
					maxLookahead = len(tokens)
				}
				for j := 2; j < maxLookahead; j++ {
					if tokens[j].Removed {
						continue
					}
					if strings.TrimSpace(tokens[j].Text) != "" {
						hasRight = true
						break
					}
				}
				if hasRight {
					tokens[0].Removed = true
					tokens[1].Removed = true
					return left
				}
			}
		}
	}
	// Case A2: two-token group prefix before star.
	// Example: ["SFSub&", "六四位元字幕組", "★", "..."].
	if len(tokens) >= 3 {
		sep := strings.TrimSpace(tokens[2].Text)
		if sep == "★" || sep == "☆" {
			leftA := strings.TrimSpace(tokens[0].Text)
			leftB := strings.TrimSpace(tokens[1].Text)
			joined := strings.TrimSpace(leftA + " " + leftB)
			if validLeft(joined) || (strings.HasSuffix(leftA, "&") && hasStrongGroupKeyword(leftB)) {
				hasRight := false
				maxLookahead := 20
				if maxLookahead > len(tokens) {
					maxLookahead = len(tokens)
				}
				for j := 3; j < maxLookahead; j++ {
					if tokens[j].Removed {
						continue
					}
					if strings.TrimSpace(tokens[j].Text) != "" {
						hasRight = true
						break
					}
				}
				if hasRight {
					tokens[0].Removed = true
					tokens[1].Removed = true
					tokens[2].Removed = true
					return joined
				}
			}
		}
	}

	// Case B: delimiter is embedded in the first token.
	text := tokens[0].Text
	idx := strings.IndexAny(text, "★☆")
	if idx <= 0 {
		return ""
	}
	left := strings.TrimSpace(text[:idx])
	right := strings.TrimSpace(text[idx+len("★"):])
	if left == "" || right == "" {
		return ""
	}
	if !validLeft(left) {
		return ""
	}
	tokens[0].Text = right
	return left
}

var episodePattern = regexp.MustCompile(`(?i)^(?:(e|ep|第))?(\d{1,4})(?:\.(\d{1,2}))?(?:[vV]\d{1,2})?$`)
var episodeWithParenSuffixPattern = regexp.MustCompile(`(?i)^(?:(e|ep|第))?(\d{1,4})(?:\.(\d{1,2}))?[（(][^）)]{1,32}[）)]$`)
var episodeVersionSuffixPattern = regexp.MustCompile(`(?i)^(?:\d{1,4}(?:\.\d{1,2})?)[vV]\d{1,2}$`)
var chineseEpisodeSuffixPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})(?:话|集)$`)
var chineseEpisodeWithSuffixMarkerPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})(?:话|集|話)[①②③④⑤⑥⑦⑧⑨⑩⑪⑫⑬⑭⑮⑯⑰⑱⑲⑳]$`)
var chineseEpisodeLeadingPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})(?:话|集|話)(?:$|(?:\s|[&＆(（\[【]).*)`)
var embeddedChineseEpisodeSuffixPattern = regexp.MustCompile(`第(\d{1,4})(?:集|话|話)$`)
var yearPattern = regexp.MustCompile(`^(?:19|20)\d{2}$`)
var starEpisodeEmbeddedPattern = regexp.MustCompile(`(?:★|☆)(\d{1,4})(?:★|☆)`)
var splitDecimalEpisodeRightPattern = regexp.MustCompile(`^\d{1,2}$`)
var dateYMDPattern = regexp.MustCompile(`(?i)(?:\b(?:19|20)\d{2}[./-](?:0?[1-9]|1[0-2])[./-](?:0?[1-9]|[12]\d|3[01])\b)`)
var spKindEpisodePattern = regexp.MustCompile(`(?i)^(?:sp|ova|oad|ona|ex|ncop|nced|pv)0*(\d{1,3})$`)
var episodeWithVersionPattern = regexp.MustCompile(`(?i)^(?:(?:e|ep|第))?(\d{1,4})(?:\.(\d{1,2}))?[vV](\d{1,2})$`)
var episodeEndSuffixPattern = regexp.MustCompile(`(?i)^(\d{1,4})(end|fin|final)$`)
var standaloneVersionPattern = regexp.MustCompile(`(?i)^v(\d{1,2})$`)
var regionHKMOTWEmbeddedPattern = regexp.MustCompile(`(?i)(?:仅限港澳台地区|僅限港澳台地區)`)
var regionHKMOTWRemovalPattern = regexp.MustCompile(`(?i)[(（]?\s*(?:仅限港澳台地区|僅限港澳台地區)\s*[)）]?`)
var bracketScheduleNoticeDatePattern = regexp.MustCompile(`\d{1,2}[./]\d{1,2}`)

func isBracketScheduleNoticeText(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if !strings.Contains(ss, "停播") && !strings.Contains(ss, "停更") {
		return false
	}
	if bracketScheduleNoticeDatePattern.MatchString(ss) || dateYMDPattern.MatchString(ss) {
		return true
	}
	if strings.Contains(ss, "，") || strings.Contains(ss, ",") || strings.Contains(ss, "。") || strings.Contains(ss, ";") || strings.Contains(ss, "；") {
		return true
	}
	return false
}

func isEpisodeBoundaryRightToken(t string) bool {
	tt := strings.TrimSpace(t)
	if tt == "" {
		return false
	}
	switch tt {
	case "-", "[", "【", "(", "（":
		return true
	}
	if standaloneVersionPattern.MatchString(tt) {
		return true
	}
	up := strings.ToUpper(tt)
	return up == "END" || up == "FIN" || up == "FINAL"
}

func bracketBoundsAt(tokens []model.Token, idx int) (int, int, bool) {
	if idx < 0 || idx >= len(tokens) || !isInBracket(tokens, idx) {
		return -1, -1, false
	}
	openIdx := -1
	openTok := ""
	depth := 0
	for j := idx; j >= 0; j-- {
		t := strings.TrimSpace(tokens[j].Text)
		if t == "]" || t == "】" {
			depth++
			continue
		}
		if t == "[" || t == "【" {
			if depth == 0 {
				openIdx = j
				openTok = t
				break
			}
			depth--
		}
	}
	if openIdx < 0 {
		return -1, -1, false
	}
	closeTok := "]"
	if openTok == "【" {
		closeTok = "】"
	}
	d := 0
	for j := openIdx; j < len(tokens); j++ {
		t := strings.TrimSpace(tokens[j].Text)
		if t == openTok {
			d++
			continue
		}
		if t == closeTok {
			d--
			if d == 0 {
				return openIdx, j, true
			}
		}
	}
	return -1, -1, false
}

func firstMeaningfulTokenInRange(tokens []model.Token, start int, end int) (int, string) {
	if start < 0 {
		start = 0
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	for i := start; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		return i, t
	}
	return -1, ""
}

func isLikelyBracketTitleWordToken(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" {
		return false
	}
	if t == "-" || t == "~" || t == ":" || t == "：" || t == "|" || t == "｜" || t == "/" || t == "／" {
		return false
	}
	if episodeOfTotalPattern.MatchString(t) {
		return false
	}
	if standaloneVersionPattern.MatchString(t) {
		return false
	}
	up := strings.ToUpper(t)
	if up == "END" || up == "FIN" || up == "FINAL" || up == "V2" || up == "V3" {
		return false
	}
	if looksLikeTitleCutMetaToken(t) || isLikelyMetaBracketText(t) {
		return false
	}
	return hasCJK(t) || hasLatin(t)
}

// Guard title-leading numeric tokens inside bracket title spans:
// [16 位元的輝煌 ...], [86-不存在的戰區] should stay in title, not become episode primary.
func isBracketLeadingNumericTitlePrefix(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	openIdx, closeIdx, ok := bracketBoundsAt(tokens, idx)
	if !ok {
		return false
	}
	for j := openIdx + 1; j < idx; j++ {
		if tokens[j].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[j].Text)
		if t == "" || t == "-" || t == "~" || t == ":" || t == "：" {
			continue
		}
		return false
	}
	n1Idx, n1 := firstMeaningfulTokenInRange(tokens, idx+1, closeIdx)
	if n1Idx < 0 {
		return false
	}
	if n1 == "-" || n1 == "~" {
		_, n2 := firstMeaningfulTokenInRange(tokens, n1Idx+1, closeIdx)
		if n2 == "" || episodeOfTotalPattern.MatchString(strings.TrimSpace(n2)) {
			return false
		}
		return isLikelyBracketTitleWordToken(n2)
	}
	return isLikelyBracketTitleWordToken(n1)
}

func hasUpperASCII(s string) bool {
	for _, r := range s {
		if 'A' <= r && r <= 'Z' {
			return true
		}
	}
	return false
}

func isInParen(tokens []model.Token, idx int) bool {
	depth := 0
	for i := 0; i < idx; i++ {
		if tokens[i].Text == "(" || tokens[i].Text == "（" {
			depth++
		} else if tokens[i].Text == ")" || tokens[i].Text == "）" {
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0
}

func parenBlockInnerText(tokens []model.Token, idx int) string {
	if idx < 0 || idx >= len(tokens) {
		return ""
	}
	for j := idx; j >= 0; j-- {
		if tokens[j].Text != "(" && tokens[j].Text != "（" {
			continue
		}
		openIdx, closeIdx, inner, ok := parseParenBlock(tokens, j, len(tokens))
		if !ok {
			continue
		}
		if openIdx <= idx && idx <= closeIdx {
			return strings.TrimSpace(inner)
		}
	}
	return ""
}

func looksLikeDateOrTimeText(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "整理时间") || strings.Contains(low, "整理時間") {
		return true
	}
	compact := reLooksLikeDateOrTimeText1.ReplaceAllString(low, "$1")
	if dateYMDPattern.MatchString(low) || dateYMDPattern.MatchString(compact) {
		return true
	}
	return false
}

func looksLikeCodecNumber(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if cur != "264" && cur != "265" {
		return false
	}
	if idx-1 >= 0 {
		p := strings.ToLower(strings.TrimSpace(tokens[idx-1].Text))
		if p == "h" || p == "x" {
			return true
		}
		if p == "." && idx-2 >= 0 {
			pp := strings.ToLower(strings.TrimSpace(tokens[idx-2].Text))
			if pp == "h" {
				return true
			}
		}
	}
	return false
}

func looksLikeCodecPrefixToken(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return false
	}
	switch low {
	case "hevc", "avc", "h264", "h265", "h266", "x264", "x265", "av1", "mpeg2", "mpeg4", "xvid", "divx":
		return true
	default:
		return false
	}
}

func looksLikeBracketCodecNumber(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	if !isInBracket(tokens, idx) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	if idx-1 >= 0 {
		prev := strings.TrimSpace(tokens[idx-1].Text)
		if prev == "-" && idx-2 >= 0 {
			if looksLikeCodecPrefixToken(strings.TrimSpace(tokens[idx-2].Text)) {
				return true
			}
		}
	}
	return false
}

var episodeSPBracketCombinedPattern = regexp.MustCompile(`(?i)^0*(\d{1,3})\s*[_ .-]\s*(sp|ova|oad|ona|ex|pv|op|ed)\s*$`)
var partIndexLeadPattern = regexp.MustCompile(`(?i)^(?:part|pt|cour|cours)$`)
var seasonSignalTokenPattern = regexp.MustCompile(`(?i)^(?:s\d{1,2}|season|saison|cour|cours)$`)
var seasonSignalCJKTokenPattern = regexp.MustCompile(`^第?[一二三四五六七八九十0-9]{1,3}(?:季|期|部)$`)
var episodePrefixedTokenPattern = regexp.MustCompile(`(?i)^(?:e|ep)\d{1,4}(?:\.\d{1,2})?(?:[vV]\d{1,2})?$`)
var chineseEpisodeDecimalSuffixPattern = regexp.MustCompile(`^第?(\d{1,4})\.(\d{1,2})(?:集|话|話|回)$`)
var chineseEpisodeDecimalTailPattern = regexp.MustCompile(`^(\d{1,2})(?:集|话|話|回)$`)
var chineseEpisodeDecimalHeadPattern = regexp.MustCompile(`^第(\d{1,4})$`)
var bracketCatalogIDPrefixPattern = regexp.MustCompile(`^[A-Z]{3,}[A-Z0-9]*$`)

func hasDashSeparatorAroundToken(tokens []model.Token, idx int) bool {
	if idx-1 >= 0 {
		sep := strings.TrimSpace(tokens[idx-1].Text)
		if sep == "-" || sep == "~" {
			return true
		}
	}
	if idx+1 < len(tokens) {
		sep := strings.TrimSpace(tokens[idx+1].Text)
		if sep == "-" || sep == "~" {
			return true
		}
	}
	return false
}

func isStandaloneNumericBracketBlock(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	if !isInBracket(tokens, idx) {
		return false
	}
	openIdx, closeIdx, ok := bracketBoundsAt(tokens, idx)
	if !ok {
		return false
	}
	found := false
	for j := openIdx + 1; j < closeIdx; j++ {
		if tokens[j].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[j].Text)
		if t == "" {
			continue
		}
		if j == idx && episodeOfTotalPattern.MatchString(t) {
			found = true
			continue
		}
		return false
	}
	return found
}

func looksLikeVolumeContextNumberToken(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	if idx-1 >= 0 {
		prev := strings.TrimSpace(tokens[idx-1].Text)
		if volumeWordPattern.MatchString(prev) {
			return true
		}
	}
	if idx+1 < len(tokens) {
		next := strings.TrimSpace(tokens[idx+1].Text)
		if volumeWordPattern.MatchString(next) {
			return true
		}
	}
	if idx-2 >= 0 {
		prevSep := strings.TrimSpace(tokens[idx-1].Text)
		prevLead := strings.TrimSpace(tokens[idx-2].Text)
		if prevSep == "." && volumeWordPattern.MatchString(prevLead) {
			return true
		}
	}
	if idx+2 < len(tokens) {
		nextSep := strings.TrimSpace(tokens[idx+1].Text)
		nextLead := strings.TrimSpace(tokens[idx+2].Text)
		if nextSep == "." && volumeWordPattern.MatchString(nextLead) {
			return true
		}
	}
	return false
}

func detectEpisodeFromSPBracketSegment(tokens []model.Token) (string, int, bool) {
	for i := 0; i < len(tokens); i++ {
		if !isInBracket(tokens, i) {
			continue
		}
		cur := strings.TrimSpace(tokens[i].Text)
		if cur == "" {
			continue
		}
		if mm := episodeSPBracketCombinedPattern.FindStringSubmatch(cur); len(mm) == 3 {
			n := strings.TrimLeft(strings.TrimSpace(mm[1]), "0")
			if n == "" {
				n = "0"
			}
			return n, i, true
		}
		if !episodeOfTotalPattern.MatchString(cur) || len(cur) > 3 {
			continue
		}
		if i+2 >= len(tokens) {
			continue
		}
		if !isInBracket(tokens, i+1) || !isInBracket(tokens, i+2) {
			continue
		}
		sep := strings.TrimSpace(tokens[i+1].Text)
		if sep != "_" && sep != "-" && sep != "." && sep != "&" && sep != "＆" {
			continue
		}
		next := strings.TrimSpace(tokens[i+2].Text)
		if m := spKindPattern.FindStringSubmatch(next); len(m) < 2 {
			continue
		}
		n := strings.TrimLeft(cur, "0")
		if n == "" {
			n = "0"
		}
		return n, i, true
	}
	return "", -1, false
}

func detectEpisodeWithVersion(tokens []model.Token) (string, int, string, int) {
	bestIdx := -1
	best := ""
	bestScore := -1
	bestVer := ""
	bestConsumedEnd := -1
	bestKeywordIdx := -1
	bestKeywordDashIdx := -1

	// Pattern: "Episode 52" / "- Episode 52" (common in English TV rip naming).
	for i := 0; i+1 < len(tokens); i++ {
		if epScanSkipped(tokens, i) || epScanSkipped(tokens, i+1) {
			continue
		}
		kw := strings.TrimSpace(tokens[i].Text)
		if !(strings.EqualFold(kw, "episode") || strings.EqualFold(kw, "ep")) {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		d := strings.TrimSpace(tokens[i+1].Text)
		if !episodeOfTotalPattern.MatchString(d) {
			continue
		}
		if yearPattern.MatchString(d) {
			continue
		}
		score := 74
		dashIdx := -1
		// Prefer the explicit "- Episode N" style, but do not require it.
		if i-1 >= 0 {
			prev := strings.TrimSpace(tokens[i-1].Text)
			if prev == "-" {
				score += 4
				dashIdx = i - 1
			}
		}
		n := strings.TrimLeft(d, "0")
		if n == "" {
			n = "0"
		}
		if score > bestScore {
			best = n
			bestIdx = i + 1
			bestScore = score
			bestConsumedEnd = i + 1
			bestKeywordIdx = i
			bestKeywordDashIdx = dashIdx
		}
	}

	for i := 0; i < len(tokens); i++ {
		if epScanSkipped(tokens, i) {
			// Protected = known-title span: numbers inside are title text,
			// not episode numbers. Removed = already consumed elsewhere.
			continue
		}
		if isTitlePrefixedDigitRangeInBracket(tokens, i) {
			continue
		}
		if isRightSideOfPlainDigitRangeInTitle(tokens, i) {
			continue
		}
		if isRightSideOfSharedPrefixSeasonEpisodeRange(tokens, i) {
			continue
		}
		inBracket := isInBracket(tokens, i)
		if inBracket {
			if txt := bracketBlockInnerText(tokens, i); txt != "" {
				if isBracketScheduleNoticeText(txt) {
					continue
				}
				if isLikelyMetaBracketText(txt) && !isEpisodeLikeBracketText(txt) {
					continue
				}
			}
			if isBracketLeadingNumericTitlePrefix(tokens, i) {
				continue
			}
		}
		if isInParen(tokens, i) {
			continue
		}
		if looksLikeCodecNumber(tokens, i) {
			continue
		}
		if looksLikeDottedNumericSequence(tokens, i) {
			continue
		}
		if looksLikeDashSeparatedTechSuffix(tokens, i) {
			continue
		}
		if yearPattern.MatchString(tokens[i].Text) {
			continue
		}
		// Pattern: "... 06 Sub ..." where episode number is followed by a subtitle marker.
		// Note: earlier passes (tech_subs) may mark the following "Sub" token as Removed,
		// but we still want to detect the episode boundary.
		if i+1 < len(tokens) {
			next := strings.TrimSpace(tokens[i+1].Text)
			if strings.EqualFold(next, "sub") || strings.EqualFold(next, "subs") {
				cur := strings.TrimSpace(tokens[i].Text)
				if episodeOfTotalPattern.MatchString(cur) && !yearPattern.MatchString(cur) {
					n := strings.TrimLeft(cur, "0")
					if n == "" {
						n = "0"
					}
					if n != "0" {
						score := len(cur) + 92
						if score > bestScore {
							bestScore = score
							bestIdx = i
							best = n
							bestVer = ""
						}
					}
				}
			}
		}
		// Support version-suffixed episode tokens like "02v2" / "12.5v2".
		if mm := episodeWithVersionPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(mm) == 4 {
			n := strings.TrimLeft(mm[1], "0")
			if n == "" {
				n = "0"
			}
			if strings.TrimSpace(mm[2]) != "" {
				n = n + "." + strings.TrimSpace(mm[2])
			}
			ver := strings.TrimLeft(mm[3], "0")
			if ver == "" {
				ver = mm[3]
			}
			score := len(mm[1]) + 97
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = n
				bestVer = "v" + ver
			}
			continue
		}
		// Support episode tokens with trailing parenthetical marker:
		// e.g. "03（Onair）" / "03(Premium)".
		if mm := episodeWithParenSuffixPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(mm) >= 4 {
			if yearPattern.MatchString(strings.TrimSpace(mm[2])) {
				continue
			}
			n := strings.TrimLeft(mm[2], "0")
			if n == "" {
				n = "0"
			}
			if strings.TrimSpace(mm[3]) != "" {
				n = n + "." + strings.TrimSpace(mm[3])
			}
			if n != "0" {
				score := len(mm[2]) + 91
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = n
					bestVer = ""
				}
			}
			continue
		}
		// Compact end-flag episode token, e.g. "12END"/"12FIN".
		if mm := episodeEndSuffixPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(mm) == 3 {
			n := strings.TrimLeft(mm[1], "0")
			if n == "" {
				n = "0"
			}
			if n != "0" {
				score := len(mm[1]) + 96
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = n
					bestVer = ""
				}
			}
			continue
		}
		// Split end-flag episode token inside a bracket block, e.g. "48_END" / "48-FIN".
		if inBracket && i+2 < len(tokens) && !tokens[i+1].Removed && !tokens[i+2].Removed {
			cur := strings.TrimSpace(tokens[i].Text)
			sep := strings.TrimSpace(tokens[i+1].Text)
			next := strings.ToUpper(strings.TrimSpace(tokens[i+2].Text))
			if episodeOfTotalPattern.MatchString(cur) && (sep == "_" || sep == "-") && (next == "END" || next == "FIN" || next == "FINAL") {
				n := strings.TrimLeft(cur, "0")
				if n == "" {
					n = "0"
				}
				if n != "0" {
					score := len(cur) + 96
					if score > bestScore {
						bestScore = score
						bestIdx = i
						best = n
						bestVer = ""
					}
				}
				continue
			}
		}
		if inBracket {
			if looksLikeBracketCodecNumber(tokens, i) {
				continue
			}
			n := strings.TrimSpace(tokens[i].Text)
			if episodeOfTotalPattern.MatchString(n) {
				if looksLikeVolumeContextNumberToken(tokens, i) {
					continue
				}
				if hasMatchingSeasonNumberAroundToken(tokens, i, 12) && !hasDashSeparatorAroundToken(tokens, i) && !isStandaloneNumericBracketBlock(tokens, i) {
					continue
				}
				if i-2 >= 0 {
					prevSep := strings.TrimSpace(tokens[i-1].Text)
					prevLead := strings.TrimSpace(tokens[i-2].Text)
					if prevSep == "-" && bracketCatalogIDPrefixPattern.MatchString(prevLead) {
						continue
					}
				}
				nn := strings.TrimLeft(n, "0")
				if nn == "" {
					nn = "0"
				}
				if isBracketLeadingNumericTitlePrefix(tokens, i) {
					continue
				}
				score := len(n) + 40
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = nn
				}
				continue
			}
		}
		// Support explicit SP-kind episode tokens like "OVA02" / "SP01" / "OAD3".
		// These are strong episode markers and should not be missed.
		if mm := spKindEpisodePattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(mm) == 2 {
			nn := strings.TrimLeft(mm[1], "0")
			if nn == "" {
				nn = "0"
			}
			if nn != "0" {
				score := len(mm[1]) + 98
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = nn
				}
				continue
			}
		}
		// Support patterns like "...★01★..." where tokenizer keeps the whole token.
		if mm := starEpisodeEmbeddedPattern.FindStringSubmatch(tokens[i].Text); len(mm) == 2 {
			nn := strings.TrimLeft(mm[1], "0")
			if nn == "" {
				nn = "0"
			}
			score := len(mm[1]) + 60
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = nn
			}
			continue
		}
		// Support cases like "作品名...第08集" where tokenizer keeps the episode suffix attached.
		if mm := embeddedChineseEpisodeSuffixPattern.FindStringSubmatch(tokens[i].Text); len(mm) == 2 {
			nn := strings.TrimLeft(mm[1], "0")
			if nn == "" {
				nn = "0"
			}
			if nn != "0" {
				score := len(mm[1]) + 95
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = nn
				}
				continue
			}
		}
		if strings.TrimSpace(tokens[i].Text) == "第" {
			if i+2 < len(tokens) && !tokens[i+1].Removed && !tokens[i+2].Removed {
				n := strings.TrimSpace(tokens[i+1].Text)
				suf := strings.TrimSpace(tokens[i+2].Text)
				if reDetectEpisodeWithVersion1.MatchString(n) && (suf == "集" || suf == "话" || suf == "話") {
					nn := strings.TrimLeft(n, "0")
					if nn == "" {
						nn = "0"
					}
					if nn != "0" {
						score := len(n) + 90
						if score > bestScore {
							bestScore = score
							bestIdx = i
							best = nn
						}
						continue
					}
				}
			}
		}
		if mm := chineseEpisodeSuffixPattern.FindStringSubmatch(tokens[i].Text); len(mm) == 2 {
			n := strings.TrimLeft(mm[1], "0")
			if n == "" {
				n = "0"
			}
			if n == "0" {
				continue
			}
			score := len(mm[1]) + 90
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = n
			}
			continue
		}
		if mm := chineseEpisodeWithSuffixMarkerPattern.FindStringSubmatch(tokens[i].Text); len(mm) == 2 {
			n := strings.TrimLeft(mm[1], "0")
			if n == "" {
				n = "0"
			}
			score := len(mm[1]) + 90
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = n
			}
			continue
		}
		if mm := chineseEpisodeLeadingPattern.FindStringSubmatch(tokens[i].Text); len(mm) == 2 {
			n := strings.TrimLeft(mm[1], "0")
			if n == "" {
				n = "0"
			}
			score := len(mm[1]) + 86
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = n
			}
			continue
		}
		if mm := chineseEpisodeDecimalSuffixPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(mm) == 3 {
			maj := strings.TrimLeft(mm[1], "0")
			if maj == "" {
				maj = "0"
			}
			dec := strings.TrimSpace(mm[2])
			if dec != "" {
				n := maj + "." + dec
				score := len(mm[1]) + 93
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = n
				}
				continue
			}
		}
		if i+2 < len(tokens) {
			left := strings.TrimSpace(tokens[i].Text)
			dot := strings.TrimSpace(tokens[i+1].Text)
			right := strings.TrimSpace(tokens[i+2].Text)
			if strings.HasPrefix(left, "第") && dot == "." {
				if lm := chineseEpisodeDecimalHeadPattern.FindStringSubmatch(left); len(lm) == 2 {
					if rm := chineseEpisodeDecimalTailPattern.FindStringSubmatch(right); len(rm) == 2 {
						maj := strings.TrimLeft(lm[1], "0")
						if maj == "" {
							maj = "0"
						}
						dec := strings.TrimSpace(rm[1])
						if dec != "" {
							n := maj + "." + dec
							score := len(lm[1]) + 93
							if score > bestScore {
								bestScore = score
								bestIdx = i
								best = n
							}
							continue
						}
					}
				}
			}
		}
		if i+2 < len(tokens) {
			left := strings.TrimSpace(tokens[i].Text)
			dot := strings.TrimSpace(tokens[i+1].Text)
			right := strings.TrimSpace(tokens[i+2].Text)
			if dot == "." && splitDecimalEpisodeRightPattern.MatchString(right) {
				if mm := episodePattern.FindStringSubmatch(left); len(mm) >= 3 {
					if len(mm) >= 4 && strings.TrimSpace(mm[3]) != "" {
						// Already a single-token decimal episode like 12.5.
					} else {
						prefix := strings.TrimSpace(mm[1])
						maj := strings.TrimLeft(mm[2], "0")
						if maj == "" {
							maj = "0"
						}
						n := maj + "." + right
						score := len(mm[2])
						ok := false
						if prefix != "" {
							ok = true
							score += 72
						} else if inBracket {
							if isBracketLeadingNumericTitlePrefix(tokens, i) {
								continue
							}
							ok = true
							score += 96
						} else if i-1 >= 0 {
							sep := strings.TrimSpace(tokens[i-1].Text)
							if sep == "-" || sep == "★" || sep == "☆" {
								rightBoundary := false
								if i+3 < len(tokens) {
									rightBoundary = isEpisodeBoundaryRightToken(tokens[i+3].Text)
								}
								if sep == "-" && !rightBoundary {
									continue
								}
								ok = true
								if sep == "-" {
									score += 94
								} else {
									score += 92
								}
							}
						}
						if ok && score > bestScore {
							bestScore = score
							bestIdx = i
							best = n
							bestVer = ""
							bestConsumedEnd = i + 2
						}
						if ok {
							continue
						}
					}
				}
			}
		}
		m := episodePattern.FindStringSubmatch(tokens[i].Text)
		if len(m) >= 3 {
			prefix := strings.TrimSpace(m[1])
			n := strings.TrimLeft(m[2], "0")
			if n == "" {
				n = "0"
			}
			if len(m) >= 4 {
				dec := strings.TrimSpace(m[3])
				if dec != "" {
					n = n + "." + dec
				}
			}
			// Guard: bracketed catalog IDs like "[KAXA-9023]" are product codes, not episodes.
			if prefix == "" && inBracket && i-2 >= 0 {
				prevSep := strings.TrimSpace(tokens[i-1].Text)
				prevLead := strings.TrimSpace(tokens[i-2].Text)
				if prevSep == "-" && bracketCatalogIDPrefixPattern.MatchString(prevLead) {
					continue
				}
			}
			if prefix == "" && looksLikeVolumeContextNumberToken(tokens, i) {
				continue
			}
			if prefix == "" && inBracket && hasMatchingSeasonNumberAroundToken(tokens, i, 12) && !hasDashSeparatorAroundToken(tokens, i) && !isStandaloneNumericBracketBlock(tokens, i) {
				continue
			}
			signalScore := 0
			if prefix == "" {
				if shouldSkipPartIndexEpisodeCandidate(tokens, i, inBracket) {
					continue
				}
				ok := false
				if inBracket {
					if isBracketLeadingNumericTitlePrefix(tokens, i) {
						continue
					}
					ok = true
					signalScore = 96
				}
				if i-1 >= 0 {
					sep := strings.TrimSpace(tokens[i-1].Text)
					if sep == "-" {
						ok = true
						if signalScore < 94 {
							signalScore = 94
						}
						rightBoundary := false
						if i+1 < len(tokens) {
							rightBoundary = isEpisodeBoundaryRightToken(tokens[i+1].Text)
						}
						if i-2 >= 0 {
							prev2 := strings.TrimSpace(tokens[i-2].Text)
							if looksLikeCodecPrefixToken(prev2) {
								ok = false
							}
							if inBracket && bracketCatalogIDPrefixPattern.MatchString(prev2) {
								ok = false
							}
						}
						// Guard: reject cases like "Gintama-3 nen" where the number is followed by a word.
						if !inBracket {
							if i+1 < len(tokens) {
								nextWord := strings.TrimSpace(tokens[i+1].Text)
								if !rightBoundary && (looksLikeLowercaseLatinWordToken(nextWord) || looksLikeBitDepthWordToken(nextWord)) {
									ok = false
								}
							}
						}
						// avoid U-20 style (letter + '-' + digits)
						if i-2 >= 0 {
							p := strings.TrimSpace(tokens[i-2].Text)
							if len(p) <= 2 {
								allLetters := true
								for _, r := range p {
									if !(('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')) {
										allLetters = false
										break
									}
								}
								if allLetters && hasUpperASCII(p) {
									if !rightBoundary {
										ok = false
									}
								}
							}
						}
						// Guard: title season suffix before bracket episode, e.g.
						// "... Monogatari -2 [01] ...", should prefer [01].
						if ok && !inBracket && hasLaterBracketEpisodeCandidate(tokens, i+1) {
							if hasMatchingSeasonNumberAroundToken(tokens, i, 12) {
								ok = false
							}
						}
					} else if sep == "★" || sep == "☆" {
						ok = true
						if signalScore < 92 {
							signalScore = 92
						}
					}
				}
				if !ok {
					if i+1 < len(tokens) {
						next := strings.TrimSpace(tokens[i+1].Text)
						if next == "[" || next == "【" || next == "(" || next == "（" {
							if n != "0" {
								if n != "720" && n != "1080" && n != "2160" && n != "480" && n != "540" {
									if hasLaterBracketEpisodeCandidate(tokens, i+1) {
										if hasMatchingSeasonNumberAroundToken(tokens, i, 12) {
											continue
										}
										if i-1 >= 0 {
											prev := strings.TrimSpace(tokens[i-1].Text)
											if isLikelyBracketTitleWordToken(prev) && !strings.EqualFold(prev, "episode") && !strings.EqualFold(prev, "ep") {
												continue
											}
										}
									}
									ok = true
									if signalScore < 88 {
										signalScore = 88
									}
								}
							}
						}
					}
					if !ok {
						continue
					}
				}
				if n == "0" {
					isStrongZero := inBracket
					if !isStrongZero && i-1 >= 0 && strings.TrimSpace(tokens[i-1].Text) == "-" {
						if i+1 < len(tokens) && isEpisodeBoundaryRightToken(tokens[i+1].Text) {
							isStrongZero = true
						}
					}
					if !isStrongZero {
						continue
					}
				}
			}
			score := len(m[2])
			if prefix != "" {
				// Medium EP signal: EP12 / E12 / 第12
				score += 72
			} else if signalScore > 0 {
				// Strong EP signal: bracketed number / dash-number / star-number.
				score += signalScore
			}
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = n
				bestVer = ""
				bestConsumedEnd = i
			}
		}
	}
	if bestIdx >= 0 {
		if bestConsumedEnd < bestIdx {
			bestConsumedEnd = bestIdx
		}
		if bestKeywordIdx >= 0 && bestKeywordIdx < len(tokens) {
			tokens[bestKeywordIdx].Removed = true
		}
		if bestKeywordDashIdx >= 0 && bestKeywordDashIdx < len(tokens) {
			tokens[bestKeywordDashIdx].Removed = true
		}
		for j := bestIdx + 1; j <= bestConsumedEnd && j < len(tokens); j++ {
			tokens[j].Removed = true
		}
		text := strings.TrimSpace(tokens[bestIdx].Text)
		// If the episode token is itself a special/extra marker token like "OVA02",
		// or a version-suffixed token like "05v2", remove it to avoid polluting title
		// and being misclassified as tail_group later.
		if spKindEpisodePattern.MatchString(text) || episodeVersionSuffixPattern.MatchString(text) {
			tokens[bestIdx].Removed = true
		}
	}
	if bestIdx >= 0 && bestVer != "" {
		return best, bestIdx, bestVer, bestIdx
	}
	return best, bestIdx, "", -1
}

func shouldSkipPartIndexEpisodeCandidate(tokens []model.Token, idx int, inBracket bool) bool {
	if inBracket || idx <= 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	prev := strings.TrimSpace(tokens[idx-1].Text)
	if prev == "" {
		return false
	}
	if !partIndexLeadPattern.MatchString(prev) {
		return false
	}
	return hasSeasonSignalAroundToken(tokens, idx, 6)
}

func hasSeasonSignalAroundToken(tokens []model.Token, idx int, window int) bool {
	if len(tokens) == 0 || idx < 0 || idx >= len(tokens) {
		return false
	}
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := idx + window
	if end >= len(tokens) {
		end = len(tokens) - 1
	}
	for i := start; i <= end; i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if seasonSignalTokenPattern.MatchString(txt) || seasonSignalCJKTokenPattern.MatchString(txt) {
			return true
		}
	}
	return false
}

func hasPartIndexLeadBeforeToken(tokens []model.Token, idx int) bool {
	if idx <= 0 || idx >= len(tokens) {
		return false
	}
	prev := strings.TrimSpace(tokens[idx-1].Text)
	if prev == "" {
		return false
	}
	return partIndexLeadPattern.MatchString(prev)
}

func hasMatchingSeasonNumberAroundToken(tokens []model.Token, idx int, window int) bool {
	if len(tokens) == 0 || idx < 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimLeft(strings.TrimSpace(tokens[idx].Text), "0")
	if cur == "" {
		cur = "0"
	}
	if !episodeOfTotalPattern.MatchString(cur) && cur != "0" {
		return false
	}
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := idx + window
	if end >= len(tokens) {
		end = len(tokens) - 1
	}
	matchSeasonVal := func(v string) bool {
		v = strings.TrimLeft(strings.TrimSpace(v), "0")
		if v == "" {
			v = "0"
		}
		return v == cur
	}
	for i := start; i <= end; i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if m := seasonOnlyPattern.FindStringSubmatch(txt); len(m) == 2 && matchSeasonVal(m[1]) {
			return true
		}
		if m := seasonEmbeddedPattern.FindStringSubmatch(txt); len(m) == 2 && matchSeasonVal(m[1]) {
			return true
		}
		if m := seasonCJKPattern.FindStringSubmatch(txt); len(m) == 2 {
			if v := parseSmallSeasonNumberToken(m[1]); v != "" && matchSeasonVal(v) {
				return true
			}
		}
		if m := seasonOrdinalPattern.FindStringSubmatch(txt); len(m) == 2 {
			if i+1 < len(tokens) && seasonWordLoosePattern.MatchString(strings.TrimSpace(tokens[i+1].Text)) && matchSeasonVal(m[1]) {
				return true
			}
		}
		if seasonWordLoosePattern.MatchString(txt) {
			if i+1 < len(tokens) {
				if v := parseSmallSeasonNumberToken(strings.TrimSpace(tokens[i+1].Text)); v != "" && matchSeasonVal(v) {
					return true
				}
			}
		}
	}
	return false
}

func hasLaterBracketEpisodeCandidate(tokens []model.Token, from int) bool {
	if from < 0 {
		from = 0
	}
	for i := from; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if !isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" || !episodeOfTotalPattern.MatchString(txt) {
			continue
		}
		if looksLikeVolumeContextNumberToken(tokens, i) || yearPattern.MatchString(txt) {
			continue
		}
		if isTitlePrefixedDigitRangeInBracket(tokens, i) {
			continue
		}
		if hasMatchingSeasonNumberAroundToken(tokens, i, 12) && !hasDashSeparatorAroundToken(tokens, i) && !isStandaloneNumericBracketBlock(tokens, i) {
			continue
		}
		return true
	}
	return false
}

// Regular expressions hoisted out of function bodies so they are compiled once at init.
var (
	reExtractSeasonKeywordValue1 = regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})期$`)
	reExtractSeasonKeywordValue2 = regexp.MustCompile(`^(\d{1,2})期$`)
	reExtractSeasonKeywordValue3 = regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})季.+$`)
	reExtractSeasonKeywordValue4 = regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})期.+$`)
	reDetectEpisodeRange1        = regexp.MustCompile(`(?i)^第?(\d{1,4})\s*[-~]\s*(\d{1,4})(?:集|话|話|回|章)(?:\s|$)`)
	reLooksLikeDateOrTimeText1   = regexp.MustCompile(`\s*([./-])\s*`)
	reDetectEpisodeWithVersion1  = regexp.MustCompile(`^\d{1,4}$`)
)
