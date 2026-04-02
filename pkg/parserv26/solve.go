package parserv26

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"title-parser/pkg/model"
)

var (
	sxxexxExtractPattern = regexp.MustCompile(`(?i)^s(\d{1,2})e(\d{1,4})$`)
	sxxexxVerPattern     = regexp.MustCompile(`(?i)^s(\d{1,2})e(\d{1,4})v\d+$`)
	epExtractPattern     = regexp.MustCompile(`(?i)^(?:e|ep)(\d{1,4})$`)
	epRangeExtract       = regexp.MustCompile(`(?i)^(?:e|ep)?(\d{1,4})[-~](?:e|ep)?(\d{1,4})$`)
	epListExtract        = regexp.MustCompile(`(?i)^(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?(?:,(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?)+$`)
	epListPartExtract    = regexp.MustCompile(`(?i)^(?:ep|e)?\d{1,4}(?:[-~](?:ep|e)?\d{1,4})?$`)
	jpEpExtractPattern   = regexp.MustCompile(`^第?(\d{1,4})(話|话|集)$`)
	digitPattern         = regexp.MustCompile(`^\d{1,4}$`)
	groupSepNormalizeRe  = regexp.MustCompile(`\s*([\-_.])\s*`)
	titleNumDashAlphaRe  = regexp.MustCompile(`(?i)\b(\d+)\s*-\s*([a-z])`)
)

func solveStructure(tokens []model.Token, cands []SpanCandidate) (model.ParseResult, *Solution) {
	result := model.ParseResult{}
	sol := &Solution{}

	group := pickBestGroup(cands, len(tokens))
	if group != nil {
		group.Value = renderSpan(tokens, group.TokenStart, group.TokenEnd)
		group.Value = normalizeGroupValue(group.Value)
		sol.Group = group
		result.Attributes = append(result.Attributes, model.Attribute{
			Type:       model.AttrGroup,
			Normalized: group.Value,
			TokenStart: group.TokenStart,
			TokenEnd:   group.TokenEnd,
			Confidence: group.Score,
			Source:     "v26_solve",
		})
	}

	season, episode := pickBestEpisodeAndSeason(tokens, cands)
	seasonValue := ""
	if season != nil {
		sol.Season = season
		seasonValue = strings.TrimSpace(season.Value)
	}
	if episode != nil {
		sol.Episode = episode
		episodeValue := strings.TrimSpace(episode.Value)
		episodeRange := strings.TrimSpace(episode.EpisodeRange)
		episodeList := append([]string(nil), episode.EpisodeList...)
		primary := &model.EPEntity{
			TokenStart: episode.TokenStart,
			TokenEnd:   episode.TokenEnd,
			Source:     "v26_solve",
			Axis:       "ABS",
			Season:     seasonValue,
		}
		if seasonValue != "" {
			primary.Axis = "SE"
		}
		switch {
		case episodeRange != "":
			primary.Kind = "RANGE"
			primary.EpisodeRange = episodeRange
		case len(episodeList) > 0:
			primary.Kind = "EP_LIST"
			primary.Episode = firstV26EpisodeListHead(episodeList)
			primary.EpisodeList = strings.Join(episodeList, ",")
		case episodeValue != "":
			primary.Kind = "EP"
			primary.Episode = episodeValue
		}
		result.EPSemantics = buildV26EPSemantics(primary, seasonValue)
	}

	blocked := make([]bool, len(tokens))
	markBlockedFromSelection(blocked, sol.Group)
	markBlockedFromSelection(blocked, sol.Season)
	markBlockedFromSelection(blocked, sol.Episode)
	for _, c := range cands {
		if c.Score < 0.65 {
			continue
		}
		switch c.Slot {
		case SlotTech, SlotLanguage, SlotNoise:
			markBlocked(blocked, c.TokenStart, c.TokenEnd)
		}
	}

	title, epTitle := pickTitleAndEpisodeTitle(tokens, cands, blocked, sol.Episode)
	if title != nil {
		title.Value = renderSpan(tokens, title.TokenStart, title.TokenEnd)
		sol.Title = title
		result.Attributes = append(result.Attributes, model.Attribute{
			Type:       model.AttrTitle,
			Normalized: title.Value,
			TokenStart: title.TokenStart,
			TokenEnd:   title.TokenEnd,
			Confidence: title.Score,
			Source:     "v26_solve",
		})
	}
	if epTitle != nil {
		epTitle.Value = renderSpan(tokens, epTitle.TokenStart, epTitle.TokenEnd)
		if strings.TrimSpace(epTitle.Value) != "" {
			result.Attributes = append(result.Attributes, model.Attribute{
				Type:       model.AttrNote,
				Normalized: "episode_title:" + strings.TrimSpace(epTitle.Value),
				TokenStart: epTitle.TokenStart,
				TokenEnd:   epTitle.TokenEnd,
				Confidence: epTitle.Score,
				Source:     "v26_solve",
			})
		}
	}

	if result.EPSemantics != nil && strings.TrimSpace(result.EPSemantics.Kind) != "" {
		result.ContentKind = "TV_EP"
		result.ContentConfidence = 0.7
	} else if strings.TrimSpace(result.CanonicalTitle()) != "" {
		result.ContentKind = "UNKNOWN"
		result.ContentConfidence = 0.5
	}

	return result, sol
}

