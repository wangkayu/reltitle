package parser

import (
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
	// "title-parser/pkg/onnx" // Temporarily disabled for HTTP server
	"title-parser/pkg/quality"
	"title-parser/pkg/tokenize"
	"title-parser/pkg/xlmr"
)

type Parser struct {
	cfg        *config.Config
	groupIndex map[string]*config.GroupDef // 字幕组索引: normalized pattern -> GroupDef
}

var wxhResolutionPattern = regexp.MustCompile(`(?i)^(\d{2,5})\s*[x×X]\s*(\d{2,5})$`)

func extractWxHResolutionAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if attrs == nil || len(tokens) == 0 {
		return
	}
	techStart, hasTechStart := firstTechTokenIndexFromAttrs(*attrs)
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if hasTechStart && i+2 < techStart {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		m := wxhResolutionPattern.FindStringSubmatch(t)
		if len(m) != 3 {
			continue
		}
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		if w <= 0 || h <= 0 {
			continue
		}
		// Keep orientation-insensitive handling.
		if w < h {
			w, h = h, w
		}
		// Map common sizes to p-based resolution.
		norm := ""
		switch {
		case w == 3840 && h == 2160:
			norm = "2160p"
		case w == 1920 && h == 1080:
			norm = "1080p"
		case w == 1280 && h == 720:
			norm = "720p"
		case w == 720 && h == 480:
			norm = "480p"
		}
		if norm == "" {
			norm = strconv.Itoa(w) + "x" + strconv.Itoa(h)
		}
		*attrs = append(*attrs, model.Attribute{Type: model.AttrResolution, Normalized: norm, TokenStart: i, TokenEnd: i + 1, Confidence: 0.75, Source: "resolution_wxh"})
		tokens[i].Removed = true
	}
}

func extractYearAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute, hasTech bool) {
	if attrs == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		y := strings.TrimSpace(tokens[i].Text)
		if !yearPattern.MatchString(y) {
			continue
		}
		*attrs = append(*attrs, model.Attribute{Type: model.AttrYear, Normalized: y, TokenStart: i, TokenEnd: i + 1, Confidence: 0.7, Source: "year_token"})

		remove := hasTech || isInBracket(tokens, i)
		if !remove {
			if i-1 >= 0 {
				p := strings.TrimSpace(tokens[i-1].Text)
				if p == "." || p == "-" || p == "_" || p == "/" {
					remove = true
				}
			}
		}
		if !remove {
			if i+1 < len(tokens) {
				n := strings.TrimSpace(tokens[i+1].Text)
				if n == "." || n == "-" || n == "_" || n == "/" {
					remove = true
				}
			}
		}
		if remove {
			tokens[i].Removed = true
		}
	}
}

func looksLikeDashSeparatedTechSuffix(tokens []model.Token, idx int) bool {
	// e.g. E-AC-3 should not yield episode=3
	if idx < 2 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if cur == "" {
		return false
	}
	if cur != "3" {
		return false
	}
	if strings.TrimSpace(tokens[idx-1].Text) != "-" {
		return false
	}
	prev := strings.ToUpper(strings.TrimSpace(tokens[idx-2].Text))
	if prev == "AC" {
		return true
	}
	return false
}

func removePreYearNoiseNumberTokens(tokens []model.Token) {
	if len(tokens) == 0 {
		return
	}
	countPrevTitleWords := func(idx int) int {
		cnt := 0
		start := idx - 8
		if start < 0 {
			start = 0
		}
		for i := start; i < idx; i++ {
			if i < 0 || i >= len(tokens) {
				continue
			}
			if tokens[i].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[i].Text)
			if t == "" {
				continue
			}
			if t == "-" || t == "/" || t == "|" || t == "\\" || t == "." {
				cnt = 0
				continue
			}
			allLetters := true
			for _, r := range t {
				if !(('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')) {
					allLetters = false
					break
				}
			}
			if allLetters {
				cnt++
			}
		}
		return cnt
	}

	for i := 0; i < len(tokens); i++ {
		y := strings.TrimSpace(tokens[i].Text)
		if !yearPattern.MatchString(y) {
			continue
		}
		j := i - 1
		for j >= 0 {
			pj := strings.TrimSpace(tokens[j].Text)
			if pj == "" || pj == "." || pj == "-" || pj == "_" {
				j--
				continue
			}
			break
		}
		if j < 0 {
			continue
		}
		cand := strings.TrimSpace(tokens[j].Text)
		if !regexp.MustCompile(`^\d{2,4}$`).MatchString(cand) {
			continue
		}
		// Only remove when we already have a multi-word latin title prefix to reduce false positives.
		if countPrevTitleWords(j) < 2 {
			continue
		}
		// Avoid removing obvious resolutions.
		if cand == "480" || cand == "720" || cand == "1080" || cand == "2160" {
			continue
		}
		tokens[j].Removed = true
	}
}

func isTitlePrefixedDigitRangeInParen(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	if !isInParen(tokens, idx) {
		return false
	}
	// Detect patterns like: ( 藤本树 17 - 26 ) where 17-26 is part of the title.
	if idx+2 >= len(tokens) {
		return false
	}
	if !episodeOfTotalPattern.MatchString(strings.TrimSpace(tokens[idx].Text)) {
		return false
	}
	sep := strings.TrimSpace(tokens[idx+1].Text)
	if sep != "-" && sep != "~" {
		return false
	}
	if !episodeOfTotalPattern.MatchString(strings.TrimSpace(tokens[idx+2].Text)) {
		return false
	}
	// If there is meaningful title text before the range within the same paren block,
	// treat this range as title suffix, not episode_range.
	openIdx := -1
	for j := idx; j >= 0; j-- {
		if tokens[j].Text == "(" || tokens[j].Text == "（" {
			openIdx = j
			break
		}
	}
	if openIdx < 0 {
		return false
	}
	for j := idx - 1; j > openIdx; j-- {
		if tokens[j].Removed {
			continue
		}
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
		return true
	}
	return false
}

func looksLikeLowercaseLatinWordToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if len([]rune(s)) < 2 || len([]rune(s)) > 6 {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func looksLikeAudioCodecTokenSimple(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	low = strings.ReplaceAll(low, "_", "-")
	codecSet := map[string]bool{"aac": true, "flac": true, "opus": true, "ac3": true, "eac3": true, "ddp": true, "dd": true, "truehd": true, "dts": true, "dts-hdma": true, "dtshdma": true, "pcm": true}
	return codecSet[low]
}

func looksLikeDottedNumericSequence(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	if tokens[idx].Removed {
		return false
	}
	if !isInBracket(tokens, idx) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	if idx-1 >= 0 && !tokens[idx-1].Removed {
		if strings.TrimSpace(tokens[idx-1].Text) == "." {
			if idx-2 >= 0 && !tokens[idx-2].Removed {
				p := strings.TrimSpace(tokens[idx-2].Text)
				if episodeOfTotalPattern.MatchString(p) {
					return true
				}
				if looksLikeAudioCodecTokenSimple(p) {
					return true
				}
			}
		}
	}
	if idx+1 < len(tokens) && !tokens[idx+1].Removed {
		if strings.TrimSpace(tokens[idx+1].Text) == "." {
			if idx+2 < len(tokens) && !tokens[idx+2].Removed {
				n := strings.TrimSpace(tokens[idx+2].Text)
				if episodeOfTotalPattern.MatchString(n) {
					return true
				}
			}
		}
	}
	return false
}

func isRightSideOfPlainDigitRangeInTitle(tokens []model.Token, idx int) bool {
	// Guard for cases like:
	//   "藤本树 17 - 26 / Fujimoto Tatsuki 17 - 26 - 07 ..."
	// where "17-26" is part of title/alias, not an episode.
	if idx < 2 || idx >= len(tokens) {
		return false
	}
	if isInBracket(tokens, idx) || isInParen(tokens, idx) {
		return false
	}
	if strings.TrimSpace(tokens[idx].Text) == "" {
		return false
	}
	if tokens[idx-1].Removed || tokens[idx-2].Removed {
		return false
	}
	sep := strings.TrimSpace(tokens[idx-1].Text)
	if sep != "-" && sep != "~" {
		return false
	}
	left := strings.TrimSpace(tokens[idx-2].Text)
	right := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(left) || !episodeOfTotalPattern.MatchString(right) {
		return false
	}
	// Strong signal: followed by '/' => title/alias split.
	if idx+1 < len(tokens) {
		if strings.TrimSpace(tokens[idx+1].Text) == "/" {
			return true
		}
	}
	// Another strong signal: followed by '-' + digits + tech bracket (e.g. "- 07 [")
	if idx+3 < len(tokens) {
		if strings.TrimSpace(tokens[idx+1].Text) == "-" {
			n := strings.TrimSpace(tokens[idx+2].Text)
			if episodeOfTotalPattern.MatchString(n) {
				b := strings.TrimSpace(tokens[idx+3].Text)
				if b == "[" || b == "【" || b == "(" || b == "（" {
					return true
				}
			}
		}
	}
	return false
}

func consumeStarWrappedLeadingMeta(tokens []model.Token, start int) (int, bool) {
	star := strings.TrimSpace(tokens[start].Text)
	if star != "★" && star != "☆" {
		return start, false
	}
	maxLookahead := start + 16
	if maxLookahead > len(tokens) {
		maxLookahead = len(tokens)
	}
	end := -1
	for j := start + 1; j < maxLookahead; j++ {
		if tokens[j].Removed {
			continue
		}
		if strings.TrimSpace(tokens[j].Text) == star {
			end = j
			break
		}
	}
	if end < 0 {
		return start, false
	}
	innerParts := make([]string, 0, end-start)
	for k := start + 1; k < end; k++ {
		if tokens[k].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[k].Text)
		if t == "" {
			continue
		}
		if t == "-" || t == "·" {
			continue
		}
		innerParts = append(innerParts, t)
	}
	inner := strings.TrimSpace(strings.Join(innerParts, ""))
	if inner == "" {
		return start, false
	}
	if looksLikeLeadingTitleMeta(inner) {
		return end + 1, true
	}
	return start, false
}

func New(cfg *config.Config) *Parser {
	p := &Parser{
		cfg:        cfg,
		groupIndex: make(map[string]*config.GroupDef),
	}

	// 构建字幕组索引
	if cfg.GroupList.Groups != nil {
		for i := range cfg.GroupList.Groups {
			group := &cfg.GroupList.Groups[i]
			for _, pattern := range group.Patterns {
				normalized := strings.ToLower(pattern)
				p.groupIndex[normalized] = group
			}
		}
	}

	return p
}

// isKnownGroup checks if a token is a known subtitle group
// Returns the group definition and confidence boost if found
func (p *Parser) isKnownGroup(token string) (*config.GroupDef, bool) {
	normalized := strings.ToLower(strings.TrimSpace(token))
	if normalized == "" {
		return nil, false
	}

	group, ok := p.groupIndex[normalized]
	return group, ok
}

func (p *Parser) Parse(rawTitle string) (*model.ParseResult, error) {
	return p.ParseWithHints(rawTitle, nil)
}

