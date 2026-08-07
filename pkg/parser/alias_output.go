package parser

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"title-parser/pkg/model"
	"title-parser/pkg/tokenize"
)

func mergeConsecutiveLatin(parts []string) []string {
	out := make([]string, 0, len(parts))
	buf := make([]string, 0, 8)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		out = append(out, strings.Join(buf, " "))
		buf = buf[:0]
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if isMostlyLatinToken(p) {
			buf = append(buf, p)
			continue
		}
		flush()
		out = append(out, p)
	}
	flush()
	return out
}

func extractStrictCustomReleaseTagAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	hasReleaseMetaForCustomUpper := func() bool {
		for i := 0; i < len(tokens); i++ {
			if tokens[i].Removed {
				continue
			}
			t := strings.ToUpper(strings.TrimSpace(tokens[i].Text))
			if t == "" {
				continue
			}
			if t == "VOSTFR" || t == "VF" || t == "MULTI" || t == "BR" || t == "BDRIP" || t == "DVDRIP" {
				return true
			}
		}
		return false
	}
	hasFrenchLangMeta := func() bool {
		for i := 0; i < len(tokens); i++ {
			if tokens[i].Removed {
				continue
			}
			t := strings.ToUpper(strings.TrimSpace(tokens[i].Text))
			if t == "" {
				continue
			}
			if t == "VOSTFR" || t == "VF" || t == "FRENCH" || t == "FRANCAIS" || t == "TRUEFRENCH" {
				return true
			}
		}
		return false
	}

	allowUpper := hasReleaseMetaForCustomUpper()
	allowTitleCase := hasFrenchLangMeta()
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		// Only treat CUSTOM as release tag when the title also contains strong release meta.
		if txt == "CUSTOM" {
			if !allowUpper {
				continue
			}
		} else if txt == "Custom" {
			// TitleCase 'Custom' is ambiguous. Only treat it as a release tag when French language signals exist.
			if !allowTitleCase {
				continue
			}
		} else {
			continue
		}
		*out = append(*out, model.Attribute{Type: model.AttrEdition, Normalized: "CUSTOM", TokenStart: i, TokenEnd: i + 1, Confidence: 0.6, Source: "custom_tag"})
		tokens[i].Removed = true
		// If surrounded by parentheses, remove them too.
		if i-1 >= 0 {
			p := strings.TrimSpace(tokens[i-1].Text)
			if p == "(" || p == "（" {
				tokens[i-1].Removed = true
			}
		}
		if i+1 < len(tokens) {
			n := strings.TrimSpace(tokens[i+1].Text)
			if n == ")" || n == "）" {
				tokens[i+1].Removed = true
			}
		}
	}
}

func extractGuardedDVDRIPReleaseTagAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	// Guard: only treat DVDRIP as release meta if other strong meta/tech signals exist.
	// This avoids invading the title block in rare cases where DVDRIP is part of a real title.
	hasStrongMeta := func() bool {
		for i := 0; i < len(tokens); i++ {
			if tokens[i].Removed {
				continue
			}
			t := strings.ToUpper(strings.TrimSpace(tokens[i].Text))
			if t == "" {
				continue
			}
			// Language / release ecosystem signals
			if t == "VOSTFR" || t == "VF" || t == "MULTI" || t == "BR" || t == "BDRIP" {
				return true
			}
			// Common codec / container / audio signals
			if t == "X264" || t == "X265" || t == "H264" || t == "H265" || t == "AVC" || t == "HEVC" || t == "AAC" || t == "AC3" || t == "EAC3" || t == "DDP" || t == "FLAC" || t == "MKV" || t == "MP4" {
				return true
			}
			// Common resolution signals
			if strings.HasSuffix(t, "P") {
				if regexp.MustCompile(`^\d{3,4}P$`).MatchString(t) {
					return true
				}
			}
		}
		return false
	}

	if !hasStrongMeta() {
		return
	}

	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if !strings.EqualFold(txt, "DVDRIP") {
			continue
		}
		*out = append(*out, model.Attribute{Type: model.AttrSourceType, Normalized: "DVDRip", TokenStart: i, TokenEnd: i + 1, Confidence: 0.7, Source: "dvdrip_tag"})
		tokens[i].Removed = true
	}
}

func isMostlyLatinToken(s string) bool {
	hasLetter := false
	for _, r := range s {
		if ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') {
			hasLetter = true
			continue
		}
		if ('0' <= r && r <= '9') || r == '-' || r == '\'' {
			continue
		}
		if r > 127 {
			return false
		}
	}
	return hasLetter
}

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

func hasKana(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return true
		}
	}
	return false
}

func hasLatin(s string) bool {
	for _, r := range s {
		if ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') {
			return true
		}
	}
	return false
}

