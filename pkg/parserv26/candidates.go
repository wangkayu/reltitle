package parserv26

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

var (
	sxxexxPattern       = regexp.MustCompile(`(?i)^s(\d{1,2})e(\d{1,4})$`)
	sxxexxVerCandPat    = regexp.MustCompile(`(?i)^s(\d{1,2})e(\d{1,4})v\d+$`)
	seasonPattern       = regexp.MustCompile(`(?i)^s(\d{1,2})$`)
	epPattern           = regexp.MustCompile(`(?i)^(?:e|ep)(\d{1,4})$`)
	epRangeTokenPat     = regexp.MustCompile(`(?i)^(?:e|ep)?(\d{1,4})[-~](?:e|ep)?(\d{1,4})$`)
	epListTokenPat      = regexp.MustCompile(`(?i)^(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?(?:,(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?)+$`)
	epListStartTokenPat = regexp.MustCompile(`(?i)^(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?(?:,(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?)*$`)
	epListJoinTokenPat  = regexp.MustCompile(`(?i)^(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?(?:,(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?)*$`)
	jpEpisodeTokenPat   = regexp.MustCompile(`^第?(\d{1,4})(話|话|集)$`)
	resPattern          = regexp.MustCompile(`(?i)^\\d{3,4}p$`)
	headParenNonGroup   = map[string]struct{}{
		"partial":  {},
		"v2":       {},
		"v3":       {},
		"v4":       {},
		"complete": {},
		"end":      {},
		"fin":      {},
		"test":     {},
	}
	pseudoGroupTokens = map[string]struct{}{
		"sub":       {},
		"subs":      {},
		"subtitle":  {},
		"subtitles": {},
		"multi":     {},
		"dual":      {},
		"audio":     {},
		"vostfr":    {},
		"webrip":    {},
		"webdl":     {},
		"web-dl":    {},
		"aac":       {},
		"dl":        {},
		"avc":       {},
		"hevc":      {},
		"rgb":       {},
		"h":         {},
		"h264":      {},
		"x264":      {},
		"x265":      {},
	}
)