func (p *Parser) ParseWithHints(rawTitle string, hints *ParseHints) (*model.ParseResult, error) {
	if strings.TrimSpace(rawTitle) == "" {
		return nil, errors.New("empty title")
	}

	toks := tokenize.Tokenize(rawTitle)
	tokens := make([]model.Token, 0, len(toks))
	for _, t := range toks {
		tokens = append(tokens, model.Token{Text: t})
	}

	attrs := make([]model.Attribute, 0, 16)
	seasonRange := extractSeasonRangeAndRemoveFromTokens(tokens, &attrs)
	extractPackSeasonExtrasAndRemoveFromTokens(tokens, &attrs, seasonRange)
	extractStrictCustomReleaseTagAttrsAndRemoveFromTokens(tokens, &attrs)
	extractGuardedDVDRIPReleaseTagAttrsAndRemoveFromTokens(tokens, &attrs)
	extractPlatformRegionAttrsAndRemoveFromTokens(tokens, &attrs)
	version := extractVersionAttrsAndRemoveFromTokens(tokens, &attrs)
	extractSearchHintIndexAttrsAndRemoveFromTokens(tokens, &attrs)
	extractSeasonShowTagAttrsAndRemoveFromTokens(tokens, &attrs)
	p.applyLexicon(tokens, &attrs)
	promoteFrenchMultiNoteToAudioFlag(attrs)
	normalizePlatformAttrs(attrs)
	normalizeSourceTypeAttrs(attrs)
	convertRegionAttrsToPlatformRegion(attrs)
	extractWxHResolutionAttrsAndRemoveFromTokens(tokens, &attrs)
	extractYearAttrsAndRemoveFromTokens(tokens, &attrs, hasTechAttr(attrs))
	removePreYearNoiseNumberTokens(tokens)
	tailParenBlock := detectTailParenBlock(tokens)

	group, groupSpan := extractHeadBracketGroup(tokens)

	if group != "" {
		// If we can detect a reliable tail group from raw title (e.g. "-ANi"), the head bracket
		// is more likely a bracketed title, not a release group. Do not remove it as group.
		if tg := detectTailGroupFromRawTitle(rawTitle); tg != "" {
			ng := normalizeGroupText(group)
			nt := normalizeGroupText(tg)
			if !strings.EqualFold(ng, nt) {
				group = ""
			}
		}
	}

	if group != "" {
		// Group normalization is now handled by grouplist.yaml configuration
		// Apply normal normalization
		group = normalizeGroupText(group)

		attrs = append(attrs, model.Attribute{Type: model.AttrGroup, Normalized: group, TokenStart: groupSpan[0], TokenEnd: groupSpan[1], Confidence: 0.9, Source: "heuristic"})
		for i := groupSpan[0]; i < groupSpan[1]; i++ {
			tokens[i].Removed = true
		}
	}
	if group == "" {
		if g := extractInlineStarGroup(tokens); g != "" {
			group = normalizeGroupText(g)
			attrs = append(attrs, model.Attribute{Type: model.AttrGroup, Normalized: group, TokenStart: 0, TokenEnd: 1, Confidence: 0.7, Source: "heuristic"})
		}
	}
	if group == "" {
		if g, gIdx := detectTechPrefixedGroup(tokens); g != "" {
			group = normalizeGroupText(g)
			attrs = append(attrs, model.Attribute{Type: model.AttrGroup, Normalized: group, TokenStart: gIdx, TokenEnd: gIdx + 1, Confidence: 0.7, Source: "heuristic"})
		}
	}

	packKind := detectPackKind(p.cfg, rawTitle)
	ctx := newParseContext(rawTitle, packKind, tailParenBlock)
	ctx.SeasonRange = seasonRange
	ctx.Group = group
	for _, pass := range defaultParsePasses() {
		pass(tokens, &attrs, ctx)
	}
	// PackKind may be refined/overridden by parse passes (e.g. contents markers like TV+OAD).
	packKind = strings.TrimSpace(ctx.PackKind)
	season := ctx.Season
	seasonIdx := ctx.SeasonIdx
	seasonRange = ctx.SeasonRange
	spKind := ctx.SPKind
	movieIndex := ctx.MovieIndex
	movieIndexIdx := ctx.MovieIndexIdx
	hasMovieKeyword := ctx.HasMovieKeyword
	episodeRange := ctx.EpisodeRange
	episodeRangeIdx := ctx.EpisodeRangeIdx
	episode := ctx.Episode
	episodeIdx := ctx.EpisodeIdx
	epslot := ctx.EPSlot

	// Optional overrides (evaluation / reranking). Apply after parse passes finalize episode fields
	// so title span inference uses the overridden episode anchor.
	if hints != nil {
		if strings.TrimSpace(hints.OverrideEpisode) != "" && hints.OverrideEpisodeIdx >= 0 {
			episode = strings.TrimSpace(hints.OverrideEpisode)
			episodeIdx = hints.OverrideEpisodeIdx
			// Keep range empty when overriding a single episode.
			episodeRange = ""
			episodeRangeIdx = -1
			ctx.Episode = episode
			ctx.EpisodeIdx = episodeIdx
			ctx.EpisodeRange = ""
			ctx.EpisodeRangeIdx = -1
			// Rebuild EPSlot for consistency.
			ctx.EPSlot = buildEPSlotCandidates(tokens, ctx.Season, ctx.SeasonIdx, ctx.EpisodeFromSxxExx, ctx.SxxExxIdx, ctx.Episode, ctx.EpisodeIdx, ctx.EpisodeRange, ctx.EpisodeRangeIdx, ctx.SPKind)
			finalizeEPSlotPrimary(ctx.EPSlot, ctx.Season, ctx.Episode, ctx.EpisodeRange, ctx.SPKind, ctx.EpisodeIdx, ctx.EpisodeRangeIdx, ctx.SxxExxIdx)
			epslot = ctx.EPSlot
		}
		if hints.OverrideTitleStart >= 0 && hints.OverrideTitleEnd > hints.OverrideTitleStart {
			ctx.ForcedTitleStart = hints.OverrideTitleStart
			ctx.ForcedTitleEnd = hints.OverrideTitleEnd
			ctx.ForcedTitle = extractTitleFromSpanAllowRemoved(tokens, ctx.ForcedTitleStart, ctx.ForcedTitleEnd, true)
		}
	}

	seasonCutIdx := -1
	// Only cut title at plain Sxx season tokens that are part of the main title stream.
	// Do NOT cut at "Season" tokens which are often inside parentheses (we remove the whole paren block instead).
	if seasonIdx >= 0 && seasonIdx < len(tokens) {
		if !isInBracket(tokens, seasonIdx) && !isInParen(tokens, seasonIdx) {
			if seasonOnlyPattern.MatchString(tokens[seasonIdx].Text) || seasonEmbeddedPattern.MatchString(tokens[seasonIdx].Text) {
				seasonCutIdx = seasonIdx
			}
		}
	}

	_, titleEnd0 := inferTitleTokenSpan(tokens, episodeIdx, episodeRangeIdx, seasonCutIdx)
	parenAliases := extractParenAliasesAndRemoveFromTitle(tokens, titleEnd0, episodeIdx, episodeRangeIdx, &attrs)

	title := extractTitle(tokens, episodeIdx, episodeRangeIdx, seasonCutIdx)
	title, aliases := inferTitleAndAliasesFromTitle(title)
	titleStart, titleEnd := inferTitleTokenSpan(tokens, episodeIdx, episodeRangeIdx, seasonCutIdx)
	aliases = mergeAliases(ctx.ForcedAliases, aliases)
	if strings.TrimSpace(ctx.ForcedTitle) != "" {
		ft := ctx.ForcedTitle
		// If forced title contains strong delimiters (e.g. from bracket title extraction),
		// still infer main title + aliases deterministically.
		if strings.ContainsAny(ft, "/／|｜;；\\") {
			ftMain, ftAliases := inferTitleAndAliasesFromTitle(ft)
			if strings.TrimSpace(ftMain) != "" {
				title = ftMain
				aliases = mergeAliases(aliases, ftAliases)
			} else {
				title = ft
			}
		} else {
			title = ft
		}
		if ctx.ForcedTitleStart >= 0 && ctx.ForcedTitleEnd > ctx.ForcedTitleStart {
			titleStart = ctx.ForcedTitleStart
			titleEnd = ctx.ForcedTitleEnd
		}
	}
	if titleStart >= 0 && titleEnd > titleStart {
		techStart, hasTechStart := firstTechTokenIndexFromAttrs(attrs)
		if hasTechStart {
			if techStart < 0 {
				techStart = 0
			}
			if techStart > len(tokens) {
				techStart = len(tokens)
			}
			if techStart > titleStart && titleEnd > techStart {
				titleEnd = techStart
			}
		}
	}
	if titleStart >= 0 && titleEnd > titleStart {
		attrs = append(attrs, model.Attribute{Type: model.AttrType("title"), Normalized: title, TokenStart: titleStart, TokenEnd: titleEnd, Confidence: 0.7, Source: "heuristic"})
		aliases = mergeAliases(aliases, inferAliasesFromAdjacentBrackets(tokens, titleEnd, episodeIdx, episodeRangeIdx))
	}
	aliases = mergeAliases(aliases, parenAliases)
	aliases = mergeAliases(aliases, inferPostEpisodeSearchHintAliasesFromParens(tokens, episodeIdx, episodeRangeIdx, &attrs))
	if episodeIdx >= 0 && episode != "" {
		attrs = append(attrs, model.Attribute{Type: model.AttrEpisode, Normalized: episode, TokenStart: episodeIdx, TokenEnd: episodeIdx + 1, Confidence: 0.7, Source: "heuristic"})
	}
	if strings.TrimSpace(ctx.EpisodeVersion) != "" {
		// Align with V1: use generic AttrVersion.
		norm := strings.TrimSpace(ctx.EpisodeVersion)
		if !strings.HasPrefix(strings.ToLower(norm), "v") {
			norm = "v" + norm
		}
		if ctx.EpisodeVersionIdx >= 0 {
			attrs = append(attrs, model.Attribute{Type: model.AttrVersion, Normalized: norm, TokenStart: ctx.EpisodeVersionIdx, TokenEnd: ctx.EpisodeVersionIdx + 1, Confidence: 0.7, Source: "episode_version"})
		} else {
			attrs = append(attrs, model.Attribute{Type: model.AttrVersion, Normalized: norm, Confidence: 0.65, Source: "episode_version"})
		}
		if version == "" {
			version = norm
		}
	}
	if episodeRangeIdx >= 0 && episodeRange != "" {
		attrs = append(attrs, model.Attribute{Type: model.AttrEpisodeRange, Normalized: episodeRange, TokenStart: episodeRangeIdx, TokenEnd: episodeRangeIdx + 1, Confidence: 0.7, Source: "heuristic"})
	}
	if seasonIdx >= 0 && season != "" {
		attrs = append(attrs, model.Attribute{Type: model.AttrSeason, Normalized: season, TokenStart: seasonIdx, TokenEnd: seasonIdx + 1, Confidence: 0.7, Source: "heuristic"})
	}
	if movieIndex != "" {
		st := movieIndexIdx
		if st < 0 {
			st = -1
		}
		ed := st
		if ed >= 0 {
			ed = st + 1
		}
		attrs = append(attrs, model.Attribute{Type: model.AttrMovieIndex, Normalized: movieIndex, TokenStart: st, TokenEnd: ed, Confidence: 0.7, Source: "heuristic"})
	}

	if group == "" {
		if tg, tgIdx := detectTailGroup(tokens); tg != "" {
			group = normalizeGroupText(tg)
			attrs = append(attrs, model.Attribute{Type: model.AttrGroup, Normalized: group, TokenStart: tgIdx, TokenEnd: tgIdx + 1, Confidence: 0.7, Source: "heuristic"})
		}
	}
	if group == "" {
		if tg := detectTailGroupFromRawTitle(rawTitle); tg != "" {
			group = normalizeGroupText(tg)
			attrs = append(attrs, model.Attribute{Type: model.AttrGroup, Normalized: group, TokenStart: -1, TokenEnd: -1, Confidence: 0.65, Source: "heuristic_raw"})
			attrs = append(attrs, model.Attribute{Type: model.AttrTailGroup, Normalized: group, TokenStart: -1, TokenEnd: -1, Confidence: 0.65, Source: "heuristic_raw"})
		}
	}
	if tg, tgIdx := detectTailGroup(tokens); tg != "" {
		attrs = append(attrs, model.Attribute{Type: model.AttrTailGroup, Normalized: normalizeGroupText(tg), TokenStart: tgIdx, TokenEnd: tgIdx + 1, Confidence: 0.7, Source: "heuristic"})
	}

	hasTech := hasTechAttr(attrs)
	hasYear := false
	for _, a := range attrs {
		if a.Type == model.AttrYear {
			if strings.TrimSpace(a.Normalized) != "" {
				hasYear = true
				break
			}
		}
	}
	if hasTech {
		aliases = mergeAliases(aliases, inferTailAliasesFromPipes(tokens, attrs, group))
		aliases = mergeAliases(aliases, inferTailAliasesFromParens(tokens, &attrs))
	}
	if strings.TrimSpace(packKind) == "" {
		if strings.TrimSpace(episode) == "" && strings.TrimSpace(episodeRange) == "" {
			if regexp.MustCompile(`(?i)\bBD\b`).MatchString(rawTitle) {
				packKind = "bd"
			}
		}
	}
	epRoute := computeEpRoute(tokens, episode, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword)
	contentKind, contentConf := inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, hasYear)
	templateID, templateConf, routeID := matchTemplate(p.cfg, epRoute, contentKind, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, attrs, tokens)
	if strings.HasPrefix(routeID, "streaming_release_") {
		applyStreamingReleaseRouteV1(tailParenBlock, &aliases, &attrs)
	}
	origTitle := title
	title = sanitizePrimaryTitleForOutput(title)
	aliases = sanitizeAliasesForOutput(aliases, title)
	if strings.TrimSpace(title) != "" && aliasSemanticKey(title) != aliasSemanticKey(origTitle) {
		attrs = upsertTitleAttribute(attrs, title, "title_cleanup")
	}
	attrs = dedupeAttributes(attrs)

	res := &model.ParseResult{
		RawTitle:           rawTitle,
		Group:              group,
		Title:              title,
		Aliases:            aliases,
		Episode:            episode,
		EpisodeRange:       episodeRange,
		EpRoute:            epRoute,
		Season:             season,
		SeasonRange:        seasonRange,
		Version:            version,
		PackKind:           packKind,
		SPKind:             spKind,
		MovieIndex:         movieIndex,
		ContentKind:        contentKind,
		ContentConfidence:  contentConf,
		TemplateID:         templateID,
		TemplateConfidence: templateConf,
		TailParenBlock:     tailParenBlock,
		EPSlot:             epslot,
		AudioSlot:          ctx.AudioSlot,
		Attributes:         attrs,
		Tokens:             tokens,
	}

	if tokenONNXEnabled() {
		// ONNX temporarily disabled for HTTP server
		// res.TokenONNX = onnx.InferTokenBoundary(rawTitle, tokens, attrs)
		// applyONNXIfEnabled(res)
	}

	// XLM-R 质量门控和候选重排
	if xlmrONNXEnabled() {
		qualityCheck := quality.CheckTitleQuality(res)

		// 如果质量检查失败,触发 XLM-R
		if quality.ShouldTriggerXLMR(qualityCheck) {
			engine, err := xlmr.GetEngine()
			if err == nil {
				// 提取 token 文本
				tokenTexts := quality.ExtractTokenTexts(res)

				// XLM-R 推理
				labels, err := engine.Infer(tokenTexts)
				if err == nil {
					// 从标签推断新的 title span
					newTitleStart, newTitleEnd := quality.InferTitleSpanFromLabels(res.Tokens, labels)

					if newTitleStart >= 0 && newTitleEnd > newTitleStart {
						// 提取新的 title
						newTitle := quality.ExtractTitleFromSpan(res.Tokens, newTitleStart, newTitleEnd)

						// 验证新 title 质量
						if strings.TrimSpace(newTitle) != "" && len([]rune(newTitle)) >= 2 {
							// 使用新 title 重新解析
							overrideHints := &ParseHints{
								OverrideTitleStart: newTitleStart,
								OverrideTitleEnd:   newTitleEnd,
							}

							// 递归调用（但不会再次触发 XLM-R,因为 hints 已设置）
							overrideRes, err := p.ParseWithHints(rawTitle, overrideHints)
							if err == nil {
								// 验证新结果质量
								newQualityCheck := quality.CheckTitleQuality(overrideRes)

								// 如果新结果更好,使用新结果
								if newQualityCheck.Status == "PASS" || newQualityCheck.Score > qualityCheck.Score {
									// 填充 ML 字段
									overrideRes.ML = &model.MLOutput{
										RawTitleHash: rawTitle,
										TokenLabels:  labels,
										Risk: &model.MLRisk{
											Level:   qualityCheck.RiskLevel,
											Reasons: qualityCheck.Reasons,
										},
									}
									res = overrideRes
								}
							}
						}
					}

					// 即使没有重排,也填充 ML 字段（用于调试）
					if res.ML == nil {
						res.ML = &model.MLOutput{
							RawTitleHash: rawTitle,
							TokenLabels:  labels,
							Risk: &model.MLRisk{
								Level:   qualityCheck.RiskLevel,
								Reasons: qualityCheck.Reasons,
							},
						}
					}
				}
			}
		}
	}

	return res, nil
}

var yearMonthShowTagPattern = regexp.MustCompile(`^(?:\d{4}年)?\d{1,2}月番$`)

func extractSearchHintIndexAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		text := strings.TrimSpace(tokens[i].Text)
		if text == "" {
			continue
		}
		if !strings.Contains(text, "检索用") {
			continue
		}
		if !strings.Contains(text, "(") && !strings.Contains(text, "（") {
			continue
		}

		end := i
		for j := i; j < len(tokens); j++ {
			if tokens[j].Removed {
				continue
			}
			end = j
			tj := strings.TrimSpace(tokens[j].Text)
			if strings.Contains(tj, ")") || strings.Contains(tj, "）") {
				break
			}
		}

		for k := i; k <= end; k++ {
			tokens[k].Removed = true
		}
		*out = append(*out, model.Attribute{Type: model.AttrNote, Normalized: "检索用", TokenStart: i, TokenEnd: end + 1, Confidence: 0.7, Source: "heuristic"})
		i = end
	}
}

func extractSeasonShowTagAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if !isInBracket(tokens, i) {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if !yearMonthShowTagPattern.MatchString(t) {
			continue
		}
		tokens[i].Removed = true
		*out = append(*out, model.Attribute{Type: model.AttrNote, Normalized: t, TokenStart: i, TokenEnd: i + 1, Confidence: 0.7, Source: "heuristic"})
	}
}

func dedupeAttributes(attrs []model.Attribute) []model.Attribute {
	if len(attrs) <= 1 {
		return attrs
	}
	keyOf := func(a model.Attribute) string {
		t := strings.TrimSpace(string(a.Type))
		n := strings.TrimSpace(a.Normalized)
		return t + "\t" + n
	}
	bestIdx := make(map[string]int, len(attrs))
	keep := make([]bool, len(attrs))
	for i := range attrs {
		a := attrs[i]
		k := keyOf(a)
		if prev, ok := bestIdx[k]; ok {
			pa := attrs[prev]
			if a.Confidence > pa.Confidence {
				bestIdx[k] = i
				continue
			}
			if a.Confidence == pa.Confidence {
				plen := pa.TokenEnd - pa.TokenStart
				alen := a.TokenEnd - a.TokenStart
				if alen > plen {
					bestIdx[k] = i
				}
			}
			continue
		}
		bestIdx[k] = i
	}
	for _, idx := range bestIdx {
		if idx >= 0 && idx < len(keep) {
			keep[idx] = true
		}
	}
	out := make([]model.Attribute, 0, len(bestIdx))
	for i := range attrs {
		if keep[i] {
			out = append(out, attrs[i])
		}
	}
	return out
}