func trimOuterWrappedPair(s string, open rune, close rune) (string, bool) {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) < 2 || rs[0] != open || rs[len(rs)-1] != close {
		return s, false
	}
	depth := 0
	for i, r := range rs {
		if r == open {
			depth++
		} else if r == close {
			depth--
			if depth < 0 {
				return s, false
			}
			if depth == 0 && i < len(rs)-1 {
				// Outer wrapper closes before the end => do not strip.
				return s, false
			}
		}
	}
	if depth != 0 {
		return s, false
	}
	return strings.TrimSpace(string(rs[1 : len(rs)-1])), true
}

func trimOuterWrappedBrackets(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	pairs := [][2]rune{
		{'[', ']'},
		{'(', ')'},
		{'{', '}'},
		{'【', '】'},
		{'（', '）'},
		{'「', '」'},
		{'『', '』'},
	}
	for {
		changed := false
		for _, p := range pairs {
			if inner, ok := trimOuterWrappedPair(s, p[0], p[1]); ok {
				s = inner
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return strings.TrimSpace(s)
}

func cleanAliasToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = trimOuterWrappedBrackets(s)
	s = normalizeTitleMain(s)
	s = stripFileExtSuffix(s)
	return strings.TrimSpace(s)
}

func (p *Parser) normalizeAliasCandidateForOutput(s string) string {
	ss := cleanAliasToken(s)
	ss = strings.TrimSpace(ss)
	if ss == "" {
		return ""
	}
	prev := ""
	for ss != prev {
		prev = ss
		ss = reAliasTrimTVSeasonTail.ReplaceAllString(ss, "")
		ss = reAliasTrimSeasonWordTail.ReplaceAllString(ss, "")
		ss = reAliasTrimEpisodeWordTail.ReplaceAllString(ss, "")
		ss = p.trimAliasPlatformTail(ss)
		ss = reAliasTrimTVAnimePrefix.ReplaceAllString(ss, "")
		if p != nil && len(p.aliasInlineStripWords) > 0 {
			fields := strings.Fields(ss)
			if len(fields) > 0 {
				kept := make([]string, 0, len(fields))
				for _, f := range fields {
					norm := strings.Trim(f, "[](){}【】（）.,;:!?'\"-_")
					if norm == "" {
						continue
					}
					if _, ok := p.aliasInlineStripWords[strings.ToLower(norm)]; ok {
						continue
					}
					kept = append(kept, f)
				}
				ss = strings.Join(kept, " ")
			}
		}
		ss = reTitleTrimMetaTail.ReplaceAllString(ss, "")
		ss = reTitleTrimVolTail.ReplaceAllString(ss, "")
		ss = regexp.MustCompile(`\s+`).ReplaceAllString(ss, " ")
		ss = strings.TrimSpace(ss)
		ss = strings.Trim(ss, "-_/;:,+ ")
	}
	return strings.TrimSpace(cleanAliasToken(ss))
}

func (p *Parser) trimAliasPlatformTail(s string) string {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return ""
	}
	// Keep legacy fallback for complex tails; dictionary pass runs first.
	legacyTrim := func(v string) string {
		return strings.TrimSpace(reAliasTrimPlatformTail.ReplaceAllString(v, ""))
	}
	if p == nil || len(p.platformCanonical) == 0 {
		return legacyTrim(ss)
	}
	for {
		fields := strings.Fields(ss)
		if len(fields) == 0 {
			return ""
		}
		removed := 0
		last := strings.ToLower(strings.Trim(fields[len(fields)-1], "[](){}【】（）.,;:!?\"'_-"))
		if last != "" {
			if _, ok := p.platformCanonical[last]; ok {
				removed = 1
			} else if last == "tv" {
				removed = 1
			}
		}
		if len(fields) >= 2 {
			last2a := strings.ToLower(strings.Trim(fields[len(fields)-2], "[](){}【】（）.,;:!?\"'_-"))
			last2b := strings.ToLower(strings.Trim(fields[len(fields)-1], "[](){}【】（）.,;:!?\"'_-"))
			if last2a != "" && last2b != "" {
				pair := last2a + " " + last2b
				if pair == "france tv" {
					removed = 2
				}
			}
		}
		if len(fields) >= 3 {
			a := strings.ToLower(strings.Trim(fields[len(fields)-3], "[](){}【】（）.,;:!?\"'_-"))
			b := strings.ToLower(strings.Trim(fields[len(fields)-2], "[](){}【】（）.,;:!?\"'_-"))
			c := strings.ToLower(strings.Trim(fields[len(fields)-1], "[](){}【】（）.,;:!?\"'_-"))
			if a == "b-global" && b == "donghua" && c == "tv" {
				removed = 3
			}
		}
		if removed == 0 {
			break
		}
		fields = fields[:len(fields)-removed]
		ss = strings.TrimSpace(strings.Join(fields, " "))
	}
	return legacyTrim(ss)
}

func aliasSemanticKey(s string) string {
	ss := strings.TrimSpace(strings.ToLower(s))
	if ss == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range ss {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeAliasTrailingAbsoluteEpisode(alias string, primaryEpisode string, season string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ""
	}
	epN, err := strconv.Atoi(strings.TrimSpace(primaryEpisode))
	if err != nil || epN < 0 {
		return alias
	}
	seasonN, err := strconv.Atoi(strings.TrimSpace(season))
	if err != nil || seasonN <= 1 {
		return alias
	}
	mm := reAliasTrailingEpisodeTail.FindStringSubmatch(alias)
	if len(mm) != 3 {
		return alias
	}
	tailN, err := strconv.Atoi(strings.TrimSpace(mm[2]))
	if err != nil || tailN < 0 {
		return alias
	}
	if tailN == epN {
		return alias
	}
	diff := tailN - epN
	if diff < 0 {
		diff = -diff
	}
	if diff < 10 {
		return alias
	}
	prefix := strings.TrimSpace(mm[1])
	if prefix == "" {
		return alias
	}
	if !hasCJK(prefix) && !hasLatin(prefix) {
		return alias
	}
	return strings.TrimSpace(strings.Trim(prefix, "-_/;:,+ "))
}

var reAliasTrimSummaryCatalogTail = regexp.MustCompile(`(?i)\s*(?:[-_/|:()\[\]{}【】（）]|\s)+0?\d{1,4}(?:[.\s]+\d{1,2})?\s*(?:总集篇|總集篇|总集|總集|recap|special)\s*$`)

func normalizeAliasTrailingSummaryCatalogMarker(alias string) string {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ""
	}
	prev := ""
	for alias != prev {
		prev = alias
		alias = strings.TrimSpace(reAliasTrimSummaryCatalogTail.ReplaceAllString(alias, ""))
		alias = strings.TrimSpace(strings.Trim(alias, "-_/|:,+ "))
	}
	return alias
}

func (p *Parser) sanitizeAliasesForOutput(aliases []string, title string, episode string, season string, spKind string) []string {
	if len(aliases) == 0 {
		return nil
	}
	titleKey := aliasSemanticKey(title)
	out := make([]string, 0, len(aliases))
	seen := map[string]bool{}
	for _, a := range aliases {
		a = normalizeAliasTrailingAbsoluteEpisode(a, episode, season)
		a = normalizeAliasTrailingSummaryCatalogMarker(a)
		a = p.normalizeAliasCandidateForOutput(a)
		if stripped := stripExplicitSeasonSuffix(a); stripped != "" {
			a = p.normalizeAliasCandidateForOutput(stripped)
		}
		a = normalizeAliasTrailingAbsoluteEpisode(a, episode, season)
		a = normalizeAliasTrailingSummaryCatalogMarker(a)
		a = trimAliasTailBySPKind(a, spKind)
		if a == "" || !p.isValidAliasToken(a) {
			continue
		}
		if isLikelySearchHintText(a) || isAliasMetaSemanticOnly(a) {
			continue
		}
		if isStreamMetaText(a) || reTailMetaToken.MatchString(strings.TrimSpace(a)) {
			continue
		}
		if p.isAliasMetaOnlyToken(a) {
			continue
		}
		if reAliasMovieIndexSingle.MatchString(strings.TrimSpace(a)) {
			continue
		}
		if reAliasTVContentPhrase.MatchString(a) {
			continue
		}
		low := strings.ToLower(a)
		if strings.Contains(low, "tv +") || strings.Contains(low, "tv+") {
			continue
		}
		if p.isAliasUploaderAtPlatform(a) {
			continue
		}
		k := aliasSemanticKey(a)
		if k == "" || seen[k] || (titleKey != "" && k == titleKey) {
			continue
		}
		seen[k] = true
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var reAliasTrimOVATail = regexp.MustCompile(`(?i)\s*(?:[-_/|:()\[\]{}【】（）]|\s)+OVA\s*$`)
var reAliasTrimOADTail = regexp.MustCompile(`(?i)\s*(?:[-_/|:()\[\]{}【】（）]|\s)+OAD\s*$`)
var reAliasTrimONATail = regexp.MustCompile(`(?i)\s*(?:[-_/|:()\[\]{}【】（）]|\s)+ONA\s*$`)
var reAliasTrimSPTail = regexp.MustCompile(`(?i)(?:\s+(?:SP)|(?:[-_/|:()\[\]{}【】（）]|\s)+(?:SP)|(?:[-_/|:()\[\]{}【】（）]\s*)+(?:SPECIAL))\s*$`)
var reAliasTrimSPTailCJK = regexp.MustCompile(`\s*(?:[-_/|:()\[\]{}【】（）]|\s)+(?:特别篇|特別篇)\s*$`)

func trimAliasTailBySPKind(alias string, spKind string) string {
	alias = strings.TrimSpace(alias)
	k := strings.ToUpper(strings.TrimSpace(spKind))
	if alias == "" || k == "" {
		return alias
	}
	prev := ""
	for alias != prev {
		prev = alias
		switch k {
		case "OVA":
			alias = strings.TrimSpace(reAliasTrimOVATail.ReplaceAllString(alias, ""))
		case "OAD":
			alias = strings.TrimSpace(reAliasTrimOADTail.ReplaceAllString(alias, ""))
		case "ONA":
			alias = strings.TrimSpace(reAliasTrimONATail.ReplaceAllString(alias, ""))
		case "SP":
			alias = strings.TrimSpace(reAliasTrimSPTail.ReplaceAllString(alias, ""))
			alias = strings.TrimSpace(reAliasTrimSPTailCJK.ReplaceAllString(alias, ""))
		}
		alias = strings.TrimSpace(strings.Trim(alias, "-_/|:,+ "))
	}
	return alias
}

func isGenericBridgeTitleToken(s string) bool {
	ss := strings.TrimSpace(strings.ToLower(s))
	if ss == "" {
		return false
	}
	switch ss {
	case "系列", "シリーズ", "series", "franchise",
		"電影", "电影", "movie", "movies",
		"電影版", "电影版",
		"劇場版", "剧场版",
		"正片", "特典", "合集", "全集", "枪版", "槍版":
		return true
	default:
		return false
	}
}

func looksLikeLatinAliasFragment(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || hasCJK(s) {
		return false
	}
	hasLetter := false
	for _, r := range s {
		if ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') {
			hasLetter = true
			continue
		}
		if ('0' <= r && r <= '9') || r == ' ' || r == '-' || r == '\'' || r == '’' || r == '.' || r == '!' || r == '?' || r == '！' || r == '？' || r == ':' || r == '：' {
			continue
		}
		return false
	}
	return hasLetter
}

func aliasPromotionScore(s string) int {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return -1
	}
	score := 0
	if hasCJK(ss) {
		score += 60
	}
	if hasLatin(ss) {
		score += 50
	}
	if strings.Contains(ss, " ") {
		score += 10
	}
	score += len([]rune(ss))
	return score
}

func removeAliasBySemanticKey(aliases []string, key string) []string {
	if len(aliases) == 0 || key == "" {
		return aliases
	}
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if aliasSemanticKey(a) == key {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p *Parser) rebalanceGenericPrimaryTitleFromAliases(title string, aliases []string) (string, []string) {
	title = strings.TrimSpace(title)
	if title == "" || len(aliases) == 0 {
		return title, aliases
	}
	if !isGenericBridgeTitleToken(title) {
		return title, aliases
	}

	best := ""
	bestScore := -1
	joinedFromFragments := false
	for _, a := range aliases {
		c := strings.TrimSpace(a)
		if c == "" {
			continue
		}
		if isLikelySearchHintText(c) || isAliasMetaSemanticOnly(c) || isStreamMetaText(c) {
			continue
		}
		if p != nil && !p.isValidAliasToken(c) {
			continue
		}
		sc := aliasPromotionScore(c)
		if sc > bestScore {
			bestScore = sc
			best = c
		}
	}
	// Also consider joining latin fragments, e.g. "BanG" + "Dream!FILM" + "LIVE".
	allLatin := true
	frags := make([]string, 0, len(aliases))
	for _, a := range aliases {
		c := strings.TrimSpace(a)
		if !looksLikeLatinAliasFragment(c) {
			allLatin = false
			break
		}
		frags = append(frags, c)
	}
	if allLatin && len(frags) >= 2 {
		joined := strings.TrimSpace(strings.Join(frags, " "))
		joined = p.normalizeAliasCandidateForOutput(joined)
		if joined != "" && !isAliasMetaSemanticOnly(joined) && !isLikelySearchHintText(joined) {
			sc := aliasPromotionScore(joined) + 5
			if sc > bestScore {
				bestScore = sc
				best = joined
				joinedFromFragments = true
			}
		}
	}
	if best == "" {
		return title, aliases
	}

	titleKey := aliasSemanticKey(title)
	bestKey := aliasSemanticKey(best)
	if bestKey == "" || bestKey == titleKey {
		return title, aliases
	}
	if joinedFromFragments {
		out := make([]string, 0, len(aliases))
		for _, a := range aliases {
			if looksLikeLatinAliasFragment(a) {
				continue
			}
			out = append(out, a)
		}
		if len(out) == 0 {
			out = nil
		}
		return best, out
	}
	newAliases := removeAliasBySemanticKey(aliases, bestKey)
	return best, newAliases
}

func rebalanceSingleSlashSeasonBridgePrimaryFromRaw(rawTitle string, title string, aliases []string) (string, []string) {
	title = strings.TrimSpace(title)
	if title == "" || len(aliases) == 0 || rawTitle == "" {
		return title, aliases
	}
	if !hasCJK(title) || hasLatin(title) {
		return title, aliases
	}
	left, right, ok := extractSingleSlashSeasonBridgeSides(rawTitle)
	if !ok {
		return title, aliases
	}
	leftKey := aliasSemanticKey(left)
	rightKey := aliasSemanticKey(right)
	titleKey := aliasSemanticKey(title)
	if leftKey == "" || rightKey == "" || titleKey == "" {
		return title, aliases
	}
	if titleKey != rightKey {
		return title, aliases
	}
	hasLeftAlias := false
	for _, a := range aliases {
		if aliasSemanticKey(a) == leftKey {
			hasLeftAlias = true
			break
		}
	}
	if !hasLeftAlias {
		return title, aliases
	}
	out := removeAliasBySemanticKey(aliases, leftKey)
	if rightKey != leftKey {
		out = mergeAliases([]string{right}, out)
	}
	return left, out
}

func extractSingleSlashSeasonBridgeSides(rawTitle string) (string, string, bool) {
	s := strings.TrimSpace(rawTitle)
	if s == "" {
		return "", "", false
	}
	s = regexp.MustCompile(`^\s*(?:\[[^\]]+\]|【[^】]+】)\s*`).ReplaceAllString(s, "")
	if loc := regexp.MustCompile(`\s+-\s+\d{1,4}(?:\.\d+)?\b`).FindStringIndex(s); loc != nil {
		s = strings.TrimSpace(s[:loc[0]])
	}
	if loc := regexp.MustCompile(`\[\s*\d{1,4}(?:\.\d+)?\s*\]`).FindStringIndex(s); loc != nil {
		s = strings.TrimSpace(s[:loc[0]])
	}
	norm := strings.ReplaceAll(s, "／", "/")
	if strings.Count(norm, "/") != 1 {
		return "", "", false
	}
	parts := strings.SplitN(norm, "/", 2)
	left := normalizeTitleMain(strings.TrimSpace(parts[0]))
	right := normalizeTitleMain(strings.TrimSpace(parts[1]))
	if stripped := stripExplicitSeasonSuffix(left); stripped != "" {
		left = stripped
	}
	if stripped := stripExplicitSeasonSuffix(right); stripped != "" {
		right = stripped
	}
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return "", "", false
	}
	if !hasLatin(left) || hasCJK(left) {
		return "", "", false
	}
	if !hasCJK(right) || hasLatin(right) {
		return "", "", false
	}
	if extractExplicitSeasonNumber(parts[0]) == "" && extractExplicitSeasonNumber(parts[1]) == "" {
		return "", "", false
	}
	return left, right, true
}

func sanitizePrimaryTitleForOutput(title string) string {
	s := normalizeTitleMain(title)
	if s == "" {
		return ""
	}
	prev := ""
	for s != prev {
		prev = s
		s = strings.TrimSpace(s)
		s = reTitleStripTVAnimeSegmentPrefix.ReplaceAllString(s, "$1")
		s = reAliasTrimPlatformTail.ReplaceAllString(s, "")
		s = reTitleTrimTVSeriesTail.ReplaceAllString(s, "")
		s = reTitleTrimTVEpisodeListTail.ReplaceAllString(s, "")
		s = reTitleTrimTVSeasonTail.ReplaceAllString(s, "")
		s = reTitleTrimTVTail.ReplaceAllString(s, "")
		s = reTitleTrimSpecialTail.ReplaceAllString(s, "")
		s = reTitleTrimEpisodeWordTail.ReplaceAllString(s, "")
		s = reTitleTrimPackSpecialTail.ReplaceAllString(s, "")
		s = reTitleTrimMetaTail.ReplaceAllString(s, "")
		s = reTitleTrimVolTail.ReplaceAllString(s, "")
		s = reTitleTrimDVDBundleTail.ReplaceAllString(s, "")
		s = reTitleTrimDVDEpisodesTail.ReplaceAllString(s, "")
		s = strings.TrimSpace(strings.Trim(s, "-_/;:,+ "))
	}
	return normalizeTitleMain(s)
}

func trimTitleTailByPackKind(title string, packKind string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if strings.TrimSpace(packKind) == "" {
		return title
	}
	t := strings.TrimSpace(reTitleTrimPlainPackTail.ReplaceAllString(title, ""))
	if t == "" {
		return title
	}
	return t
}

func trimTitleTailByGroup(title string, group string) string {
	title = strings.TrimSpace(title)
	group = strings.TrimSpace(normalizeGroupText(group))
	if title == "" || group == "" {
		return title
	}
	reExact := regexp.MustCompile(`(?i)\s*-\s*` + regexp.QuoteMeta(group) + `\s*$`)
	out := strings.TrimSpace(reExact.ReplaceAllString(title, ""))
	if out == title && strings.Contains(group, "-") {
		partsRaw := strings.Split(group, "-")
		parts := make([]string, 0, len(partsRaw))
		for _, p := range partsRaw {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			parts = append(parts, regexp.QuoteMeta(p))
		}
		if len(parts) >= 2 {
			flexGroup := strings.Join(parts, `\s*-\s*`)
			reFlex := regexp.MustCompile(`(?i)\s*-\s*` + flexGroup + `\s*$`)
			out = strings.TrimSpace(reFlex.ReplaceAllString(title, ""))
		}
	}
	if out == "" {
		return title
	}
	return out
}

func consumeTailGroupTokensByCanonicalGroup(tokens []model.Token, group string) {
	group = strings.TrimSpace(normalizeGroupText(group))
	if group == "" || len(tokens) == 0 {
		return
	}
	partsRaw := strings.Split(group, "-")
	parts := make([]string, 0, len(partsRaw))
	for _, p := range partsRaw {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return
	}

	// Match split form near tail: "- Tsundere - Raws"
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		for i := len(tokens) - 1; i >= 0; i-- {
			if tokens[i].Removed || isInBracket(tokens, i) || isInParen(tokens, i) {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(tokens[i].Text), last) {
				continue
			}
			j := i - 1
			k := len(parts) - 2
			ok := true
			for k >= 0 {
				for j >= 0 && strings.TrimSpace(tokens[j].Text) == "" {
					j--
				}
				if j < 0 || strings.TrimSpace(tokens[j].Text) != "-" {
					ok = false
					break
				}
				j--
				for j >= 0 && strings.TrimSpace(tokens[j].Text) == "" {
					j--
				}
				if j < 0 || !strings.EqualFold(strings.TrimSpace(tokens[j].Text), parts[k]) {
					ok = false
					break
				}
				k--
				j--
			}
			if !ok {
				continue
			}
			start := j + 1
			leadDash := start - 1
			if leadDash >= 0 && strings.TrimSpace(tokens[leadDash].Text) == "-" && !isInBracket(tokens, leadDash) && !isInParen(tokens, leadDash) {
				tokens[leadDash].Removed = true
			}
			for x := start; x <= i && x < len(tokens); x++ {
				if isInBracket(tokens, x) || isInParen(tokens, x) {
					continue
				}
				tokens[x].Removed = true
			}
			return
		}
	}

	// Match single token near tail: "-VARYG" / "VARYG".
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].Removed || isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(tokens[i].Text), group) {
			continue
		}
		tokens[i].Removed = true
		if i-1 >= 0 && strings.TrimSpace(tokens[i-1].Text) == "-" && !isInBracket(tokens, i-1) && !isInParen(tokens, i-1) {
			tokens[i-1].Removed = true
		}
		return
	}
}

