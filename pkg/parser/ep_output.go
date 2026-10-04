package parser

import (
	"regexp"
	"strings"

	"reltitle/pkg/model"
)

func stripLegacyEPAttributes(attrs []model.Attribute) []model.Attribute {
	attrs = removeAttributeType(attrs, model.AttrEpisode)
	attrs = removeAttributeType(attrs, model.AttrEpisodeList)
	attrs = removeAttributeType(attrs, model.AttrEpisodeRange)
	attrs = removeAttributeType(attrs, model.AttrSeason)
	attrs = removeAttributeType(attrs, model.AttrSeasonRange)
	attrs = removeAttributeType(attrs, model.AttrSPKind)
	return attrs
}

func pruneCanonicalEpisodeSignalsForOutput(rawTitle string, contentKind string, packKind string, season string, episode string, episodeList string, episodeRange string, spKind string, movieIndex string, movieIndexRange string, epslot *model.EPSlot) (string, string, string, string, *model.EPSlot) {
	kind := strings.ToUpper(strings.TrimSpace(contentKind))
	if strings.TrimSpace(movieIndex) != "" && strings.TrimSpace(movieIndexRange) == "" && strings.TrimSpace(season) == "" && strings.TrimSpace(episodeRange) != "" && movieDashEpisodePattern.MatchString(rawTitle) {
		return "MOVIE", "", "", "", nil
	}
	if kind != "BATCH" {
		return contentKind, episode, episodeList, episodeRange, epslot
	}
	pk := strings.TrimSpace(packKind)
	if pk == "" {
		return contentKind, episode, episodeList, episodeRange, epslot
	}
	if strings.TrimSpace(spKind) != "" || strings.TrimSpace(season) != "" || strings.TrimSpace(movieIndexRange) != "" {
		return contentKind, episode, episodeList, episodeRange, epslot
	}
	if strings.TrimSpace(episode) != "" && strings.TrimSpace(episodeList) == "" && strings.TrimSpace(episodeRange) == "" && strings.TrimSpace(movieIndex) == "" {
		return contentKind, "", "", "", nil
	}
	if strings.TrimSpace(episodeRange) != "" && hasAmbiguousExtraNumberAfterPrimaryRange(rawTitle, episodeRange) {
		return contentKind, "", "", "", nil
	}
	return contentKind, episode, episodeList, episodeRange, epslot
}

func hasAmbiguousExtraNumberAfterPrimaryRange(rawTitle string, episodeRange string) bool {
	bounds := strings.SplitN(normalizeEpisodeRangeText(strings.TrimSpace(episodeRange)), "-", 2)
	if len(bounds) != 2 {
		return false
	}
	left := strings.TrimSpace(bounds[0])
	right := strings.TrimSpace(bounds[1])
	if left == "" || right == "" {
		return false
	}
	pat := regexp.MustCompile(`(?i)\b0*` + regexp.QuoteMeta(left) + `\s*[-~]\s*0*` + regexp.QuoteMeta(right) + `\b`)
	loc := pat.FindStringIndex(rawTitle)
	if loc == nil {
		return false
	}
	tail := rawTitle[loc[1]:]
	if !reHasAmbiguousExtraNumberAfterPrimaryRange1.MatchString(tail) {
		return false
	}
	return reHasAmbiguousExtraNumberAfterPrimaryRange2.MatchString(tail)
}

func pruneBatchEPSlotForConservativeOutput(epslot *model.EPSlot, contentKind string, episode string, episodeRange string, spKind string) *model.EPSlot {
	if epslot == nil {
		return nil
	}
	kind := strings.ToUpper(strings.TrimSpace(contentKind))
	if kind != "BATCH" && kind != "MOVIE" {
		return epslot
	}
	// For BATCH/MOVIE outputs, keep epslot only when we have a confirmed primary or
	// explicit canonical episode-like fields. Otherwise candidate-only epslot is
	// often noisy (e.g. digits inside title fragments) and should be suppressed.
	if epslot.Primary != nil {
		if shouldPruneWeakBatchRangePrimary(epslot, episodeRange, spKind) {
			return nil
		}
		return epslot
	}
	if strings.TrimSpace(episode) != "" || strings.TrimSpace(episodeRange) != "" || strings.TrimSpace(spKind) != "" {
		return epslot
	}
	return nil
}

func shouldPruneWeakBatchRangePrimary(epslot *model.EPSlot, episodeRange string, spKind string) bool {
	if epslot == nil || epslot.Primary == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(epslot.Primary.Kind), "RANGE") {
		return false
	}
	if strings.TrimSpace(spKind) != "" {
		return false
	}
	rangeText := strings.TrimSpace(episodeRange)
	if rangeText == "" {
		rangeText = strings.TrimSpace(epslot.Primary.EpisodeRange)
	}
	if rangeText == "" {
		return false
	}
	source := strings.ToLower(strings.TrimSpace(epslot.Primary.Source))
	if source != "episode_range" && source != "episode_range_final" {
		return false
	}
	bounds := strings.SplitN(normalizeEpisodeRangeText(rangeText), "-", 2)
	if len(bounds) != 2 {
		return false
	}
	left := strings.TrimSpace(bounds[0])
	right := strings.TrimSpace(bounds[1])
	for _, c := range epslot.Candidates {
		kind := strings.ToUpper(strings.TrimSpace(c.Kind))
		if kind != "EP" && kind != "EP_LIST" {
			continue
		}
		ep := strings.TrimSpace(c.Episode)
		if ep == "" || ep == left || ep == right {
			continue
		}
		return true
	}
	return false
}

