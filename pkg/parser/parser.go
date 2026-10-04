package parser

import (
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"

	"reltitle/pkg/config"
	"reltitle/pkg/model"
	"reltitle/pkg/tokenize"
)

type Parser struct {
	cfg                   *config.Config
	groupIndex            map[string]*config.GroupDef // 字幕组索引: normalized pattern -> GroupDef
	aliasInlineStripWords map[string]struct{}
	aliasNoiseExactWords  map[string]struct{}
	aliasNoisePatterns    []*regexp.Regexp
	notePromotionRules    []notePromotionRule
	groupRouteRules       []groupRouteRule
	groupSlot2Rules       []groupSlot2PlatformRule
	packProfileRules      []packProfileRule
	sourceTypeCanonical   map[string]string
	platformCanonical     map[string]string
	videoCodecCanonical   map[string]string
	audioCodecCanonical   map[string]string
	containerCanonical    map[string]string
	audioFlagCanonical    map[string]string
	subFlagCanonical      map[string]string
	subLangCanonical      map[string]string
	audioLangCanonical    map[string]string
	packKindSourceType    map[string]string
}

type notePromotionRule struct {
	values              map[string]struct{}
	targetType          model.AttrType
	normalized          string
	normalizedFromToken bool
}

type groupRouteRule struct {
	id                      string
	priority                int
	groups                  map[string]struct{}
	rawMustContainAny       []string
	requireEpisodeRange     bool
	requireEpisodeLike      bool
	requireEpisodeWithTitle bool
	episodeTitleSepIn       map[string]struct{}
	forceGroup              string
	forceTitle              string
	clearAliases            bool
	forceTemplateID         string
	forceContentKind        string
	forceRouteID            string
	titleStripPrefixes      []string
	aliasStripPrefixes      []string
}

type groupSlot2PlatformRule struct {
	id                        string
	priority                  int
	groups                    map[string]struct{}
	groupRegex                *regexp.Regexp
	slotIndex                 int
	yueKeywords               []string
	distributorKeywords       []string
	languageMetaKeywords      []string
	trimForInfer              []string
	platformWhitelist         []groupSlot2PlatformWhitelist
	emitAudioLang             string
	noteOnDistributor         string
	inferUnknownPlatform      bool
	unknownPlatformConfidence float64
}

type groupSlot2PlatformWhitelist struct {
	containsAny []string
	normalized  string
}

type packProfileRule struct {
	id                  string
	priority            int
	emit                string
	contentKindIn       map[string]struct{}
	packKindIn          map[string]struct{}
	packKindNotIn       map[string]struct{}
	requiresContentsAll map[string]struct{}
	requiresContentsAny map[string]struct{}
	forbidContentsAny   map[string]struct{}
	hasSeason           *bool
	hasSeasonRange      *bool
	minContentsDistinct int
}

var wxhResolutionPattern = regexp.MustCompile(`(?i)^(\d{2,5})\s*[x×X]\s*(\d{2,5})(?:\s*([ip]))?$`)