func computeEpRoute(tokens []model.Token, episode string, episodeRange string, season string, seasonRange string, spKind string, movieIndex string, hasMovieKeyword bool) string {
	// Priority (high -> low):
	// RANGE (TV signal) > SXXEXX > ABS_EP > PLAIN_EP > MOVIE_ONLY > SP_ONLY > NONE
	if strings.TrimSpace(episodeRange) != "" {
		return "RANGE"
	}
	// Tagged ranges like ABS:01-12 / TV01-12 / OVA:01-05.
	for i := 0; i < len(tokens); i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if m := taggedRangePattern.FindStringSubmatch(txt); len(m) == 3 {
			val := strings.TrimSpace(m[2])
			if strings.ContainsAny(val, "-~") {
				return "RANGE"
			}
		}
	}
	if hasSxxExxInTokens(tokens) && strings.TrimSpace(season) != "" && strings.TrimSpace(episode) != "" {
		return "SXXEXX"
	}
	// Tagged ABS episode like ABS:12.
	for i := 0; i < len(tokens); i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if m := taggedRangePattern.FindStringSubmatch(txt); len(m) == 3 {
			axis := strings.ToUpper(strings.TrimSpace(m[1]))
			val := strings.TrimSpace(m[2])
			if axis == "ABS" && !strings.ContainsAny(val, "-~") {
				return "ABS_EP"
			}
		}
	}
	if strings.TrimSpace(episode) != "" {
		return "PLAIN_EP"
	}
	// For season packs, do not route as SP_ONLY even if sp_kind exists.
	// These are still packs (BATCH) and should match BATCH templates.
	if strings.TrimSpace(seasonRange) != "" {
		return "NONE"
	}
	if strings.TrimSpace(season) != "" {
		return "NONE"
	}
	if strings.TrimSpace(movieIndex) != "" || hasMovieKeyword {
		return "MOVIE_ONLY"
	}
	if strings.TrimSpace(spKind) != "" {
		return "SP_ONLY"
	}
	return "NONE"
}

var packExtraMarkerPattern = regexp.MustCompile(`(?i)^(?:\+\s*)?(movie|special|sp|ova|oad|ona|ex)$`)
var packSeasonTokenPattern = regexp.MustCompile(`(?i)^s0*\d{1,2}$`)

func extractPackSeasonExtrasAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute, seasonRange string) {
	if out == nil || len(tokens) == 0 {
		return
	}

	forcePack := strings.TrimSpace(seasonRange) != ""

	isExtraMarker := func(t string) (string, bool) {
		t = strings.TrimSpace(t)
		if t == "" {
			return "", false
		}
		m := packExtraMarkerPattern.FindStringSubmatch(t)
		if len(m) == 2 {
			return strings.ToUpper(strings.TrimSpace(m[1])), true
		}
		return "", false
	}

	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		open := tokens[i].Text
		close := ""
		if open == "[" {
			close = "]"
		} else if open == "【" {
			close = "】"
		} else {
			continue
		}

		depth := 0
		end := -1
		for j := i; j < len(tokens); j++ {
			if tokens[j].Removed {
				continue
			}
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

		// Decide whether this bracket block is a season pack marker block.
		hasSeasonToken := false
		for k := i + 1; k < end; k++ {
			if tokens[k].Removed {
				continue
			}
			if packSeasonTokenPattern.MatchString(strings.TrimSpace(tokens[k].Text)) {
				hasSeasonToken = true
				break
			}
		}
		isSeasonPackBlock := forcePack || hasSeasonToken
		if !isSeasonPackBlock {
			i = end
			continue
		}

		// Extract patterns like "+ OVA" / "+ MOVIE" inside the block.
		for k := i + 1; k < end; k++ {
			if tokens[k].Removed {
				continue
			}
			tk := strings.TrimSpace(tokens[k].Text)
			if tk == "" {
				continue
			}
			if tk == "+" || tk == "＋" {
				// Lookahead to the next meaningful token.
				n := -1
				for m := k + 1; m < end; m++ {
					if tokens[m].Removed {
						continue
					}
					mt := strings.TrimSpace(tokens[m].Text)
					if mt == "" {
						continue
					}
					n = m
					break
				}
				if n < 0 {
					continue
				}
				if norm, ok := isExtraMarker(tokens[n].Text); ok {
					*out = append(*out, model.Attribute{Type: model.AttrContents, Normalized: norm, TokenStart: k, TokenEnd: n + 1, Confidence: 0.85, Source: "pack_extras"})
					tokens[k].Removed = true
					tokens[n].Removed = true
					k = n
				}
				continue
			}
			// Also support combined tokens like "+OVA".
			if norm, ok := isExtraMarker(tk); ok {
				// If previous token is '+', remove it as well.
				if k-1 >= i+1 {
					prev := strings.TrimSpace(tokens[k-1].Text)
					if prev == "+" || prev == "＋" {
						tokens[k-1].Removed = true
					}
				}
				*out = append(*out, model.Attribute{Type: model.AttrContents, Normalized: norm, TokenStart: k, TokenEnd: k + 1, Confidence: 0.85, Source: "pack_extras"})
				tokens[k].Removed = true
				continue
			}
		}
		i = end
	}

	// Also handle non-bracket patterns like: "Title S01 + OVA 1080p".
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		if !packSeasonTokenPattern.MatchString(strings.TrimSpace(tokens[i].Text)) {
			continue
		}
		// Lookahead a small window for "+ <extra>".
		for k := i + 1; k < len(tokens) && k <= i+8; k++ {
			if tokens[k].Removed {
				continue
			}
			if isInBracket(tokens, k) || isInParen(tokens, k) {
				continue
			}
			tk := strings.TrimSpace(tokens[k].Text)
			if tk != "+" && tk != "＋" {
				continue
			}
			// Find next meaningful token.
			n := -1
			for m := k + 1; m < len(tokens) && m <= i+10; m++ {
				if tokens[m].Removed {
					continue
				}
				if isInBracket(tokens, m) || isInParen(tokens, m) {
					continue
				}
				mt := strings.TrimSpace(tokens[m].Text)
				if mt == "" {
					continue
				}
				n = m
				break
			}
			if n < 0 {
				continue
			}
			if norm, ok := isExtraMarker(tokens[n].Text); ok {
				*out = append(*out, model.Attribute{Type: model.AttrContents, Normalized: norm, TokenStart: k, TokenEnd: n + 1, Confidence: 0.85, Source: "pack_extras"})
				tokens[k].Removed = true
				tokens[n].Removed = true
				k = n
			}
		}
	}
}

func normalizePlatformAttrs(attrs []model.Attribute) {
	for i := range attrs {
		if attrs[i].Type != model.AttrPlatform {
			continue
		}
		attrs[i].Normalized = normalizePlatformCode(attrs[i].Normalized)
	}
}

func classifyAndEmitTailMetaNote(part string, attrs *[]model.Attribute) bool {
	if attrs == nil {
		return false
	}
	p := strings.TrimSpace(part)
	if p == "" {
		return false
	}
	low := strings.ToLower(p)
	note := ""
	switch {
	case low == "weekly":
		note = "WEEKLY"
	case low == "batch":
		note = "BATCH"
	case low == "reseed":
		note = "RESEED"
	case low == "complete" || low == "fin" || low == "end":
		note = "COMPLETE"
	case strings.HasPrefix(low, "final season"):
		note = "FINAL_SEASON"
	}
	if note == "" {
		return false
	}
	*attrs = append(*attrs, model.Attribute{Type: model.AttrNote, Normalized: note, Confidence: 0.6, Source: "tail_meta"})
	return true
}

func normalizeSourceTypeAttrs(attrs []model.Attribute) {
	for i := range attrs {
		if attrs[i].Type != model.AttrSourceType {
			continue
		}
		n := strings.ToUpper(strings.TrimSpace(attrs[i].Normalized))
		if n == "WEBRIP" || n == "WEB-RIP" || n == "WEB RIP" {
			attrs[i].Normalized = "WEBRip"
			continue
		}
		if n == "WEBDL" || n == "WEB-DL" || n == "WEB DL" {
			attrs[i].Normalized = "WEB-DL"
			continue
		}
		attrs[i].Normalized = strings.TrimSpace(attrs[i].Normalized)
	}
}

func normalizePlatformCode(s string) string {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return ""
	}
	up := strings.ToUpper(ss)
	switch up {
	case "CRUNCHYROLL", "CR":
		return "CR"
	case "BAHAMUT", "BAHA":
		return "BAHA"
	case "BILIBILI_GLOBAL", "BILIBILI":
		return "BILIBILI"
	case "AMAZON", "AMZN":
		return "AMZN"
	case "ABEMA":
		return "ABEMA"
	case "HIDIVE":
		return "HIDIVE"
	case "ADN":
		return "ADN"
	case "VIUTV":
		return "VIUTV"
	default:
		return up
	}
}

func convertRegionAttrsToPlatformRegion(attrs []model.Attribute) {
	for i := range attrs {
		if attrs[i].Type != model.AttrType("region") {
			continue
		}
		attrs[i].Type = model.AttrPlatformRegion
	}
}

func detectTailParenBlock(tokens []model.Token) string {
	if len(tokens) == 0 {
		return ""
	}
	last := -1
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		last = i
		break
	}
	if last < 0 {
		return ""
	}
	close := strings.TrimSpace(tokens[last].Text)
	var openTok string
	if close == ")" {
		openTok = "("
	} else if close == "）" {
		openTok = "（"
	} else {
		return ""
	}
	depth := 0
	openIdx := -1
	for i := last; i >= 0; i-- {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if t == close {
			depth++
			continue
		}
		if t == openTok {
			depth--
			if depth == 0 {
				openIdx = i
				break
			}
		}
	}
	if openIdx < 0 {
		return ""
	}
	end := last
	if end <= openIdx {
		return ""
	}
	parts := make([]string, 0, end-openIdx)
	for k := openIdx + 1; k < end; k++ {
		t := strings.TrimSpace(tokens[k].Text)
		if t == "" {
			continue
		}
		parts = append(parts, t)
	}
	inner := strings.TrimSpace(strings.Join(parts, " "))
	if inner == "" {
		return ""
	}
	return inner
}

func hasAttrType(attrs []model.Attribute, t model.AttrType) bool {
	for _, a := range attrs {
		if a.Type == t {
			return true
		}
	}
	return false
}

func isStreamingRelease(tokens []model.Token, attrs []model.Attribute) bool {
	if !hasAttrType(attrs, model.AttrPlatform) {
		return false
	}
	for _, a := range attrs {
		if a.Type != model.AttrSourceType {
			continue
		}
		n := strings.ToUpper(strings.TrimSpace(a.Normalized))
		if n == "WEB-DL" || n == "WEBRIP" || n == "WEB-RIP" {
			return true
		}
	}
	return false
}

var reStreamMetaFlag = regexp.MustCompile(`(?i)\b(?:multi[-_ ]?subs?|multisub|dual[-_ ]?audio|multi[-_ ]?audio|english\s*dub|dub|vostfr|vf)\b`)

var reTailMetaToken = regexp.MustCompile(`(?i)^(?:weekly|batch|reseed|complete|fin|end|final\s+season|season|season\s*\d{1,2}|s\d{1,2}|\d{1,2}(?:st|nd|rd|th))$`)

var reHintPrefix = regexp.MustCompile(`(?i)^(检索用|检索|檢索用|檢索|索引|搜索用|搜尋用|搜索|搜尋|search)\s*[:：]\s*(.+)$`)

var reAliasTrimTVSeasonTail = regexp.MustCompile(`(?i)(?:^|\s+)(?:tv|tb|тv|тв)\s*[-_ ]\s*\d{1,2}\s*$`)
var reAliasTrimPlatformTail = regexp.MustCompile(`(?i)\s+(?:b-?global(?:\s+donghua)?|baha|abema|cr|amzn|amazon|netflix|nf|unext|at-?x|bs11|bs-?ntv|bs-?fuji|france\s*tv|tv)\s*$`)
var reAliasTrimTVAnimePrefix = regexp.MustCompile(`(?i)^(?:tv\s*アニメ|tv\s*anime|tv\s*animation)\s+`)
var reTitleTrimTVSeasonTail = regexp.MustCompile(`(?i)\s+(?:tv|tb|тv|тв)\s*[-_ ]?\s*\d{1,3}\s*$`)
var reTitleTrimTVTail = regexp.MustCompile(`(?i)\s+(?:tv|tb|тv|тв)\s*$`)
var reTitleTrimTVSeriesTail = regexp.MustCompile(`(?i)\s+tv\s+series\s*$`)
var reTitleStripTVAnimeSegmentPrefix = regexp.MustCompile(`(?i)(^|/\s*)tv\s*(?:アニメ|anime|animation)\s+`)
var reTitleTrimMetaTail = regexp.MustCompile(`(?i)\s+(?:hd|sd|fhd|uhd|retail|webrip|web[- ]?dl|bdrip|bluray|blu-ray|remux|dvd|hdtv|mkv|mp4|avi|x26[45]|h\.?26[45]|hevc|av1|aac|ac3|e-?ac-?3|flac|dts(?:-?hd|-?hdma)?|hdma|truehd|ma\d{1,2}p|10[- ]?bit|8[- ]?bit)\s*$`)
var reTitleTrimVolTail = regexp.MustCompile(`(?i)\s+(?:vol\.?(?:\s*\d{1,3})?|volume\s*\d{1,3})\s*$`)

func normalizeStreamMetaText(s string) string {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return ""
	}
	// Collapse tokenization artifacts like "Multi - Audio" => "Multi-Audio".
	ss = strings.ReplaceAll(ss, " - ", "-")
	ss = strings.ReplaceAll(ss, " _ ", "_")
	// Collapse repeated whitespace.
	ss = regexp.MustCompile(`\s+`).ReplaceAllString(ss, " ")
	return strings.TrimSpace(ss)
}

func isStreamMetaText(s string) bool {
	ss := normalizeStreamMetaText(s)
	if ss == "" {
		return false
	}
	return reStreamMetaFlag.MatchString(ss)
}

func classifyAndEmitStreamMeta(part string, attrs *[]model.Attribute) {
	if attrs == nil {
		return
	}
	p := normalizeStreamMetaText(part)
	if p == "" {
		return
	}
	up := strings.ToUpper(p)
	if strings.Contains(up, "SUB") {
		if strings.Contains(up, "MULTI") {
			*attrs = append(*attrs, model.Attribute{Type: model.AttrSubFlag, Normalized: "MULTI_SUBS", Confidence: 0.6, Source: "route_streaming_release_v1"})
		} else {
			*attrs = append(*attrs, model.Attribute{Type: model.AttrSubFlag, Normalized: "SUBS", Confidence: 0.6, Source: "route_streaming_release_v1"})
		}
	}
	if strings.Contains(up, "DUB") {
		if strings.Contains(up, "ENGLISH") {
			*attrs = append(*attrs, model.Attribute{Type: model.AttrAudioFlag, Normalized: "ENGLISH_DUB", Confidence: 0.6, Source: "route_streaming_release_v1"})
		} else {
			*attrs = append(*attrs, model.Attribute{Type: model.AttrAudioFlag, Normalized: "DUB", Confidence: 0.6, Source: "route_streaming_release_v1"})
		}
		return
	}
	if strings.Contains(up, "AUDIO") {
		if strings.Contains(up, "DUAL") {
			*attrs = append(*attrs, model.Attribute{Type: model.AttrAudioFlag, Normalized: "DUAL_AUDIO", Confidence: 0.6, Source: "route_streaming_release_v1"})
			return
		}
		if strings.Contains(up, "MULTI") {
			*attrs = append(*attrs, model.Attribute{Type: model.AttrAudioFlag, Normalized: "MULTI_AUDIO", Confidence: 0.6, Source: "route_streaming_release_v1"})
			return
		}
		*attrs = append(*attrs, model.Attribute{Type: model.AttrAudioFlag, Normalized: "AUDIO", Confidence: 0.6, Source: "route_streaming_release_v1"})
	}
}