func shouldPreferSPPrimaryForSpecial(contentKind string, spKind string, epslot *model.EPSlot) bool {
	if !strings.EqualFold(strings.TrimSpace(contentKind), "SPECIAL") {
		return false
	}
	if strings.TrimSpace(spKind) == "" || epslot == nil || epslot.Primary == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(epslot.Primary.Kind), "EP") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(epslot.Primary.Source), "ep_digit_token")
}

func forceSPPrimaryFromSPKind(epslot *model.EPSlot, season string, spKind string) *model.EPSlot {
	k := strings.ToUpper(strings.TrimSpace(spKind))
	if k == "" {
		return epslot
	}
	primary := model.EPEntity{
		Kind:   "SP",
		Axis:   k,
		Season: strings.TrimSpace(season),
		SPKind: k,
		Source: "sp_kind_final",
	}
	if epslot == nil {
		return &model.EPSlot{Primary: &primary, Candidates: []model.EPEntity{{Kind: "SP", Axis: k, Season: strings.TrimSpace(season), SPKind: k, Source: "sp_kind"}}}
	}
	candidates := make([]model.EPEntity, 0, len(epslot.Candidates)+1)
	candidates = append(candidates, model.EPEntity{Kind: "SP", Axis: k, Season: strings.TrimSpace(season), SPKind: k, Source: "sp_kind"})
	for _, c := range epslot.Candidates {
		if strings.EqualFold(strings.TrimSpace(c.Kind), "SP") && strings.EqualFold(strings.TrimSpace(c.SPKind), k) {
			continue
		}
		candidates = append(candidates, c)
	}
	epslot.Primary = &primary
	epslot.Candidates = candidates
	return epslot
}

func isUnknownEPSlotAxis(axis string) bool {
	ax := strings.ToUpper(strings.TrimSpace(axis))
	return ax == "" || ax == "UNKNOWN"
}

func inferEPAxisFromTokenContext(tokens []model.Token, tokenIdx int) string {
	if tokenIdx < 0 || tokenIdx >= len(tokens) {
		return ""
	}
	cur := strings.TrimSpace(tokens[tokenIdx].Text)
	if cur == "" {
		return ""
	}
	if chineseEpisodeSuffixPattern.MatchString(cur) || chineseEpisodeWithSuffixMarkerPattern.MatchString(cur) || embeddedChineseEpisodeSuffixPattern.MatchString(cur) {
		return "ABS"
	}
	if isInBracket(tokens, tokenIdx) && episodeOfTotalPattern.MatchString(cur) {
		return "ABS"
	}
	prev := ""
	next := ""
	if tokenIdx-1 >= 0 {
		prev = strings.TrimSpace(tokens[tokenIdx-1].Text)
	}
	if tokenIdx+1 < len(tokens) {
		next = strings.TrimSpace(tokens[tokenIdx+1].Text)
	}
	if strings.EqualFold(prev, "episode") || strings.EqualFold(prev, "ep") ||
		strings.EqualFold(next, "episode") || strings.EqualFold(next, "ep") {
		return "ABS"
	}
	if episodePrefixedTokenPattern.MatchString(cur) {
		return "ABS"
	}
	if prev == "第" && (next == "集" || next == "话" || next == "話") {
		return "ABS"
	}
	if next == "集" || next == "话" || next == "話" {
		return "ABS"
	}
	if prev == "-" && (next == "[" || next == "【" || next == "(" || next == "（") && episodeOfTotalPattern.MatchString(cur) {
		return "ABS"
	}
	return ""
}

func adjudicateEPSlotPrimaryAxis(epslot *model.EPSlot, tokens []model.Token, season string) *model.EPSlot {
	if epslot == nil || epslot.Primary == nil {
		return epslot
	}
	primary := epslot.Primary
	kind := strings.ToUpper(strings.TrimSpace(primary.Kind))
	if kind == "" {
		return epslot
	}
	if kind == "SP" {
		if isUnknownEPSlotAxis(primary.Axis) {
			ax := strings.ToUpper(strings.TrimSpace(primary.SPKind))
			if ax == "" {
				ax = "SP"
			}
			primary.Axis = ax
		}
		return epslot
	}
	if !isUnknownEPSlotAxis(primary.Axis) {
		return epslot
	}
	if strings.TrimSpace(primary.Season) != "" || strings.TrimSpace(season) != "" {
		primary.Axis = "SE"
		return epslot
	}
	src := strings.ToLower(strings.TrimSpace(primary.Source))
	if strings.Contains(src, "sxxexx") {
		primary.Axis = "SE"
		return epslot
	}
	if kind == "EP" {
		if axis := inferEPAxisFromTokenContext(tokens, primary.TokenStart); axis != "" {
			primary.Axis = axis
			return epslot
		}
	}
	return epslot
}

// Regular expressions hoisted out of function bodies so they are compiled once at init.
var (
	reHasAmbiguousExtraNumberAfterPrimaryRange1 = regexp.MustCompile(`(?i)\b(?:and|&|\+|plus)\b`)
	reHasAmbiguousExtraNumberAfterPrimaryRange2 = regexp.MustCompile(`\b\d{1,4}\b`)
)