func upsertTitleAttribute(attrs []model.Attribute, title string, source string) []model.Attribute {
	title = strings.TrimSpace(title)
	if title == "" {
		return attrs
	}
	out := make([]model.Attribute, len(attrs))
	copy(out, attrs)
	updated := false
	for i := range out {
		if out[i].Type != model.AttrTitle {
			continue
		}
		out[i].Normalized = title
		if strings.TrimSpace(source) != "" {
			out[i].Source = source
		}
		if out[i].Confidence <= 0 {
			out[i].Confidence = 0.7
		}
		updated = true
	}
	if !updated {
		out = append(out, model.Attribute{
			Type:       model.AttrTitle,
			Normalized: title,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.7,
			Source:     source,
		})
	}
	return out
}

func upsertScalarAttribute(attrs []model.Attribute, t model.AttrType, value string, source string) []model.Attribute {
	value = strings.TrimSpace(value)
	if value == "" {
		return attrs
	}
	out := make([]model.Attribute, len(attrs))
	copy(out, attrs)
	updated := false
	for i := range out {
		if out[i].Type != t {
			continue
		}
		out[i].Normalized = value
		if strings.TrimSpace(source) != "" {
			out[i].Source = source
		}
		if out[i].Confidence <= 0 {
			out[i].Confidence = 0.7
		}
		updated = true
	}
	if !updated {
		out = append(out, model.Attribute{
			Type:       t,
			Normalized: value,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.7,
			Source:     source,
		})
	}
	return out
}