func filterOutStreamMetaAliases(aliases []string) []string {
	if len(aliases) == 0 {
		return nil
	}
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if isStreamMetaText(a) {
			continue
		}
		if reTailMetaToken.MatchString(strings.TrimSpace(a)) {
			continue
		}
		// Catch tokenization artifacts like "- Subs" / "Dual - Audio".
		aa := strings.TrimSpace(strings.ReplaceAll(a, " ", ""))
		if strings.EqualFold(aa, "-subs") || strings.EqualFold(aa, "-audio") {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func applyStreamingReleaseRouteV1(tailParenBlock string, aliases *[]string, attrs *[]model.Attribute) {
	if aliases == nil {
		return
	}
	*aliases = filterOutStreamMetaAliases(*aliases)

	block := strings.TrimSpace(tailParenBlock)
	if block == "" {
		return
	}
	// Split by common delimiters (including fullwidth variants).
	spl := regexp.MustCompile(`[,，;；\|/]+`).Split(block, -1)
	aliasCands := make([]string, 0, len(spl))
	for _, it := range spl {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if isStreamMetaText(it) {
			classifyAndEmitStreamMeta(it, attrs)
			continue
		}
		if reTailMetaToken.MatchString(it) {
			continue
		}
		it = cleanAliasToken(it)
		if !isValidAliasToken(it) {
			continue
		}
		aliasCands = append(aliasCands, it)
	}
	if len(aliasCands) == 0 {
		return
	}
	*aliases = mergeAliases(*aliases, aliasCands)
}

func tokenONNXEnabled() bool {
	v := strings.TrimSpace(os.Getenv("TITLEPARSER_TOKEN_ONNX"))
	if v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return true
	}
	v = strings.TrimSpace(os.Getenv("TITLEPARSER_TOKEN_ONNX_APPLY"))
	if v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return true
	}
	return false
}

func xlmrONNXEnabled() bool {
	v := strings.TrimSpace(os.Getenv("XLMR_ONNX_ENABLED"))
	return v == "1" || strings.EqualFold(v, "true")
}

func applyONNXIfEnabled(res *model.ParseResult) {
	if res == nil {
		return
	}
	v := strings.TrimSpace(os.Getenv("TITLEPARSER_TOKEN_ONNX_APPLY"))
	if v == "" || v == "0" || strings.EqualFold(v, "false") {
		return
	}
	if res.TokenONNX == nil {
		return
	}
	if strings.TrimSpace(res.TokenONNX.Error) != "" {
		return
	}
	if res.TokenONNX.Fields == nil {
		return
	}

	if strings.TrimSpace(res.Group) == "" {
		g := strings.TrimSpace(res.TokenONNX.Fields["group"])
		if g != "" {
			res.Group = g
		}
	}

	if strings.TrimSpace(res.Episode) == "" {
		ep := strings.TrimSpace(res.TokenONNX.Fields["episode"])
		if isSafeEpisode(ep) {
			res.Episode = strings.TrimLeft(ep, "0")
			if res.Episode == "" {
				res.Episode = ep
			}
		}
	}

	if strings.TrimSpace(res.Title) == "" {
		t := strings.TrimSpace(res.TokenONNX.Fields["title"])
		if isSafeTitle(t, res.Group) {
			res.Title = t
		}
	}
}

var safeEpisodePattern = regexp.MustCompile(`^0*\d{1,4}(?:\.\d{1,2})?$`)

func isSafeEpisode(ep string) bool {
	ep = strings.TrimSpace(ep)
	if ep == "" {
		return false
	}
	return safeEpisodePattern.MatchString(ep)
}

func isSafeTitle(title string, group string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	if len([]rune(title)) > 80 {
		return false
	}
	if strings.TrimSpace(group) != "" && strings.EqualFold(strings.TrimSpace(group), title) {
		return false
	}
	low := strings.ToLower(title)
	if strings.Contains(low, "1080p") || strings.Contains(low, "2160p") || strings.Contains(low, "720p") {
		return false
	}
	if strings.Contains(low, "hevc") || strings.Contains(low, "x265") || strings.Contains(low, "h265") || strings.Contains(low, "x264") || strings.Contains(low, "h264") {
		return false
	}
	if strings.Contains(low, "webrip") || strings.Contains(low, "web-dl") || strings.Contains(low, "webdl") || strings.Contains(low, "bluray") {
		return false
	}
	return true
}

var reSxxExxToken = regexp.MustCompile(`(?i)^s\d{1,2}e\d{1,4}$`)

func hasSxxExxInTokens(tokens []model.Token) bool {
	for i := 0; i < len(tokens); i++ {
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if reSxxExxToken.MatchString(t) {
			return true
		}
	}
	return false
}

func stripSeasonOrPartSuffix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Guard: numeric ranges like "17-26" are often part of title/alias (e.g. SweetSub releases),
	// not a season/part suffix.
	if regexp.MustCompile(`\b\d{1,4}\s*-\s*\d{1,4}\s*$`).FindStringIndex(s) != nil {
		return ""
	}
	if !looksLikeSeasonOrPartSuffix(s) {
		return ""
	}
	// Remove explicit season expressions.
	out := s
	out = regexp.MustCompile(`(?i)\b\d{1,2}(?:st|nd|rd|th)?\s*season\b`).ReplaceAllString(out, "")
	out = regexp.MustCompile(`(?i)\bseason\s*\d{1,2}\b`).ReplaceAllString(out, "")
	out = regexp.MustCompile(`第\s*[一二三四五六七八九十0-9]{1,3}\s*(?:季|期|部)`).ReplaceAllString(out, "")
	out = strings.TrimSpace(out)
	// Remove common sequel suffixes at end.
	out = regexp.MustCompile(`\s+[0-9]{1,2}\s*$`).ReplaceAllString(out, "")
	out = regexp.MustCompile(`(?i)\s+(?:ii|iii|iv|v|vi|vii|viii|ix|x)\s*$`).ReplaceAllString(out, "")
	out = regexp.MustCompile(`\s*[贰貳弐二三四五六七八九十]\s*$`).ReplaceAllString(out, "")
	out = strings.TrimSpace(out)
	if out == "" || out == s {
		return ""
	}
	return normalizeTitleMain(out)
}

var reExplicitSeasonMarker = regexp.MustCompile(`(?i)\b(?:season\b|s\d{1,2}\b)`)
var reExplicitSeasonMarkerCJK = regexp.MustCompile(`第\s*[一二三四五六七八九十0-9]{1,3}\s*(?:季|期|部)`)

func stripExplicitSeasonSuffix(s string) string {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return ""
	}
	if !reExplicitSeasonMarker.MatchString(ss) && !reExplicitSeasonMarkerCJK.MatchString(ss) {
		return ""
	}
	if stripped := stripSeasonOrPartSuffix(ss); stripped != "" {
		return stripped
	}
	return ""
}

func looksLikeSeasonOrPartSuffix(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Common patterns: title + " 2" / " II" / "贰" / "第2季" etc.
	if regexp.MustCompile(`(?i)\bseason\b`).MatchString(s) {
		return true
	}
	if regexp.MustCompile(`(?i)\b\d{1,2}(?:st|nd|rd|th)?\s*season\b`).MatchString(s) {
		return true
	}
	if regexp.MustCompile(`第\s*[一二三四五六七八九十0-9]{1,3}\s*(?:季|期|部)`).FindStringIndex(s) != nil {
		return true
	}
	if regexp.MustCompile(`\s(?:[0-9]{1,2}|[一二三四五六七八九十]|[贰貳弐参參三四五六七八九十])\s*$`).FindStringIndex(s) != nil {
		return true
	}
	return false
}

func splitByStrongTitleDelims(s string) []string {
	if s == "" {
		return nil
	}
	if !strings.ContainsAny(s, "/／|｜;；\\") {
		return []string{s}
	}
	norm := strings.ReplaceAll(s, "｜", "|")
	norm = strings.ReplaceAll(norm, "／", "/")
	norm = strings.ReplaceAll(norm, "；", ";")
	parts := strings.FieldsFunc(norm, func(r rune) bool {
		return r == '/' || r == '|' || r == ';' || r == '\\'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

func chooseMainTitlePart(parts []string) string {
	best := ""
	bestScore := -1
	for idx, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		score := 0
		if hasCJK(p) {
			score += 20
		}
		if hasLatin(p) {
			score += 10
		}
		l := len([]rune(p))
		if l >= 2 {
			score += l
		}
		if idx == 0 {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			best = p
		}
	}
	if best == "" && len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return best
}

func extractPlatformRegionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		orig := strings.TrimSpace(tokens[i].Text)
		if orig == "" {
			continue
		}
		if regionHKMOTWEmbeddedPattern.FindStringIndex(orig) == nil {
			continue
		}
		*out = append(*out, model.Attribute{Type: model.AttrPlatformRegion, Normalized: "HK+MO+TW", TokenStart: i, TokenEnd: i + 1, Confidence: 0.8, Source: "region_restriction"})
		cleaned := regionHKMOTWRemovalPattern.ReplaceAllString(orig, "")
		cleaned = strings.TrimSpace(cleaned)
		cleaned = strings.Trim(cleaned, "（）()[]【】")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			tokens[i].Removed = true
			continue
		}
		tokens[i].Text = cleaned
	}
}

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
		*out = append(*out, model.Attribute{Type: model.AttrNote, Normalized: "CUSTOM", TokenStart: i, TokenEnd: i + 1, Confidence: 0.6, Source: "custom_tag"})
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
		*out = append(*out, model.Attribute{Type: model.AttrNote, Normalized: "DVDRIP", TokenStart: i, TokenEnd: i + 1, Confidence: 0.7, Source: "dvdrip_tag"})
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

func cleanAliasToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, "[](){}【】（）『』「」")
	s = strings.TrimSpace(s)
	s = normalizeTitleMain(s)
	s = stripFileExtSuffix(s)
	return strings.TrimSpace(s)
}

func normalizeAliasCandidateForOutput(s string) string {
	ss := cleanAliasToken(s)
	ss = strings.TrimSpace(ss)
	if ss == "" {
		return ""
	}
	prev := ""
	for ss != prev {
		prev = ss
		ss = reAliasTrimTVSeasonTail.ReplaceAllString(ss, "")
		ss = reAliasTrimPlatformTail.ReplaceAllString(ss, "")
		ss = reAliasTrimTVAnimePrefix.ReplaceAllString(ss, "")
		ss = reTitleTrimMetaTail.ReplaceAllString(ss, "")
		ss = reTitleTrimVolTail.ReplaceAllString(ss, "")
		ss = strings.TrimSpace(ss)
		ss = strings.Trim(ss, "-_/;:,+ ")
	}
	return strings.TrimSpace(cleanAliasToken(ss))
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

func sanitizeAliasesForOutput(aliases []string, title string) []string {
	if len(aliases) == 0 {
		return nil
	}
	titleKey := aliasSemanticKey(title)
	out := make([]string, 0, len(aliases))
	seen := map[string]bool{}
	for _, a := range aliases {
		a = normalizeAliasCandidateForOutput(a)
		if a == "" || !isValidAliasToken(a) {
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
		s = reTitleTrimTVSeasonTail.ReplaceAllString(s, "")
		s = reTitleTrimTVTail.ReplaceAllString(s, "")
		s = reTitleTrimMetaTail.ReplaceAllString(s, "")
		s = reTitleTrimVolTail.ReplaceAllString(s, "")
		s = strings.TrimSpace(strings.Trim(s, "-_/;:,+ "))
	}
	return normalizeTitleMain(s)
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
		if out[i].Type != model.AttrType("title") {
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
			Type:       model.AttrType("title"),
			Normalized: title,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.7,
			Source:     source,
		})
	}
	return out
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

func stripFileExtSuffix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.TrimSpace(reFileExtSuffix.ReplaceAllString(s, ""))
}

func isValidAliasToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
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
	if regexp.MustCompile(`(?i)^[0-9a-f]{8}$`).MatchString(low) {
		return false
	}
	if strings.Contains(s, "招募") || strings.Contains(s, "仅限") || strings.Contains(s, "港澳") || strings.Contains(s, "臺") || strings.Contains(s, "台") {
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
	if strings.Contains(low, "1080p") || strings.Contains(low, "2160p") || strings.Contains(low, "720p") {
		return false
	}
	if strings.Contains(low, "hevc") || strings.Contains(low, "x265") || strings.Contains(low, "h265") || strings.Contains(low, "x264") || strings.Contains(low, "h264") {
		return false
	}
	if strings.Contains(low, "webrip") || strings.Contains(low, "web-dl") || strings.Contains(low, "webdl") || strings.Contains(low, "bluray") {
		return false
	}
	if episodeOfTotalPattern.MatchString(s) {
		return false
	}
	return true
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

func inferAliasesFromAdjacentBrackets(tokens []model.Token, titleEnd int, episodeIdx int, episodeRangeIdx int) []string {
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
			if tokens[k].Removed {
				continue
			}
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
		if isLikelyMetaBracketText(inner) {
			i = end
			continue
		}
		splits := splitByStrongTitleDelims(inner)
		for _, s := range splits {
			s = cleanAliasToken(s)
			if stripped := stripExplicitSeasonSuffix(s); stripped != "" {
				s = stripped
			}
			if !isValidAliasToken(s) {
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

var tailGroupFromRawTitlePattern = regexp.MustCompile(`(?i)(?:^|\s)-(?:\s*)?([A-Za-z][A-Za-z0-9]{1,24}(?:-[A-Za-z0-9]{1,24}){0,4}-(?:Raws|Subs))\b`)
var tailGroupFromRawTitleAnyPattern = regexp.MustCompile(`(?i)-(\s*)?([A-Za-z][A-Za-z0-9]{2,20})\s*$`)

func detectTailGroupFromRawTitle(rawTitle string) string {
	s := strings.TrimSpace(rawTitle)
	if s == "" {
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
	return ""
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

func extractParenAliasesAndRemoveFromTitle(tokens []model.Token, titleEnd int, episodeIdx int, episodeRangeIdx int, attrs *[]model.Attribute) []string {
	if titleEnd < 0 {
		return nil
	}
	start := titleEnd
	end := len(tokens)
	if episodeRangeIdx >= 0 {
		end = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < end {
		end = episodeIdx
	}
	if end <= start {
		return nil
	}

	aliases := make([]string, 0, 8)
	for i := start; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		if tokens[i].Text != "(" && tokens[i].Text != "（" {
			continue
		}
		openIdx, closeIdx, inner, ok := parseParenBlock(tokens, i, end)
		if !ok {
			continue
		}
		inner = strings.TrimSpace(inner)
		if inner == "" {
			for k := openIdx; k <= closeIdx; k++ {
				tokens[k].Removed = true
			}
			i = closeIdx
			continue
		}
		if strings.Contains(inner, "※") {
			i = closeIdx
			continue
		}
		cands, isSearchHint, hintKeyword := splitAliasCandidatesFromParenContent(inner)
		if looksLikeParenMetaOrTech(tokens, openIdx, end) && !isSearchHint {
			if attrs != nil {
				spl := regexp.MustCompile(`[,，;；\|/]+`).Split(inner, -1)
				for _, it := range spl {
					it = strings.TrimSpace(it)
					if it == "" {
						continue
					}
					if isStreamMetaText(it) {
						classifyAndEmitStreamMeta(it, attrs)
						continue
					}
					if reTailMetaToken.MatchString(it) {
						classifyAndEmitTailMetaNote(it, attrs)
						continue
					}
				}
			}
			for k := openIdx; k <= closeIdx; k++ {
				tokens[k].Removed = true
			}
			i = closeIdx
			continue
		}
		if isSearchHint && attrs != nil {
			hk := strings.TrimSpace(hintKeyword)
			if hk != "" {
				*attrs = append(*attrs, model.Attribute{Type: model.AttrNote, Normalized: hk, Confidence: 0.6, Source: "hint_keyword"})
			}
		}
		for _, c := range cands {
			c = cleanAliasToken(c)
			if isSearchHint {
				if c == "" {
					continue
				}
				if isLikelyMetaBracketText(c) {
					continue
				}
				if !hasCJK(c) && len([]rune(c)) < 3 {
					continue
				}
			} else {
				if !isValidAliasToken(c) {
					continue
				}
				if stripped := stripExplicitSeasonSuffix(c); stripped != "" {
					c = stripped
					if !isValidAliasToken(c) {
						continue
					}
				}
				if reTailMetaToken.MatchString(c) {
					classifyAndEmitTailMetaNote(c, attrs)
					continue
				}
			}
			aliases = append(aliases, c)
		}
		for k := openIdx; k <= closeIdx; k++ {
			tokens[k].Removed = true
		}
		i = closeIdx
	}
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}

func inferTailAliasesFromParens(tokens []model.Token, attrs *[]model.Attribute) []string {
	if attrs == nil {
		return nil
	}
	isPlatformOnlyParen := func(inner string) bool {
		inner = strings.TrimSpace(inner)
		if inner == "" {
			return false
		}
		for _, a := range *attrs {
			if a.Type != model.AttrPlatform {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(a.Normalized), inner) {
				return true
			}
		}
		return false
	}
	techStart, ok := firstTechTokenIndexFromAttrs(*attrs)
	if !ok {
		return nil
	}
	aliases := make([]string, 0, 8)
	for i := techStart; i < len(tokens); i++ {
		if tokens[i].Text != "(" && tokens[i].Text != "（" {
			continue
		}
		openRemoved := tokens[i].Removed
		_, closeIdx, inner, ok := parseParenBlockAll(tokens, i, len(tokens))
		if !ok {
			continue
		}
		inner = strings.TrimSpace(inner)
		if inner == "" {
			i = closeIdx
			continue
		}
		if strings.Contains(inner, "※") {
			i = closeIdx
			continue
		}
		if isPlatformOnlyParen(inner) {
			i = closeIdx
			continue
		}
		cands, isSearchHint, hintKeyword := splitAliasCandidatesFromParenContent(inner)
		if isLikelyMetaBracketText(inner) && !isSearchHint {
			i = closeIdx
			continue
		}
		if isSearchHint {
			hk := strings.TrimSpace(hintKeyword)
			if hk != "" {
				*attrs = append(*attrs, model.Attribute{Type: model.AttrNote, Normalized: hk, Confidence: 0.6, Source: "hint_keyword"})
			}
		}
		for _, c := range cands {
			c = cleanAliasToken(c)
			if isSearchHint {
				if c == "" {
					continue
				}
				if isLikelyMetaBracketText(c) {
					continue
				}
				if !hasCJK(c) && len([]rune(c)) < 3 {
					continue
				}
			} else {
				if !isValidAliasToken(c) {
					continue
				}
				if stripped := stripExplicitSeasonSuffix(c); stripped != "" {
					c = stripped
					if !isValidAliasToken(c) {
						continue
					}
				}
				if isStreamMetaText(c) {
					classifyAndEmitStreamMeta(c, attrs)
					continue
				}
				if reTailMetaToken.MatchString(c) {
					classifyAndEmitTailMetaNote(c, attrs)
					continue
				}
			}
			if len([]rune(c)) > 80 {
				continue
			}
			if openRemoved && !isSearchHint {
				continue
			}
			aliases = append(aliases, c)
		}
		i = closeIdx
	}
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}

func parseParenBlockAll(tokens []model.Token, openIdx int, limit int) (int, int, string, bool) {
	if openIdx < 0 || openIdx >= len(tokens) {
		return openIdx, openIdx, "", false
	}
	op := tokens[openIdx].Text
	if op != "(" && op != "（" {
		return openIdx, openIdx, "", false
	}
	close := ")"
	if op == "（" {
		close = "）"
	}
	end := -1
	depth := 0
	maxJ := limit
	if maxJ > len(tokens) {
		maxJ = len(tokens)
	}
	for j := openIdx; j < maxJ; j++ {
		if tokens[j].Text == op {
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
		return openIdx, openIdx, "", false
	}
	parts := make([]string, 0, end-openIdx)
	for k := openIdx + 1; k < end; k++ {
		t := strings.TrimSpace(tokens[k].Text)
		if t == "" {
			continue
		}
		parts = append(parts, t)
	}
	return openIdx, end, strings.TrimSpace(strings.Join(parts, " ")), true
}

func parseParenBlock(tokens []model.Token, openIdx int, limit int) (int, int, string, bool) {
	if openIdx < 0 || openIdx >= len(tokens) {
		return openIdx, openIdx, "", false
	}
	op := tokens[openIdx].Text
	if op != "(" && op != "（" {
		return openIdx, openIdx, "", false
	}
	close := ")"
	if op == "（" {
		close = "）"
	}
	end := -1
	depth := 0
	maxJ := limit
	if maxJ > len(tokens) {
		maxJ = len(tokens)
	}
	for j := openIdx; j < maxJ; j++ {
		if tokens[j].Text == op {
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
		return openIdx, openIdx, "", false
	}
	parts := make([]string, 0, end-openIdx)
	for k := openIdx + 1; k < end; k++ {
		if tokens[k].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[k].Text)
		if t == "" {
			continue
		}
		parts = append(parts, t)
	}
	return openIdx, end, strings.TrimSpace(strings.Join(parts, " ")), true
}

func splitAliasCandidatesFromParenContent(content string) ([]string, bool, string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, false, ""
	}
	searchHint := false
	hintKeyword := ""
	if m := reHintPrefix.FindStringSubmatch(content); len(m) == 3 {
		hintKeyword = strings.TrimSpace(m[1])
		content = strings.TrimSpace(m[2])
		searchHint = true
	}
	if content == "" {
		return nil, searchHint, hintKeyword
	}
	allowComma := searchHint || reStreamMetaFlag.MatchString(content)
	norm := strings.NewReplacer(
		"｜", "|",
		"／", "/",
		"；", ";",
		"，", ",",
		"、", ",",
	).Replace(content)
	parts := strings.FieldsFunc(norm, func(r rune) bool {
		if r == ',' {
			return allowComma
		}
		return r == '/' || r == '|' || r == ';' || r == '\\'
	})
	// For search-hint text like "检索用: Princession Orchestra": do NOT split by whitespace.
	// Only split when explicit delimiters like '/' '|' ';' '\\' ',' are present.
	if searchHint {
		// Search-hint lists (often slash-separated) are high-risk pollution; do not treat them as aliases.
		if strings.ContainsAny(norm, "/|;\\,") {
			return nil, searchHint, hintKeyword
		}
		cand := strings.TrimSpace(norm)
		if cand == "" {
			return nil, searchHint, hintKeyword
		}
		// Enforce a consistent minimal length for hint-derived aliases (after removing spaces),
		// so short tokens like "Q 娃" are dropped.
		compact := strings.ReplaceAll(cand, " ", "")
		if len([]rune(compact)) < 4 {
			return nil, searchHint, hintKeyword
		}
		return []string{cand}, searchHint, hintKeyword
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{content}, searchHint, hintKeyword
	}
	return out, searchHint, hintKeyword
}

func headAliasBracketPair(s string) (open string, close string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "(" {
		return "(", ")", true
	}
	if s == "（" {
		return "（", "）", true
	}
	if s == "[" {
		return "[", "]", true
	}
	if s == "【" {
		return "【", "】", true
	}
	return "", "", false
}

func inferTitleTokenSpan(tokens []model.Token, episodeIdx int, episodeRangeIdx int, seasonCutIdx int) (int, int) {
	start := titleStartAfterLeadingEmptyBracketBlocks(tokens)

	end := len(tokens)
	if episodeRangeIdx >= 0 {
		end = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < end {
		end = episodeIdx
	}
	if seasonCutIdx >= start && seasonCutIdx >= 0 && seasonCutIdx < end {
		end = seasonCutIdx
	}

	dotCount := 0
	for i := start; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		if strings.TrimSpace(tokens[i].Text) == "." {
			dotCount++
		}
	}
	dotMode := dotCount >= 4

	seenTitle := false
	for i := start; i < end; i++ {
		if !seenTitle {
			if tokens[i].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[i].Text)
			if t == "" {
				continue
			}
			if t == "[" || t == "]" || t == "【" || t == "】" || t == "(" || t == ")" || t == "（" || t == "）" {
				continue
			}
			if t == "-" || t == "|" || t == "｜" || t == "/" || t == "／" {
				continue
			}
			if isTitleNoiseToken(t) {
				continue
			}
			seenTitle = true
		} else {
			if tokens[i].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[i].Text)
			if dotMode {
				if looksLikeDotSeparatedMetaCutToken(t) {
					end = i
					for end > start {
						j := end - 1
						for j >= start {
							if tokens[j].Removed {
								j--
								continue
							}
							break
						}
						if j < start {
							break
						}
						pj := strings.TrimSpace(tokens[j].Text)
						if pj == "." || pj == "_" || pj == "-" {
							end = j
							continue
						}
						break
					}
					break
				}
			}
			if looksLikeTitleCutMetaToken(t) {
				end = i
				for j := i - 1; j >= start && j >= 0; j-- {
					if tokens[j].Removed {
						continue
					}
					pj := strings.TrimSpace(tokens[j].Text)
					if pj == "" {
						continue
					}
					if pj == ":" || pj == "：" || pj == ";" || pj == "；" {
						end = j
					}
					break
				}
				break
			}
		}
		if seenTitle {
			if tokens[i].Text == "[" || tokens[i].Text == "【" {
				end = i
				break
			}
		}
		if tokens[i].Text == "(" || tokens[i].Text == "（" {
			if looksLikeParenMetaOrTech(tokens, i, end) {
				if ni, ok := removeParenBlockTokens(tokens, i, end); ok {
					i = ni - 1
					continue
				}
			} else if seenTitle {
				openIdx, closeIdx, inner, ok := parseParenBlock(tokens, i, end)
				if ok {
					inner = strings.TrimSpace(inner)
					if inner != "" && !strings.Contains(inner, "※") {
						_, isSearchHint, _ := splitAliasCandidatesFromParenContent(inner)
						next := nextVisibleTokenText(tokens, closeIdx+1, end)
						if isSearchHint || next == "" || next == "[" || next == "|" || next == "｜" {
							end = openIdx
							break
						}
					}
				}
			}
		}
		if tokens[i].Text == "|" || tokens[i].Text == "｜" {
			end = i
			break
		}
	}

	first := -1
	last := -1
	for i := start; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if t == "-" || t == "|" || t == "｜" || t == "/" {
			continue
		}
		if dotMode {
			if t == "." {
				continue
			}
		}
		if t == "[" || t == "]" || t == "(" || t == ")" || t == "（" || t == "）" {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	if first < 0 || last < 0 {
		return -1, -1
	}
	return first, last + 1
}

var reDotSeparatedRes = regexp.MustCompile(`(?i)^\d{3,4}p$`)

func looksLikeDotSeparatedMetaCutToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	low = strings.TrimSpace(strings.Trim(low, "._-"))
	if low == "" {
		return false
	}
	if reDotSeparatedRes.MatchString(low) {
		return true
	}
	if low == "dvdiso" {
		return true
	}
	if low == "webrip" || low == "web-dl" || low == "webdl" || low == "bluray" || low == "bdrip" || low == "dvdrip" {
		return true
	}
	if low == "hdr" {
		return true
	}
	if low == "hevc" || low == "x265" || low == "h265" || low == "x264" || low == "h264" {
		return true
	}
	if low == "mpeg" || low == "xvid" {
		return true
	}
	if low == "vastfr" || low == "vostfr" || low == "multi" || low == "truefrench" {
		return true
	}
	return false
}

func skipLeadingBracketBlocks(tokens []model.Token) int {
	i := 0
	for i < len(tokens) && tokens[i].Text == "[" {
		end := -1
		depth := 0
		for j := i; j < len(tokens); j++ {
			if tokens[j].Text == "[" {
				depth++
			} else if tokens[j].Text == "]" {
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
			break
		}
		i = end + 1
	}
	return i
}

func titleStartAfterLeadingEmptyBracketBlocks(tokens []model.Token) int {
	i := 0
	for i < len(tokens) {
		for i < len(tokens) && (tokens[i].Removed || strings.TrimSpace(tokens[i].Text) == "") {
			i++
		}
		if i >= len(tokens) {
			return i
		}
		if looksLikeLeadingTitleMeta(tokens[i].Text) {
			i++
			continue
		}
		if tokens[i].Text == "★" || tokens[i].Text == "☆" {
			if ni, ok := consumeStarWrappedLeadingMeta(tokens, i); ok {
				i = ni
				continue
			}
		}
		open := strings.TrimSpace(tokens[i].Text)
		if open != "[" && open != "【" {
			return i
		}
		close := "]"
		if open == "【" {
			close = "】"
		}
		end := -1
		depth := 0
		for j := i; j < len(tokens); j++ {
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
			return i
		}
		innerHasVisible := false
		innerParts := make([]string, 0, 8)
		for k := i + 1; k < end; k++ {
			if tokens[k].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[k].Text)
			if t == "" {
				continue
			}
			if t == "[" || t == "]" || t == "【" || t == "】" {
				continue
			}
			innerHasVisible = true
			innerParts = append(innerParts, t)
			break
		}
		if innerHasVisible {
			blockText := strings.TrimSpace(strings.Join(innerParts, " "))
			if isLikelyMetaBracketText(blockText) || looksLikeLeadingTitleMeta(blockText) {
				i = end + 1
				continue
			}
			for k := i + 1; k < end; k++ {
				if tokens[k].Removed {
					continue
				}
				t := strings.TrimSpace(tokens[k].Text)
				if t == "" || t == "[" || t == "]" || t == "【" || t == "】" {
					continue
				}
				return k
			}
			return i
		}
		i = end + 1
	}
	return i
}

var reMonthNewShow = regexp.MustCompile(`^\d{1,2}月新番$`)

func looksLikeLeadingTitleMeta(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	s = strings.TrimSpace(strings.Trim(s, "★☆"))
	if s == "整理搬运" || s == "整理搬運" {
		return true
	}
	if reMonthNewShow.MatchString(s) {
		return true
	}
	if strings.Contains(s, "新番") {
		return true
	}
	if strings.Contains(s, "代理商") {
		return true
	}
	if strings.Contains(s, "粤语") || strings.Contains(s, "粵語") {
		return true
	}
	if strings.Contains(s, "招募") {
		return true
	}
	if strings.Contains(s, "检索") {
		return true
	}
	return false
}

func looksLikeTitleCutMetaToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	low = strings.TrimSpace(strings.TrimLeft(low, ":："))
	if strings.Contains(low, "tv动画") {
		return true
	}
	// Common Chinese meta sections in pack titles.
	if strings.Contains(s, "OVA") || strings.Contains(s, "ova") {
		if strings.Contains(s, "篇") {
			return true
		}
	}
	if strings.Contains(s, "剧场") || strings.Contains(s, "劇場") {
		return true
	}
	if s == "漫画" || s == "漫畫" || s == "音乐" || s == "音樂" {
		return true
	}
	if strings.Contains(s, "音轨") || strings.Contains(s, "音軌") {
		return true
	}
	if strings.Contains(s, "字幕") {
		return true
	}
	if strings.Contains(s, "合集") || strings.Contains(s, "全集") {
		return true
	}
	if strings.Contains(s, "整理时间") || strings.Contains(s, "整理時間") {
		return true
	}
	return false
}

func isShortLatinTitleCandidate(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if hasCJK(s) {
		return false
	}
	if !hasLatin(s) {
		return false
	}
	if strings.ContainsAny(s, "!?,") {
		return false
	}
	if len([]rune(s)) > 28 {
		return false
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "webrip") || strings.Contains(low, "web-dl") || strings.Contains(low, "bluray") {
		return false
	}
	return true
}

func (p *Parser) applyLexicon(tokens []model.Token, out *[]model.Attribute) {
	lower := make([]string, 0, len(tokens))
	for _, t := range tokens {
		lower = append(lower, strings.ToLower(t.Text))
	}

	brDepth, parDepth := computeBracketParenDepth(tokens)

	for _, e := range p.cfg.Lexicon.Entries {
		if len(e.AnyOf) > 0 {
			for _, clause := range e.AnyOf {
				scope := strings.TrimSpace(clause.Scope)
				if scope == "" {
					scope = strings.TrimSpace(e.Scope)
				}
				applyLexiconClause(tokens, lower, brDepth, parDepth, scope, clause.Tokens, clause.Emit, clause.Remove, clause.Confidence, out)
			}
			continue
		}
		applyLexiconClause(tokens, lower, brDepth, parDepth, strings.TrimSpace(e.Scope), e.Tokens, e.Emit, e.Remove, e.Confidence, out)
	}
}

func applyLexiconClause(tokens []model.Token, lower []string, brDepth []int, parDepth []int, scope string, pat []string, emits []config.LexiconEmit, remove bool, conf float64, out *[]model.Attribute) {
	if len(pat) == 0 {
		return
	}

	scope = strings.ToLower(strings.TrimSpace(scope))

	pl := make([]string, 0, len(pat))
	for _, t := range pat {
		pl = append(pl, strings.ToLower(t))
	}

	for i := 0; i+len(pl) <= len(lower); i++ {
		if !lexiconScopeOK(brDepth, parDepth, scope, i, i+len(pl)) {
			continue
		}
		// If any token in this window has been removed by earlier deterministic logic,
		// skip matching to avoid emitting attributes from consumed tokens.
		windowOK := true
		for k := i; k < i+len(pl); k++ {
			if k >= 0 && k < len(tokens) && tokens[k].Removed {
				windowOK = false
				break
			}
		}
		if !windowOK {
			continue
		}
		ok := true
		for j := 0; j < len(pl); j++ {
			if lower[i+j] != pl[j] {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}

		start := i
		end := i + len(pl)

		for _, em := range emits {
			idx := em.TokenIndex
			if idx < 0 {
				idx = len(pl) - 1
			}
			abs := start + idx
			if abs >= start && abs < end {
				*out = append(*out, model.Attribute{Type: model.AttrType(em.Type), Normalized: em.Normalized, TokenStart: abs, TokenEnd: abs + 1, Confidence: conf, Source: "lexicon"})
			}
		}

		if remove {
			for k := start; k < end; k++ {
				tokens[k].Removed = true
			}
		}
	}
}

func computeBracketParenDepth(tokens []model.Token) ([]int, []int) {
	br := make([]int, len(tokens))
	pr := make([]int, len(tokens))
	bd := 0
	pd := 0
	for i := 0; i < len(tokens); i++ {
		t := tokens[i].Text
		br[i] = bd
		pr[i] = pd
		if t == "[" || t == "【" {
			bd++
			continue
		}
		if t == "]" || t == "】" {
			if bd > 0 {
				bd--
			}
			continue
		}
		if t == "(" || t == "（" {
			pd++
			continue
		}
		if t == ")" || t == "）" {
			if pd > 0 {
				pd--
			}
			continue
		}
	}
	return br, pr
}

func lexiconScopeOK(brDepth []int, parDepth []int, scope string, start int, end int) bool {
	if scope == "" || scope == "any" || scope == "anywhere" {
		return true
	}
	if start < 0 {
		start = 0
	}
	if end > len(brDepth) {
		end = len(brDepth)
	}
	if start >= end {
		return false
	}

	switch scope {
	case "bracket", "brackets", "[]":
		for i := start; i < end; i++ {
			if brDepth[i] <= 0 {
				return false
			}
		}
		return true
	case "paren", "parens", "()":
		for i := start; i < end; i++ {
			if parDepth[i] <= 0 {
				return false
			}
		}
		return true
	default:
		return true
	}
}

func extractHeadBracketGroup(tokens []model.Token) (string, [2]int) {
	if len(tokens) < 3 {
		return "", [2]int{0, 0}
	}
	open0, close0, ok0 := headGroupBracketPair(tokens[0].Text)
	if !ok0 {
		return "", [2]int{0, 0}
	}

	blocks := make([][3]int, 0, 4)
	i := 0
	_ = open0
	_ = close0
	for i < len(tokens) {
		open, close, ok := headGroupBracketPair(tokens[i].Text)
		if !ok {
			break
		}
		end := -1
		depth := 0
		for j := i; j < len(tokens); j++ {
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
			break
		}
		blocks = append(blocks, [3]int{i, end + 1, end})
		i = end + 1
	}
	if len(blocks) == 0 {
		return "", [2]int{0, 0}
	}

	// Be conservative: only treat the *first* leading bracket block as a head group.
	// Later blocks are often title / episode / language / tech sections; misclassifying them
	// as group will remove the actual title and cause parser_title_empty.
	b := blocks[0]
	parts := make([]string, 0, b[1]-b[0])
	for k := b[0] + 1; k < b[1]-1; k++ {
		if tokens[k].Removed {
			continue
		}
		if strings.TrimSpace(tokens[k].Text) == "" {
			continue
		}
		parts = append(parts, tokens[k].Text)
	}
	g := strings.TrimSpace(strings.Join(parts, " "))
	if g == "" {
		return "", [2]int{0, 0}
	}
	if g == "整理搬运" || g == "整理搬運" {
		return "", [2]int{0, 0}
	}
	if !isGroupCandidateText(g) {
		return "", [2]int{0, 0}
	}
	if isLikelyMetaBracketText(g) || isEpisodeLikeBracketText(g) {
		return "", [2]int{0, 0}
	}
	return g, [2]int{b[0], b[1]}
}

func headGroupBracketPair(s string) (open string, close string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "[" {
		return "[", "]", true
	}
	if s == "【" {
		return "【", "】", true
	}
	return "", "", false
}

func isGroupCandidateText(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if len([]rune(s)) > 32 {
		return false
	}
	allDigits := true
	for _, r := range s {
		if !(('0' <= r && r <= '9') || r == '-' || r == '~' || r == ' ' || r == '+') {
			allDigits = false
			break
		}
	}
	if allDigits {
		return false
	}
	for _, r := range s {
		if ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') || ('0' <= r && r <= '9') {
			continue
		}
		if r == ' ' || r == '_' || r == '-' || r == '.' || r == '&' || r == '＆' {
			continue
		}
		if r > 127 {
			continue
		}
		return false
	}
	return true
}

func isLikelyMetaBracketText(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	up := strings.ToUpper(strings.TrimSpace(s))
	trim := strings.TrimSpace(s)
	if trim == "无字幕" || trim == "無字幕" {
		return true
	}
	if strings.Contains(trim, "超分") {
		return true
	}
	if strings.Contains(trim, "合集") || strings.Contains(trim, "合輯") {
		// E.g. "個人製作合集".
		return true
	}
	if strings.Contains(trim, "新番") {
		return true
	}
	if trim == "转" || trim == "轉" || trim == "转载" || trim == "轉載" || trim == "转发" || trim == "轉發" {
		return true
	}
	if low == "weekly" || low == "batch" || low == "reseed" || low == "complete" {
		return true
	}
	if strings.HasPrefix(low, "final season") {
		return true
	}
	if low == "1080p" || low == "2160p" || low == "720p" {
		return true
	}
	if strings.Contains(low, "1080p") || strings.Contains(low, "2160p") || strings.Contains(low, "720p") {
		return true
	}
	if up == "MP4" || up == "MKV" || up == "AVI" {
		return true
	}
	if up == "GB" || up == "BIG5" || up == "CHS" || up == "CHT" || up == "JP" {
		return true
	}
	if up == "BDMV" {
		return true
	}
	// Common combined language tags like "CHS&JPN" / "CHT&JPN" / "CHS&CHT&JPN".
	if regexp.MustCompile(`(?i)^(?:CHS|CHT|JPN|JP|ENG)(?:\s*(?:&|＆)\s*(?:CHS|CHT|JPN|JP|ENG))+$`).MatchString(trim) {
		return true
	}
	if strings.Contains(s, "简") || strings.Contains(s, "繁") {
		if strings.Contains(s, "简中") || strings.Contains(s, "繁中") || strings.Contains(s, "简体") || strings.Contains(s, "繁体") || strings.Contains(s, "内嵌") || strings.Contains(s, "内封") || strings.Contains(s, "双语") {
			return true
		}
	}
	if strings.Contains(s, "招募") || strings.Contains(s, "时轴") {
		return true
	}
	if strings.Contains(s, "附字幕") {
		return true
	}
	if strings.Contains(s, "分角色") {
		return true
	}
	return false
}

func isEpisodeLikeBracketText(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if regexp.MustCompile(`(?i)^第?\d{1,4}(?:\s*-\s*\d{1,4})?(?:集|话|話|回|章)(?:\s|$)`).MatchString(ss) {
		return true
	}
	if regexp.MustCompile(`(?i)^0?\d{1,4}\s*(?:&|＆)\s*(?:sp|ova|oad|ona|ex|pv|op|ed)(?:\s|$)`).MatchString(ss) {
		return true
	}
	if regexp.MustCompile(`^0?\d{1,4}[_\s-]+.+$`).MatchString(ss) {
		return true
	}
	return false
}

func scoreGroupCandidate(s string) int {
	low := strings.ToLower(strings.TrimSpace(s))
	score := 0
	if strings.Contains(s, "字幕") || strings.Contains(s, "漢化") || strings.Contains(s, "汉化") {
		score += 200
	}
	if strings.Contains(low, "subs") || strings.Contains(low, "sub") || strings.Contains(low, "raw") {
		score += 120
	}
	if strings.Contains(s, "&") || strings.Contains(s, "＆") {
		score += 40
	}
	if len([]rune(s)) <= 16 {
		score += 10
	}
	return score
}

var sxxExxPattern = regexp.MustCompile(`(?i)^s(\d{1,2})e(\d{1,4})(?:v\d+)?(?:[^0-9a-z].*)?$`)
var seasonOnlyPattern = regexp.MustCompile(`(?i)^s(\d{1,2})$`)
var seasonEmbeddedPattern = regexp.MustCompile(`(?i)^s(\d{1,2})[^0-9a-z].*$`)
var seasonWordPattern = regexp.MustCompile(`(?i)^season$`)
var seasonOrdinalPattern = regexp.MustCompile(`(?i)^(\d{1,2})(?:st|nd|rd|th)$`)
var seasonCJKPattern = regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})季$`)

func detectSxxExx(tokens []model.Token) (season string, episode string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		m := sxxExxPattern.FindStringSubmatch(tokens[i].Text)
		if len(m) == 3 {
			return strings.TrimLeft(m[1], "0"), strings.TrimLeft(m[2], "0"), i
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

		// 第三期 / 第3期 / 3期
		if m := regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})期$`).FindStringSubmatch(text); len(m) == 2 {
			if v := parseSmallSeasonNumberToken(m[1]); v != "" {
				return v, i
			}
		}
		if m := regexp.MustCompile(`^(\d{1,2})期$`).FindStringSubmatch(text); len(m) == 2 {
			return strings.TrimLeft(m[1], "0"), i
		}

		// Embedded: 第三季_xxx / 第3季-xxx
		if m := regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})季.+$`).FindStringSubmatch(text); len(m) == 2 {
			if v := parseSmallSeasonNumberToken(m[1]); v != "" {
				return v, i
			}
		}
		if m := regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})期.+$`).FindStringSubmatch(text); len(m) == 2 {
			if v := parseSmallSeasonNumberToken(m[1]); v != "" {
				return v, i
			}
		}

		// 第三季 / 第3季 / 三季
		if m := seasonCJKPattern.FindStringSubmatch(text); len(m) == 2 {
			if v := parseSmallSeasonNumberToken(m[1]); v != "" {
				return v, i
			}
		}

		// Season 3
		if seasonWordPattern.MatchString(text) {
			if i+1 < len(tokens) {
				next := strings.TrimSpace(tokens[i+1].Text)
				if v := parseSmallSeasonNumberToken(next); v != "" {
					return v, i
				}
			}
		}

		// 2nd Season
		if m := seasonOrdinalPattern.FindStringSubmatch(text); len(m) == 2 {
			if i+1 < len(tokens) && seasonWordPattern.MatchString(strings.TrimSpace(tokens[i+1].Text)) {
				return strings.TrimLeft(m[1], "0"), i
			}
		}
	}
	return "", -1
}

