package parser

import (
	"regexp"
	"strings"

	"title-parser/pkg/model"
)

// protectTitleFractionSpans marks "N/N" fraction-like digit segments in the
// title zone as Protected, so season/episode/range detection never mistakes
// title digits for episode markers.
//
// This is a pure structural rule (no title dictionary involved):
//   - segment shape must be  数字 斜杠 数字   (e.g. "22/7", "22／7", "7/11")
//   - a following "N/N/N" run (episode selection list) is NOT protected
//   - a "17 - 26"-style range inside the chain invalidates it (such a range is
//     indistinguishable from a real batch range, e.g. "Made in Abyss 01-13 /
//     メイドインアビス 01-13" must keep its range)
//   - the chain must end at an episode slot / tech paren / block boundary / EOL
//
// Titles like "藤本树 17-26" (digits AFTER words) deliberately do NOT match;
// they are covered by the title dictionary instead.
func protectTitleFractionSpans(tokens []model.Token, groupEnd int) {
	if len(tokens) == 0 {
		return
	}
	start := groupEnd
	if start < 0 || start >= len(tokens) {
		start = 0
	}
	isSlash := func(s string) bool { return s == "/" || s == "／" }
	isNum := func(s string) bool { return episodeOfTotalPattern.MatchString(strings.TrimSpace(s)) }

	for i := start; i+2 < len(tokens); i++ {
		if tokens[i].Protected || tokens[i].Removed {
			continue
		}
		if !isNum(tokens[i].Text) || !isSlash(tokens[i+1].Text) || !isNum(tokens[i+2].Text) {
			continue
		}
		// Episode selection list "01 / 03 / 05": keep it detectable, do not protect.
		if i+3 < len(tokens) && isSlash(tokens[i+3].Text) && i+4 < len(tokens) && isNum(tokens[i+4].Text) {
			continue
		}
		end := i + 3
		valid := true
		k := i + 3
		for k < len(tokens) {
			t := strings.TrimSpace(tokens[k].Text)
			if t == "" {
				k++
				continue
			}
			if isSlash(t) {
				k++
				continue
			}
			if isNum(t) {
				// A "N - M" range inside the chain (digits AFTER a word segment)
				// invalidates the whole chain: it may be a real batch range.
				if k+2 < len(tokens) && strings.TrimSpace(tokens[k+1].Text) == "-" && isNum(tokens[k+2].Text) {
					valid = false
					break
				}
				end = k + 1
				k++
				continue
			}
			// Word token (latin word / CJK / other title text).
			if t == "-" || t == "~" || t == "[" || t == "]" || t == "(" || t == ")" || t == "【" || t == "】" {
				break
			}
			end = k + 1
			k++
			continue
		}
		if !valid {
			continue
		}
		if !titleChainEndsAtBoundary(tokens, end) {
			continue
		}
		for j := i; j < end; j++ {
			tokens[j].Protected = true
		}
	}
}

// epSlotBlockPattern matches bracket content that is clearly an episode slot:
// "04" / "01-13" / "01-13END" / "1-24FIN" / "03v2".
var epSlotBlockPattern = regexp.MustCompile(`(?i)^\d{1,4}(?:\s*[-~]\s*\d{1,4})?(?:end|fin|final|v\d+)?$`)

// titleChainEndsAtBoundary reports whether the chain [.., end) is followed by a
// legitimate boundary: EOL, an episode slot ("- 04"), a tech/year paren, or a
// bracket block that is an episode slot.
func titleChainEndsAtBoundary(tokens []model.Token, end int) bool {
	if end >= len(tokens) {
		return true
	}
	t := strings.TrimSpace(tokens[end].Text)
	switch t {
	case "-", "~":
		if end+1 < len(tokens) && episodeOfTotalPattern.MatchString(strings.TrimSpace(tokens[end+1].Text)) {
			return true
		}
		return false
	case "(", "（":
		return true
	case "[":
		if inner := bracketBlockInnerText(tokens, end); inner != "" {
			return epSlotBlockPattern.MatchString(strings.TrimSpace(inner))
		}
		return false
	case "]":
		// The chain ended inside a bracket block; the following bracket block
		// must be an episode slot for the chain to be a valid title.
		for j := end + 1; j < len(tokens); j++ {
			if tokens[j].Text == "[" || tokens[j].Text == "【" {
				if inner := bracketBlockInnerText(tokens, j); inner != "" {
					return epSlotBlockPattern.MatchString(strings.TrimSpace(inner))
				}
				return false
			}
			if tokens[j].Removed || tokens[j].Text == "/" || tokens[j].Text == "／" {
				continue
			}
			return false
		}
		return false
	}
	return false
}