func removeAttributeType(attrs []model.Attribute, t model.AttrType) []model.Attribute {
	if len(attrs) == 0 {
		return attrs
	}
	out := make([]model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if a.Type == t {
			continue
		}
		out = append(out, a)
	}
	return out
}

func upsertGroupAttribute(attrs []model.Attribute, group string, source string) []model.Attribute {
	group = strings.TrimSpace(group)
	if group == "" {
		return attrs
	}
	out := make([]model.Attribute, len(attrs))
	copy(out, attrs)
	updated := false
	for i := range out {
		if out[i].Type != model.AttrGroup {
			continue
		}
		out[i].Normalized = group
		if strings.TrimSpace(source) != "" {
			out[i].Source = source
		}
		if out[i].Confidence <= 0 {
			out[i].Confidence = 0.8
		}
		updated = true
	}
	if !updated {
		out = append(out, model.Attribute{
			Type:       model.AttrGroup,
			Normalized: group,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.8,
			Source:     source,
		})
	}
	return out
}

func appendTitleAliasAttributes(attrs []model.Attribute, aliases []string, title string, source string) []model.Attribute {
	if len(aliases) == 0 {
		return attrs
	}
	titleKey := aliasSemanticKey(title)
	out := make([]model.Attribute, len(attrs), len(attrs)+len(aliases))
	copy(out, attrs)
	for _, a := range aliases {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if titleKey != "" && aliasSemanticKey(a) == titleKey {
			continue
		}
		out = append(out, model.Attribute{
			Type:       model.AttrTitleAlias,
			Normalized: a,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.7,
			Source:     source,
		})
	}
	return out
}