var episodeRangePattern = regexp.MustCompile(`(?i)^(\d{1,4})[-~](\d{1,4})$`)
var episodeRangeEmbeddedPattern = regexp.MustCompile(`(?i)(\d{1,4})[-~](\d{1,4})`)
var epPrefixedDigitsPattern = regexp.MustCompile(`(?i)^ep(\d{1,4})$`)
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
		if tokens[j].Removed {
			continue
		}
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
		return true
	}
	return false
}

func detectEpisodeRange(tokens []model.Token) (string, int) {
	// Pattern: "01 a 12" (Spanish/Portuguese range) in plain text.
	// We normalize it into a single token "01-12" to avoid leaving "a 12" as tail noise.
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
		if !strings.EqualFold(mid, "a") {
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
		if tokens[i].Removed {
			continue
		}
		inBracket := isInBracket(tokens, i)
		inParen := isInParen(tokens, i)
		if inBracket {
			if txt := bracketBlockInnerText(tokens, i); txt != "" {
				if isLikelyMetaBracketText(txt) && !isEpisodeLikeBracketText(txt) {
					continue
				}
				if m := regexp.MustCompile(`(?i)^第?(\d{1,4})\s*[-~]\s*(\d{1,4})(?:集|话|話|回|章)(?:\s|$)`).FindStringSubmatch(strings.TrimSpace(txt)); len(m) == 3 {
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
			if episodeOfTotalPattern.MatchString(strings.TrimSpace(tokens[i].Text)) && strings.TrimSpace(tokens[i+1].Text) == "-" {
				rawR := strings.TrimSpace(tokens[i+2].Text)
				if episodeOfTotalPattern.MatchString(rawR) {
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

func normalizeGroupText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	s = groupHyphenSpacePattern.ReplaceAllString(s, `$1-$2`)
	// Normalize '&' / '＆' without surrounding spaces.
	s = groupAmpSpacePattern.ReplaceAllString(s, "&")
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
var episodeVersionSuffixPattern = regexp.MustCompile(`(?i)^(?:\d{1,4}(?:\.\d{1,2})?)[vV]\d{1,2}$`)
var chineseEpisodeSuffixPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})(?:话|集)$`)
var embeddedChineseEpisodeSuffixPattern = regexp.MustCompile(`第(\d{1,4})(?:集|话|話)$`)
var yearPattern = regexp.MustCompile(`^(?:19|20)\d{2}$`)
var starEpisodeEmbeddedPattern = regexp.MustCompile(`(?:★|☆)(\d{1,4})(?:★|☆)`)
var dateYMDPattern = regexp.MustCompile(`(?i)(?:\b(?:19|20)\d{2}[./-](?:0?[1-9]|1[0-2])[./-](?:0?[1-9]|[12]\d|3[01])\b)`)
var spKindEpisodePattern = regexp.MustCompile(`(?i)^(?:sp|ova|oad|ona|ex)0*(\d{1,3})$`)
var episodeWithVersionPattern = regexp.MustCompile(`(?i)^(?:(?:e|ep|第))?(\d{1,4})(?:\.(\d{1,2}))?[vV](\d{1,2})$`)
var standaloneVersionPattern = regexp.MustCompile(`(?i)^v(\d{1,2})$`)
var regionHKMOTWEmbeddedPattern = regexp.MustCompile(`(?i)(?:仅限港澳台地区|僅限港澳台地區)`)
var regionHKMOTWRemovalPattern = regexp.MustCompile(`(?i)[(（]?\s*(?:仅限港澳台地区|僅限港澳台地區)\s*[)）]?`)

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
	if dateYMDPattern.MatchString(low) {
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

func detectEpisodeWithVersion(tokens []model.Token) (string, int, string, int) {
	bestIdx := -1
	best := ""
	bestScore := -1
	bestVer := ""

	// Pattern: "- Episode 52" (common in English TV rip naming).
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].Removed || tokens[i+1].Removed {
			continue
		}
		kw := strings.TrimSpace(tokens[i].Text)
		if !strings.EqualFold(kw, "episode") {
			continue
		}
		d := strings.TrimSpace(tokens[i+1].Text)
		if !episodeOfTotalPattern.MatchString(d) {
			continue
		}
		// Guard: require a dash before the keyword to reduce false positives.
		if i-1 >= 0 {
			prev := strings.TrimSpace(tokens[i-1].Text)
			if prev != "-" {
				continue
			}
		} else {
			continue
		}
		n := strings.TrimLeft(d, "0")
		if n == "" {
			n = "0"
		}
		if n == "0" {
			continue
		}
		// Remove the keyword token (and the preceding dash) to avoid polluting title.
		tokens[i].Removed = true
		if i-1 >= 0 {
			tokens[i-1].Removed = true
		}
		// Let the digit token remain as the episode anchor.
		best = n
		bestIdx = i + 1
		bestScore = 100
		break
	}

	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if isTitlePrefixedDigitRangeInBracket(tokens, i) {
			continue
		}
		if isRightSideOfPlainDigitRangeInTitle(tokens, i) {
			continue
		}
		inBracket := isInBracket(tokens, i)
		if inBracket {
			if txt := bracketBlockInnerText(tokens, i); txt != "" {
				if isLikelyMetaBracketText(txt) && !isEpisodeLikeBracketText(txt) {
					continue
				}
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
			if n == "0" {
				continue
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
		if inBracket {
			n := strings.TrimSpace(tokens[i].Text)
			if episodeOfTotalPattern.MatchString(n) {
				nn := strings.TrimLeft(n, "0")
				if nn == "" {
					nn = "0"
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
				if regexp.MustCompile(`^\d{1,4}$`).MatchString(n) && (suf == "集" || suf == "话" || suf == "話") {
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
			if prefix == "" {
				ok := false
				if inBracket {
					ok = true
				}
				if i-1 >= 0 {
					sep := strings.TrimSpace(tokens[i-1].Text)
					if sep == "-" {
						ok = true
						// Guard: reject cases like "Gintama-3 nen" where the number is followed by a word.
						if !inBracket {
							if i+1 < len(tokens) {
								nextWord := strings.TrimSpace(tokens[i+1].Text)
								if looksLikeLowercaseLatinWordToken(nextWord) {
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
									if i+1 < len(tokens) {
										next := strings.TrimSpace(tokens[i+1].Text)
										if next == "[" || next == "【" || next == "(" || next == "（" {
										} else {
											ok = false
										}
									} else {
										ok = false
									}
								}
							}
						}
					} else if sep == "★" || sep == "☆" {
						ok = true
					}
				}
				if !ok {
					if i+1 < len(tokens) {
						next := strings.TrimSpace(tokens[i+1].Text)
						if next == "[" || next == "【" || next == "(" || next == "（" {
							if n != "0" {
								if n != "720" && n != "1080" && n != "2160" && n != "480" && n != "540" {
									ok = true
								}
							}
						}
					}
					if !ok {
						continue
					}
				}
				if n == "0" {
					continue
				}
			}
			score := len(m[2])
			if prefix != "" {
				score += 100
			}
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = n
				bestVer = ""
			}
		}
	}
	if bestIdx >= 0 {
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

func extractVersionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) string {
	if out == nil || len(tokens) == 0 {
		return ""
	}
	best := ""
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		// Standalone [v2] / (v2) style. Tokenizer usually makes it a single token "v2".
		text := strings.TrimSpace(tokens[i].Text)
		m := standaloneVersionPattern.FindStringSubmatch(text)
		if len(m) != 2 {
			continue
		}
		v := strings.TrimLeft(m[1], "0")
		if v == "" {
			v = m[1]
		}
		norm := "v" + v
		*out = append(*out, model.Attribute{Type: model.AttrVersion, Normalized: norm, TokenStart: i, TokenEnd: i + 1, Confidence: 0.7, Source: "version_token"})
		// Only remove when it is clearly meta (most common inside brackets/parens).
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			tokens[i].Removed = true
		}
		if best == "" {
			best = norm
		}
	}
	return best
}

// Backward compatible wrapper.
func detectEpisode(tokens []model.Token) (string, int) {
	ep, idx, _, _ := detectEpisodeWithVersion(tokens)
	return ep, idx
}

var tailGroupPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{2,20}$`)
var tailGroupHyphenPrefixPattern = regexp.MustCompile(`(?i)^(?:x?264|x?265|h?264|h?265|\d\.\d|pcm|flac|aac|ac3|eac3|ddp|dts|opus|truehd|atmos)(?:\d\.\d)?$`)
var tailGroupTechTokenPattern = regexp.MustCompile(`(?i)^(?:dts(?:hd|hdma)?|hdma|truehd|eac3|ac3|flac|opus|pcm|ma\d{1,2}p|ddp\d{0,2})$`)

func isTailGroupToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if !tailGroupPattern.MatchString(s) {
		return false
	}
	up := strings.ToUpper(s)
	if strings.Contains(up, "RAWS") || strings.Contains(up, "SUBS") {
		return false
	}
	if up == "TIME" || up == "RAWS" || up == "SUBS" || up == "KUN" || up == "SAN" || up == "CHAN" {
		return false
	}
	if up == "DL" || up == "WEB" || up == "WEBRIP" || up == "WEB-DL" || up == "BLURAY" || up == "BDRIP" || up == "DVDRIP" {
		return false
	}
	if up == "H264" || up == "H265" || up == "X264" || up == "X265" || up == "HEVC" || up == "AVC" || up == "AAC" {
		return false
	}
	if tailGroupTechTokenPattern.MatchString(up) {
		return false
	}
	return true
}

func detectTailGroup(tokens []model.Token) (string, int) {
	lastTail := -1
	for j := len(tokens) - 1; j >= 0; j-- {
		if tokens[j].Removed {
			continue
		}
		if isInBracket(tokens, j) || isInParen(tokens, j) {
			continue
		}
		t := strings.TrimSpace(tokens[j].Text)
		if t == "" {
			continue
		}
		low := strings.ToLower(t)
		if t == "(" || t == ")" || t == "-" || t == "." || low == "raws" || low == "subs" {
			continue
		}
		lastTail = j
		break
	}

	for i := len(tokens) - 1; i >= 0; i-- {
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
		cand := ""
		if isTailGroupToken(text) {
			if i-1 >= 0 {
				sep := strings.TrimSpace(tokens[i-1].Text)
				if sep == "-" {
					// For single-token tail group, require at least one uppercase letter to avoid picking words like 'kun'.
					if !hasUpperASCII(text) {
						continue
					}
					// Must be near tail; otherwise '-' is more likely a title separator.
					if lastTail >= 0 && i < lastTail {
						continue
					}
					// Reconstruct patterns like "- Tsundere - Raws" -> "Tsundere-Raws"
					if i+2 < len(tokens) {
						sep2 := strings.TrimSpace(tokens[i+1].Text)
						next := strings.TrimSpace(tokens[i+2].Text)
						if sep2 == "-" && (strings.EqualFold(next, "Raws") || strings.EqualFold(next, "Subs")) {
							if isTailGroupToken(next) || strings.EqualFold(next, "Raws") || strings.EqualFold(next, "Subs") {
								return text + "-" + next, i
							}
						}
					}
					// Reconstruct patterns like Tsundere - Raws (tokenized split)
					if (strings.EqualFold(text, "Raws") || strings.EqualFold(text, "Subs")) && i-2 >= 0 {
						p := strings.TrimSpace(tokens[i-2].Text)
						pl := strings.ToLower(p)
						if isTailGroupToken(p) && pl != "part" {
							return p + "-" + text, i
						}
					}
					cand = text
				}
			}
		} else if strings.Contains(text, "-") {
			parts := strings.Split(text, "-")
			if len(parts) >= 2 {
				prefix := strings.TrimSpace(parts[len(parts)-2])
				last := strings.TrimSpace(parts[len(parts)-1])
				if tailGroupHyphenPrefixPattern.MatchString(prefix) && isTailGroupToken(last) {
					cand = last
				}
			}
		}
		if cand == "" {
			continue
		}
		return cand, i
	}
	return "", -1
}

func isInBracket(tokens []model.Token, idx int) bool {
	depth := 0
	for i := 0; i < idx; i++ {
		if tokens[i].Text == "[" || tokens[i].Text == "【" {
			depth++
		} else if tokens[i].Text == "]" || tokens[i].Text == "】" {
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0
}

func extractTitle(tokens []model.Token, episodeIdx int, episodeRangeIdx int, seasonCutIdx int) string {
	start := titleStartAfterLeadingEmptyBracketBlocks(tokens)

	end := len(tokens)
	if start < 0 {
		start = 0
	}
	if start > end {
		start = end
	}
	if episodeRangeIdx >= 0 {
		end = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < end {
		// If episode is embedded at the end of a title token like "...第08集",
		// keep the token in title extraction and strip the suffix later.
		if !embeddedChineseEpisodeSuffixPattern.MatchString(strings.TrimSpace(tokens[episodeIdx].Text)) {
			end = episodeIdx
		}
	}
	if seasonCutIdx >= start && seasonCutIdx >= 0 && seasonCutIdx < end {
		end = seasonCutIdx
	}

	dotCount := 0
	for i := start; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		if strings.TrimSpace(tokens[i].Text) == "." {
			dotCount++
		}
	}
	dotMode := dotCount >= 4

	seenTitle := false
	for i := start; i < end; i++ {
		if !seenTitle {
			if tokens[i].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[i].Text)
			if t == "" {
				continue
			}
			if t == "[" || t == "]" || t == "(" || t == ")" || t == "（" || t == "）" {
				continue
			}
			if t == "-" || t == "|" || t == "｜" || t == "/" || t == "／" {
				continue
			}
			if isTitleNoiseToken(t) {
				continue
			}
			seenTitle = true
		} else {
			// After we've started the title, aggressively stop when meta/pack sections begin,
			// so canonical title and derived aliases do not include them.
			if tokens[i].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[i].Text)
			if dotMode {
				if looksLikeDotSeparatedMetaCutToken(t) {
					end = i
					for end > start {
						j := end - 1
						for j >= start {
							if tokens[j].Removed {
								j--
								continue
							}
							break
						}
						if j < start {
							break
						}
						pj := strings.TrimSpace(tokens[j].Text)
						if pj == "." || pj == "_" || pj == "-" {
							end = j
							continue
						}
						break
					}
					break
				}
			}
			if looksLikeTitleCutMetaToken(t) {
				end = i
				for j := i - 1; j >= start && j >= 0; j-- {
					if tokens[j].Removed {
						continue
					}
					pj := strings.TrimSpace(tokens[j].Text)
					if pj == "" {
						continue
					}
					if pj == ":" || pj == "：" {
						end = j
					}
					break
				}
				break
			}
		}
		if tokens[i].Text == "[" {
			end = i
			break
		}
		if tokens[i].Text == "(" || tokens[i].Text == "（" {
			if looksLikeParenMetaOrTech(tokens, i, end) {
				if ni, ok := removeParenBlockTokens(tokens, i, end); ok {
					i = ni - 1
					continue
				}
			} else if seenTitle {
				openIdx, closeIdx, inner, ok := parseParenBlock(tokens, i, end)
				if ok {
					inner = strings.TrimSpace(inner)
					if inner != "" && !strings.Contains(inner, "※") {
						_, isSearchHint, _ := splitAliasCandidatesFromParenContent(inner)
						next := nextVisibleTokenText(tokens, closeIdx+1, end)
						if isSearchHint || next == "" || next == "[" || next == "|" || next == "｜" {
							end = openIdx
							break
						}
					}
				}
			}
		}
		if tokens[i].Text == "|" || tokens[i].Text == "｜" {
			end = i
			break
		}
	}
	if end < 0 {
		end = 0
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	if start >= end {
		return ""
	}

	parts := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		if tokens[i].Removed {
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
		if i == episodeIdx {
			// Strip embedded Chinese episode suffix like "第08集" from title tokens.
			if embeddedChineseEpisodeSuffixPattern.MatchString(t) {
				stripped := embeddedChineseEpisodeSuffixPattern.ReplaceAllString(t, "")
				stripped = strings.TrimSpace(stripped)
				if stripped == "" {
					continue
				}
				t = stripped
			}
		}
		if isTitleNoiseToken(t) {
			continue
		}
		if t == "|" || t == "｜" {
			continue
		}
		if t == "." {
			if dotMode {
				continue
			}
			prev := lastVisibleTokenText(tokens, start, i)
			next := nextVisibleTokenText(tokens, i+1, end)
			if isMostlyLatinToken(prev) && isMostlyLatinToken(next) {
				parts = append(parts, ".")
				continue
			}
			continue
		}
		if len([]rune(t)) == 1 {
			r := []rune(t)[0]
			if ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') {
				// Merge patterns like "离开 A 级队伍..." -> "离开A级队伍...".
				if i+1 < end && i+1 < len(tokens) && !tokens[i+1].Removed {
					nextRaw := strings.TrimSpace(tokens[i+1].Text)
					if strings.HasPrefix(nextRaw, "级") {
						prevPart := ""
						if len(parts) > 0 {
							prevPart = parts[len(parts)-1]
						}
						if prevPart != "" && hasCJK(prevPart) {
							parts[len(parts)-1] = prevPart + t + nextRaw
							i++
							continue
						}
						parts = append(parts, t+nextRaw)
						i++
						continue
					}
				}
			}
		}
		if t == "-" {
			prev := lastVisibleTokenText(tokens, start, i)
			next := nextVisibleTokenText(tokens, i+1, end)
			if isMostlyLatinToken(prev) && isMostlyLatinToken(next) {
				parts = append(parts, "-")
				continue
			}
			if episodeOfTotalPattern.MatchString(strings.TrimSpace(prev)) && episodeOfTotalPattern.MatchString(strings.TrimSpace(next)) {
				parts = append(parts, "-")
				continue
			}
			continue
		}
		if t == "/" || t == "／" {
			// Drop a trailing lone '/' when the following token is removed/meta.
			next := nextVisibleTokenText(tokens, i+1, end)
			if strings.TrimSpace(next) == "" {
				continue
			}
			parts = append(parts, "/")
			continue
		}
		parts = append(parts, t)
	}

	out := compactWideBracketSpaces(strings.TrimSpace(strings.Join(parts, " ")))
	out = normalizeParenSpacing(out)
	out = normalizeTitleMain(out)
	return out
}

func lastVisibleTokenText(tokens []model.Token, start int, idx int) string {
	for j := idx - 1; j >= start && j >= 0; j-- {
		if tokens[j].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[j].Text)
		if t == "" {
			continue
		}
		if t == "[" || t == "]" || t == "(" || t == ")" || t == "（" || t == "）" {
			continue
		}
		if t == "|" || t == "｜" || t == "/" || t == "／" {
			continue
		}
		return t
	}
	return ""
}

func nextVisibleTokenText(tokens []model.Token, idx int, end int) string {
	for i := idx; i < end && i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if t == "[" || t == "]" || t == "(" || t == ")" || t == "（" || t == "）" {
			continue
		}
		if t == "|" || t == "｜" || t == "/" || t == "／" {
			continue
		}
		return t
	}
	return ""
}

func isTitleNoiseToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Only treat as title noise when it's a standalone restriction note, not when glued to a valid title.
	// Examples to drop: "仅限港澳台地区" "（仅限港澳台地区）".
	low := strings.ToLower(s)
	_ = low
	s2 := strings.Trim(s, "()（）[]【】")
	s2 = strings.TrimSpace(s2)
	if strings.HasPrefix(s2, "仅限") {
		if strings.Contains(s2, "港澳") || strings.Contains(s2, "港澳台") || strings.Contains(s2, "港澳臺") || strings.Contains(s2, "台灣") || strings.Contains(s2, "臺灣") {
			return true
		}
	}
	return false
}