func (p *Parser) BuildCandidates(tokens []model.Token) []SpanCandidate {
	if len(tokens) == 0 {
		return nil
	}

	out := make([]SpanCandidate, 0, len(tokens)*2)
	covered := make([]bool, len(tokens))

	lexCands := p.buildLexiconCandidates(tokens)
	for _, c := range lexCands {
		out = append(out, c)
		for i := c.TokenStart; i < c.TokenEnd && i < len(covered); i++ {
			if i >= 0 {
				covered[i] = true
			}
		}
	}

	lower := make([]string, 0, len(tokens))
	for _, t := range tokens {
		lower = append(lower, normalizeToken(t.Text))
	}
	parDepth := parenDepthByToken(tokens)
	brDepth := bracketDepthByToken(tokens)
	for _, pat := range p.groupPats {
		maxStart := len(lower) - len(pat)
		for i := 0; i <= maxStart; i++ {
			if !matchPattern(lower, i, pat) {
				continue
			}
			out = append(out, SpanCandidate{
				Slot:       SlotGroup,
				TokenStart: i,
				TokenEnd:   i + len(pat),
				Score:      0.97,
				Source:     "known_group_span",
			})
			for j := i; j < i+len(pat) && j < len(covered); j++ {
				if j >= 0 {
					covered[j] = true
				}
			}
		}
	}

	for i := range tokens {
		key := normalizeToken(tokens[i].Text)
		if key == "" {
			continue
		}
		if _, ok := p.groupIndex[key]; ok {
			out = append(out, SpanCandidate{
				Slot:       SlotGroup,
				TokenStart: i,
				TokenEnd:   i + 1,
				Score:      0.95,
				Source:     "known_group",
			})
			covered[i] = true
		}
	}

	if c, ok := inferHeuristicHeadBracketGroup(tokens, out); ok {
		out = append(out, c)
		for i := c.TokenStart; i < c.TokenEnd && i < len(covered); i++ {
			if i >= 0 {
				covered[i] = true
			}
		}
	}

	if c, ok := inferHeuristicHeadPlainGroup(tokens); ok {
		out = append(out, c)
		for i := c.TokenStart; i < c.TokenEnd && i < len(covered); i++ {
			if i >= 0 {
				covered[i] = true
			}
		}
	}

	if c, ok := inferHeuristicHeadParenGroup(tokens); ok {
		out = append(out, c)
		for i := c.TokenStart; i < c.TokenEnd && i < len(covered); i++ {
			if i >= 0 {
				covered[i] = true
			}
		}
	}

	if c, ok := inferHeuristicTailGroup(tokens, out, parDepth, brDepth); ok {
		out = append(out, c)
		for i := c.TokenStart; i < c.TokenEnd && i < len(covered); i++ {
			if i >= 0 {
				covered[i] = true
			}
		}
	}

	for i := range tokens {
		key := normalizeToken(tokens[i].Text)
		if key == "" {
			continue
		}

		if epListStartTokenPat.MatchString(key) && hasEpisodeListHint(tokens, i) {
			expr, end := episodeListSpanExpr(tokens, i)
			if strings.Contains(expr, ",") && epListTokenPat.MatchString(strings.ToLower(expr)) {
				out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: end, Score: 0.84, Source: "regex_ep_list_token"})
				for k := i; k < end && k < len(covered); k++ {
					covered[k] = true
				}
				continue
			}
		}

		if epRangeTokenPat.MatchString(key) {
			out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 1, Score: 0.8, Source: "regex_ep_range_token"})
			covered[i] = true
			continue
		}
		if digitPattern.MatchString(key) && len(key) <= 3 && i+2 < len(tokens) && strings.TrimSpace(tokens[i+1].Text) == "-" {
			rightKey := normalizeToken(tokens[i+2].Text)
			if digitPattern.MatchString(rightKey) && len(rightKey) <= 3 {
				out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 3, Score: 0.8, Source: "regex_plain_range_split"})
				covered[i] = true
				covered[i+1] = true
				covered[i+2] = true
				continue
			}
		}

		if i+2 < len(tokens) && epPattern.MatchString(key) && strings.TrimSpace(tokens[i+1].Text) == "-" {
			rightKey := normalizeToken(tokens[i+2].Text)
			if epPattern.MatchString(rightKey) || digitPattern.MatchString(rightKey) {
				out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 3, Score: 0.86, Source: "regex_ep_range_split"})
				covered[i] = true
				covered[i+1] = true
				covered[i+2] = true
				continue
			}
		}

		if sxxexxPattern.MatchString(key) || sxxexxVerCandPat.MatchString(key) {
			out = append(out, SpanCandidate{Slot: SlotSeason, TokenStart: i, TokenEnd: i + 1, Score: 0.85, Source: "regex_sxxexx"})
			out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 1, Score: 0.85, Source: "regex_sxxexx"})
			covered[i] = true
			continue
		}
		if seasonPattern.MatchString(key) {
			out = append(out, SpanCandidate{Slot: SlotSeason, TokenStart: i, TokenEnd: i + 1, Score: 0.78, Source: "regex_season"})
			covered[i] = true
			continue
		}
		if jpEpisodeTokenPat.MatchString(strings.TrimSpace(tokens[i].Text)) {
			out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 1, Score: 0.76, Source: "regex_jp_episode"})
			covered[i] = true
			continue
		}
		if epPattern.MatchString(key) {
			out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 1, Score: 0.75, Source: "regex_ep"})
			covered[i] = true
			continue
		}
		if digitPattern.MatchString(key) && len(key) <= 3 {
			if i > 0 && strings.TrimSpace(tokens[i-1].Text) == "-" {
				// e.g. "8 Man - 01 [Group]" or "Title - 12"
				// Avoid selecting the tail number in "01-50" range.
				nextOK := (i+1 >= len(tokens)) || isBracketToken(strings.TrimSpace(tokens[i+1].Text))
				if !(i > 1 && digitPattern.MatchString(normalizeToken(tokens[i-2].Text))) && nextOK {
					out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 1, Score: 0.62, Source: "regex_plain_ep_dash"})
				}
			}
			if i > 0 && i+1 < len(tokens) && strings.TrimSpace(tokens[i-1].Text) == "[" && strings.TrimSpace(tokens[i+1].Text) == "]" {
				out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 1, Score: 0.58, Source: "regex_plain_ep_bracket"})
			}
			if i+1 < len(tokens) && strings.TrimSpace(tokens[i+1].Text) == "[" {
				out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 1, Score: 0.56, Source: "regex_plain_ep_before_bracket"})
			}
			if i+2 < len(tokens) && strings.EqualFold(strings.TrimSpace(tokens[i+1].Text), "of") && digitPattern.MatchString(normalizeToken(tokens[i+2].Text)) {
				out = append(out, SpanCandidate{Slot: SlotEpisode, TokenStart: i, TokenEnd: i + 1, Score: 0.6, Source: "regex_plain_ep_of"})
			}
		}
	}

	for i := range tokens {
		if covered[i] {
			continue
		}
		if looksLikeTitleToken(tokens[i].Text) {
			out = append(out, SpanCandidate{
				Slot:       SlotTitle,
				TokenStart: i,
				TokenEnd:   i + 1,
				Score:      0.35,
				Source:     "title_fallback",
			})
		}
	}

	return dedupeCandidates(out)
}

