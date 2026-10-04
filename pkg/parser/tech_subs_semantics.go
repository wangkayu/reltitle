package parser

import (
	"strings"

	"github.com/wangkayu/reltitle/pkg/model"
)

func hasSplitSCTCMarker(s string) bool {
	noSpace := strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if noSpace == "" {
		return false
	}
	for _, pat := range []string{
		"简/繁", "简／繁", "簡/繁", "簡／繁",
		"繁/简", "繁／简", "繁/簡", "繁／簡",
		"简體/繁體", "简体/繁体", "簡體/繁體", "繁體/簡體",
		"简中/繁中", "簡中/繁中", "繁中/简中", "繁中/簡中",
	} {
		if strings.Contains(noSpace, pat) {
			return true
		}
	}
	return false
}

func containsASCIIWordLike(s string, markers ...string) bool {
	if strings.TrimSpace(s) == "" || len(markers) == 0 {
		return false
	}
	low := strings.ToLower(s)
	isAlphaNum := func(b byte) bool {
		return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
	}
	for _, marker := range markers {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker == "" {
			continue
		}
		searchFrom := 0
		for searchFrom < len(low) {
			idx := strings.Index(low[searchFrom:], marker)
			if idx < 0 {
				break
			}
			idx += searchFrom
			beforeOK := idx == 0 || !isAlphaNum(low[idx-1])
			after := idx + len(marker)
			afterOK := after >= len(low) || !isAlphaNum(low[after])
			if beforeOK && afterOK {
				return true
			}
			searchFrom = idx + 1
		}
	}
	return false
}

// extractTechSubsSemanticsAndRemoveFromTokens scans bracket/paren blocks and
// plain token segments for subtitle semantics, emits subtitle attributes, and
// removes the consumed tokens. The extraction machinery (segment scanning,
// flag/language/format detection) lives in tech_subs_extract.go.
func extractTechSubsSemanticsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if len(tokens) == 0 || attrs == nil {
		return
	}
	e := &techSubsSemanticsExtract{tokens: tokens, attrs: attrs, headGroupStart: -1, headGroupEnd: -1}
	e.techStart, e.hasTechStart = firstTechTokenIndexFromAttrs(*attrs)
	if !e.hasTechStart {
		e.techStart = 0
	}
	if g, span := extractHeadBracketGroup(tokens); strings.TrimSpace(g) != "" {
		e.headGroupStart = span[0]
		e.headGroupEnd = span[1]
	}
	e.run()
}