var reSpaceBeforeFullwidthParenOpen = regexp.MustCompile(`([。！？…、，：；])\s+（`)

func normalizeParenSpacing(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "( ", "(")
	s = strings.ReplaceAll(s, "（ ", "（")
	s = strings.ReplaceAll(s, " )", ")")
	s = strings.ReplaceAll(s, " ）", "）")
	s = reSpaceBeforeFullwidthParenOpen.ReplaceAllString(s, "$1（")
	return strings.TrimSpace(s)
}

func removeParenBlockTokens(tokens []model.Token, openIdx int, limit int) (int, bool) {
	if openIdx < 0 || openIdx >= len(tokens) {
		return openIdx, false
	}
	op := tokens[openIdx].Text
	if op != "(" && op != "（" {
		return openIdx, false
	}
	close := ")"
	if op == "（" {
		close = "）"
	}
	end := -1
	depth := 0
	maxJ := limit
	if maxJ > len(tokens) {
		maxJ = len(tokens)
	}
	for j := openIdx; j < maxJ; j++ {
		if tokens[j].Text == op {
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
		return openIdx, false
	}
	for k := openIdx; k <= end; k++ {
		tokens[k].Removed = true
	}
	return end + 1, true
}

func looksLikeParenMetaOrTech(tokens []model.Token, openIdx int, limit int) bool {
	if openIdx < 0 || openIdx >= len(tokens) {
		return false
	}
	op := tokens[openIdx].Text
	if op != "(" && op != "（" {
		return false
	}
	close := ")"
	if op == "（" {
		close = "）"
	}
	end := -1
	depth := 0
	maxJ := limit
	if maxJ > len(tokens) {
		maxJ = len(tokens)
	}
	for j := openIdx; j < maxJ; j++ {
		if tokens[j].Text == op {
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
		return false
	}
	parts := make([]string, 0, end-openIdx)
	for k := openIdx + 1; k < end; k++ {
		if tokens[k].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[k].Text)
		if t == "" {
			continue
		}
		parts = append(parts, t)
	}
	inner := strings.TrimSpace(strings.Join(parts, " "))
	if inner == "" {
		return false
	}
	if isTitleNoiseToken(inner) {
		return true
	}
	low := strings.ToLower(inner)
	if reMonthNewShow.MatchString(strings.TrimSpace(strings.Trim(inner, "★☆"))) {
		return true
	}
	if strings.Contains(low, "1080p") || strings.Contains(low, "2160p") || strings.Contains(low, "720p") {
		return true
	}
	if strings.Contains(low, "webrip") || strings.Contains(low, "web-dl") || strings.Contains(low, "webdl") || strings.Contains(low, "bluray") {
		return true
	}
	if strings.Contains(low, "hevc") || strings.Contains(low, "x265") || strings.Contains(low, "h265") || strings.Contains(low, "x264") || strings.Contains(low, "h264") {
		return true
	}
	if strings.Contains(low, "mkv") || strings.Contains(low, "mp4") {
		return true
	}
	if regexp.MustCompile(`\b(?:19|20)\d{2}\b`).FindStringIndex(inner) != nil {
		return true
	}
	if regexp.MustCompile(`(?i)\b(?:s\d{1,2}|season\s*\d{1,2}|cour\s*\d{1,2}|part\s*\d{1,2})\b`).FindStringIndex(inner) != nil {
		return true
	}
	return false
}

func normalizeTitleMain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Normalize common fullwidth punctuation for stable matching/aliases.
	s = strings.ReplaceAll(s, "：", ":")
	s = strings.ReplaceAll(s, "＆", "&")
	// Remove region restriction notes when they are embedded in title tokens.
	// Keep other parenthetical notes like "（※似乎可行？）".
	reRegion := regexp.MustCompile(`（\s*仅限港澳台地区\s*）|（\s*僅限港澳台地區\s*）|\(\s*仅限港澳台地区\s*\)|\(\s*僅限港澳台地區\s*\)`)
	s = reRegion.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	// Compact spaces around hyphen only for very short left parts (e.g. "X - Clamp" -> "X-Clamp").
	reSingleLetterHyphen := regexp.MustCompile(`(?i)\b([a-z0-9])\s*-\s*([a-z0-9]{2,})\b`)
	s = reSingleLetterHyphen.ReplaceAllString(s, "$1-$2")
	// Compact spaces around hyphen for common latin honorific suffixes (e.g. "Kyuuketsuki - chan" -> "Kyuuketsuki-chan").
	// Keep this conservative to avoid merging multi-part titles like "... - The Outcast - ...".
	reLatinSuffixHyphen := regexp.MustCompile(`\b([A-Za-z0-9]{2,})\s*-\s*([a-z]{2,4})\b`)
	s = reLatinSuffixHyphen.ReplaceAllString(s, "$1-$2")
	reLatinDot := regexp.MustCompile(`(?i)\b([a-z0-9]{2,})\s*\.\s*([a-z0-9]{2,})\b`)
	s = reLatinDot.ReplaceAllString(s, "$1.$2")
	reDigitHyphen := regexp.MustCompile(`\b(\d{1,4})\s*-\s*(\d{1,4})\b`)
	s = reDigitHyphen.ReplaceAllString(s, "$1-$2")
	s = strings.TrimSpace(s)
	s = normalizeSpacedUnderscore(s)
	if hasCJK(s) && !hasLatin(s) {
		s = regexp.MustCompile(`([【（「『《〈])\s+`).ReplaceAllString(s, "$1")
		s = regexp.MustCompile(`\s+([】）」』》〉])`).ReplaceAllString(s, "$1")
		s = regexp.MustCompile(`([】）」』》〉])\s+([\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}])`).ReplaceAllString(s, "$1$2")
		s = strings.TrimSpace(s)
		s = strings.TrimRight(s, "。")
		s = strings.TrimSpace(s)
	}
	s = stripFileExtSuffix(s)
	return s
}

