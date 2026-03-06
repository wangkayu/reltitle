package parser

import (
	"regexp"
	"strings"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

var (
	templateSignalSxxExx = regexp.MustCompile(`(?i)^s(\d{1,2})e(\d{1,4})$`)
	templateSignalJPEP   = regexp.MustCompile(`^第?(\d{1,4})(?:話|话|集)$`)
	templateSignalDigits = regexp.MustCompile(`^\d{1,3}$`)
	templateSignalResolutionToken = regexp.MustCompile(`(?i)^(\d{3,4})p$`)
	templateSignalVideoCodecToken = regexp.MustCompile(`(?i)^(?:x\.?26[45]|h\.?26[45]|hevc|avc|av1)$`)
	templateSignalAudioCodecToken = regexp.MustCompile(`(?i)^(?:aac|flac|ac3|e-?ac-?3|eac3|ddp|opus|pcm|dts(?:-?hd|-?hdma)?|truehd)$`)
	templateSignalSourceTypeToken = regexp.MustCompile(`(?i)^(?:web[-_ ]?dl|web[-_ ]?rip|webrip|webdl|bdrip|bluray|blu-ray|hdtv|dvdrip|dvd|remux)$`)
	templateSignalPlatformToken   = regexp.MustCompile(`(?i)^(?:cr|crunchyroll|nf|netflix|adn|baha|bilibili|amzn|amazon|abema|hidive|viutv)$`)
)

func applyTemplateSignalAdjudication(
	cfg *config.Config,
	templateID string,
	tokens []model.Token,
	attrs []model.Attribute,
	group string,
	episode string,
	episodeIdx int,
	episodeRange string,
	episodeRangeIdx int,
	season string,
	seasonIdx int,
) ([]model.Attribute, string, string, int, string, int, string, int, []string) {
	tpl := templateByID(cfg, templateID)
	if tpl == nil || tpl.Signals == nil {
		return attrs, group, episode, episodeIdx, episodeRange, episodeRangeIdx, season, seasonIdx, nil
	}
	sig := tpl.Signals
	events := make([]string, 0, 4)

	group = strings.TrimSpace(group)
	episode = strings.TrimSpace(episode)
	episodeRange = strings.TrimSpace(episodeRange)
	season = strings.TrimSpace(season)

	titleStart, titleEnd, hasTitleSpan := titleSpanFromAttrsForSignals(attrs)
	groupStart, groupEnd, hasGroupSpan := groupSpanFromAttrsForSignals(attrs, group)
	epAnchor := maxSignalIdx(episodeIdx, episodeRangeIdx, seasonIdx)

	if sig.ForbidGroupInTitleSpan && group != "" && hasGroupSpan && hasTitleSpan {
		if spansOverlap(groupStart, groupEnd, titleStart, titleEnd) {
			if tg, tgIdx, ok := bestTailGroupAfterAnchor(attrs, epAnchor); ok {
				group = tg
				groupStart = tgIdx
				groupEnd = tgIdx + 1
				hasGroupSpan = true
				attrs = attrsWithSingleGroup(attrs, group, groupStart, groupEnd, "template_signal")
				events = append(events, "group_relocated_from_title_span")
			} else {
				group = ""
				attrs = dropAttrType(attrs, model.AttrGroup)
				hasGroupSpan = false
				events = append(events, "group_dropped_inside_title_span")
			}
		}
	}

	if sig.GroupAfterEPOnly && group != "" && epAnchor >= 0 && hasGroupSpan && groupStart <= epAnchor {
		if tg, tgIdx, ok := bestTailGroupAfterAnchor(attrs, epAnchor); ok {
			group = tg
			groupStart = tgIdx
			groupEnd = tgIdx + 1
			hasGroupSpan = true
			attrs = attrsWithSingleGroup(attrs, group, groupStart, groupEnd, "template_signal")
			events = append(events, "group_relocated_after_ep")
		} else {
			group = ""
			attrs = dropAttrType(attrs, model.AttrGroup)
			hasGroupSpan = false
			events = append(events, "group_dropped_before_ep")
		}
	}

	if strings.EqualFold(strings.TrimSpace(sig.GroupPosition), "tail") && group != "" && hasGroupSpan && hasTitleSpan {
		// For tail-group templates, group cannot be inside or before title span.
		if groupStart < titleEnd {
			if tg, tgIdx, ok := bestTailGroupAfterAnchor(attrs, epAnchor); ok {
				group = tg
				groupStart = tgIdx
				groupEnd = tgIdx + 1
				hasGroupSpan = true
				attrs = attrsWithSingleGroup(attrs, group, groupStart, groupEnd, "template_signal")
				events = append(events, "tail_group_relocated_after_title")
			} else {
				group = ""
				attrs = dropAttrType(attrs, model.AttrGroup)
				hasGroupSpan = false
				events = append(events, "tail_group_dropped_before_title")
			}
		}
	}

	if sig.EpAnchorPriority && episode != "" {
		if weakEpisodeCandidate(tokens, episodeIdx, titleStart, titleEnd, hasTitleSpan) {
			if ep2, se2, idx2, ok := findStrongBracketEpisodeAnchor(tokens, episodeIdx); ok {
				episode = ep2
				episodeIdx = idx2
				episodeRange = ""
				episodeRangeIdx = -1
				if season == "" && se2 != "" {
					season = se2
					seasonIdx = idx2
				}
				events = append(events, "episode_replaced_by_bracket_anchor")
			}
		}
	}

	if sig.TechStartsAfterEPStrict {
		var techEvents []string
		attrs, techEvents = applyTechStartsAfterEPStrict(attrs, tokens, episodeIdx, episodeRangeIdx, seasonIdx, titleStart, titleEnd, hasTitleSpan)
		events = append(events, techEvents...)
	}

	return attrs, group, episode, episodeIdx, episodeRange, episodeRangeIdx, season, seasonIdx, uniqueSignalEvents(events)
}

func applyTechStartsAfterEPStrict(
	attrs []model.Attribute,
	tokens []model.Token,
	episodeIdx int,
	episodeRangeIdx int,
	seasonIdx int,
	titleStart int,
	titleEnd int,
	hasTitleSpan bool,
) ([]model.Attribute, []string) {
	anchor := maxSignalIdx(episodeIdx, episodeRangeIdx, seasonIdx)
	if anchor < 0 || len(tokens) == 0 || len(attrs) == 0 {
		return attrs, nil
	}

	events := make([]string, 0, 6)
	for i := range attrs {
		if !isStrictTechAttrType(attrs[i].Type) {
			continue
		}
		s := attrs[i].TokenStart
		e := attrs[i].TokenEnd
		if s < 0 || e <= s || s >= len(tokens) {
			continue
		}
		beforeEP := s <= anchor
		inTitle := hasTitleSpan && spansOverlap(s, e, titleStart, titleEnd)
		if !beforeEP && !inTitle {
			continue
		}
		origType := attrs[i].Type
		attrs[i].Type = model.AttrNote
		attrs[i].Source = "template_signal_tech_before_ep"
		if attrs[i].Confidence > 0.6 {
			attrs[i].Confidence = 0.6
		}
		events = append(events, "tech_before_ep_demoted:"+string(origType))
	}

	covered := make([]bool, len(tokens))
	for _, a := range attrs {
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart || a.TokenEnd > len(tokens) {
			continue
		}
		for i := a.TokenStart; i < a.TokenEnd; i++ {
			covered[i] = true
		}
	}
	hasType := make(map[model.AttrType]bool, 8)
	for _, a := range attrs {
		if isStrictTechAttrType(a.Type) {
			hasType[a.Type] = true
		}
	}
	for i := anchor + 1; i < len(tokens); i++ {
		if covered[i] {
			continue
		}
		t := cleanTemplateSignalTechToken(tokens[i].Text)
		if t == "" {
			continue
		}
		typ, norm, ok := inferPostEPTechToken(t)
		if !ok || hasType[typ] {
			continue
		}
		attrs = append(attrs, model.Attribute{
			Type:       typ,
			Normalized: norm,
			TokenStart: i,
			TokenEnd:   i + 1,
			Confidence: 0.62,
			Source:     "template_float_infer",
		})
		hasType[typ] = true
		covered[i] = true
		events = append(events, "tech_inferred_post_ep:"+string(typ))
	}

	return attrs, events
}

func cleanTemplateSignalTechToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, "[](){}<>【】（）.,，:：;；'\"")
	return strings.TrimSpace(s)
}