func normalizeKnownTitleToken(s string) string {
	// Full-width slash/dash variants must match their half-width forms:
	// "22／7" (full-width) is the same title as "22/7".
	s = strings.ReplaceAll(s, "／", "/")
	s = strings.ReplaceAll(s, "－", "-")
	return s
}

func (p *Parser) protectKnownTitleSpans(rawTitle string, tokens []model.Token) {
	if p == nil || p.cfg == nil || len(tokens) == 0 {
		return
	}
	rawLower := strings.ToLower(rawTitle)
	for _, entry := range p.cfg.TitleKnowledge.Entries {
		if !entry.Enabled {
			continue
		}
		matched := true
		for _, required := range entry.RawMustContainAll {
			if !strings.Contains(rawLower, strings.ToLower(strings.TrimSpace(required))) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		candidates := append([]string{entry.CanonicalTitle}, entry.Aliases...)
		for _, cand := range candidates {
			cand = strings.TrimSpace(cand)
			if cand == "" {
				continue
			}
			ctoks := tokenize.Tokenize(cand)
			if len(ctoks) == 0 {
				continue
			}
			// Find every contiguous token run that matches the known title form.
			// These tokens belong to the title; season/episode/range detection must
			// never reinterpret numbers inside them (e.g. "藤本树 17-26", "22／7").
			for i := 0; i+len(ctoks) <= len(tokens); i++ {
				ok := true
				for k, ct := range ctoks {
					if normalizeKnownTitleToken(tokens[i+k].Text) != normalizeKnownTitleToken(ct) {
						ok = false
						break
					}
				}
				if !ok {
					continue
				}
				for k := 0; k < len(ctoks); k++ {
					tokens[i+k].Protected = true
				}
			}
		}
	}
}

func (p *Parser) applyTitleKnowledge(rawTitle string, title string, aliases []string) (string, []string) {
	if p == nil || p.cfg == nil {
		return title, aliases
	}
	for _, entry := range p.cfg.TitleKnowledge.Entries {
		if !entry.Enabled || strings.TrimSpace(entry.CanonicalTitle) == "" || len(entry.RawMustContainAll) == 0 {
			continue
		}
		matched := true
		for _, required := range entry.RawMustContainAll {
			if !strings.Contains(strings.ToLower(rawTitle), strings.ToLower(strings.TrimSpace(required))) {
				matched = false
				break
			}
		}
		if matched {
			canonicalTitle := strings.TrimSpace(entry.CanonicalTitle)
			// Only keep knowledge aliases that actually appear in the raw title, so
			// script variants not present in this release are not injected into output.
			rawLower := strings.ToLower(rawTitle)
			var presentAliases []string
			for _, a := range entry.Aliases {
				a = strings.TrimSpace(a)
				if a == "" {
					continue
				}
				if strings.Contains(rawLower, strings.ToLower(a)) {
					presentAliases = append(presentAliases, a)
				}
			}
			mergedAliases := mergeAliases(presentAliases, aliases)
			filteredAliases := mergedAliases[:0]
			canonicalKey := aliasSemanticKey(canonicalTitle)
			for _, alias := range mergedAliases {
				aliasKey := aliasSemanticKey(alias)
				if aliasKey != "" && strings.Contains(canonicalKey, aliasKey) {
					continue
				}
				filteredAliases = append(filteredAliases, alias)
			}
			return canonicalTitle, filteredAliases
		}
	}
	return title, aliases
}

func cleanTitleToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, "[]{}【】『』「」")
	s = strings.TrimSpace(s)
	s = normalizeTitleMain(s)
	s = stripFileExtSuffix(s)
	return strings.TrimSpace(s)
}