func compactWideBracketSpaces(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	repl := strings.NewReplacer(
		" 【", "【",
		"【 ", "【",
		" 】", "】",
		"】 ", "】",
	)
	return strings.TrimSpace(repl.Replace(s))
}

var reSpacedUnderscore = regexp.MustCompile(`\s+_\s+`)
var reTrailingUnderscore = regexp.MustCompile(`\s+_\s*$`)

func normalizeSpacedUnderscore(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if !strings.Contains(s, "_") {
		return s
	}
	s = reSpacedUnderscore.ReplaceAllString(s, "_")
	s = reTrailingUnderscore.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func hasTechAttr(attrs []model.Attribute) bool {
	for _, a := range attrs {
		s := string(a.Type)
		if s == string(model.AttrResolution) || s == string(model.AttrVideoCodec) || s == string(model.AttrAudioCodec) || s == string(model.AttrAudioChannels) || s == string(model.AttrAudioObject) || s == string(model.AttrAudioLang) || s == string(model.AttrSourceType) || s == string(model.AttrContainer) || s == string(model.AttrPlatform) || s == string(model.AttrCRC) || s == string(model.AttrTech) || s == string(model.AttrAudioFlag) || s == string(model.AttrSubFlag) || s == string(model.AttrSubLang) || s == string(model.AttrSubFormat) || s == string(model.AttrSubLangGroup) {
			return true
		}
	}
	return false
}

func firstTechTokenIndexFromAttrs(attrs []model.Attribute) (int, bool) {
	minIdx := 1 << 30
	for _, a := range attrs {
		s := string(a.Type)
		if s != string(model.AttrResolution) && s != string(model.AttrVideoCodec) && s != string(model.AttrAudioCodec) && s != string(model.AttrAudioChannels) && s != string(model.AttrAudioObject) && s != string(model.AttrAudioLang) && s != string(model.AttrSourceType) && s != string(model.AttrContainer) && s != string(model.AttrPlatform) && s != string(model.AttrCRC) && s != string(model.AttrTech) && s != string(model.AttrAudioFlag) && s != string(model.AttrSubFlag) && s != string(model.AttrSubLang) && s != string(model.AttrSubFormat) && s != string(model.AttrSubLangGroup) {
			continue
		}
		if a.TokenStart >= 0 && a.TokenStart < minIdx {
			minIdx = a.TokenStart
		}
	}
	if minIdx == 1<<30 {
		return -1, false
	}
	return minIdx, true
}

func inferTailAliasesFromPipes(tokens []model.Token, attrs []model.Attribute, group string) []string {
	techStart, ok := firstTechTokenIndexFromAttrs(attrs)
	if !ok {
		return nil
	}
	if techStart < 0 || techStart >= len(tokens) {
		return nil
	}
	aliases := make([]string, 0, 4)
	seen := map[string]bool{}
	groupKey := strings.ToLower(strings.TrimSpace(group))
	if groupKey != "" {
		seen[groupKey] = true
	}
	for i := techStart; i < len(tokens); i++ {
		if tokens[i].Text != "|" && tokens[i].Text != "｜" {
			continue
		}
		segStart := i + 1
		segEnd := len(tokens)
		for j := segStart; j < len(tokens); j++ {
			if tokens[j].Text == "|" || tokens[j].Text == "｜" {
				segEnd = j
				break
			}
		}
		if segStart >= segEnd {
			i = segEnd
			continue
		}
		parts := make([]string, 0, segEnd-segStart)
		for k := segStart; k < segEnd; k++ {
			if tokens[k].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[k].Text)
			if t == "" {
				continue
			}
			if t == "[" || t == "]" || t == "(" || t == ")" || t == "（" || t == "）" || t == "{" || t == "}" || t == "【" || t == "】" {
				continue
			}
			parts = append(parts, t)
		}
		inner := strings.TrimSpace(strings.Join(parts, " "))
		if inner == "" {
			i = segEnd
			continue
		}
		inner = cleanAliasToken(inner)
		if stripped := stripExplicitSeasonSuffix(inner); stripped != "" {
			inner = stripped
		}
		if !isValidAliasToken(inner) {
			i = segEnd
			continue
		}
		if isTailGroupLike(inner) {
			i = segEnd
			continue
		}
		k := strings.ToLower(strings.TrimSpace(inner))
		if k != "" && !seen[k] {
			seen[k] = true
			aliases = append(aliases, inner)
		}
		i = segEnd
	}
	if len(aliases) == 0 {
		return nil
	}
	return aliases
}

func isTailGroupLike(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if strings.HasPrefix(ss, "-") || strings.HasPrefix(ss, "@") {
		return true
	}
	if regexp.MustCompile(`^[A-Z0-9]{2,10}$`).MatchString(ss) {
		return true
	}
	return false
}

func detectPackKind(cfg *config.Config, rawTitle string) string {
	lower := strings.ToLower(rawTitle)

	keywords := make([]string, 0, 16)
	for _, r := range cfg.Inference.ContentTypeInference {
		if len(r.PackKeywords) > 0 {
			keywords = append(keywords, r.PackKeywords...)
		}
	}

	if strings.Contains(rawTitle, "TV动画") || strings.Contains(rawTitle, "tv动画") {
		hasEnum := strings.Contains(rawTitle, "+") || strings.Contains(rawTitle, "；") || strings.Contains(rawTitle, ";")
		if hasEnum {
			hits := 0
			if strings.Contains(rawTitle, "OVA") || strings.Contains(rawTitle, "ova") || strings.Contains(rawTitle, "OVA篇") {
				hits++
			}
			if strings.Contains(rawTitle, "小说") {
				hits++
			}
			if strings.Contains(rawTitle, "漫画") {
				hits++
			}
			if strings.Contains(rawTitle, "CD") || strings.Contains(rawTitle, "cd") {
				hits++
			}
			if strings.Contains(rawTitle, "其他") {
				hits++
			}
			if hits >= 2 {
				return "mixed_pack"
			}
		}
	}

	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(kw)) {
			return kw
		}
	}

	return ""
}

var spKindPattern = regexp.MustCompile(`(?i)^(sp|ova|oad|ona|ex)(\d{1,3})?$`)

func detectSPKind(tokens []model.Token) string {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if m := regexp.MustCompile(`(?i)^0?\d{1,3}\s*(?:&|＆)\s*(sp|ova|oad|ona|ex)\b`).FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(m) == 2 {
			return strings.ToUpper(m[1])
		}
		m := spKindPattern.FindStringSubmatch(tokens[i].Text)
		if len(m) >= 2 {
			return strings.ToUpper(m[1])
		}
	}
	return ""
}

func extractSeasonRangeAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) string {
	if out == nil {
		return ""
	}
	best := ""
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i].Removed || tokens[i+1].Removed || tokens[i+2].Removed {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		l := strings.TrimSpace(tokens[i].Text)
		mid := strings.TrimSpace(tokens[i+1].Text)
		r := strings.TrimSpace(tokens[i+2].Text)
		if mid != "-" {
			continue
		}
		ml := regexp.MustCompile(`(?i)^s(\d{1,2})$`).FindStringSubmatch(l)
		mr := regexp.MustCompile(`(?i)^s(\d{1,2})$`).FindStringSubmatch(r)
		if len(ml) != 2 || len(mr) != 2 {
			continue
		}
		ls := strings.TrimLeft(ml[1], "0")
		rs := strings.TrimLeft(mr[1], "0")
		if ls == "" {
			ls = "0"
		}
		if rs == "" {
			rs = "0"
		}
		if ls == "0" || rs == "0" {
			continue
		}
		cand := "S" + ls + "-S" + rs
		// Remove tokens so season detection does not pick only one side (e.g., S2).
		tokens[i].Removed = true
		tokens[i+1].Removed = true
		tokens[i+2].Removed = true
		*out = append(*out, model.Attribute{Type: model.AttrSeasonRange, Normalized: cand, TokenStart: i, TokenEnd: i + 3, Confidence: 0.85, Source: "season_range"})
		if best == "" {
			best = cand
		}
	}
	return best
}

var movieIndexPattern = regexp.MustCompile(`(?i)^(?:movie)?\s*(\d{1,3})$`)
var jpMovieIndexPattern = regexp.MustCompile(`(?i)(?:劇場版|剧场版)(?:第)?(\d{1,3})(?:部)?`)
var filmIndexPattern = regexp.MustCompile(`(?i)^film[\s._-]?(\d{1,3})$`)

func detectMovieIndexAndKeyword(tokens []model.Token) (movieIndex string, tokenIdx int, hasMovieKeyword bool) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		text := strings.TrimSpace(tokens[i].Text)
		lt := strings.ToLower(text)
		if m := filmIndexPattern.FindStringSubmatch(text); len(m) == 2 {
			return strings.TrimLeft(m[1], "0"), i, true
		}
		if lt == "film" {
			hasMovieKeyword = true
			j := i + 1
			if j < len(tokens) {
				next := strings.TrimSpace(tokens[j].Text)
				if next == "." || next == "_" || next == "-" {
					j++
				}
			}
			if j < len(tokens) {
				next := strings.TrimSpace(tokens[j].Text)
				m := movieIndexPattern.FindStringSubmatch(next)
				if len(m) == 2 {
					return strings.TrimLeft(m[1], "0"), j, true
				}
			}
		}
		if lt == "movie" || lt == "themovie" || lt == "eiga" || lt == "gekijouban" || strings.Contains(text, "劇場版") || strings.Contains(text, "剧场版") || strings.Contains(text, "映画") || strings.Contains(text, "大电影") || strings.Contains(text, "大電影") {
			hasMovieKeyword = true
			j := i + 1
			if j < len(tokens) {
				next := strings.TrimSpace(tokens[j].Text)
				if next == "." || next == "_" || next == "-" {
					j++
				}
			}
			if j < len(tokens) {
				m := movieIndexPattern.FindStringSubmatch(tokens[j].Text)
				if len(m) == 2 {
					return strings.TrimLeft(m[1], "0"), j, true
				}
			}
		}
		m := jpMovieIndexPattern.FindStringSubmatch(text)
		if len(m) == 2 {
			return strings.TrimLeft(m[1], "0"), i, true
		}
		if lt == "movie" {
			hasMovieKeyword = true
		}
	}
	return "", -1, hasMovieKeyword
}