func (p *Parser) buildLexiconCandidates(tokens []model.Token) []SpanCandidate {
	lower := make([]string, 0, len(tokens))
	for _, t := range tokens {
		lower = append(lower, normalizeToken(t.Text))
	}

	out := make([]SpanCandidate, 0, len(tokens))
	for _, entry := range p.cfg.Lexicon.Entries {
		for _, clause := range flattenLexiconEntry(entry) {
			if len(clause.Tokens) == 0 {
				continue
			}
			pat := normalizePattern(clause.Tokens)
			if len(pat) == 0 {
				continue
			}

			maxStart := len(lower) - len(pat)
			for i := 0; i <= maxStart; i++ {
				if !matchPattern(lower, i, pat) {
					continue
				}

				slot, ok := bestSlotFromEmit(clause.Emit)
				if !ok {
					continue
				}
				score := clause.Confidence
				if score <= 0 {
					score = 0.6
				}
				out = append(out, SpanCandidate{
					Slot:       slot,
					TokenStart: i,
					TokenEnd:   i + len(pat),
					Score:      score,
					Source:     "lexicon:" + entry.ID,
				})
			}
		}
	}
	return out
}

func flattenLexiconEntry(e config.LexiconEntry) []config.LexiconClause {
	out := make([]config.LexiconClause, 0, len(e.AnyOf)+1)
	if len(e.Tokens) > 0 {
		out = append(out, config.LexiconClause{
			Scope:      e.Scope,
			Tokens:     e.Tokens,
			Emit:       e.Emit,
			Remove:     e.Remove,
			Confidence: e.Confidence,
		})
	}
	out = append(out, e.AnyOf...)
	return out
}

func normalizePattern(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		n := normalizeToken(t)
		if n == "" {
			continue
		}
		out = append(out, n)
	}
	return out
}

func matchPattern(tokens []string, start int, pattern []string) bool {
	if start < 0 || start+len(pattern) > len(tokens) {
		return false
	}
	for i := 0; i < len(pattern); i++ {
		if tokens[start+i] != pattern[i] {
			return false
		}
	}
	return true
}

func bestSlotFromEmit(emits []config.LexiconEmit) (CandidateSlot, bool) {
	best := SlotNoise
	rank := -1
	for _, em := range emits {
		slot, ok := attrTypeToSlot(em.Type)
		if !ok {
			continue
		}
		r := slotPriority(slot)
		if r > rank {
			best = slot
			rank = r
		}
	}
	if rank < 0 {
		return "", false
	}
	return best, true
}

func slotPriority(slot CandidateSlot) int {
	switch slot {
	case SlotGroup:
		return 7
	case SlotSeason:
		return 6
	case SlotEpisode:
		return 5
	case SlotTech:
		return 4
	case SlotLanguage:
		return 3
	case SlotAlias:
		return 2
	case SlotTitle:
		return 1
	default:
		return 0
	}
}

func attrTypeToSlot(attrType string) (CandidateSlot, bool) {
	switch strings.ToLower(strings.TrimSpace(attrType)) {
	case "group", "tail_group":
		return SlotGroup, true
	case "episode", "episode_range", "special_flag", "special_index":
		return SlotEpisode, true
	case "season", "season_range":
		return SlotSeason, true
	case "source_type", "resolution", "video_codec", "audio_codec", "bit_depth", "container", "platform", "platform_region", "origin_region", "crc", "tech", "year", "version":
		return SlotTech, true
	case "sub_flag", "sub_lang", "sub_format", "sub_lang_group", "audio_flag", "audio_lang", "audio_channels", "audio_object":
		return SlotLanguage, true
	case "title_alias", "episode_title", "content_note":
		return SlotAlias, true
	case "title":
		return SlotTitle, true
	case "noise", "note":
		return SlotNoise, true
	default:
		return "", false
	}
}

