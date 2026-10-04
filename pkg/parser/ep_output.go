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
	if !regexp.MustCompile(`(?i)\b(?:and|&|\+|plus)\b`).MatchString(tail) {
		return false
	}
	return regexp.MustCompile(`\b\d{1,4}\b`).MatchString(tail)
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

func ensureSPPrimaryFromSPKind(epslot *model.EPSlot, season string, episode string, episodeRange string, episodeList string, spKind string) *model.EPSlot {
	k := strings.ToUpper(strings.TrimSpace(spKind))
	if k == "" {
		return epslot
	}
	if strings.TrimSpace(episode) != "" || strings.TrimSpace(episodeRange) != "" || strings.TrimSpace(episodeList) != "" {
		return epslot
	}
	if epslot == nil {
		e := model.EPEntity{
			Kind:   "SP",
			Axis:   k,
			Season: season,
			SPKind: k,
			Source: "sp_kind_final",
		}
		return &model.EPSlot{
			Primary:    &e,
			Candidates: []model.EPEntity{{Kind: "SP", Axis: k, Season: season, SPKind: k, Source: "sp_kind"}},
		}
	}
	if epslot.Primary == nil {
		e := model.EPEntity{
			Kind:   "SP",
			Axis:   k,
			Season: season,
			SPKind: k,
			Source: "sp_kind_final",
		}
		epslot.Primary = &e
	}
	hasSPCandidate := false
	for _, c := range epslot.Candidates {
		if strings.EqualFold(strings.TrimSpace(c.Kind), "SP") && strings.EqualFold(strings.TrimSpace(c.SPKind), k) {
			hasSPCandidate = true
			break
		}
	}
	if !hasSPCandidate {
		epslot.Candidates = append(epslot.Candidates, model.EPEntity{
			Kind:   "SP",
			Axis:   k,
			Season: season,
			SPKind: k,
			Source: "sp_kind",
		})
	}
	return epslot
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

func isUnknownOutput(contentKind string, templateID string) bool {
	if strings.EqualFold(strings.TrimSpace(contentKind), "UNKNOWN") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(templateID), "TF_UNKNOWN_DEFAULT")
}

func normalizeContentKindByTemplateID(contentKind string, templateID string) string {
	tid := strings.TrimSpace(templateID)
	if tid == "" {
		return contentKind
	}
	cur := strings.ToUpper(strings.TrimSpace(contentKind))
	switch {
	case strings.HasPrefix(tid, "TF_BATCH_"):
		return "BATCH"
	case strings.HasPrefix(tid, "TF_MOVIE_"):
		if cur == "" || cur == "UNKNOWN" || cur == "TV_EP" {
			return "MOVIE"
		}
		return contentKind
	case strings.HasPrefix(tid, "TF_SPECIAL_"), strings.HasPrefix(tid, "TF_SP_"):
		if cur == "" || cur == "UNKNOWN" {
			return "SPECIAL"
		}
		return contentKind
	default:
		return contentKind
	}
}

func pruneUnknownCoreAttributes(attrs []model.Attribute) []model.Attribute {
	if len(attrs) == 0 {
		return attrs
	}
	out := make([]model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if !keepUnknownOutputAttribute(a) {
			continue
		}
		if strings.TrimSpace(a.Normalized) == "" {
			continue
		}
		out = append(out, a)
	}
	return dedupeAttributes(out)
}

func keepUnknownOutputAttribute(a model.Attribute) bool {
	switch a.Type {
	case model.AttrGroup, model.AttrTitle, model.AttrTitleAlias,
		model.AttrEdition, model.AttrReleaseStatus,
		model.AttrSourceType, model.AttrResolution, model.AttrVideoCodec,
		model.AttrAudioCodec, model.AttrBitDepth, model.AttrContainer,
		model.AttrPlatform, model.AttrPlatformRegion, model.AttrOriginRegion,
		model.AttrHDR, model.AttrCRC, model.AttrSubFlag, model.AttrSubLang,
		model.AttrSubFormat, model.AttrSubLangGroup, model.AttrAudioFlag,
		model.AttrAudioLang, model.AttrAudioChannels, model.AttrAudioObject:
		return true
	case model.AttrNote:
		return keepUnknownOutputNote(a.Normalized)
	default:
		return false
	}
}