func pickBestGroup(cands []SpanCandidate, totalTokens int) *SlotSelection {
	var best *SlotSelection
	bestRank := -1.0
	for _, c := range cands {
		if c.Slot != SlotGroup {
			continue
		}
		rank := c.Score
		if c.TokenStart <= 1 {
			rank += 0.15
		}
		if c.TokenEnd >= totalTokens-1 {
			rank += 0.1
		}
		if strings.Contains(c.Source, "known_group_span") {
			rank += 0.2
		} else if strings.Contains(c.Source, "known_group") {
			rank += 0.1
		}
		if rank > bestRank {
			bestRank = rank
			best = &SlotSelection{
				Slot:       SlotGroup,
				TokenStart: c.TokenStart,
				TokenEnd:   c.TokenEnd,
				Score:      c.Score,
				Source:     c.Source,
			}
		}
	}
	return best
}

func pickBestEpisodeAndSeason(tokens []model.Token, cands []SpanCandidate) (*SlotSelection, *SlotSelection) {
	var bestEP *SlotSelection
	var bestSE *SlotSelection
	bestEPRank := -1.0
	bestSERank := -1.0

	hasSeasonBySpan := map[string]bool{}
	for _, c := range cands {
		if c.Slot == SlotSeason {
			hasSeasonBySpan[spanKey(c.TokenStart, c.TokenEnd)] = true
		}
	}

	for _, c := range cands {
		if c.Slot != SlotEpisode && c.Slot != SlotSeason {
			continue
		}
		if c.TokenStart < 0 || c.TokenStart >= len(tokens) {
			continue
		}
		t := strings.TrimSpace(tokens[c.TokenStart].Text)
		if t == "" {
			continue
		}

		if c.Slot == SlotEpisode {
			epToken := compactSpanToken(tokens, c.TokenStart, c.TokenEnd)
			if epToken == "" {
				epToken = t
			}
			if se, ep, epRange, epList, ok := parseEpisodeToken(epToken); ok {
				rank := c.Score
				if hasSeasonBySpan[spanKey(c.TokenStart, c.TokenEnd)] {
					rank += 0.08
				}
				if rank > bestEPRank {
					bestEPRank = rank
					bestEP = &SlotSelection{
						Slot:         SlotEpisode,
						TokenStart:   c.TokenStart,
						TokenEnd:     c.TokenEnd,
						Score:        c.Score,
						Source:       c.Source,
						Value:        ep,
						EpisodeRange: epRange,
						EpisodeList:  append([]string(nil), epList...),
					}
				}
				if se != "" && rank > bestSERank {
					bestSERank = rank
					bestSE = &SlotSelection{
						Slot:       SlotSeason,
						TokenStart: c.TokenStart,
						TokenEnd:   c.TokenEnd,
						Score:      c.Score,
						Source:     c.Source,
						Value:      se,
					}
				}
			}
		}

		if c.Slot == SlotSeason {
			if se, ok := parseSeasonToken(t); ok {
				rank := c.Score
				if rank > bestSERank {
					bestSERank = rank
					bestSE = &SlotSelection{
						Slot:       SlotSeason,
						TokenStart: c.TokenStart,
						TokenEnd:   c.TokenEnd,
						Score:      c.Score,
						Source:     c.Source,
						Value:      se,
					}
				}
			}
		}
	}

	return bestSE, bestEP
}