var reFileExtSuffix = regexp.MustCompile(`(?i)\.(?:mkv|mp4|avi|mov|wmv|flv|ts|m2ts|mpg|mpeg|iso|m4v)$`)
var episodeSPAliasPattern = regexp.MustCompile(`(?i)^\d{1,4}\s*[_ .-]\s*(?:sp|ova|oad|ona|ex|pv|op|ed)$`)
var episodeSPAliasAmpPattern = regexp.MustCompile(`(?i)^\d{1,4}\s*(?:&|＆)\s*(?:sp|ova|oad|ona|ex|pv|op|ed)$`)

func stripFileExtSuffix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.TrimSpace(reFileExtSuffix.ReplaceAllString(s, ""))
}

func (p *Parser) isValidAliasToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, "先行版") || strings.Contains(s, "先行版本") {
		return false
	}
	up := strings.ToUpper(s)
	if up == "SP" || up == "OVA" || up == "OAD" || up == "ONA" || up == "EX" || up == "PV" || up == "OP" || up == "ED" {
		return false
	}
	if regexp.MustCompile(`^\d{1,4}\s*[-~]\s*\d{1,4}$`).MatchString(s) {
		return false
	}
	// Versioned episode tokens like "07v2" are not aliases.
	if regexp.MustCompile(`(?i)^\d{1,4}[vV]\d{1,2}$`).MatchString(s) {
		return false
	}
	// Pure version tags like "v2" / "ver2" should not be aliases.
	if regexp.MustCompile(`(?i)^v(?:er)?\.?\s*\d{1,3}$`).MatchString(s) {
		return false
	}
	if regexp.MustCompile(`^\d{1,4}(?:\s+\d{1,4})+$`).MatchString(s) {
		return false
	}
	if len([]rune(s)) < 2 {
		return false
	}
	low := strings.ToLower(s)
	// Obvious metadata tags should not become aliases.
	if strings.EqualFold(s, "BD") {
		return false
	}
	if regexp.MustCompile(`(?i)^[a-z]{2}-[a-z]{2}$`).MatchString(low) {
		return false
	}
	if regexp.MustCompile(`(?i)^(?:sp|ova|oad|ona|ex|pv|op|ed)\d{1,3}$`).MatchString(low) {
		return false
	}
	if episodeSPAliasPattern.MatchString(s) || episodeSPAliasAmpPattern.MatchString(s) {
		return false
	}
	if regexp.MustCompile(`(?i)^[0-9a-f]{8}$`).MatchString(low) {
		return false
	}
	if strings.Contains(s, "招募") || strings.Contains(s, "仅限") || strings.Contains(s, "港澳") || strings.Contains(s, "臺") || strings.Contains(s, "台") || strings.Contains(s, "停播") || strings.Contains(s, "停更") {
		return false
	}
	// Obvious media/container metadata should not become aliases.
	if strings.Contains(low, "bdrip") {
		return false
	}
	if regexp.MustCompile(`(?i)\bvol\s*\.?\s*\d{1,3}\b`).FindStringIndex(low) != nil {
		return false
	}
	if regexp.MustCompile(`(?i)\bvolume\s*\d{1,3}\b`).FindStringIndex(low) != nil {
		return false
	}
	if p.hasAliasMetaFromDictionary(s) {
		return false
	}
	if episodeOfTotalPattern.MatchString(s) {
		return false
	}
	return true
}