func keepUnknownOutputNote(normalized string) bool {
	up := strings.ToUpper(strings.TrimSpace(normalized))
	switch up {
	case "RETAIL", "BOOTLEG", "USBD", "JPBD", "UHD", "BONUS_VIDEO", "RECRUITMENT":
		return true
	default:
		return false
	}
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

func pruneGroupSpanTechArtifacts(attrs []model.Attribute, groupSpan [2]int) []model.Attribute {
	if len(attrs) == 0 || groupSpan[1] <= groupSpan[0] {
		return attrs
	}
	isGroupSpan := func(a model.Attribute) bool {
		return a.TokenStart >= groupSpan[0] && a.TokenEnd > a.TokenStart && a.TokenEnd <= groupSpan[1]
	}
	shouldPrune := func(a model.Attribute) bool {
		if !isGroupSpan(a) {
			return false
		}
		switch a.Type {
		case model.AttrSubFlag,
			model.AttrSubLang,
			model.AttrSubFormat,
			model.AttrSubLangGroup,
			model.AttrAudioLang,
			model.AttrAudioFlag,
			model.AttrAudioCodec,
			model.AttrAudioChannels,
			model.AttrAudioObject,
			model.AttrSourceType,
			model.AttrResolution,
			model.AttrVideoCodec,
			model.AttrContainer,
			model.AttrPlatform,
			model.AttrTech:
			return true
		default:
			return false
		}
	}
	out := make([]model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if shouldPrune(a) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func audioCodecSpecificityRank(norm string) int {
	n := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(norm), "-", ""))
	switch n {
	case "DTSHDMA":
		return 5
	case "DTSHD", "TRUEHD":
		return 4
	case "EAC3":
		return 3
	case "DTS", "AC3", "AAC", "FLAC", "OPUS", "PCM", "MP3":
		return 2
	default:
		return 1
	}
}

func collapseOverlappingSpecificAudioCodecs(attrs []model.Attribute) []model.Attribute {
	if len(attrs) <= 1 {
		return attrs
	}
	drop := make([]bool, len(attrs))
	for i, a := range attrs {
		if a.Type != model.AttrAudioCodec {
			continue
		}
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart {
			continue
		}
		ra := audioCodecSpecificityRank(a.Normalized)
		for j, b := range attrs {
			if i == j || b.Type != model.AttrAudioCodec {
				continue
			}
			if b.TokenStart < 0 || b.TokenEnd <= b.TokenStart {
				continue
			}
			rb := audioCodecSpecificityRank(b.Normalized)
			if rb <= ra {
				continue
			}
			if b.TokenStart <= a.TokenStart && b.TokenEnd >= a.TokenEnd {
				drop[i] = true
				break
			}
		}
	}
	kept := make([]model.Attribute, 0, len(attrs))
	for i, a := range attrs {
		if drop[i] {
			continue
		}
		kept = append(kept, a)
	}
	return kept
}

func reconcileAudioSlotWithAttributes(slot *model.AudioSlot, attrs []model.Attribute) *model.AudioSlot {
	if slot == nil || len(slot.Tracks) == 0 || len(attrs) == 0 {
		return slot
	}
	canonCodec := func(s string) string {
		return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", ""))
	}
	audioAttrs := make([]model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if a.Type != model.AttrAudioCodec {
			continue
		}
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart {
			continue
		}
		audioAttrs = append(audioAttrs, a)
	}
	if len(audioAttrs) == 0 {
		return slot
	}
	out := make([]model.AudioTrack, 0, len(slot.Tracks))
	for _, tr := range slot.Tracks {
		if tr.TokenStart < 0 || tr.TokenEnd <= tr.TokenStart {
			out = append(out, tr)
			continue
		}
		keep := true
		trackRank := audioCodecSpecificityRank(tr.Codec)
		for _, a := range audioAttrs {
			if a.TokenStart <= tr.TokenStart && a.TokenEnd >= tr.TokenEnd && audioCodecSpecificityRank(a.Normalized) > trackRank {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, tr)
		}
	}
	for _, a := range audioAttrs {
		found := false
		for _, tr := range out {
			if canonCodec(tr.Codec) == canonCodec(a.Normalized) && tr.TokenStart == a.TokenStart && tr.TokenEnd == a.TokenEnd {
				found = true
				break
			}
		}
		if found {
			continue
		}
		out = append(out, model.AudioTrack{
			Codec:      a.Normalized,
			TokenStart: a.TokenStart,
			TokenEnd:   a.TokenEnd,
			Source:     "audio_slot_reconcile",
		})
	}
	if len(out) == 0 {
		return nil
	}
	slot.Tracks = out
	return slot
}

func (p *Parser) promoteNoteAttributes(attrs []model.Attribute) []model.Attribute {
	if p == nil {
		return attrs
	}
	if len(p.notePromotionRules) > 0 {
		for i := range attrs {
			if attrs[i].Type != model.AttrNote {
				continue
			}
			n := strings.TrimSpace(attrs[i].Normalized)
			if n == "" {
				continue
			}
			low := strings.ToLower(n)
			for _, rule := range p.notePromotionRules {
				if _, ok := rule.values[low]; !ok {
					continue
				}
				attrs[i].Type = rule.targetType
				if rule.normalizedFromToken {
					switch rule.targetType {
					case model.AttrPlatform:
						attrs[i].Normalized = p.normalizePlatformCode(n)
					case model.AttrSourceType:
						attrs[i].Normalized = p.normalizeSourceType(n)
					default:
						attrs[i].Normalized = strings.TrimSpace(strings.ToUpper(n))
					}
				} else if strings.TrimSpace(rule.normalized) != "" {
					attrs[i].Normalized = strings.TrimSpace(rule.normalized)
				}
				if attrs[i].Source == "" {
					attrs[i].Source = "note_promote_dict"
				}
				break
			}
		}
	}
	// Contextual promotion: ESP is ambiguous between audio/subtitle language.
	// Only promote to subtitle language when subtitle signal already exists.
	hasSubSignal := false
	for _, a := range attrs {
		if a.Type == model.AttrSubFlag {
			n := strings.ToUpper(strings.TrimSpace(a.Normalized))
			if n == "SUBS" || n == "MULTI_SUBS" || n == "CC" || n == "SOFT" || n == "HARD" || n == "EX" {
				hasSubSignal = true
				break
			}
		}
	}
	if hasSubSignal {
		for i := range attrs {
			if attrs[i].Type != model.AttrNote {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(attrs[i].Normalized), "ESP") {
				continue
			}
			attrs[i].Type = model.AttrSubLang
			attrs[i].Normalized = "SPA"
			if attrs[i].Source == "" {
				attrs[i].Source = "note_promote_context"
			}
		}
	}
	// Composite subtitle/audio notes -> structured language attributes.
	hasAttr := func(typ model.AttrType, norm string) bool {
		for _, a := range attrs {
			if a.Type != typ {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(a.Normalized), strings.TrimSpace(norm)) {
				return true
			}
		}
		return false
	}
	appendAttr := func(typ model.AttrType, norm string, ref model.Attribute) {
		if hasAttr(typ, norm) {
			return
		}
		attrs = append(attrs, model.Attribute{
			Type:       typ,
			Normalized: norm,
			TokenStart: ref.TokenStart,
			TokenEnd:   ref.TokenEnd,
			Confidence: ref.Confidence,
			Source:     "note_promote_context",
		})
	}
	for i := range attrs {
		if attrs[i].Type != model.AttrNote {
			continue
		}
		n := strings.ToUpper(strings.TrimSpace(attrs[i].Normalized))
		switch n {
		case "CHS_SUBS":
			attrs[i].Type = model.AttrSubLang
			attrs[i].Normalized = "SC"
			appendAttr(model.AttrSubFlag, "soft", attrs[i])
		case "JPN_AUDIO_CHS_SUBS":
			attrs[i].Type = model.AttrAudioLang
			attrs[i].Normalized = "JPN"
			appendAttr(model.AttrSubLang, "SC", attrs[i])
			appendAttr(model.AttrSubFlag, "soft", attrs[i])
		case "ENG_AUDIO_CHS_SUBS":
			attrs[i].Type = model.AttrAudioLang
			attrs[i].Normalized = "ENG"
			appendAttr(model.AttrSubLang, "SC", attrs[i])
			appendAttr(model.AttrSubFlag, "soft", attrs[i])
		case "CHN_AUDIO_CHS_SUBS":
			attrs[i].Type = model.AttrAudioLang
			attrs[i].Normalized = "CHN"
			appendAttr(model.AttrSubLang, "SC", attrs[i])
			appendAttr(model.AttrSubFlag, "soft", attrs[i])
		case "CHN_YUE_AUDIO":
			attrs[i].Type = model.AttrAudioLang
			attrs[i].Normalized = "CHN"
			appendAttr(model.AttrAudioLang, "YUE", attrs[i])
			appendAttr(model.AttrAudioFlag, "DUAL_AUDIO", attrs[i])
		case "CHN_YUE_AUDIO_SUBS":
			attrs[i].Type = model.AttrAudioLang
			attrs[i].Normalized = "CHN"
			appendAttr(model.AttrAudioLang, "YUE", attrs[i])
			appendAttr(model.AttrAudioFlag, "DUAL_AUDIO", attrs[i])
			appendAttr(model.AttrSubFlag, "soft", attrs[i])
		case "CHT_HARDSUB":
			attrs[i].Type = model.AttrSubLang
			attrs[i].Normalized = "TC"
			appendAttr(model.AttrSubFlag, "hard", attrs[i])
		}
	}
	return attrs
}

var seasonNotePattern = regexp.MustCompile(`(?i)^season\s*0*(\d{1,2})$`)
var seasonRangeCompactNotePattern = regexp.MustCompile(`(?i)^s0*(\d{1,2})\s*[-~]\s*s?0*(\d{1,2})$`)
var seasonRangeNotePattern = regexp.MustCompile(`(?i)^season\s*0*(\d{1,2})\s*[-~]\s*0*(\d{1,2})$`)

func normalizeMetaCompareKey(v string) string {
	s := strings.ToUpper(strings.TrimSpace(v))
	if s == "" {
		return ""
	}
	repl := strings.NewReplacer(
		" ", "",
		"-", "",
		"_", "",
		".", "",
		"／", "/",
		"/", "",
		"·", "",
		":", "",
		"：", "",
	)
	return repl.Replace(s)
}

func dropRedundantNoteAttributes(attrs []model.Attribute, season string, seasonRange string) []model.Attribute {
	if len(attrs) == 0 {
		return attrs
	}
	hasPlatform := map[string]struct{}{}
	hasResolution := map[string]struct{}{}
	hasSourceType := map[string]struct{}{}
	packKindPresent := false
	movieSignalPresent := false
	canonicalSeason := normalizeSeasonMarkerValue(season)
	canonicalSeasonRange := normalizeSeasonRangeMarkerValue(seasonRange)

	for _, a := range attrs {
		n := strings.TrimSpace(a.Normalized)
		if n == "" {
			continue
		}
		switch a.Type {
		case model.AttrPlatform:
			hasPlatform[normalizeMetaCompareKey(n)] = struct{}{}
		case model.AttrResolution:
			hasResolution[normalizeMetaCompareKey(n)] = struct{}{}
		case model.AttrSourceType:
			hasSourceType[normalizeMetaCompareKey(n)] = struct{}{}
		case model.AttrPackKind:
			packKindPresent = true
		case model.AttrContents:
			if strings.EqualFold(strings.TrimSpace(n), "MOVIE") {
				movieSignalPresent = true
			}
		case model.AttrMovieFlag:
			movieSignalPresent = true
		}
	}

	out := make([]model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if a.Type != model.AttrNote {
			out = append(out, a)
			continue
		}
		n := strings.TrimSpace(a.Normalized)
		if n == "" {
			continue
		}
		nk := normalizeMetaCompareKey(n)
		up := strings.ToUpper(n)

		if _, ok := hasPlatform[nk]; ok {
			continue
		}
		if _, ok := hasResolution[nk]; ok {
			continue
		}
		if _, ok := hasSourceType[nk]; ok {
			continue
		}
		if canonicalSeason != "" && normalizeSeasonMarkerValue(n) == canonicalSeason {
			continue
		}
		if canonicalSeasonRange != "" && normalizeSeasonRangeMarkerValue(n) == canonicalSeasonRange {
			continue
		}
		if packKindPresent {
			if up == "TV_PACK" || up == "BATCH" || n == "全集" || n == "合集" {
				continue
			}
		}
		if movieSignalPresent && up == "MOVIE" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func normalizeSeasonMarkerValue(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}
	if v := parseSmallSeasonNumberToken(s); v != "" {
		return normalizeNonZeroNumberString(v)
	}
	if v := seasonTokenValue(s); v != "" {
		return normalizeNonZeroNumberString(v)
	}
	if m := seasonNotePattern.FindStringSubmatch(s); len(m) == 2 {
		return normalizeNonZeroNumberString(m[1])
	}
	return ""
}

func normalizeSeasonRangeMarkerValue(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}
	if m := seasonRangeCompactNotePattern.FindStringSubmatch(s); len(m) == 3 {
		a := normalizeNonZeroNumberString(m[1])
		b := normalizeNonZeroNumberString(m[2])
		if a != "" && b != "" {
			return "S" + a + "-S" + b
		}
		return ""
	}
	if m := seasonRangeNotePattern.FindStringSubmatch(s); len(m) == 3 {
		a := normalizeNonZeroNumberString(m[1])
		b := normalizeNonZeroNumberString(m[2])
		if a != "" && b != "" {
			return "S" + a + "-S" + b
		}
		return ""
	}
	return ""
}

func normalizeNonZeroNumberString(text string) string {
	s := strings.TrimLeft(strings.TrimSpace(text), "0")
	if s == "" {
		return "0"
	}
	return s
}

func extractExplicitSeasonNumber(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := regexp.MustCompile(`(?i)\bS(\d{1,2})\b`).FindStringSubmatch(s); len(m) == 2 {
		v := strings.TrimLeft(m[1], "0")
		if v == "" {
			v = m[1]
		}
		return v
	}
	if m := regexp.MustCompile(`(?i)\bSeason\s*(\d{1,2})\b`).FindStringSubmatch(s); len(m) == 2 {
		v := strings.TrimLeft(m[1], "0")
		if v == "" {
			v = m[1]
		}
		return v
	}
	if m := regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})季$`).FindStringSubmatch(s); len(m) == 2 {
		if v := parseSmallSeasonNumberToken(m[1]); v != "" {
			return v
		}
	}
	return ""
}