func inferPostEPTechToken(token string) (model.AttrType, string, bool) {
	if token == "" {
		return "", "", false
	}
	low := strings.ToLower(strings.TrimSpace(token))
	if low == "" {
		return "", "", false
	}
	if m := templateSignalResolutionToken.FindStringSubmatch(low); len(m) == 2 {
		return model.AttrResolution, m[1] + "p", true
	}
	if templateSignalSourceTypeToken.MatchString(low) {
		switch {
		case strings.HasPrefix(low, "web"):
			if strings.Contains(low, "dl") {
				return model.AttrSourceType, "WEB-DL", true
			}
			return model.AttrSourceType, "WEBRip", true
		case strings.Contains(low, "bdrip"):
			return model.AttrSourceType, "BDRip", true
		case strings.Contains(low, "blu"):
			return model.AttrSourceType, "BluRay", true
		case strings.Contains(low, "hdtv"):
			return model.AttrSourceType, "HDTV", true
		case strings.Contains(low, "dvdrip"):
			return model.AttrSourceType, "DVDRip", true
		case strings.Contains(low, "dvd"):
			return model.AttrSourceType, "DVD", true
		case strings.Contains(low, "remux"):
			return model.AttrSourceType, "Remux", true
		}
	}
	if templateSignalVideoCodecToken.MatchString(low) {
		n := strings.ReplaceAll(low, ".", "")
		switch n {
		case "hevc", "avc", "av1":
			return model.AttrVideoCodec, strings.ToUpper(n), true
		default:
			return model.AttrVideoCodec, n, true
		}
	}
	if templateSignalAudioCodecToken.MatchString(low) {
		n := strings.ReplaceAll(low, "-", "")
		switch n {
		case "aac", "flac", "ac3", "eac3", "ddp", "opus", "pcm", "truehd":
			return model.AttrAudioCodec, strings.ToUpper(n), true
		default:
			return model.AttrAudioCodec, strings.ToUpper(low), true
		}
	}
	if templateSignalPlatformToken.MatchString(low) {
		return model.AttrPlatform, normalizePlatformCode(low), true
	}
	return "", "", false
}