var reAliasMetaTokenSplit = regexp.MustCompile(`[\s,./+_|&:;()\[\]{}-]+`)

func (p *Parser) hasAliasMetaFromDictionary(s string) bool {
	ss := strings.TrimSpace(strings.ToLower(s))
	if ss == "" {
		return false
	}
	if p != nil {
		if _, ok := p.aliasNoiseExactWords[ss]; ok {
			return true
		}
		for _, re := range p.aliasNoisePatterns {
			if re != nil && re.MatchString(strings.TrimSpace(s)) {
				return true
			}
		}
	}
	if p != nil && p.isAliasMetaOnlyToken(ss) {
		return true
	}
	if reAliasTechSingle.MatchString(strings.TrimSpace(s)) {
		return true
	}
	if reAliasTechList.MatchString(strings.TrimSpace(s)) {
		return true
	}
	tokens := reAliasMetaTokenSplit.Split(ss, -1)
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if p != nil && p.isAliasMetaOnlyToken(t) {
			return true
		}
		if p != nil {
			if _, ok := p.normalizeVideoCodecToken(t); ok {
				return true
			}
			if _, ok := p.normalizeAudioCodecToken(t); ok {
				return true
			}
			if _, ok := p.normalizeContainerToken(t); ok {
				return true
			}
			if _, ok := p.normalizeAudioFlagToken(t); ok {
				return true
			}
		}
	}
	return false
}

func mergeAliases(a []string, b []string) []string {
	if len(a) == 0 {
		if len(b) == 0 {
			return nil
		}
		out := make([]string, 0, len(b))
		seen := map[string]bool{}
		for _, x := range b {
			x = strings.TrimSpace(x)
			if x == "" {
				continue
			}
			k := strings.ToLower(x)
			if !seen[k] {
				seen[k] = true
				out = append(out, x)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	if len(b) == 0 {
		return a
	}
	out := make([]string, 0, len(a)+len(b))
	seen := map[string]bool{}
	for _, x := range a {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		k := strings.ToLower(x)
		if !seen[k] {
			seen[k] = true
			out = append(out, x)
		}
	}
	for _, x := range b {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		k := strings.ToLower(x)
		if !seen[k] {
			seen[k] = true
			out = append(out, x)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