func parseEpisodeToken(token string) (season string, episode string, episodeRange string, episodeList []string, ok bool) {
	t := strings.ToLower(strings.TrimSpace(token))
	t = strings.ReplaceAll(t, " ", "")
	if m := sxxexxVerPattern.FindStringSubmatch(t); len(m) == 3 {
		return trimLeadingZeros(m[1]), trimLeadingZeros(m[2]), "", nil, true
	}
	if m := sxxexxExtractPattern.FindStringSubmatch(t); len(m) == 3 {
		return trimLeadingZeros(m[1]), trimLeadingZeros(m[2]), "", nil, true
	}
	if m := epRangeExtract.FindStringSubmatch(t); len(m) == 3 {
		l := trimLeadingZeros(m[1])
		r := trimLeadingZeros(m[2])
		return "", "", l + "-" + r, nil, true
	}
	if epListExtract.MatchString(t) {
		parts := strings.Split(t, ",")
		list := make([]string, 0, len(parts))
		seen := make(map[string]struct{}, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(strings.ToLower(p))
			if p == "" || !epListPartExtract.MatchString(p) {
				return "", "", "", nil, false
			}
			if strings.Contains(p, "-") || strings.Contains(p, "~") {
				sep := "-"
				if strings.Contains(p, "~") {
					sep = "~"
				}
				rng := strings.SplitN(p, sep, 2)
				if len(rng) != 2 {
					return "", "", "", nil, false
				}
				l, okL := normalizeEpisodeListAtomV26(rng[0])
				r, okR := normalizeEpisodeListAtomV26(rng[1])
				if !okL || !okR {
					return "", "", "", nil, false
				}
				part := l + "-" + r
				if _, dup := seen[part]; dup {
					continue
				}
				seen[part] = struct{}{}
				list = append(list, part)
				continue
			}
			n, ok := normalizeEpisodeListAtomV26(p)
			if !ok {
				return "", "", "", nil, false
			}
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			list = append(list, n)
		}
		if len(list) >= 2 {
			return "", "", "", list, true
		}
		return "", "", "", nil, false
	}
	if m := epExtractPattern.FindStringSubmatch(t); len(m) == 2 {
		return "", trimLeadingZeros(m[1]), "", nil, true
	}
	if m := jpEpExtractPattern.FindStringSubmatch(token); len(m) == 3 {
		return "", trimLeadingZeros(m[1]), "", nil, true
	}
	if digitPattern.MatchString(t) {
		v := trimLeadingZeros(t)
		if v != "" {
			return "", v, "", nil, true
		}
	}
	return "", "", "", nil, false
}

func normalizeEpisodeListAtomV26(s string) (string, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimPrefix(s, "ep")
	s = strings.TrimPrefix(s, "e")
	if !digitPattern.MatchString(s) {
		return "", false
	}
	s = trimLeadingZeros(s)
	if s == "" || s == "0" {
		return "", false
	}
	return s, true
}

func parseSeasonToken(token string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(token))
	if m := sxxexxExtractPattern.FindStringSubmatch(t); len(m) == 3 {
		return trimLeadingZeros(m[1]), true
	}
	if strings.HasPrefix(t, "s") && len(t) > 1 && digitPattern.MatchString(t[1:]) {
		return trimLeadingZeros(t[1:]), true
	}
	return "", false
}

type titleSeg struct {
	start int
	end   int
	score float64
}