func isStrictTechAttrType(t model.AttrType) bool {
	switch t {
	case model.AttrResolution,
		model.AttrSourceType,
		model.AttrVideoCodec,
		model.AttrAudioCodec,
		model.AttrBitDepth,
		model.AttrContainer,
		model.AttrPlatform:
		return true
	default:
		return false
	}
}

func titleSpanFromAttrsForSignals(attrs []model.Attribute) (int, int, bool) {
	minS := 1 << 30
	maxE := -1
	for _, a := range attrs {
		if a.Type != model.AttrTitle {
			continue
		}
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart {
			continue
		}
		if a.TokenStart < minS {
			minS = a.TokenStart
		}
		if a.TokenEnd > maxE {
			maxE = a.TokenEnd
		}
	}
	if minS == 1<<30 || maxE < 0 {
		return 0, 0, false
	}
	return minS, maxE, true
}

func groupSpanFromAttrsForSignals(attrs []model.Attribute, group string) (int, int, bool) {
	group = strings.TrimSpace(group)
	if group == "" {
		return 0, 0, false
	}
	bestIdx := -1
	bestConf := -1.0
	for i, a := range attrs {
		if a.Type != model.AttrGroup {
			continue
		}
		if strings.TrimSpace(a.Normalized) != group {
			continue
		}
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart {
			continue
		}
		if a.Confidence > bestConf {
			bestConf = a.Confidence
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return 0, 0, false
	}
	a := attrs[bestIdx]
	return a.TokenStart, a.TokenEnd, true
}

func bestTailGroupAfterAnchor(attrs []model.Attribute, anchor int) (string, int, bool) {
	best := ""
	bestIdx := -1
	bestConf := -1.0
	for _, a := range attrs {
		if a.Type != model.AttrTailGroup {
			continue
		}
		n := normalizeGroupText(strings.TrimSpace(a.Normalized))
		if n == "" {
			continue
		}
		if anchor >= 0 && a.TokenStart >= 0 && a.TokenStart <= anchor {
			continue
		}
		if anchor >= 0 && a.TokenStart < 0 {
			continue
		}
		if a.Confidence > bestConf {
			bestConf = a.Confidence
			best = n
			bestIdx = a.TokenStart
		}
	}
	if best == "" {
		return "", -1, false
	}
	return best, bestIdx, true
}

func attrsWithSingleGroup(attrs []model.Attribute, group string, tokenStart int, tokenEnd int, source string) []model.Attribute {
	group = strings.TrimSpace(group)
	out := make([]model.Attribute, 0, len(attrs)+1)
	for _, a := range attrs {
		if a.Type == model.AttrGroup {
			continue
		}
		out = append(out, a)
	}
	if group == "" {
		return out
	}
	out = append(out, model.Attribute{
		Type:       model.AttrGroup,
		Normalized: group,
		TokenStart: tokenStart,
		TokenEnd:   tokenEnd,
		Confidence: 0.8,
		Source:     source,
	})
	return out
}

func dropAttrType(attrs []model.Attribute, t model.AttrType) []model.Attribute {
	out := make([]model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if a.Type == t {
			continue
		}
		out = append(out, a)
	}
	return out
}

func spansOverlap(aStart int, aEnd int, bStart int, bEnd int) bool {
	if aEnd <= aStart || bEnd <= bStart {
		return false
	}
	return aStart < bEnd && bStart < aEnd
}

func maxSignalIdx(vals ...int) int {
	best := -1
	for _, v := range vals {
		if v > best {
			best = v
		}
	}
	return best
}

func weakEpisodeCandidate(tokens []model.Token, episodeIdx int, titleStart int, titleEnd int, hasTitleSpan bool) bool {
	if episodeIdx < 0 || episodeIdx >= len(tokens) {
		return false
	}
	if hasTitleSpan && episodeIdx >= titleStart && episodeIdx < titleEnd {
		return true
	}
	t := strings.TrimSpace(tokens[episodeIdx].Text)
	if t == "" {
		return true
	}
	if hasAlphaOrCJK(t) && !templateSignalSxxExx.MatchString(t) && !templateSignalJPEP.MatchString(t) {
		return true
	}
	if templateSignalDigits.MatchString(t) {
		if episodeIdx+1 < len(tokens) {
			next := strings.TrimSpace(tokens[episodeIdx+1].Text)
			if hasAlphaOrCJK(next) {
				return true
			}
		}
	}
	return false
}

func findStrongBracketEpisodeAnchor(tokens []model.Token, currentIdx int) (string, string, int, bool) {
	for i := currentIdx + 1; i < len(tokens); i++ {
		if !isInBracket(tokens, i) {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		if m := templateSignalSxxExx.FindStringSubmatch(t); len(m) == 3 {
			return trimLeadingZerosSignal(m[2]), trimLeadingZerosSignal(m[1]), i, true
		}
		if m := templateSignalJPEP.FindStringSubmatch(t); len(m) == 2 {
			return trimLeadingZerosSignal(m[1]), "", i, true
		}
		if templateSignalDigits.MatchString(t) {
			return trimLeadingZerosSignal(t), "", i, true
		}
	}
	return "", "", -1, false
}

func trimLeadingZerosSignal(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for len(s) > 1 && strings.HasPrefix(s, "0") {
		s = s[1:]
	}
	return s
}

func hasAlphaOrCJK(s string) bool {
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return true
		}
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
		if r >= 0x3040 && r <= 0x30FF {
			return true
		}
	}
	return false
}

func uniqueSignalEvents(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