func looksLikeTitleToken(token string) bool {
	s := strings.TrimSpace(token)
	if s == "" {
		return false
	}
	if isBracketToken(s) || isSeparatorToken(s) {
		return false
	}
	hasLetterOrDigit := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasLetterOrDigit = true
			break
		}
	}
	return hasLetterOrDigit
}

func isBracketToken(s string) bool {
	switch s {
	case "[", "]", "(", ")", "{", "}", "【", "】", "「", "」", "『", "』":
		return true
	default:
		return false
	}
}

func isSeparatorToken(s string) bool {
	switch s {
	case ".", "-", "－", "–", "—", "_", "/", "／", "|", "｜", "★", "☆", ";", "；", "+", "＋", ":", "：":
		return true
	default:
		return false
	}
}

func inferHeuristicTailGroup(tokens []model.Token, cands []SpanCandidate, parDepth []int, brDepth []int) (SpanCandidate, bool) {
	if len(tokens) < 3 {
		return SpanCandidate{}, false
	}
	hasTechBefore := false
	for dash := len(tokens) - 2; dash >= 0; dash-- {
		if strings.TrimSpace(tokens[dash].Text) != "-" || isBracketToken(strings.TrimSpace(tokens[dash+1].Text)) {
			continue
		}
		start := dash + 1
		if start < 0 || start >= len(tokens) {
			continue
		}
		if len(parDepth) == len(tokens) && start < len(parDepth) && parDepth[start] > 0 {
			continue
		}
		if len(brDepth) == len(tokens) && start < len(brDepth) && brDepth[start] > 0 {
			continue
		}

		hasTechBefore = false
		for _, c := range cands {
			if c.Score < 0.65 {
				continue
			}
			if c.TokenStart >= start {
				continue
			}
			if c.Slot == SlotTech || c.Slot == SlotLanguage {
				hasTechBefore = true
				break
			}
		}
		if !hasTechBefore {
			continue
		}

		// Expand left for patterns like "Tsundere - Raws" or "Yangire - Raws".
		for start-2 >= 0 {
			sep := strings.TrimSpace(tokens[start-1].Text)
			prev := strings.TrimSpace(tokens[start-2].Text)
			if !isSeparatorToken(sep) || isBracketToken(prev) || prev == "" || isLikelyTechOrLang(prev) {
				break
			}
			nPrev := normalizeToken(prev)
			if digitPattern.MatchString(nPrev) {
				break
			}
			if len(nPrev) <= 2 && !isLikelyGroupToken(nPrev) {
				break
			}
			start -= 2
		}

		end := start
		wordCount := 0
		numCount := 0
		longAlphaCount := 0
		shortAlphaCount := 0
		words := make([]string, 0, 4)
		for end < len(tokens) {
			t := strings.TrimSpace(tokens[end].Text)
			if t == "" {
				break
			}
			if isBracketToken(t) {
				break
			}
			if isLikelyTechOrLang(t) {
				wordCount = 0
				break
			}
			if !isSeparatorToken(t) {
				wordCount++
				nt := normalizeToken(t)
				words = append(words, nt)
				if digitPattern.MatchString(nt) {
					numCount++
				}
				if len(nt) >= 4 && hasAlpha(nt) {
					longAlphaCount++
				}
				if len(nt) >= 2 && len(nt) <= 4 && hasAlpha(nt) {
					shortAlphaCount++
				}
			}
			end++
			if end-start > 8 {
				break
			}
		}
		if end <= start || wordCount == 0 || wordCount > 3 {
			continue
		}
		if numCount > 0 {
			continue
		}
		if isPseudoGroup(words) {
			continue
		}
		if longAlphaCount == 0 && shortAlphaCount == 0 {
			continue
		}
		return SpanCandidate{
			Slot:       SlotGroup,
			TokenStart: start,
			TokenEnd:   end,
			Score:      0.7,
			Source:     "heuristic_tail_group",
		}, true
	}
	return SpanCandidate{}, false
}

func isLikelyGroupToken(t string) bool {
	if t == "" {
		return false
	}
	if hasAlpha(t) && len(t) >= 3 {
		return true
	}
	return false
}