func pickTitleAndEpisodeTitle(tokens []model.Token, cands []SpanCandidate, blocked []bool, episode *SlotSelection) (*SlotSelection, *SlotSelection) {
	segments := collectTitleSegments(tokens, cands, blocked)
	if len(segments) == 0 {
		return fallbackTitleFromCandidates(cands), nil
	}

	bestAll := bestTitleSeg(segments, func(_ titleSeg) bool { return true })
	if bestAll == nil {
		return fallbackTitleFromCandidates(cands), nil
	}

	main := bestAll
	var epTitle *titleSeg
	if episode != nil {
		before := bestTitleSeg(segments, func(s titleSeg) bool {
			return s.end <= episode.TokenStart
		})
		after := bestTitleSeg(segments, func(s titleSeg) bool {
			return s.start > episode.TokenEnd
		})
		if before != nil {
			main = before
			// Keep post-episode phrase as episode title candidate when available.
			if after != nil && after.start > before.end {
				epTitle = after
			}
		}
	}

	toSel := func(s *titleSeg, source string) *SlotSelection {
		if s == nil || s.end-s.start <= 0 {
			return nil
		}
		return &SlotSelection{
			Slot:       SlotTitle,
			TokenStart: s.start,
			TokenEnd:   s.end,
			Score:      clamp01(s.score / 10.0),
			Source:     source,
		}
	}

	return toSel(main, "global_title_segment"), toSel(epTitle, "episode_title_segment")
}

func collectTitleSegments(tokens []model.Token, cands []SpanCandidate, blocked []bool) []titleSeg {
	techBoundary := firstTechBoundary(cands, len(tokens), blocked)
	segments := make([]titleSeg, 0, 8)

	i := 0
	for i < len(tokens) {
		if blocked[i] || !titleTokenAllowedAt(tokens, i) {
			i++
			continue
		}
		if techBoundary >= 0 && i >= techBoundary {
			i++
			continue
		}
		start := i
		end := i
		for end < len(tokens) {
			if blocked[end] {
				break
			}
			if techBoundary >= 0 && end >= techBoundary {
				break
			}
			t := strings.TrimSpace(tokens[end].Text)
			if t == "" || isBracketToken(t) {
				break
			}
			if isLikelyTechOrLang(t) {
				break
			}
			if !titleTokenAllowedAt(tokens, end) && !isConnectorToken(t) {
				break
			}
			end++
		}
		if end > start {
			segments = append(segments, titleSeg{
				start: start,
				end:   end,
				score: scoreSegment(tokens, cands, start, end),
			})
		}
		i = max(end, i+1)
	}
	return segments
}

func bestTitleSeg(segments []titleSeg, keep func(titleSeg) bool) *titleSeg {
	var best *titleSeg
	for _, s := range segments {
		if !keep(s) {
			continue
		}
		if best == nil || s.score > best.score || (s.score == best.score && (s.end-s.start) > (best.end-best.start)) {
			cur := s
			best = &cur
		}
	}
	return best
}

func scoreSegment(tokens []model.Token, cands []SpanCandidate, start, end int) float64 {
	words := 0
	hasLetter := false
	plusCount := 0
	packLikeCount := 0
	cjkLongWord := 0
	firstWordIdx := -1
	for i := start; i < end; i++ {
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" || isConnectorToken(t) {
			if t == "+" {
				plusCount++
			}
			continue
		}
		if firstWordIdx < 0 {
			firstWordIdx = i
		}
		words++
		if looksLikePackOrPartToken(t) {
			packLikeCount++
		}
		if containsCJK(t) && len([]rune(t)) >= 4 {
			cjkLongWord++
		}
		for _, r := range t {
			if unicode.IsLetter(r) {
				hasLetter = true
				break
			}
		}
	}
	if words == 0 {
		return 0
	}
	score := float64(words)
	if hasLetter {
		score += 1.5
	}
	if start <= 2 {
		score += 1.0
	}
	if cjkLongWord > 0 {
		score += 0.8
	}
	if firstWordIdx >= 0 {
		firstNorm := normalizeToken(strings.TrimSpace(tokens[firstWordIdx].Text))
		if start > 0 && digitPattern.MatchString(firstNorm) && len(firstNorm) <= 2 {
			// Penalize middle numeric-start segment like "25 COMPLETE ...".
			score -= 1.6
		}
	}
	score -= 0.8 * float64(plusCount)
	score -= 1.2 * float64(packLikeCount)
	for _, c := range cands {
		if c.Slot != SlotTitle {
			continue
		}
		if c.TokenStart >= start && c.TokenEnd <= end {
			score += 0.4
		}
	}
	return score
}