func New(cfg *config.Config) *Parser {
	if cfg == nil {
		cfg = &config.Config{}
	}
	p := &Parser{
		cfg:                   cfg,
		groupIndex:            make(map[string]*config.GroupDef),
		aliasInlineStripWords: make(map[string]struct{}),
		aliasNoiseExactWords:  make(map[string]struct{}),
		sourceTypeCanonical:   buildTypeCanonicalMap(cfg, "source_type"),
		platformCanonical:     buildTypeCanonicalMap(cfg, "platform"),
		videoCodecCanonical:   buildTypeCanonicalMap(cfg, "video_codec"),
		audioCodecCanonical:   buildTypeCanonicalMap(cfg, "audio_codec"),
		containerCanonical:    buildTypeCanonicalMap(cfg, "container"),
		audioFlagCanonical:    buildTypeCanonicalMap(cfg, "audio_flag"),
		subFlagCanonical:      buildTypeCanonicalMap(cfg, "sub_flag"),
		subLangCanonical:      buildTypeCanonicalMap(cfg, "sub_lang"),
		audioLangCanonical:    buildTypeCanonicalMap(cfg, "audio_lang"),
		packKindSourceType:    buildPackKindSourceTypeMap(cfg),
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

	for _, w := range cfg.DictAliasNoiseWords.AliasNoise.InlineStripWordsCaseInsensitive {
		ww := strings.ToLower(strings.TrimSpace(w))
		if ww == "" {
			continue
		}
		p.aliasInlineStripWords[ww] = struct{}{}
	}
	for _, w := range cfg.DictAliasNoiseWords.AliasNoise.ExactWordsCaseInsensitive {
		ww := strings.ToLower(strings.TrimSpace(w))
		if ww == "" {
			continue
		}
		p.aliasNoiseExactWords[ww] = struct{}{}
	}
	for _, pat := range cfg.DictAliasNoiseWords.AliasNoise.RegexPatterns {
		pp := strings.TrimSpace(pat)
		if pp == "" {
			continue
		}
		re, err := regexp.Compile(pp)
		if err != nil {
			continue
		}
		p.aliasNoisePatterns = append(p.aliasNoisePatterns, re)
	}

	for _, r := range cfg.DictNotePromotion.Rules {
		targetType := model.AttrType(strings.TrimSpace(r.PromoteTo.Type))
		if targetType == "" {
			continue
		}
		vals := make(map[string]struct{})
		for _, v := range r.Match.NoteValuesCaseInsensitive {
			vv := strings.ToLower(strings.TrimSpace(v))
			if vv == "" {
				continue
			}
			vals[vv] = struct{}{}
		}
		if len(vals) == 0 {
			continue
		}
		p.notePromotionRules = append(p.notePromotionRules, notePromotionRule{
			values:              vals,
			targetType:          targetType,
			normalized:          strings.TrimSpace(r.PromoteTo.Normalized),
			normalizedFromToken: r.PromoteTo.NormalizedFromToken,
		})
	}

	for _, r := range cfg.GroupRoute.Overrides {
		if !r.Enabled {
			continue
		}
		id := strings.TrimSpace(r.ID)
		if id == "" {
			continue
		}
		groupSet := make(map[string]struct{}, len(r.Match.GroupIn))
		for _, g := range r.Match.GroupIn {
			gg := strings.ToLower(strings.TrimSpace(g))
			if gg == "" {
				continue
			}
			groupSet[gg] = struct{}{}
		}
		rawContains := make([]string, 0, len(r.Match.RawMustContainAny))
		for _, s := range r.Match.RawMustContainAny {
			ss := strings.ToLower(strings.TrimSpace(s))
			if ss == "" {
				continue
			}
			rawContains = append(rawContains, ss)
		}
		titleStripPrefixes := make([]string, 0, len(r.Output.TitleStripPrefixes))
		for _, s := range r.Output.TitleStripPrefixes {
			ss := strings.TrimSpace(s)
			if ss == "" {
				continue
			}
			titleStripPrefixes = append(titleStripPrefixes, ss)
		}
		aliasStripPrefixes := make([]string, 0, len(r.Output.AliasStripPrefixes))
		for _, s := range r.Output.AliasStripPrefixes {
			ss := strings.TrimSpace(s)
			if ss == "" {
				continue
			}
			aliasStripPrefixes = append(aliasStripPrefixes, ss)
		}
		episodeTitleSepIn := map[string]struct{}{}
		for _, s := range r.Match.EpisodeTitleSepIn {
			if sep := normalizeEpisodeTitleSepConstraint(s); sep != "" {
				episodeTitleSepIn[sep] = struct{}{}
			}
		}
		p.groupRouteRules = append(p.groupRouteRules, groupRouteRule{
			id:                      id,
			priority:                r.Priority,
			groups:                  groupSet,
			rawMustContainAny:       rawContains,
			requireEpisodeRange:     r.Match.RequireEpisodeRange,
			requireEpisodeLike:      r.Match.RequireEpisodeLike,
			requireEpisodeWithTitle: r.Match.RequireEpisodeWithTitle,
			episodeTitleSepIn:       episodeTitleSepIn,
			forceGroup:              strings.TrimSpace(r.Output.ForceGroup),
			forceTitle:              strings.TrimSpace(r.Output.ForceTitle),
			clearAliases:            r.Output.ClearAliases,
			forceTemplateID:         strings.TrimSpace(r.Output.ForceTemplateID),
			forceContentKind:        strings.TrimSpace(r.Output.ForceContentKind),
			forceRouteID:            strings.TrimSpace(r.Output.ForceRouteID),
			titleStripPrefixes:      titleStripPrefixes,
			aliasStripPrefixes:      aliasStripPrefixes,
		})
	}
	sort.SliceStable(p.groupRouteRules, func(i, j int) bool {
		return p.groupRouteRules[i].priority > p.groupRouteRules[j].priority
	})

	for _, r := range cfg.GroupSlot2Platform.Rules {
		if !r.Enabled {
			continue
		}
		id := strings.TrimSpace(r.ID)
		if id == "" {
			continue
		}
		groupSet := make(map[string]struct{}, len(r.Match.GroupIn))
		for _, g := range r.Match.GroupIn {
			gg := strings.ToLower(strings.TrimSpace(g))
			if gg == "" {
				continue
			}
			groupSet[gg] = struct{}{}
		}
		var groupRegex *regexp.Regexp
		if ss := strings.TrimSpace(r.Match.GroupRegex); ss != "" {
			if re, err := regexp.Compile(ss); err == nil {
				groupRegex = re
			}
		}
		slotIndex := r.Match.SlotIndex
		if slotIndex <= 0 {
			slotIndex = 2
		}
		toLowerList := func(in []string) []string {
			out := make([]string, 0, len(in))
			for _, s := range in {
				ss := strings.ToLower(strings.TrimSpace(s))
				if ss == "" {
					continue
				}
				out = append(out, ss)
			}
			return out
		}
		yueKeywords := toLowerList(r.Keywords.Yue)
		distributorKeywords := toLowerList(r.Keywords.Distributor)
		languageMetaKeywords := toLowerList(r.Keywords.LanguageMeta)
		trimForInfer := toLowerList(r.Keywords.TrimForInfer)
		if len(yueKeywords) == 0 {
			yueKeywords = []string{"粤语", "粵語", "cantonese"}
		}
		if len(distributorKeywords) == 0 {
			distributorKeywords = []string{"代理商", "發行商", "发行商"}
		}
		platformWhitelist := make([]groupSlot2PlatformWhitelist, 0, len(r.Whitelist))
		for _, w := range r.Whitelist {
			norm := strings.TrimSpace(w.Normalized)
			if norm == "" {
				continue
			}
			containsAny := toLowerList(w.ContainsAny)
			if len(containsAny) == 0 {
				continue
			}
			platformWhitelist = append(platformWhitelist, groupSlot2PlatformWhitelist{
				containsAny: containsAny,
				normalized:  norm,
			})
		}
		emitAudioLang := strings.TrimSpace(r.Output.EmitAudioLang)
		if emitAudioLang == "" {
			emitAudioLang = "YUE"
		}
		noteOnDistributor := strings.TrimSpace(r.Output.NoteOnDistributor)
		if noteOnDistributor == "" {
			noteOnDistributor = "CHANNEL_DISTRIBUTOR"
		}
		unknownConf := r.Output.UnknownPlatformConfidence
		if unknownConf <= 0 {
			unknownConf = 0.6
		}
		p.groupSlot2Rules = append(p.groupSlot2Rules, groupSlot2PlatformRule{
			id:                        id,
			priority:                  r.Priority,
			groups:                    groupSet,
			groupRegex:                groupRegex,
			slotIndex:                 slotIndex,
			yueKeywords:               yueKeywords,
			distributorKeywords:       distributorKeywords,
			languageMetaKeywords:      languageMetaKeywords,
			trimForInfer:              trimForInfer,
			platformWhitelist:         platformWhitelist,
			emitAudioLang:             emitAudioLang,
			noteOnDistributor:         noteOnDistributor,
			inferUnknownPlatform:      r.Output.InferUnknownPlatform,
			unknownPlatformConfidence: unknownConf,
		})
	}
	sort.SliceStable(p.groupSlot2Rules, func(i, j int) bool {
		return p.groupSlot2Rules[i].priority > p.groupSlot2Rules[j].priority
	})

	toUpperSet := func(in []string) map[string]struct{} {
		out := make(map[string]struct{}, len(in))
		for _, s := range in {
			ss := strings.ToUpper(strings.TrimSpace(s))
			if ss == "" {
				continue
			}
			out[ss] = struct{}{}
		}
		return out
	}
	toLowerSet := func(in []string) map[string]struct{} {
		out := make(map[string]struct{}, len(in))
		for _, s := range in {
			ss := strings.ToLower(strings.TrimSpace(s))
			if ss == "" {
				continue
			}
			out[ss] = struct{}{}
		}
		return out
	}
	for _, r := range cfg.PackProfileRules.Rules {
		if !r.Enabled {
			continue
		}
		id := strings.TrimSpace(r.ID)
		emit := strings.TrimSpace(r.Emit)
		if id == "" || emit == "" {
			continue
		}
		minDistinct := r.When.MinContentsDistinct
		if minDistinct < 0 {
			minDistinct = 0
		}
		p.packProfileRules = append(p.packProfileRules, packProfileRule{
			id:                  id,
			priority:            r.Priority,
			emit:                emit,
			contentKindIn:       toUpperSet(r.When.ContentKindIn),
			packKindIn:          toLowerSet(r.When.PackKindIn),
			packKindNotIn:       toLowerSet(r.When.PackKindNotIn),
			requiresContentsAll: toUpperSet(r.When.RequiresContentsAll),
			requiresContentsAny: toUpperSet(r.When.RequiresContentsAny),
			forbidContentsAny:   toUpperSet(r.When.ForbidContentsAny),
			hasSeason:           r.When.HasSeason,
			hasSeasonRange:      r.When.HasSeasonRange,
			minContentsDistinct: minDistinct,
		})
	}
	sort.SliceStable(p.packProfileRules, func(i, j int) bool {
		return p.packProfileRules[i].priority > p.packProfileRules[j].priority
	})

	return p
}

func selectGroupFromAttrs(attrs []model.Attribute) (string, [2]int, bool) {
	best := ""
	span := [2]int{0, 0}
	bestScore := -1
	for _, a := range attrs {
		if a.Type != model.AttrGroup {
			continue
		}
		g := strings.TrimSpace(a.Normalized)
		if g == "" {
			continue
		}
		score := 0
		if strings.EqualFold(strings.TrimSpace(a.Source), "lexicon") {
			score += 100
		}
		if a.Confidence > 0 {
			score += int(a.Confidence * 10)
		}
		if score > bestScore {
			bestScore = score
			best = g
			span = [2]int{a.TokenStart, a.TokenEnd}
		}
	}
	if bestScore < 0 {
		return "", [2]int{0, 0}, false
	}
	return best, span, true
}

func hasGroupAttr(attrs []model.Attribute, group string) bool {
	group = strings.TrimSpace(group)
	if group == "" {
		return false
	}
	for _, a := range attrs {
		if a.Type != model.AttrGroup {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(a.Normalized), group) {
			return true
		}
	}
	return false
}

func buildTypeCanonicalMap(cfg *config.Config, targetType string) map[string]string {
	out := make(map[string]string)
	if cfg == nil {
		return out
	}
	targetKey := strings.ToLower(strings.TrimSpace(targetType))
	if targetKey == "" {
		return out
	}
	for _, r := range cfg.DictTokenSynonyms.Rules {
		if strings.ToLower(strings.TrimSpace(r.Emit.Type)) != targetKey {
			continue
		}
		norm := strings.TrimSpace(r.Emit.Normalized)
		if norm == "" {
			continue
		}
		out[strings.ToLower(norm)] = norm
		for _, raw := range r.RawTokens {
			rr := strings.ToLower(strings.TrimSpace(raw))
			if rr == "" {
				continue
			}
			out[rr] = norm
		}
	}
	for _, canonical := range readStringSlice(cfg.DictCanonicalValues.Canonical[targetKey]) {
		cc := strings.TrimSpace(canonical)
		if cc == "" {
			continue
		}
		key := strings.ToLower(cc)
		if _, exists := out[key]; !exists {
			out[key] = cc
		}
	}
	for raw, canonical := range readNestedStringMap(cfg.DictCanonicalValues.Canonical, "mappings", targetKey+"_aliases") {
		rr := strings.ToLower(strings.TrimSpace(raw))
		cc := strings.TrimSpace(canonical)
		if rr == "" || cc == "" {
			continue
		}
		out[rr] = cc
		if _, exists := out[strings.ToLower(cc)]; !exists {
			out[strings.ToLower(cc)] = cc
		}
	}
	return out
}

func buildPackKindSourceTypeMap(cfg *config.Config) map[string]string {
	out := make(map[string]string)
	if cfg == nil {
		return out
	}
	for raw, canonical := range readNestedStringMap(cfg.DictCanonicalValues.Canonical, "mappings", "pack_kind_to_source_type") {
		rr := strings.ToLower(strings.TrimSpace(raw))
		cc := strings.TrimSpace(canonical)
		if rr == "" || cc == "" {
			continue
		}
		out[rr] = cc
	}
	return out
}

func readNestedStringMap(root map[string]any, path ...string) map[string]string {
	if len(path) == 0 {
		return map[string]string{}
	}
	cur := mapStringAny(root)
	for i, key := range path {
		if cur == nil {
			return map[string]string{}
		}
		v, ok := cur[key]
		if !ok {
			return map[string]string{}
		}
		if i == len(path)-1 {
			return mapStringString(v)
		}
		cur = mapStringAny(v)
	}
	return map[string]string{}
}

func readStringSlice(v any) []string {
	switch vv := v.(type) {
	case []string:
		return append([]string(nil), vv...)
	case []any:
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			s, ok := item.(string)
			if !ok {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func mapStringAny(v any) map[string]any {
	switch vv := v.(type) {
	case map[string]any:
		return vv
	case map[any]any:
		out := make(map[string]any, len(vv))
		for k, val := range vv {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = val
		}
		return out
	default:
		return nil
	}
}

func mapStringString(v any) map[string]string {
	switch vv := v.(type) {
	case map[string]string:
		return vv
	case map[string]any:
		out := make(map[string]string, len(vv))
		for k, val := range vv {
			s, ok := val.(string)
			if !ok {
				continue
			}
			out[k] = s
		}
		return out
	case map[any]any:
		out := make(map[string]string, len(vv))
		for k, val := range vv {
			ks, ok1 := k.(string)
			vs, ok2 := val.(string)
			if !ok1 || !ok2 {
				continue
			}
			out[ks] = vs
		}
		return out
	default:
		return map[string]string{}
	}
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

func normalizeRawTitleForParsing(raw string) string {
	s := strings.Map(func(r rune) rune {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\u2060', '\ufeff':
			return -1
		default:
			return r
		}
	}, raw)
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	unwrap := func(prefix string) (string, bool) {
		if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, "]") {
			return "", false
		}
		if len(s) <= len(prefix) {
			return "", false
		}
		inner := strings.TrimSpace(s[len(prefix) : len(s)-1])
		if inner == "" {
			return "", false
		}
		return "[" + inner, true
	}
	if out, ok := unwrap("[搬運["); ok {
		return out
	}
	if out, ok := unwrap("[搬运["); ok {
		return out
	}
	return s
}

func (p *Parser) Parse(rawTitle string) (*model.ParseResult, error) {
	if strings.TrimSpace(rawTitle) == "" {
		return nil, errors.New("empty title")
	}
	originalRawTitle := rawTitle
	rawTitle = normalizeRawTitleForParsing(rawTitle)

	toks := tokenize.Tokenize(rawTitle)
	tokens := make([]model.Token, 0, len(toks))
	for _, t := range toks {
		tokens = append(tokens, model.Token{Text: t})
	}
	p.protectKnownTitleSpans(rawTitle, tokens)

	attrs := make([]model.Attribute, 0, 16)
	seasonRange := extractSeasonRangeAndRemoveFromTokens(tokens)
	extractPackSeasonExtrasAndRemoveFromTokens(tokens, &attrs, seasonRange)
	extractStrictCustomReleaseTagAttrsAndRemoveFromTokens(tokens, &attrs)
	extractGuardedDVDRIPReleaseTagAttrsAndRemoveFromTokens(tokens, &attrs)
	extractPlatformRegionAttrsAndRemoveFromTokens(tokens, &attrs)
	version := extractVersionAttrsAndRemoveFromTokens(tokens, &attrs)
	extractEditionAttrsAndRemoveFromTokens(tokens, &attrs, shouldUseUnderscoreEditionRoute(rawTitle))
	extractSearchHintIndexAttrsAndRemoveFromTokens(tokens, &attrs)
	extractSeasonShowTagAttrsAndRemoveFromTokens(tokens, &attrs)
	p.applyLexicon(tokens, &attrs)
	p.extractStableTechSpanComboAttrsAndRemoveFromTokens(tokens, &attrs)
	p.extractLooseCodecBitDepthAttrsAndRemoveFromTokens(tokens, &attrs)
	extractDashedAudioCodecAttrsAndRemoveFromTokens(tokens, &attrs)
	p.extractMaProfileAudioCodecAttrsAndRemoveFromTokens(tokens, &attrs)
	p.extractStandaloneMaProfileBitDepthAttrsAndRemoveFromTokens(tokens, &attrs)
	p.extractStandaloneSourceTypeAttrsAndRemoveFromTokens(tokens, &attrs)
	p.extractStandaloneAudioCodecAttrsAndRemoveFromTokens(tokens, &attrs)
	extractCRCAttrsAndRemoveFromTokens(tokens, &attrs)
	p.extractVolumeEpisodeMetaMarkersAndRemoveFromTokens(tokens, &attrs)
	extractLooseBitDepthAttrsAndRemoveFromTokens(tokens, &attrs)
	promoteFrenchMultiNoteToAudioFlag(attrs)
	p.normalizePlatformAttrs(attrs)
	p.normalizeSourceTypeAttrs(attrs)
	convertRegionAttrsToPlatformRegion(attrs)
	extractWxHResolutionAttrsAndRemoveFromTokens(tokens, &attrs)
	extractYearAttrsAndRemoveFromTokens(tokens, &attrs, hasTechAttr(attrs))
	removePreYearNoiseNumberTokens(tokens)
	tailParenBlock := detectTailParenBlock(tokens)

	group := ""
	groupSpan := [2]int{0, 0}
	if g0, span0, ok := selectGroupFromAttrs(attrs); ok {
		group = normalizeGroupText(g0)
		groupSpan = span0
	}
	if group == "" {
		group, groupSpan = extractHeadBracketGroup(tokens)
	}
	if group == "" {
		group, groupSpan = extractHeadParenGroup(tokens)
	}
	if group == "" {
		group, groupSpan = extractHeadBracketGroupAfterLeadingParenMeta(tokens)
	}
	if group == "" {
		if g := detectHeadBracketGroupFromRawTitle(rawTitle); g != "" {
			group = g
			groupSpan = [2]int{-1, -1}
		}
	}

	if group != "" {
		// If we can detect a reliable tail group from raw title (e.g. "-ANi"), the head bracket
		// is more likely a bracketed title, not a release group. Do not remove it as group.
		// A head value present in grouplist.yaml has stronger evidence and must not be
		// displaced by a free-form trailing bracket.
		_, knownHeadGroup := p.isKnownGroup(group)
		if !knownHeadGroup && !hasExplicitGroupIdentityMarker(group) {
			if tg := p.detectTailGroupFromRawTitle(rawTitle); tg != "" {
				ng := normalizeGroupText(group)
				nt := normalizeGroupText(tg)
				if !strings.EqualFold(ng, nt) {
					group = ""
				}
			}
		}
	}

	if group != "" {
		// Group normalization is now handled by grouplist.yaml configuration
		// Apply normal normalization
		group = normalizeGroupText(group)
		if !hasGroupAttr(attrs, group) {
			attrs = append(attrs, model.Attribute{Type: model.AttrGroup, Normalized: group, TokenStart: groupSpan[0], TokenEnd: groupSpan[1], Confidence: 0.9, Source: "heuristic"})
		}
		if groupSpan[0] >= 0 && groupSpan[1] > groupSpan[0] {
			for i := groupSpan[0]; i < groupSpan[1] && i < len(tokens); i++ {
				tokens[i].Removed = true
			}
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
	attrs = pruneGroupSpanTechArtifacts(attrs, groupSpan)

	// Structural title protection (dictionary-free): "N/N" fraction-like digit
	// segments in the title zone (e.g. "22/7") and leading digit titles in a
	// strict template slot (e.g. "86 - 03") must not be mistaken for
	// season/episode markers by later detection passes.
	protectTitleFractionSpans(tokens, groupSpan[1])
	protectTitleLeadingDigits(tokens, groupSpan[1])

	packKind := detectPackKind(p.cfg, rawTitle)
	ctx := newParseContext(rawTitle, packKind, tailParenBlock)
	ctx.AliasValidFunc = p.isValidAliasToken
	ctx.GroupSlot2Rules = p.groupSlot2Rules
	ctx.AudioCodecCanonical = p.audioCodecCanonical
	ctx.SeasonRange = seasonRange
	ctx.Group = group
	for _, pass := range defaultParsePasses() {
		pass(tokens, &attrs, ctx)
	}
	attrs = pruneGroupSpanTechArtifacts(attrs, groupSpan)
	// PackKind may be refined/overridden by parse passes (e.g. contents markers like TV+OAD).
	packKind = strings.TrimSpace(ctx.PackKind)
	if hasAttrNormalized(attrs, model.AttrNote, "TV_PACK") {
		packKind = "TV_PACK"
	}
	season := ctx.Season
	seasonIdx := ctx.SeasonIdx
	seasonRange = ctx.SeasonRange
	spKind := ctx.SPKind
	movieIndex := ctx.MovieIndex
	movieIndexIdx := ctx.MovieIndexIdx
	movieIndexRange := ctx.MovieIndexRange
	hasMovieKeyword := ctx.HasMovieKeyword
	episodeRange := ctx.EpisodeRange
	episodeRangeIdx := ctx.EpisodeRangeIdx
	episodeList := ctx.EpisodeList
	episode := ctx.Episode
	episodeIdx := ctx.EpisodeIdx
	epslot := ctx.EPSlot

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

	if shouldDemoteEpisodeToMovieIndex(rawTitle, episode, episodeRange, season, seasonRange, packKind, hasMovieKeyword, movieIndex, movieIndexRange) {
		if strings.TrimSpace(movieIndex) == "" {
			movieIndex = strings.TrimSpace(episode)
			movieIndexIdx = episodeIdx
		}
		episode = ""
		episodeIdx = -1
		epslot = nil
	}

	_, titleEnd0 := p.inferTitleTokenSpan(tokens, episodeIdx, episodeRangeIdx, seasonCutIdx)
	parenAliases := p.extractParenAliasesAndRemoveFromTitle(tokens, titleEnd0, episodeIdx, episodeRangeIdx, &attrs)

	title := p.extractTitle(tokens, episodeIdx, episodeRangeIdx, seasonCutIdx)
	originalTitleSpan := strings.TrimSpace(title)
	if strings.TrimSpace(season) == "" {
		if s := extractExplicitSeasonNumber(originalTitleSpan); s != "" {
			season = s
		} else if s, idx := detectSeasonKeywordAny(tokens); s != "" {
			season = s
			seasonIdx = idx
		}
	}
	title, aliases := inferTitleAndAliasesFromTitleCanonical(title, p.isValidAliasToken)
	title, aliases = restorePartSuffixFromOriginalTitle(originalTitleSpan, title, aliases)
	titleStart, titleEnd := p.inferTitleTokenSpan(tokens, episodeIdx, episodeRangeIdx, seasonCutIdx)
	pipeScanStart := titleStartAfterLeadingEmptyBracketBlocks(tokens)
	if len(groupSpan) == 2 && groupSpan[1] > pipeScanStart {
		pipeScanStart = groupSpan[1]
	}
	pipeScanEnd := len(tokens)
	if episodeRangeIdx >= 0 && episodeRangeIdx < pipeScanEnd {
		pipeScanEnd = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < pipeScanEnd {
		pipeScanEnd = episodeIdx
	}
	if seasonCutIdx >= 0 && seasonCutIdx < pipeScanEnd {
		pipeScanEnd = seasonCutIdx
	}
	if techStart, ok := firstTechTokenIndexFromAttrs(attrs); ok && techStart >= 0 && techStart < pipeScanEnd {
		// Do not let early pre-title tech/meta tokens (e.g. slot2 platform markers in bracket)
		// block alias-chain extraction in the main title span.
		if titleStart < 0 || techStart > titleStart {
			pipeScanEnd = techStart
		}
	}
	if pipeTitle, pipeAliases, ok := p.extractPreEpisodePipeAliasesAndConsume(tokens, pipeScanStart, pipeScanEnd); ok {
		if strings.TrimSpace(pipeTitle) != "" {
			title = pipeTitle
		}
		aliases = mergeAliases(pipeAliases, aliases)
	}
	aliases = mergeAliases(ctx.ForcedAliases, aliases)
	if strings.TrimSpace(title) == "" {
		scanStart := 0
		if len(groupSpan) == 2 && groupSpan[1] > scanStart {
			scanStart = groupSpan[1]
		}
		if fbTitle, fbAliases, fbStart, fbEnd := p.inferBracketTitleFallback(tokens, scanStart, episodeIdx, episodeRangeIdx); strings.TrimSpace(fbTitle) != "" {
			title = fbTitle
			aliases = mergeAliases(aliases, fbAliases)
			titleStart = fbStart
			titleEnd = fbEnd
		}
	}
	if strings.TrimSpace(ctx.ForcedTitle) != "" {
		ft := ctx.ForcedTitle
		// If forced title contains strong delimiters (e.g. from bracket title extraction),
		// still infer main title + aliases deterministically.
		if strings.ContainsAny(ft, "/／|｜;；\\") {
			ftMain, ftAliases := inferTitleAndAliasesFromTitleCanonical(ft, p.isValidAliasToken)
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
	title = trimMovieDashEpisodeTailFromTitle(rawTitle, title, episode, movieIndex)
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
		attrs = append(attrs, model.Attribute{Type: model.AttrTitle, Normalized: title, TokenStart: titleStart, TokenEnd: titleEnd, Confidence: 0.7, Source: "heuristic"})
		aliases = mergeAliases(aliases, p.inferAliasesFromAdjacentBrackets(tokens, titleEnd, episodeIdx, episodeRangeIdx))
		aliases = mergeAliases(aliases, p.inferTitleGapSingleSlashAlias(tokens, title, titleEnd, episodeIdx, episodeRangeIdx))
		aliases = mergeAliases(aliases, p.inferTitleGapDashAlias(tokens, title, titleEnd, episodeIdx, episodeRangeIdx))
		aliases = mergeAliases(aliases, p.inferTitleGapBilingualRunAlias(tokens, title, titleEnd, episodeIdx, episodeRangeIdx))
		title, aliases = p.promoteInlineBracketContinuationIntoTitle(tokens, title, aliases, titleEnd, episodeIdx, episodeRangeIdx, attrs)
	}
	aliases = mergeAliases(aliases, parenAliases)
	aliases = mergeAliases(aliases, inferPostEpisodeSearchHintAliasesFromParens(tokens, episodeIdx, episodeRangeIdx, &attrs))
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
		epAnchor := episodeIdx
		if episodeRangeIdx > epAnchor {
			epAnchor = episodeRangeIdx
		}
		if seasonIdx > epAnchor {
			epAnchor = seasonIdx
		}
		tg, tgIdx := detectTailGroup(tokens)
		if tg != "" && epAnchor >= 0 && tgIdx >= 0 && tgIdx <= epAnchor {
			if tg2, tgIdx2 := detectTailGroupAfter(tokens, epAnchor); tg2 != "" {
				tg, tgIdx = tg2, tgIdx2
			} else {
				tg, tgIdx = "", -1
			}
		}
		if tg != "" {
			group = normalizeGroupText(tg)
			attrs = append(attrs, model.Attribute{Type: model.AttrGroup, Normalized: group, TokenStart: tgIdx, TokenEnd: tgIdx + 1, Confidence: 0.7, Source: "heuristic"})
		}
	}
	if group == "" {
		if tg := p.detectTailGroupFromRawTitle(rawTitle); tg != "" {
			group = normalizeGroupText(tg)
			attrs = append(attrs, model.Attribute{Type: model.AttrGroup, Normalized: group, TokenStart: -1, TokenEnd: -1, Confidence: 0.65, Source: "heuristic_raw"})
			attrs = append(attrs, model.Attribute{Type: model.AttrTailGroup, Normalized: group, TokenStart: -1, TokenEnd: -1, Confidence: 0.65, Source: "heuristic_raw"})
		}
	}
	epAnchor := episodeIdx
	if episodeRangeIdx > epAnchor {
		epAnchor = episodeRangeIdx
	}
	if seasonIdx > epAnchor {
		epAnchor = seasonIdx
	}
	tg, tgIdx := detectTailGroup(tokens)
	if tg != "" && epAnchor >= 0 && tgIdx >= 0 && tgIdx <= epAnchor {
		if tg2, tgIdx2 := detectTailGroupAfter(tokens, epAnchor); tg2 != "" {
			tg, tgIdx = tg2, tgIdx2
		} else {
			tg, tgIdx = "", -1
		}
	}
	if tg != "" {
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
		aliases = mergeAliases(aliases, p.inferTailAliasesFromPipes(tokens, attrs, group))
		aliases = mergeAliases(aliases, p.inferTailAliasesFromParens(tokens, &attrs, episodeIdx, episodeRangeIdx, seasonIdx))
	}
	if len(aliases) > 0 {
		var contentMarkers []string
		aliases, contentMarkers = extractContentsFromAliases(aliases)
		if len(contentMarkers) > 0 {
			emitContentsAttrs(&attrs, contentMarkers, "alias_contents_marker")
		}
	}
	if rawMarkers := extractContentsFromRawParens(rawTitle); len(rawMarkers) > 0 {
		emitContentsAttrs(&attrs, rawMarkers, "raw_paren_contents_marker")
	}
	attrs = appendEmbeddedJapaneseDubAttrs(attrs, rawTitle)
	attrs = appendEmbeddedChineseDubAttrs(attrs, rawTitle)
	attrs = appendEmbeddedEnglishSubAttrs(attrs, rawTitle)
	attrs = dropContentsAttrsInsideTitleSpan(attrs, titleStart, titleEnd)
	if strings.TrimSpace(spKind) == "" {
		spKind = inferSPKindFromContentsAttrs(attrs)
	}
	if strings.TrimSpace(packKind) == "" {
		packKind = inferPackKindFromContentsAttrs(packKind, attrs)
	}
	if strings.TrimSpace(packKind) == "" {
		if strings.TrimSpace(episode) == "" && strings.TrimSpace(episodeRange) == "" {
			if reParse1.MatchString(rawTitle) {
				packKind = "bd"
			}
		}
	}
	if !hasMovieKeyword && hasMovieKeywordInRawTitle(rawTitle) {
		hasMovieKeyword = true
	}
	if shouldDropEpisodeRangeForMovieRange(rawTitle, movieIndexRange, episode, episodeRange) {
		episode = ""
		episodeIdx = -1
		episodeList = ""
		episodeRange = ""
		episodeRangeIdx = -1
		epslot = nil
	}
	movieRouteHardExcluded := hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
	movieContentSignal := hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
	epRoute := computeEpRoute(tokens, episode, episodeList, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
	contentKind, contentConf := inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	if ep2, mi2, epIdx2, miIdx2, changed := normalizeMovieEpisodeSignals(rawTitle, contentKind, episode, episodeRange, season, seasonRange, packKind, hasMovieKeyword, movieIndex, movieIndexRange, episodeIdx, movieIndexIdx); changed {
		episode, movieIndex, episodeIdx, movieIndexIdx = ep2, mi2, epIdx2, miIdx2
		title = trimMovieDashEpisodeTailFromTitle(rawTitle, title, episode, movieIndex)
		epslot = nil
		movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
		movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
		epRoute = computeEpRoute(tokens, episode, episodeList, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
		contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	}
	templateID, templateConf, routeID := matchTemplate(p.cfg, epRoute, contentKind, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, attrs, tokens)
	contentKind = normalizeContentKindByTemplateID(contentKind, templateID)
	attrs, group, episode, episodeIdx, episodeRange, episodeRangeIdx, season, seasonIdx, signalEvents := applyTemplateSignalAdjudication(
		p.cfg,
		templateID,
		tokens,
		attrs,
		group,
		episode,
		episodeIdx,
		episodeRange,
		episodeRangeIdx,
		season,
		seasonIdx,
	)
	if os.Getenv("TITLEPARSER_TEMPLATE_SIGNAL_TRACE") == "1" {
		for _, ev := range signalEvents {
			if strings.TrimSpace(ev) == "" {
				continue
			}
			attrs = append(attrs, model.Attribute{
				Type:       model.AttrNote,
				Normalized: "template_signal:" + ev,
				TokenStart: -1,
				TokenEnd:   -1,
				Confidence: 0.6,
				Source:     "template_signal_trace",
			})
		}
	}
	movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
	movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
	epRoute = computeEpRoute(tokens, episode, episodeList, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
	contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	if shouldDropEpisodeRangeForMovieRange(rawTitle, movieIndexRange, episode, episodeRange) {
		episode = ""
		episodeIdx = -1
		episodeList = ""
		episodeRange = ""
		episodeRangeIdx = -1
		epslot = nil
		movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
		movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
		epRoute = computeEpRoute(tokens, episode, episodeList, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
		contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	}
	if ep2, mi2, epIdx2, miIdx2, changed := normalizeMovieEpisodeSignals(rawTitle, contentKind, episode, episodeRange, season, seasonRange, packKind, hasMovieKeyword, movieIndex, movieIndexRange, episodeIdx, movieIndexIdx); changed {
		episode, movieIndex, episodeIdx, movieIndexIdx = ep2, mi2, epIdx2, miIdx2
		title = trimMovieDashEpisodeTailFromTitle(rawTitle, title, episode, movieIndex)
		epslot = nil
		movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
		movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
		epRoute = computeEpRoute(tokens, episode, episodeList, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
		contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	}
	templateID, templateConf, routeID = matchTemplate(p.cfg, epRoute, contentKind, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, attrs, tokens)
	contentKind = normalizeContentKindByTemplateID(contentKind, templateID)
	if strings.HasPrefix(routeID, "streaming_release_") {
		p.applyStreamingReleaseRouteV1(tailParenBlock, &aliases, &attrs)
	}
	title = trimMovieRangeStartTailFromTitle(rawTitle, title, movieIndexRange, episode, episodeRange)
	origTitle := title
	title = sanitizePrimaryTitleForOutput(title)
	title = trimTitleTailByPackKind(title, packKind)
	title = trimAliasTailBySPKind(title, spKind)
	aliases = p.sanitizeAliasesForOutput(aliases, title, episode, season, spKind)
	title, aliases = p.rebalanceGenericPrimaryTitleFromAliases(title, aliases)
	if bridgedTitle, bridgedAliases, ok := applyBracketUnderscoreAliasBridge(rawTitle, title, aliases); ok {
		title, aliases = bridgedTitle, bridgedAliases
	}
	if strings.TrimSpace(title) != "" && aliasSemanticKey(title) != aliasSemanticKey(origTitle) {
		attrs = upsertTitleAttribute(attrs, title, "title_cleanup")
	}
	group, title, aliases, templateID, contentKind, routeID, overrideApplied := p.applyGroupRouteOverride(rawTitle, group, episodeRange, title, aliases, templateID, contentKind, routeID)
	title = trimTitleTailByGroup(title, group)
	for _, a := range attrs {
		if a.Type != model.AttrTailGroup {
			continue
		}
		if strings.TrimSpace(a.Normalized) == "" {
			continue
		}
		title = trimTitleTailByGroup(title, a.Normalized)
	}
	title = trimAliasTailBySPKind(title, spKind)
	if overrideApplied && strings.HasPrefix(routeID, "group_epseg_") {
		if ep, sp, epTitle, ok := extractEpisodeLikeFromThirdBracketDetailed(rawTitle); ok {
			episode = ep
			if strings.TrimSpace(sp) != "" {
				spKind = sp
			}
			isMarukoRoute := strings.EqualFold(strings.TrimSpace(routeID), "group_epseg_maruko_v1")
			aliases = pruneEpisodeLikeAliases(aliases)
			overrideAxis := "ABS"
			if strings.TrimSpace(season) != "" {
				overrideAxis = "SE"
			}
			primary := &model.EPEntity{
				Kind:       "EP",
				Axis:       overrideAxis,
				Episode:    episode,
				SPKind:     spKind,
				TokenStart: -1,
				TokenEnd:   -1,
				Source:     "group_route_override",
			}
			if isMarukoRoute {
				epslot = &model.EPSlot{
					Primary:    primary,
					Candidates: []model.EPEntity{*primary},
				}
				tailTitle := strings.TrimSpace(epTitle)
				if tailTitle == "" {
					tailTitle = strings.TrimSpace(extractTailBetweenThirdAndFourthBracket(rawTitle))
				}
				if tailTitle != "" {
					attrs = append(attrs, model.Attribute{
						Type:       model.AttrNote,
						Normalized: tailTitle,
						TokenStart: -1,
						TokenEnd:   -1,
						Confidence: 0.7,
						Source:     "group_route_override",
					})
				}
				if dm := bracketDatePattern.FindStringSubmatch(rawTitle); len(dm) > 1 && strings.TrimSpace(dm[1]) != "" {
					attrs = append(attrs, model.Attribute{
						Type:       model.AttrNote,
						Normalized: strings.TrimSpace(dm[1]),
						TokenStart: -1,
						TokenEnd:   -1,
						Confidence: 0.7,
						Source:     "group_route_override",
					})
				}
			} else if epslot == nil || epslot.Primary == nil || strings.TrimSpace(epslot.Primary.Episode) == "" {
				epslot = &model.EPSlot{
					Primary:    primary,
					Candidates: []model.EPEntity{*primary},
				}
			}
			movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
			movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
			epRoute = computeEpRoute(tokens, episode, episodeList, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
			contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
			if tid, tconf, _ := matchTemplate(p.cfg, epRoute, contentKind, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, attrs, tokens); strings.TrimSpace(tid) != "" && tid != "TF_UNKNOWN_DEFAULT" {
				templateID = tid
				templateConf = tconf
			}
		}
	}
	if overrideApplied && strings.EqualFold(strings.TrimSpace(routeID), "group_aptx4869_conan_bracket_ep_v1") {
		if forcedTitle, ep, epTitle, ok := extractAPTX4869ConanEpisodeTitleRoute(rawTitle); ok {
			title = forcedTitle
			episode = ep
			episodeIdx = -1
			episodeList = ""
			episodeRange = ""
			episodeRangeIdx = -1
			aliases = pruneEpisodeLikeAliases(aliases)
			if strings.TrimSpace(epTitle) != "" {
				attrs = append(attrs, model.Attribute{
					Type:       model.AttrNote,
					Normalized: strings.TrimSpace(epTitle),
					TokenStart: -1,
					TokenEnd:   -1,
					Confidence: 0.7,
					Source:     "group_route_override",
				})
			}
			primary := &model.EPEntity{
				Kind:       "EP",
				Axis:       "ABS",
				Episode:    episode,
				TokenStart: -1,
				TokenEnd:   -1,
				Source:     "group_route_override",
			}
			epslot = &model.EPSlot{
				Primary:    primary,
				Candidates: []model.EPEntity{*primary},
			}
			movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
			movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
			epRoute = computeEpRoute(tokens, episode, episodeList, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
			contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
			if tid, tconf, _ := matchTemplate(p.cfg, epRoute, contentKind, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, attrs, tokens); strings.TrimSpace(tid) != "" && tid != "TF_UNKNOWN_DEFAULT" {
				templateID = tid
				templateConf = tconf
			}
		}
	}
	if overrideApplied {
		contentKind = normalizeContentKindByTemplateID(contentKind, templateID)
		title, aliases = applyGroupTitleAliasSpecialization(rawTitle, routeID, title, aliases)
	}
	if overrideApplied {
		if strings.TrimSpace(title) != "" {
			attrs = upsertTitleAttribute(attrs, title, "group_route_override")
		}
		attrs = upsertGroupAttribute(attrs, group, "group_route_override")
	}
	// Group-route overrides may inject raw alias candidates; run final alias/title cleanup once more.
	aliases = p.sanitizeAliasesForOutput(aliases, title, episode, season, spKind)
	title, aliases = rebalanceSingleSlashSeasonBridgePrimaryFromRaw(rawTitle, title, aliases)
	title, aliases = p.rebalanceGenericPrimaryTitleFromAliases(title, aliases)
	title, aliases = p.applyTitleKnowledge(rawTitle, title, aliases)
	packProfile := p.inferPackProfile(contentKind, packKind, season, seasonRange, attrs)
	attrs = upsertTitleAttribute(attrs, title, "canonical")
	attrs = upsertGroupAttribute(attrs, group, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrVersion, version, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrPackKind, packKind, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrPackProfile, packProfile, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrMovieIndex, movieIndex, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrMovieIndexRange, movieIndexRange, "canonical")
	attrs = appendTitleAliasAttributes(attrs, aliases, title, "canonical")
	attrs = p.promoteNoteAttributes(attrs)
	attrs = dropRedundantNoteAttributes(attrs, season, seasonRange)
	p.normalizePlatformAttrs(attrs)
	p.normalizeSourceTypeAttrs(attrs)
	p.normalizeAudioAttrs(attrs)
	attrs = p.normalizeSubtitleAttrs(attrs, tokens)
	attrs = p.enrichBatchSourceTypeFromPackKind(attrs, contentKind)
	attrs = dedupeAttributes(attrs)
	attrs = collapseOverlappingSpecificAudioCodecs(attrs)
	ctx.AudioSlot = reconcileAudioSlotWithAttributes(ctx.AudioSlot, attrs)
	normalizeAudioSlotLanguages(ctx.AudioSlot)
	epslot = syncEPSlotWithCanonicalSignals(epslot, season, episode, episodeRange, episodeList, spKind, episodeIdx, episodeRangeIdx, ctx.EpisodeListIdx)
	epslot = adjudicateEPSlotPrimaryAxis(epslot, tokens, season)
	epslot = pruneBatchEPSlotForConservativeOutput(epslot, contentKind, episode, episodeRange, spKind)
	if shouldPreferSPPrimaryForSpecial(contentKind, spKind, epslot) {
		episode = ""
		episodeIdx = -1
		episodeList = ""
		ctx.EpisodeListIdx = -1
		episodeRange = ""
		episodeRangeIdx = -1
		epslot = forceSPPrimaryFromSPKind(epslot, season, spKind)
	}
	contentKind, episode, episodeList, episodeRange, epslot = pruneCanonicalEpisodeSignalsForOutput(rawTitle, contentKind, packKind, season, episode, episodeList, episodeRange, spKind, movieIndex, movieIndexRange, epslot)
	if isUnknownOutput(contentKind, templateID) {
		attrs = pruneUnknownCoreAttributes(attrs)
		epslot = nil
		ctx.AudioSlot = nil
	}
	consumeTokensByTechAttrSpans(tokens, attrs)
	consumeTokensByEPSlotPrimarySpan(tokens, epslot)
	finalTitle := finalTitleValueFromAttributes(attrs, title)
	finalAliases := finalTitleAliasValuesFromAttributes(attrs)
	p.consumeTitleGapSeasonBridgeMarkers(tokens, titleEnd, episodeIdx, episodeRangeIdx, attrs)
	p.consumeTitleGapLatinAliasRuns(tokens, finalTitle, finalAliases, titleEnd, episodeIdx, episodeRangeIdx, attrs)
	p.consumeTitleGapAliasSemanticTokens(tokens, finalAliases, titleEnd, episodeIdx, episodeRangeIdx, attrs)
	p.consumeTitleGapBatchTVMarkers(tokens, titleEnd, episodeIdx, episodeRangeIdx, attrs)
	p.consumeAliasChainTokensByFinalAliases(tokens, groupSpan, finalTitle, finalAliases, episodeIdx, episodeRangeIdx, seasonIdx, attrs)
	p.consumeGroupSpecificTitleChainDelimiters(tokens, groupSpan, group, episodeIdx, episodeRangeIdx, attrs)
	consumeTailGroupTokensByCanonicalGroup(tokens, group)
	removeBracketsAroundFullyConsumedBlocks(tokens)
	attrs = stripLegacyEPAttributes(attrs)

	epSemantics := buildCanonicalEPSemantics(originalRawTitle, tokens, epslot, season, seasonRange, episode, episodeList, episodeRange, spKind)
	if isUnknownOutput(contentKind, templateID) {
		epSemantics = nil
	}

	res := &model.ParseResult{
		RawTitle:           originalRawTitle,
		ContentKind:        contentKind,
		ContentConfidence:  contentConf,
		TemplateID:         templateID,
		TemplateConfidence: templateConf,
		EPSemantics:        epSemantics,
		AudioSlot:          ctx.AudioSlot,
		Attributes:         attrs,
		Tokens:             tokens,
	}

	return res, nil
}

// Regular expressions hoisted out of function bodies so they are compiled once at init.
var (
	reParse1 = regexp.MustCompile(`(?i)\bBD\b`)
)