func hasAlpha(t string) bool {
	for _, r := range t {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func inferHeuristicHeadParenGroup(tokens []model.Token) (SpanCandidate, bool) {
	if len(tokens) < 3 {
		return SpanCandidate{}, false
	}
	if strings.TrimSpace(tokens[0].Text) != "(" {
		return SpanCandidate{}, false
	}
	closeIdx := -1
	for i := 1; i < len(tokens) && i <= 8; i++ {
		if strings.TrimSpace(tokens[i].Text) == ")" {
			closeIdx = i
			break
		}
	}
	if closeIdx <= 1 {
		return SpanCandidate{}, false
	}
	wordCount := 0
	var content []string
	hasCJK := false
	for i := 1; i < closeIdx; i++ {
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" || isSeparatorToken(t) {
			continue
		}
		if isLikelyTechOrLang(t) || resPattern.MatchString(normalizeToken(t)) {
			return SpanCandidate{}, false
		}
		wordCount++
		content = append(content, normalizeToken(t))
		if containsCJK(t) {
			hasCJK = true
		}
	}
	if wordCount == 0 || wordCount > 4 {
		return SpanCandidate{}, false
	}
	if hasCJK {
		return SpanCandidate{}, false
	}
	if wordCount == 1 {
		if _, reject := headParenNonGroup[content[0]]; reject {
			return SpanCandidate{}, false
		}
	}
	return SpanCandidate{
		Slot:       SlotGroup,
		TokenStart: 1,
		TokenEnd:   closeIdx,
		Score:      0.72,
		Source:     "heuristic_head_paren_group",
	}, true
}

func inferHeuristicHeadBracketGroup(tokens []model.Token, cands []SpanCandidate) (SpanCandidate, bool) {
	if len(tokens) < 3 {
		return SpanCandidate{}, false
	}
	_ = cands
	limit := 12
	for i := 0; i < len(tokens) && i <= limit; i++ {
		if strings.TrimSpace(tokens[i].Text) != "[" {
			continue
		}
		closeIdx := -1
		for j := i + 1; j < len(tokens) && j <= i+10; j++ {
			if strings.TrimSpace(tokens[j].Text) == "]" {
				closeIdx = j
				break
			}
		}
		if closeIdx <= i+1 {
			continue
		}
		wordCount := 0
		groupHint := false
		for k := i + 1; k < closeIdx; k++ {
			t := strings.TrimSpace(tokens[k].Text)
			if t == "" || isSeparatorToken(t) {
				continue
			}
			nt := normalizeToken(t)
			if isLikelyTechOrLang(t) || resPattern.MatchString(nt) {
				wordCount = 0
				break
			}
			if _, bad := pseudoGroupTokens[nt]; bad {
				wordCount = 0
				break
			}
			if containsCJK(t) && strings.Contains(t, "字幕组") {
				groupHint = true
			}
			if strings.Contains(nt, "raw") || strings.Contains(nt, "subs") {
				groupHint = true
			}
			wordCount++
		}
		if wordCount == 0 || wordCount > 6 {
			continue
		}
		if !groupHint && wordCount == 1 {
			// avoid weak single-token bracket being treated as group by default
			continue
		}
		return SpanCandidate{
			Slot:       SlotGroup,
			TokenStart: i + 1,
			TokenEnd:   closeIdx,
			Score:      0.8,
			Source:     "heuristic_head_bracket_group",
		}, true
	}
	return SpanCandidate{}, false
}

func inferHeuristicHeadPlainGroup(tokens []model.Token) (SpanCandidate, bool) {
	if len(tokens) < 2 {
		return SpanCandidate{}, false
	}
	head := strings.TrimSpace(tokens[0].Text)
	if head == "" || isBracketToken(head) || isSeparatorToken(head) || isLikelyTechOrLang(head) {
		return SpanCandidate{}, false
	}
	nHead := normalizeToken(head)
	if !(strings.Contains(nHead, "raw") || strings.Contains(nHead, "fansub") || strings.HasSuffix(nHead, "subs") || strings.HasSuffix(nHead, "sub")) {
		return SpanCandidate{}, false
	}

	next := strings.TrimSpace(tokens[1].Text)
	if next == "" || isBracketToken(next) || isSeparatorToken(next) || isLikelyTechOrLang(next) || !looksLikeTitleToken(next) {
		return SpanCandidate{}, false
	}

	return SpanCandidate{
		Slot:       SlotGroup,
		TokenStart: 0,
		TokenEnd:   1,
		Score:      0.68,
		Source:     "heuristic_head_plain_group",
	}, true
}

func isLikelyTechOrLang(token string) bool {
	t := normalizeToken(token)
	if t == "" {
		return false
	}
	switch t {
	case "web", "webrip", "web-dl", "webdl", "dl", "blu-ray", "bluray", "bdrip", "dvdrip", "hdtv", "vhs", "x264", "x265", "h264", "h265", "hevc", "av1", "avc", "aac", "ac3", "flac", "vostfr", "multi", "dual", "rgb", "8bit", "10bit", "nf", "cr", "baha", "unext", "yuzukitv", "tbs":
		return true
	}
	return resPattern.MatchString(t)
}

func isPseudoGroup(words []string) bool {
	if len(words) == 0 {
		return true
	}
	for _, w := range words {
		if _, bad := pseudoGroupTokens[w]; bad {
			return true
		}
	}
	if len(words) == 1 {
		w := words[0]
		if strings.HasSuffix(w, "subs") || strings.HasSuffix(w, "audio") {
			return true
		}
	}
	return false
}

func containsCJK(t string) bool {
	for _, r := range t {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

func parenDepthByToken(tokens []model.Token) []int {
	depth := 0
	out := make([]int, len(tokens))
	for i, tk := range tokens {
		t := strings.TrimSpace(tk.Text)
		if t == ")" || t == "）" {
			if depth > 0 {
				depth--
			}
		}
		out[i] = depth
		if t == "(" || t == "（" {
			depth++
		}
	}
	return out
}

func bracketDepthByToken(tokens []model.Token) []int {
	depth := 0
	out := make([]int, len(tokens))
	for i, tk := range tokens {
		t := strings.TrimSpace(tk.Text)
		if t == "]" || t == "】" {
			if depth > 0 {
				depth--
			}
		}
		out[i] = depth
		if t == "[" || t == "【" {
			depth++
		}
	}
	return out
}

func firstTechBoundaryFromCandidates(cands []SpanCandidate, tokenCount int) int {
	b := tokenCount
	for _, c := range cands {
		if c.Score < 0.65 {
			continue
		}
		switch c.Slot {
		case SlotTech, SlotLanguage, SlotNoise:
			if c.TokenStart >= 0 && c.TokenStart < b {
				b = c.TokenStart
			}
		}
	}
	if b == tokenCount {
		return -1
	}
	return b
}

func normalizeToken(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func hasEpisodeListHint(tokens []model.Token, idx int) bool {
	for j := idx - 1; j >= 0; j-- {
		t := strings.TrimSpace(tokens[j].Text)
		if t == "" || isSeparatorToken(t) {
			continue
		}
		n := normalizeToken(t)
		return n == "tv" || n == "ep" || n == "episode" || n == "ova" || n == "oad" || n == "ona" || n == "sp"
	}
	return false
}

func episodeListSpanExpr(tokens []model.Token, start int) (string, int) {
	expr := strings.TrimSpace(tokens[start].Text)
	end := start + 1
	j := start
	for j+2 < len(tokens) {
		sep := strings.TrimSpace(tokens[j+1].Text)
		if sep != "-" && sep != "~" {
			break
		}
		nxt := normalizeToken(strings.TrimSpace(tokens[j+2].Text))
		if !epListJoinTokenPat.MatchString(nxt) {
			break
		}
		expr += sep + nxt
		j += 2
		end = j + 1
	}
	return expr, end
}

func dedupeCandidates(in []SpanCandidate) []SpanCandidate {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]int, len(in))
	out := make([]SpanCandidate, 0, len(in))
	for _, c := range in {
		key := string(c.Slot) + "|" + itoa(c.TokenStart) + "|" + itoa(c.TokenEnd)
		if idx, ok := seen[key]; ok {
			if c.Score > out[idx].Score {
				out[idx] = c
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, c)
	}
	return out
}

func buildSignals(cands []SpanCandidate) []string {
	if len(cands) == 0 {
		return []string{"cand:none"}
	}
	count := map[CandidateSlot]int{}
	for _, c := range cands {
		count[c.Slot]++
	}
	s := make([]string, 0, len(count)+1)
	s = append(s, "cand:total="+itoa(len(cands)))
	for _, slot := range []CandidateSlot{SlotGroup, SlotTitle, SlotAlias, SlotEpisode, SlotSeason, SlotTech, SlotLanguage, SlotNoise} {
		if n := count[slot]; n > 0 {
			s = append(s, "cand:"+string(slot)+"="+itoa(n))
		}
	}
	return s
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