func fallbackTitleFromCandidates(cands []SpanCandidate) *SlotSelection {
	var best *SlotSelection
	bestRank := -1.0
	for _, c := range cands {
		if c.Slot != SlotTitle {
			continue
		}
		rank := c.Score + 0.05*float64(c.TokenEnd-c.TokenStart)
		if rank > bestRank {
			bestRank = rank
			best = &SlotSelection{
				Slot:       SlotTitle,
				TokenStart: c.TokenStart,
				TokenEnd:   c.TokenEnd,
				Score:      c.Score,
				Source:     "title_candidate_fallback",
			}
		}
	}
	return best
}

func renderSpan(tokens []model.Token, start, end int) string {
	if start < 0 || end > len(tokens) || start >= end {
		return ""
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if b.Len() > 0 && needsSpaceBefore(t) && needsSpaceAfter(lastToken(&b)) {
			b.WriteString(" ")
		}
		b.WriteString(t)
	}
	out := strings.TrimSpace(b.String())
	out = strings.ReplaceAll(out, " ,", ",")
	out = strings.ReplaceAll(out, " .", ".")
	out = strings.ReplaceAll(out, " :", ":")
	out = strings.ReplaceAll(out, " ;", ";")
	out = strings.ReplaceAll(out, " / ", " / ")
	// Dotted ascii titles like "A . Couple . of" should collapse to "A Couple of".
	if strings.Count(out, ".") >= 2 {
		out = strings.ReplaceAll(out, ".", " ")
		out = strings.Join(strings.Fields(out), " ")
	}
	return normalizeTitleValue(out)
}

func lastToken(b *strings.Builder) string {
	s := b.String()
	if s == "" {
		return ""
	}
	return string([]rune(s)[len([]rune(s))-1:])
}

func needsSpaceBefore(token string) bool {
	if token == "" {
		return false
	}
	switch token {
	case ",", ".", ":", ";", ")", "]", "}", "】", "」", "』":
		return false
	default:
		return true
	}
}

func needsSpaceAfter(prev string) bool {
	switch prev {
	case "", "(", "[", "{", "【", "「", "『":
		return false
	default:
		return true
	}
}

func titleTokenAllowed(token string) bool {
	t := strings.TrimSpace(token)
	if t == "" || isBracketToken(t) {
		return false
	}
	if isLikelyTechOrLang(t) {
		return false
	}
	if digitPattern.MatchString(normalizeToken(t)) && len(t) <= 2 {
		return false
	}
	return true
}

func titleTokenAllowedAt(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	t := strings.TrimSpace(tokens[idx].Text)
	if !titleTokenAllowed(t) {
		n := normalizeToken(t)
		// Keep short numeric token if it behaves like part of title phrase:
		// e.g. "8 Man", "29-sai".
		if digitPattern.MatchString(n) && len(n) <= 2 {
			if idx+1 < len(tokens) {
				n1 := strings.TrimSpace(tokens[idx+1].Text)
				if isConnectorToken(n1) && idx+2 < len(tokens) && hasAlpha(strings.TrimSpace(tokens[idx+2].Text)) {
					return true
				}
				if hasAlpha(n1) {
					return true
				}
			}
		}
		return false
	}
	return true
}

func isConnectorToken(token string) bool {
	switch strings.TrimSpace(token) {
	case "-", "_", "/", "|", "&", "'", "’", ":", ".", "+":
		return true
	default:
		return false
	}
}

func markBlocked(blocked []bool, start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(blocked) {
		end = len(blocked)
	}
	for i := start; i < end; i++ {
		blocked[i] = true
	}
}

