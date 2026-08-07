package parser

import (
	"regexp"
	"strings"

	"title-parser/pkg/model"
	"title-parser/pkg/tokenize"
)

func (p *Parser) inferAliasesFromAdjacentBrackets(tokens []model.Token, titleEnd int, episodeIdx int, episodeRangeIdx int) []string {
	if titleEnd < 0 {
		return nil
	}
	epStart := len(tokens)
	if episodeRangeIdx >= 0 && episodeRangeIdx < epStart {
		epStart = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < epStart {
		epStart = episodeIdx
	}
	if epStart <= titleEnd {
		return nil
	}
	aliases := make([]string, 0, 4)
	for i := titleEnd; i < epStart; i++ {
		open, close, ok := headAliasBracketPair(tokens[i].Text)
		if !ok {
			continue
		}
		end := -1
		depth := 0
		for j := i; j < epStart; j++ {
			if tokens[j].Text == open {
				depth++
			} else if tokens[j].Text == close {
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
			continue
		}
		parts := make([]string, 0, end-i)
		for k := i + 1; k < end; k++ {
			t := strings.TrimSpace(tokens[k].Text)
			if t == "" {
				continue
			}
			parts = append(parts, t)
		}
		inner := strings.TrimSpace(strings.Join(parts, " "))
		if inner == "" {
			i = end
			continue
		}
		if isLikelyMetaBracketText(inner) || looksLikeDateOrTimeText(inner) {
			i = end
			continue
		}
		splits := splitByStrongTitleDelims(inner)
		for _, s := range splits {
			s = cleanAliasToken(s)
			if stripped := stripExplicitSeasonSuffix(s); stripped != "" {
				s = stripped
			}
			if !p.isValidAliasToken(s) {
				continue
			}
			aliases = append(aliases, s)
		}
		i = end
	}
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}

func (p *Parser) inferBracketTitleFallback(tokens []model.Token, scanStart int, episodeIdx int, episodeRangeIdx int) (string, []string, int, int) {
	if scanStart < 0 {
		scanStart = 0
	}
	if scanStart >= len(tokens) {
		return "", nil, -1, -1
	}
	epStart := len(tokens)
	if episodeRangeIdx >= 0 && episodeRangeIdx < epStart {
		epStart = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < epStart {
		epStart = episodeIdx
	}
	if epStart <= scanStart {
		return "", nil, -1, -1
	}

	for i := scanStart; i < epStart; i++ {
		open, close, ok := headAliasBracketPair(tokens[i].Text)
		if !ok {
			continue
		}
		end := -1
		depth := 0
		for j := i; j < epStart; j++ {
			if tokens[j].Text == open {
				depth++
				continue
			}
			if tokens[j].Text == close {
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
			continue
		}

		parts := make([]string, 0, end-i)
		for k := i + 1; k < end; k++ {
			t := strings.TrimSpace(tokens[k].Text)
			if t == "" {
				continue
			}
			if t == "_" || t == "-" || t == "." || t == "|" || t == "｜" {
				continue
			}
			parts = append(parts, t)
		}
		inner := strings.TrimSpace(strings.Join(parts, " "))
		if inner == "" || isLikelyMetaBracketText(inner) {
			i = end
			continue
		}
		main, aliases := inferTitleAndAliasesFromTitleCanonical(inner, p.isValidAliasToken)
		main = sanitizePrimaryTitleForOutput(main)
		if strings.TrimSpace(main) == "" {
			i = end
			continue
		}
		if isLikelyGroupLikeTitleFallback(main) {
			i = end
			continue
		}
		return main, aliases, i + 1, end
	}
	return "", nil, -1, -1
}

var bracketTitleFallbackGroupLikePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._&+\-]{1,31}$`)

func isLikelyGroupLikeTitleFallback(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return true
	}
	if hasCJK(ss) {
		return false
	}
	if strings.ContainsAny(ss, " /|｜;；:\\") {
		return false
	}
	if strings.Contains(ss, " ") {
		return false
	}
	up := strings.ToUpper(ss)
	if strings.Contains(up, "RAWS") || strings.Contains(up, "SUBS") {
		return true
	}
	return bracketTitleFallbackGroupLikePattern.MatchString(ss)
}

var tailGroupFromRawTitlePattern = regexp.MustCompile(`(?i)(?:^|\s)-(?:\s*)?([A-Za-z][A-Za-z0-9]{1,24}(?:-[A-Za-z0-9]{1,24}){0,4}-(?:Raws|Subs))\b`)
var tailGroupFromRawTitleAnyPattern = regexp.MustCompile(`(?i)-(\s*)?([A-Za-z][A-Za-z0-9]{2,20})\s*$`)
var tailGroupFromRawTitleBracketPattern = regexp.MustCompile(`(?:\[([^\[\]]{1,80})\]|【([^【】]{1,80})】)\s*$`)

func (p *Parser) detectTailGroupFromRawTitle(rawTitle string) string {
	s := strings.TrimSpace(rawTitle)
	if s == "" {
		return ""
	}
	if regexp.MustCompile(`(?i)\bcustom\b\s*$`).MatchString(s) {
		return ""
	}
	if regexp.MustCompile(`(?i)\bhi[\s_-]*res\b\s*$`).MatchString(s) {
		return ""
	}
	if m := tailGroupFromRawTitlePattern.FindStringSubmatch(s); len(m) == 2 {
		g := strings.TrimSpace(m[1])
		if g != "" {
			return g
		}
	}
	// Generic release tail group like "...-HDSWEB" / "...-VARYG".
	if m := tailGroupFromRawTitleAnyPattern.FindStringSubmatch(s); len(m) == 3 {
		g := strings.TrimSpace(m[2])
		if g != "" && isTailGroupToken(g) {
			return g
		}
	}
	// Tail bracket group like "... [SnC-Fansub]".
	if m := tailGroupFromRawTitleBracketPattern.FindStringSubmatch(s); len(m) == 3 {
		cand := strings.TrimSpace(m[1])
		if cand == "" {
			cand = strings.TrimSpace(m[2])
		}
		cand = normalizeGroupText(cand)
		if p.isTailBracketGroupCandidate(cand) {
			return cand
		}
	}
	return ""
}

func (p *Parser) isTailBracketGroupCandidate(s string) bool {
	s = normalizeGroupText(s)
	if s == "" {
		return false
	}
	if !isGroupCandidateText(s) {
		return false
	}
	if isLikelySubtitleBracketGroupText(s) {
		return false
	}
	if isLikelyMetaBracketText(s) || isEpisodeLikeBracketText(s) {
		return false
	}
	if _, known := p.isKnownGroup(s); known {
		return true
	}
	return hasExplicitGroupIdentityMarker(s)
}

var explicitGroupIdentitySuffixPattern = regexp.MustCompile(`(?i)(?:fansub|subs?|raws?|team|studio|group)$`)

func hasExplicitGroupIdentityMarker(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	for _, suffix := range []string{"字幕组", "字幕組", "字幕社", "汉化组", "漢化組", "压制组", "壓制組", "制作组", "製作組", "搬运组", "搬運組", "组", "組"} {
		if strings.HasSuffix(trimmed, suffix) {
			return true
		}
	}
	return explicitGroupIdentitySuffixPattern.MatchString(trimmed)
}

func isLikelySubtitleBracketGroupText(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return false
	}
	if reAliasSubtitleCJKMetaPhrase.MatchString(strings.TrimSpace(s)) {
		return true
	}
	// Multi-language subtitle markers like "Multi language subs", "Multi subs", etc.
	if regexp.MustCompile(`(?i)^multi\s+language\s+subs?$`).MatchString(strings.TrimSpace(s)) {
		return true
	}
	if regexp.MustCompile(`(?i)^sub(?:title|titles)?(?:[_ .-]+[\p{L}]{2,16})+$`).MatchString(strings.TrimSpace(s)) {
		return true
	}
	return regexp.MustCompile(`(?i)^sub(?:title|titles)?[_ .-]?[a-z]{2,8}$`).MatchString(low)
}

func detectHeadBracketGroupFromRawTitle(rawTitle string) string {
	rawTitle = strings.TrimSpace(rawTitle)
	if rawTitle == "" {
		return ""
	}
	rawTokens := tokenize.Tokenize(rawTitle)
	if len(rawTokens) == 0 {
		return ""
	}
	tokens := make([]model.Token, 0, len(rawTokens))
	for _, t := range rawTokens {
		tokens = append(tokens, model.Token{Text: t})
	}
	g, _ := extractHeadBracketGroup(tokens)
	return strings.TrimSpace(g)
}

func promoteFrenchMultiNoteToAudioFlag(attrs []model.Attribute) {
	if len(attrs) == 0 {
		return
	}
	hasFrench := false
	for i := range attrs {
		if attrs[i].Type == model.AttrSubFlag {
			if strings.EqualFold(strings.TrimSpace(attrs[i].Normalized), "VOSTFR") {
				hasFrench = true
				break
			}
		}
		if attrs[i].Type == model.AttrAudioLang {
			if strings.EqualFold(strings.TrimSpace(attrs[i].Normalized), "FRA") {
				hasFrench = true
				break
			}
		}
		if attrs[i].Type == model.AttrNote {
			n := strings.ToUpper(strings.TrimSpace(attrs[i].Normalized))
			if n == "VF" || n == "FRENCH" || n == "FRANCAIS" || n == "TRUEFRENCH" {
				hasFrench = true
				break
			}
		}
	}
	if !hasFrench {
		return
	}
	for i := range attrs {
		if attrs[i].Type != model.AttrNote {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(attrs[i].Normalized)) != "MULTI" {
			continue
		}
		attrs[i].Type = model.AttrAudioFlag
		attrs[i].Normalized = "MULTI_AUDIO"
		if attrs[i].Confidence < 0.65 {
			attrs[i].Confidence = 0.65
		}
		attrs[i].Source = "heuristic_multi_audio"
	}
}