func markBlockedFromSelection(blocked []bool, sel *SlotSelection) {
	if sel == nil {
		return
	}
	markBlocked(blocked, sel.TokenStart, sel.TokenEnd)
}

func trimLeadingZeros(s string) string {
	if s == "" {
		return ""
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return s
	}
	return strconv.Itoa(v)
}

func spanKey(start, end int) string {
	return strconv.Itoa(start) + ":" + strconv.Itoa(end)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func normalizeGroupValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = groupSepNormalizeRe.ReplaceAllString(s, "$1")
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, "-_. ")
}

func normalizeTitleValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.Contains(s, "|") {
		parts := strings.Split(s, "|")
		if len(parts) > 0 {
			first := strings.TrimSpace(parts[0])
			if first != "" {
				s = first
			}
		}
	}
	parts := strings.Fields(s)
	for len(parts) > 0 && isSeparatorToken(parts[0]) {
		parts = parts[1:]
	}
	for len(parts) > 0 && isSeparatorToken(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return ""
	}
	s = strings.Join(parts, " ")
	s = titleNumDashAlphaRe.ReplaceAllString(s, "$1-$2")
	s = strings.Trim(s, "-_. ")
	if !containsTitleRune(s) {
		return ""
	}
	return strings.TrimSpace(s)
}

func firstTechBoundary(cands []SpanCandidate, tokenCount int, blocked []bool) int {
	b := tokenCount
	for _, c := range cands {
		if c.Score < 0.65 {
			continue
		}
		if c.TokenStart >= 0 && c.TokenStart < len(blocked) && blocked[c.TokenStart] {
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
	// Ignore prefix tech/noise markers near the very beginning (e.g. "(V2) ..."),
	// otherwise title segmentation is truncated too early.
	if b <= 2 {
		return -1
	}
	return b
}

func looksLikePackOrPartToken(t string) bool {
	n := normalizeToken(t)
	if n == "" {
		return false
	}
	if strings.Contains(t, "剧场版") || strings.Contains(t, "特別") || strings.Contains(t, "特别") {
		return true
	}
	if strings.HasPrefix(t, "第") && strings.HasSuffix(t, "部") {
		return true
	}
	switch n {
	case "pack", "complete", "batch", "sp", "special", "specials", "tv", "ova", "oad":
		return true
	}
	return false
}

func compactSpanToken(tokens []model.Token, start, end int) string {
	if start < 0 || end > len(tokens) || start >= end {
		return ""
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		b.WriteString(t)
	}
	return b.String()
}

func containsTitleRune(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

func firstV26EpisodeListHead(items []string) string {
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if idx := strings.IndexAny(v, "-~"); idx >= 0 {
			v = strings.TrimSpace(v[:idx])
		}
		if v != "" {
			return v
		}
	}
	return ""
}

func splitV26EpisodeRange(v string) (string, string) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", ""
	}
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == '-' || r == '~'
	})
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func buildV26EPSemantics(primary *model.EPEntity, season string) *model.EPSemantics {
	if primary == nil {
		return nil
	}
	kind := strings.ToUpper(strings.TrimSpace(primary.Kind))
	if kind == "" {
		return nil
	}
	out := &model.EPSemantics{
		Kind:   kind,
		Axis:   strings.ToUpper(strings.TrimSpace(primary.Axis)),
		Season: strings.TrimSpace(season),
		SPKind: strings.TrimSpace(primary.SPKind),
		Source: strings.TrimSpace(primary.Source),
	}
	switch kind {
	case "EP":
		out.Episode = strings.TrimSpace(primary.Episode)
	case "EP_LIST":
		out.Episode = strings.TrimSpace(primary.Episode)
		if v := strings.TrimSpace(primary.EpisodeList); v != "" {
			out.EpisodeList = strings.Split(v, ",")
		}
	case "RANGE":
		out.RangeStart, out.RangeEnd = splitV26EpisodeRange(primary.EpisodeRange)
	case "SP":
		// no-op
	default:
		return nil
	}
	return out
}
