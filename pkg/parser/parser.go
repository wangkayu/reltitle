package parser

import (
	"errors"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
	// "title-parser/pkg/onnx" // Temporarily disabled for HTTP server
	"title-parser/pkg/tokenize"
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

var stableTechResolutionPattern = regexp.MustCompile(`(?i)^(\d{3,4})p$`)
var stableTechBitDepthPattern = regexp.MustCompile(`(?i)^(8|10|12)\s*bit$`)
var stableTechCodecBitDepthPattern = regexp.MustCompile(`(?i)^(hevc|x265|h265|x264|h264)\s*[-_ ]?\s*0*(8|10|12)(?:bit|b)?$`)
var maProfileBitDepthPattern = regexp.MustCompile(`(?i)^ma0*(8|10|12)p$`)
var looseBitDepthPattern = regexp.MustCompile(`(?i)^0*(8|10|12)\s*(?:-|_)?\s*bit[\.,，:：;；!?]?$`)
var stableTechSplitPattern = regexp.MustCompile(`[+_/\.]`)

func (p *Parser) extractStableTechSpanComboAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if attrs == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Text != "[" && tokens[i].Text != "【" && tokens[i].Text != "(" && tokens[i].Text != "（" {
			continue
		}

		open := i
		close := -1
		if tokens[i].Text == "[" || tokens[i].Text == "【" {
			openTok := tokens[i].Text
			closeTok := "]"
			if openTok == "【" {
				closeTok = "】"
			}
			depth := 0
			for j := i; j < len(tokens); j++ {
				t := strings.TrimSpace(tokens[j].Text)
				if t == openTok {
					depth++
					continue
				}
				if t == closeTok {
					depth--
					if depth == 0 {
						close = j
						break
					}
				}
			}
			if close <= i {
				continue
			}
		} else {
			_, c, _, ok := parseParenBlock(tokens, i, len(tokens))
			if !ok || c <= i {
				continue
			}
			close = c
		}
		if close <= open+1 {
			i = close
			continue
		}

		blockAttrs := make([]model.Attribute, 0, 6)
		recognized := make(map[int]bool)
		nonSepCount := 0
		coreCount := 0

		for j := open + 1; j < close; j++ {
			if tokens[j].Removed {
				continue
			}
			txt := strings.TrimSpace(tokens[j].Text)
			if txt == "" || isTechDelimiterToken(txt) {
				continue
			}
			nonSepCount++
			local, ok := p.inferStableTechTokenAttrs(txt, j)
			if !ok {
				continue
			}
			for _, a := range local {
				if isStableTechCoreType(a.Type) {
					coreCount++
				}
				blockAttrs = append(blockAttrs, a)
			}
			recognized[j] = true
		}

		if nonSepCount == 0 || len(blockAttrs) < 2 || coreCount == 0 {
			i = close
			continue
		}
		if float64(len(recognized))/float64(nonSepCount) < 0.6 {
			i = close
			continue
		}

		*attrs = append(*attrs, blockAttrs...)
		for idx := range recognized {
			tokens[idx].Removed = true
		}
		for j := open + 1; j < close; j++ {
			if tokens[j].Removed {
				continue
			}
			txt := strings.TrimSpace(tokens[j].Text)
			if !isTechDelimiterToken(txt) {
				continue
			}
			left := j - 1
			right := j + 1
			if left > open && right < close && recognized[left] && recognized[right] {
				tokens[j].Removed = true
			}
		}
		i = close
	}
}

func isTechDelimiterToken(s string) bool {
	switch strings.TrimSpace(s) {
	case "-", "_", "+", ",", "，", ".", "/", "／", "&", "＆":
		return true
	default:
		return false
	}
}

func isStableTechCoreType(t model.AttrType) bool {
	switch t {
	case model.AttrResolution, model.AttrSourceType, model.AttrVideoCodec, model.AttrAudioCodec, model.AttrPlatform:
		return true
	default:
		return false
	}
}

func (p *Parser) inferStableTechTokenAttrs(raw string, idx int) ([]model.Attribute, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	clean := strings.Trim(raw, "[](){}<>【】（）.,，:：;；'\"")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return nil, false
	}

	parts := []string{clean}
	if stableTechSplitPattern.MatchString(clean) {
		tmp := stableTechSplitPattern.Split(clean, -1)
		parts = make([]string, 0, len(tmp))
		for _, part := range tmp {
			pp := strings.TrimSpace(part)
			if pp == "" {
				continue
			}
			parts = append(parts, pp)
		}
		if len(parts) == 0 {
			parts = []string{clean}
		}
	}

	out := make([]model.Attribute, 0, 3)
	for _, part := range parts {
		pp := strings.TrimSpace(part)
		if pp == "" {
			continue
		}
		low := strings.ToLower(pp)

		if m := stableTechResolutionPattern.FindStringSubmatch(low); len(m) == 2 {
			out = append(out, model.Attribute{Type: model.AttrResolution, Normalized: m[1] + "p", TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.76, Source: "tech_combo_span"})
			continue
		}

		if mm := stableTechBitDepthPattern.FindStringSubmatch(low); len(mm) == 2 {
			out = append(out, model.Attribute{Type: model.AttrBitDepth, Normalized: mm[1] + "bit", TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.74, Source: "tech_combo_span"})
			continue
		}
		if mm := stableTechCodecBitDepthPattern.FindStringSubmatch(low); len(mm) == 3 {
			if norm, ok := p.normalizeVideoCodecToken(mm[1]); ok {
				out = append(out,
					model.Attribute{Type: model.AttrVideoCodec, Normalized: norm, TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.76, Source: "tech_combo_span"},
					model.Attribute{Type: model.AttrBitDepth, Normalized: mm[2] + "bit", TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.74, Source: "tech_combo_span"},
				)
				continue
			}
		}
		if low == "remux" {
			out = append(out, model.Attribute{Type: model.AttrType("remux"), Normalized: "REMUX", TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.74, Source: "tech_combo_span"})
			continue
		}
		if norm, ok := p.sourceTypeCanonical[low]; ok && strings.TrimSpace(norm) != "" {
			out = append(out, model.Attribute{Type: model.AttrSourceType, Normalized: norm, TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.76, Source: "tech_combo_span"})
			continue
		}
		if norm, ok := p.platformCanonical[low]; ok && strings.TrimSpace(norm) != "" {
			out = append(out, model.Attribute{Type: model.AttrPlatform, Normalized: norm, TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.74, Source: "tech_combo_span"})
			continue
		}
		if norm, ok := p.normalizeVideoCodecToken(low); ok {
			out = append(out, model.Attribute{Type: model.AttrVideoCodec, Normalized: norm, TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.76, Source: "tech_combo_span"})
			continue
		}
		if norm, ok := p.normalizeAudioCodecToken(low); ok {
			out = append(out, model.Attribute{Type: model.AttrAudioCodec, Normalized: norm, TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.76, Source: "tech_combo_span"})
			continue
		}
		if norm, ok := p.normalizeContainerToken(low); ok {
			out = append(out, model.Attribute{Type: model.AttrContainer, Normalized: norm, TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.74, Source: "tech_combo_span"})
			continue
		}
		if norm, ok := p.normalizeAudioFlagToken(low); ok {
			out = append(out, model.Attribute{Type: model.AttrAudioFlag, Normalized: norm, TokenStart: idx, TokenEnd: idx + 1, Confidence: 0.7, Source: "tech_combo_span"})
			continue
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func extractLooseBitDepthAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if attrs == nil || len(tokens) == 0 {
		return
	}
	seen := make(map[string]struct{}, 3)
	for _, a := range *attrs {
		if a.Type != model.AttrBitDepth {
			continue
		}
		n := strings.TrimSpace(strings.ToLower(a.Normalized))
		if n == "" {
			continue
		}
		seen[n] = struct{}{}
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		mm := looseBitDepthPattern.FindStringSubmatch(txt)
		if len(mm) != 2 {
			continue
		}
		norm := mm[1] + "bit"
		key := strings.ToLower(norm)
		if _, ok := seen[key]; ok {
			tokens[i].Removed = true
			continue
		}
		*attrs = append(*attrs, model.Attribute{
			Type:       model.AttrBitDepth,
			Normalized: norm,
			TokenStart: i,
			TokenEnd:   i + 1,
			Confidence: 0.72,
			Source:     "bit_depth_loose",
		})
		seen[key] = struct{}{}
		tokens[i].Removed = true
	}
}

func (p *Parser) extractLooseCodecBitDepthAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if attrs == nil || len(tokens) == 0 || p == nil {
		return
	}
	seenVideo := make(map[string]struct{}, 2)
	seenBit := make(map[string]struct{}, 2)
	for _, a := range *attrs {
		if a.Type == model.AttrVideoCodec {
			n := strings.ToLower(strings.TrimSpace(a.Normalized))
			if n != "" {
				seenVideo[n] = struct{}{}
			}
			continue
		}
		if a.Type == model.AttrBitDepth {
			n := strings.ToLower(strings.TrimSpace(a.Normalized))
			if n != "" {
				seenBit[n] = struct{}{}
			}
		}
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.ToLower(strings.TrimSpace(tokens[i].Text))
		if txt == "" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		mm := stableTechCodecBitDepthPattern.FindStringSubmatch(txt)
		if len(mm) != 3 {
			continue
		}
		vcodec, ok := p.normalizeVideoCodecToken(mm[1])
		if !ok || strings.TrimSpace(vcodec) == "" {
			continue
		}
		bit := mm[2] + "bit"
		if _, ok := seenVideo[strings.ToLower(vcodec)]; !ok {
			*attrs = append(*attrs, model.Attribute{
				Type:       model.AttrVideoCodec,
				Normalized: vcodec,
				TokenStart: i,
				TokenEnd:   i + 1,
				Confidence: 0.74,
				Source:     "tech_combo_token",
			})
			seenVideo[strings.ToLower(vcodec)] = struct{}{}
		}
		if _, ok := seenBit[strings.ToLower(bit)]; !ok {
			*attrs = append(*attrs, model.Attribute{
				Type:       model.AttrBitDepth,
				Normalized: bit,
				TokenStart: i,
				TokenEnd:   i + 1,
				Confidence: 0.72,
				Source:     "tech_combo_token",
			})
			seenBit[strings.ToLower(bit)] = struct{}{}
		}
		tokens[i].Removed = true
	}
}

func isLikelyTechContextToken(s string) bool {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return false
	}
	if regexp.MustCompile(`^\d{3,4}P$`).MatchString(s) {
		return true
	}
	switch s {
	case "WEB", "WEBRIP", "WEB-DL", "WEBDL",
		"BD", "BDRIP", "BLURAY", "UHD",
		"X264", "X265", "H264", "H265", "AVC", "HEVC",
		"AAC", "AC3", "EAC3", "DDP", "FLAC",
		"MKV", "MP4", "AVI", "TS", "M2TS",
		"MULTI", "CUSTOM", "VOSTFR", "VOSTEN", "VF", "DUAL",
		"REMUX", "OVA", "OAD", "ONA",
		"SET", "ISO", "ISOS", "NTSC", "PAL", "BOOTLEG",
		"AUDIO", "LATINO", "R2J", "TOKUTEN":
		return true
	default:
		return false
	}
}

func hasNearbyTechContext(tokens []model.Token, center int, radius int) bool {
	if len(tokens) == 0 {
		return false
	}
	if radius < 1 {
		radius = 1
	}
	start := center - radius
	if start < 0 {
		start = 0
	}
	end := center + radius + 1
	if end > len(tokens) {
		end = len(tokens)
	}
	for i := start; i < end; i++ {
		if i == center {
			continue
		}
		if isLikelyTechContextToken(tokens[i].Text) {
			return true
		}
	}
	return false
}

func hasNearbySpecialReleaseToken(tokens []model.Token, center int, radius int) bool {
	if len(tokens) == 0 {
		return false
	}
	if radius < 1 {
		radius = 1
	}
	start := center - radius
	if start < 0 {
		start = 0
	}
	end := center + radius + 1
	if end > len(tokens) {
		end = len(tokens)
	}
	for i := start; i < end; i++ {
		if i == center || tokens[i].Removed {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(tokens[i].Text)) {
		case "NCOP", "NCED", "PV", "CM", "OP", "ED":
			return true
		}
	}
	return false
}

func (p *Parser) hasNearbyAudioCodecToken(tokens []model.Token, center int, radius int) bool {
	if p == nil || len(tokens) == 0 {
		return false
	}
	if radius < 1 {
		radius = 1
	}
	start := center - radius
	if start < 0 {
		start = 0
	}
	end := center + radius + 1
	if end > len(tokens) {
		end = len(tokens)
	}
	for i := start; i < end; i++ {
		if i == center || tokens[i].Removed {
			continue
		}
		base := strings.TrimSpace(tokens[i].Text)
		if base == "" {
			continue
		}
		if _, ok := p.normalizeAudioCodecToken(base); ok {
			return true
		}
		if i+2 < len(tokens) && !tokens[i+1].Removed && !tokens[i+2].Removed && strings.TrimSpace(tokens[i+1].Text) == "-" {
			combined := strings.TrimSpace(tokens[i].Text) + "-" + strings.TrimSpace(tokens[i+2].Text)
			if _, ok := p.normalizeAudioCodecToken(combined); ok {
				return true
			}
		}
	}
	return false
}

func hasAudioCodecAttrNorm(attrs []model.Attribute, norm string) bool {
	norm = strings.ToUpper(strings.TrimSpace(norm))
	if norm == "" {
		return false
	}
	for _, a := range attrs {
		if a.Type != model.AttrAudioCodec {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(a.Normalized)) == norm {
			return true
		}
	}
	return false
}

func hasSourceTypeAttr(attrs []model.Attribute) bool {
	for _, a := range attrs {
		if a.Type == model.AttrSourceType && strings.TrimSpace(a.Normalized) != "" {
			return true
		}
	}
	return false
}

func extractDashedAudioCodecAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if attrs == nil || len(tokens) < 3 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		// E-AC-3 => EAC3
		if i+4 < len(tokens) &&
			strings.EqualFold(strings.TrimSpace(tokens[i].Text), "E") &&
			strings.TrimSpace(tokens[i+1].Text) == "-" &&
			strings.EqualFold(strings.TrimSpace(tokens[i+2].Text), "AC") &&
			strings.TrimSpace(tokens[i+3].Text) == "-" &&
			strings.EqualFold(strings.TrimSpace(tokens[i+4].Text), "3") {
			if !hasAudioCodecAttrNorm(*attrs, "EAC3") {
				*attrs = append(*attrs, model.Attribute{
					Type:       model.AttrAudioCodec,
					Normalized: "EAC3",
					TokenStart: i,
					TokenEnd:   i + 5,
					Confidence: 0.74,
					Source:     "codec_dashed",
				})
			}
			for k := i; k <= i+4; k++ {
				tokens[k].Removed = true
			}
			i += 4
			continue
		}
		// AC-3 => AC3
		if i+2 < len(tokens) &&
			strings.EqualFold(strings.TrimSpace(tokens[i].Text), "AC") &&
			strings.TrimSpace(tokens[i+1].Text) == "-" &&
			strings.EqualFold(strings.TrimSpace(tokens[i+2].Text), "3") {
			if !hasAudioCodecAttrNorm(*attrs, "AC3") {
				*attrs = append(*attrs, model.Attribute{
					Type:       model.AttrAudioCodec,
					Normalized: "AC3",
					TokenStart: i,
					TokenEnd:   i + 3,
					Confidence: 0.74,
					Source:     "codec_dashed",
				})
			}
			for k := i; k <= i+2; k++ {
				tokens[k].Removed = true
			}
			i += 2
		}
	}
}

func (p *Parser) extractMaProfileAudioCodecAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if p == nil || attrs == nil || len(tokens) < 3 {
		return
	}
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		base := strings.TrimSpace(tokens[i].Text)
		m := maProfileBitDepthPattern.FindStringSubmatch(base)
		if len(m) != 2 {
			continue
		}
		if tokens[i+1].Removed || tokens[i+2].Removed {
			continue
		}
		sep := strings.TrimSpace(tokens[i+1].Text)
		if sep != "_" && sep != "-" {
			continue
		}
		codecNorm, ok := p.normalizeAudioCodecToken(strings.TrimSpace(tokens[i+2].Text))
		if !ok || strings.TrimSpace(codecNorm) == "" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) && !hasNearbyTechContext(tokens, i, 4) && i < len(tokens)-4 {
			continue
		}
		bit := strings.TrimLeft(m[1], "0")
		if bit == "" {
			bit = m[1]
		}
		*attrs = append(*attrs, model.Attribute{
			Type:       model.AttrBitDepth,
			Normalized: bit + "bit",
			TokenStart: i,
			TokenEnd:   i + 1,
			Confidence: 0.72,
			Source:     "tech_combo_token",
		})
		if !hasAudioCodecAttrNorm(*attrs, codecNorm) {
			*attrs = append(*attrs, model.Attribute{
				Type:       model.AttrAudioCodec,
				Normalized: codecNorm,
				TokenStart: i + 2,
				TokenEnd:   i + 3,
				Confidence: 0.72,
				Source:     "tech_combo_token",
			})
		}
		tokens[i].Removed = true
		tokens[i+1].Removed = true
		tokens[i+2].Removed = true
		i += 2
	}
}

func (p *Parser) extractStandaloneMaProfileBitDepthAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if attrs == nil || len(tokens) == 0 {
		return
	}
	seenBit := make(map[string]struct{}, 2)
	for _, a := range *attrs {
		if a.Type != model.AttrBitDepth {
			continue
		}
		n := strings.ToLower(strings.TrimSpace(a.Normalized))
		if n != "" {
			seenBit[n] = struct{}{}
		}
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		base := strings.TrimSpace(tokens[i].Text)
		m := maProfileBitDepthPattern.FindStringSubmatch(base)
		if len(m) != 2 {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) && !hasNearbyTechContext(tokens, i, 4) && !(p != nil && p.hasNearbyAudioCodecToken(tokens, i, 4)) {
			continue
		}
		bit := strings.TrimLeft(m[1], "0")
		if bit == "" {
			bit = m[1]
		}
		norm := bit + "bit"
		if _, ok := seenBit[strings.ToLower(norm)]; !ok {
			*attrs = append(*attrs, model.Attribute{
				Type:       model.AttrBitDepth,
				Normalized: norm,
				TokenStart: i,
				TokenEnd:   i + 1,
				Confidence: 0.72,
				Source:     "tech_combo_token",
			})
			seenBit[strings.ToLower(norm)] = struct{}{}
		}
		tokens[i].Removed = true
	}
}

var crc8HexTokenPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}$`)

func extractCRCAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if attrs == nil || len(tokens) == 0 {
		return
	}
	seen := make(map[string]struct{}, 4)
	for _, a := range *attrs {
		if a.Type != model.AttrCRC {
			continue
		}
		n := strings.ToUpper(strings.TrimSpace(a.Normalized))
		if n != "" {
			seen[n] = struct{}{}
		}
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		raw := strings.TrimSpace(tokens[i].Text)
		if !crc8HexTokenPattern.MatchString(raw) {
			continue
		}
		low := strings.ToLower(raw)
		if !strings.ContainsAny(low, "abcdef") {
			// Avoid consuming plain numeric dates like 20260103.
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		if !hasNearbyTechContext(tokens, i, 10) && i < len(tokens)-6 {
			continue
		}
		norm := strings.ToUpper(raw)
		if _, ok := seen[norm]; !ok {
			*attrs = append(*attrs, model.Attribute{
				Type:       model.AttrCRC,
				Normalized: norm,
				TokenStart: i,
				TokenEnd:   i + 1,
				Confidence: 0.82,
				Source:     "crc_token",
			})
			seen[norm] = struct{}{}
		}
		tokens[i].Removed = true
	}
}

var volumeWordPattern = regexp.MustCompile(`(?i)^vol(?:ume)?\.?$`)
var shortNumericPattern = regexp.MustCompile(`^\d{1,3}$`)
var numericRangeTokenPattern = regexp.MustCompile(`^\d{1,4}\s*[-~～−–]\s*\d{1,4}$`)

func hasDiscLikeSourceTypeAttr(attrs []model.Attribute) bool {
	for _, a := range attrs {
		if a.Type != model.AttrSourceType {
			continue
		}
		n := strings.ToUpper(strings.TrimSpace(a.Normalized))
		n = strings.ReplaceAll(n, "-", "")
		n = strings.ReplaceAll(n, "_", "")
		switch {
		case strings.Contains(n, "BD"),
			strings.Contains(n, "BLURAY"),
			strings.Contains(n, "DVD"),
			strings.Contains(n, "VHS"),
			strings.Contains(n, "LASERDISC"),
			strings.Contains(n, "LD"),
			strings.Contains(n, "REMUX"):
			return true
		}
	}
	return false
}

func hasEpisodeRangeMarker(tokens []model.Token, idx int) bool {
	next := nextAliveToken(tokens, idx+1)
	if next < 0 {
		return false
	}
	marker := strings.ToLower(strings.TrimSpace(tokens[next].Text))
	switch marker {
	case "-", "~", "～", "−", "–", "to", "&", "＆", ",", "，", "/", "／":
		return true
	default:
		return false
	}
}

func (p *Parser) extractVolumeEpisodeMetaMarkersAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if len(tokens) == 0 {
		return
	}
	hasDiscSource := false
	if attrs != nil {
		hasDiscSource = hasDiscLikeSourceTypeAttr(*attrs)
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		cur := strings.TrimSpace(tokens[i].Text)
		curLower := strings.ToLower(cur)

		// Consume metadata prefixes like "Episodes 01-10" in package/tech context.
		if curLower == "episode" || curLower == "episodes" {
			j := nextAliveToken(tokens, i+1)
			if j < 0 {
				continue
			}
			next := strings.TrimSpace(tokens[j].Text)
			if (shortNumericPattern.MatchString(next) || numericRangeTokenPattern.MatchString(next)) &&
				(isInBracket(tokens, i) || isInParen(tokens, i) || hasEpisodeRangeMarker(tokens, j) || hasDiscSource) {
				tokens[i].Removed = true
			}
			continue
		}

		// Consume volume marker word in clear disc/batch packaging context.
		if !volumeWordPattern.MatchString(cur) {
			continue
		}
		j := nextAliveToken(tokens, i+1)
		dotIdx := -1
		if j >= 0 && strings.TrimSpace(tokens[j].Text) == "." {
			dotIdx = j
			j = nextAliveToken(tokens, j+1)
		}
		if j < 0 {
			continue
		}
		next := strings.TrimSpace(tokens[j].Text)
		if !shortNumericPattern.MatchString(next) {
			continue
		}
		if !(isInBracket(tokens, i) || isInParen(tokens, i) || hasDiscSource) {
			continue
		}
		tokens[i].Removed = true
		if dotIdx >= 0 {
			tokens[dotIdx].Removed = true
		}
	}
}

func (p *Parser) extractStandaloneSourceTypeAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if p == nil || attrs == nil || len(tokens) == 0 || len(p.sourceTypeCanonical) == 0 {
		return
	}
	hasSource := hasSourceTypeAttr(*attrs)
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.ToUpper(strings.TrimSpace(tokens[i].Text))
		if txt == "" {
			continue
		}
		matched := false
		spanEnd := i + 1
		norm := ""
		if v, ok := p.sourceTypeCanonical[strings.ToLower(txt)]; ok && strings.TrimSpace(v) != "" {
			norm = strings.TrimSpace(v)
			matched = true
		}
		if i+2 < len(tokens) && !tokens[i+1].Removed && !tokens[i+2].Removed && strings.TrimSpace(tokens[i+1].Text) == "-" {
			right := strings.TrimSpace(tokens[i+2].Text)
			combinedKey := strings.ToLower(txt + "-" + right)
			if v, ok := p.sourceTypeCanonical[combinedKey]; ok && strings.TrimSpace(v) != "" {
				spanEnd = i + 3
				norm = strings.TrimSpace(v)
				matched = true
			} else if strings.EqualFold(txt, "DVD") && (strings.EqualFold(right, "ISO") || strings.EqualFold(right, "ISOS") || strings.EqualFold(right, "REMUX")) {
				if v, ok := p.sourceTypeCanonical["dvd"]; ok && strings.TrimSpace(v) != "" {
					spanEnd = i + 3
					norm = strings.TrimSpace(v)
					matched = true
				}
			}
		}
		if !matched || norm == "" {
			continue
		}
		techCtx := isInBracket(tokens, i) || isInParen(tokens, i) || hasNearbyTechContext(tokens, i, 4)
		specialCtx := hasNearbySpecialReleaseToken(tokens, i, 3)
		if !techCtx && !specialCtx {
			continue
		}
		if !hasSource && !specialCtx {
			*attrs = append(*attrs, model.Attribute{
				Type:       model.AttrSourceType,
				Normalized: norm,
				TokenStart: i,
				TokenEnd:   spanEnd,
				Confidence: 0.72,
				Source:     "source_standalone",
			})
			hasSource = true
		}
		for k := i; k < spanEnd && k < len(tokens); k++ {
			tokens[k].Removed = true
		}
	}
}

func (p *Parser) extractStandaloneAudioCodecAttrsAndRemoveFromTokens(tokens []model.Token, attrs *[]model.Attribute) {
	if p == nil || attrs == nil || len(tokens) == 0 || len(p.audioCodecCanonical) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		base := strings.TrimSpace(tokens[i].Text)
		if base == "" {
			continue
		}
		spanEnd := i + 1
		norm := ""
		if i+2 < len(tokens) && !tokens[i+1].Removed && !tokens[i+2].Removed && strings.TrimSpace(tokens[i+1].Text) == "-" {
			combined := strings.TrimSpace(tokens[i].Text) + "-" + strings.TrimSpace(tokens[i+2].Text)
			if v, ok := p.normalizeAudioCodecToken(combined); ok && strings.TrimSpace(v) != "" {
				norm = strings.TrimSpace(v)
				spanEnd = i + 3
			}
		}
		if norm == "" {
			if v, ok := p.normalizeAudioCodecToken(base); ok && strings.TrimSpace(v) != "" {
				norm = strings.TrimSpace(v)
			}
		}
		if norm == "" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) && !hasNearbyTechContext(tokens, i, 4) && !p.hasNearbyAudioCodecToken(tokens, i, 4) {
			continue
		}
		if hasAudioCodecAttrNorm(*attrs, norm) {
			continue
		}
		*attrs = append(*attrs, model.Attribute{
			Type:       model.AttrAudioCodec,
			Normalized: norm,
			TokenStart: i,
			TokenEnd:   spanEnd,
			Confidence: 0.72,
			Source:     "audio_codec_standalone",
		})
		for k := i; k < spanEnd && k < len(tokens); k++ {
			tokens[k].Removed = true
		}
	}
}

func isTechTokenConsumeAttrType(t model.AttrType) bool {
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

func consumeTokensByTechAttrSpans(tokens []model.Token, attrs []model.Attribute) {
	if len(tokens) == 0 || len(attrs) == 0 {
		return
	}
	for _, a := range attrs {
		if !isTechTokenConsumeAttrType(a.Type) {
			continue
		}
		if a.TokenStart < 0 || a.TokenEnd <= a.TokenStart || a.TokenStart >= len(tokens) {
			continue
		}
		end := a.TokenEnd
		if end > len(tokens) {
			end = len(tokens)
		}
		for i := a.TokenStart; i < end; i++ {
			tokens[i].Removed = true
		}
	}
}

func consumeTokensByEPSlotPrimarySpan(tokens []model.Token, epslot *model.EPSlot) {
	if len(tokens) == 0 || epslot == nil || epslot.Primary == nil {
		return
	}
	start := epslot.Primary.TokenStart
	end := epslot.Primary.TokenEnd
	if start < 0 || end <= start || start >= len(tokens) {
		return
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	for i := start; i < end; i++ {
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

func looksLikeBitDepthWordToken(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return low == "bit" || low == "bits"
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

func (p *Parser) Parse(rawTitle string) (*model.ParseResult, error) {
	return p.ParseWithHints(rawTitle, nil)
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

func (p *Parser) ParseWithHints(rawTitle string, hints *ParseHints) (*model.ParseResult, error) {
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

	attrs := make([]model.Attribute, 0, 16)
	seasonRange := extractSeasonRangeAndRemoveFromTokens(tokens, &attrs)
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

	packKind := detectPackKind(p.cfg, rawTitle)
	ctx := newParseContext(rawTitle, packKind, tailParenBlock)
	ctx.AliasValidFunc = p.isValidAliasToken
	ctx.GroupSlot2Rules = p.groupSlot2Rules
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

	// Optional overrides (evaluation / reranking). Apply after parse passes finalize episode fields
	// so title span inference uses the overridden episode anchor.
	if hints != nil {
		if strings.TrimSpace(hints.OverrideEpisode) != "" && hints.OverrideEpisodeIdx >= 0 {
			episode = strings.TrimSpace(hints.OverrideEpisode)
			episodeIdx = hints.OverrideEpisodeIdx
			// Keep range empty when overriding a single episode.
			episodeRange = ""
			episodeRangeIdx = -1
			episodeList = ""
			ctx.Episode = episode
			ctx.EpisodeIdx = episodeIdx
			ctx.EpisodeRange = ""
			ctx.EpisodeRangeIdx = -1
			ctx.EpisodeList = ""
			ctx.EpisodeListIdx = -1
			// Rebuild EPSlot for consistency.
			ctx.EPSlot = buildEPSlotCandidates(tokens, ctx.Season, ctx.SeasonIdx, ctx.EpisodeFromSxxExx, ctx.SxxExxIdx, ctx.Episode, ctx.EpisodeIdx, ctx.EpisodeRange, ctx.EpisodeRangeIdx, ctx.EpisodeList, ctx.EpisodeListIdx, ctx.SPKind)
			finalizeEPSlotPrimary(ctx.EPSlot, ctx.Season, ctx.Episode, ctx.EpisodeRange, ctx.EpisodeList, ctx.SPKind, ctx.EpisodeIdx, ctx.EpisodeRangeIdx, ctx.EpisodeListIdx, ctx.SxxExxIdx)
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
		if tg := detectTailGroupFromRawTitle(rawTitle); tg != "" {
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
	attrs = dropContentsAttrsInsideTitleSpan(attrs, titleStart, titleEnd)
	if strings.TrimSpace(spKind) == "" {
		spKind = inferSPKindFromContentsAttrs(attrs)
	}
	if strings.TrimSpace(packKind) == "" {
		packKind = inferPackKindFromContentsAttrs(packKind, attrs)
	}
	if strings.TrimSpace(packKind) == "" {
		if strings.TrimSpace(episode) == "" && strings.TrimSpace(episodeRange) == "" {
			if regexp.MustCompile(`(?i)\bBD\b`).MatchString(rawTitle) {
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
		attrs = removeAttributeType(attrs, model.AttrEpisode)
		attrs = removeAttributeType(attrs, model.AttrEpisodeList)
		attrs = removeAttributeType(attrs, model.AttrEpisodeRange)
	}
	movieRouteHardExcluded := hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
	movieContentSignal := hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
	epRoute := computeEpRoute(tokens, episode, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
	contentKind, contentConf := inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	if ep2, mi2, epIdx2, miIdx2, changed := normalizeMovieEpisodeSignals(rawTitle, contentKind, episode, episodeRange, season, seasonRange, packKind, hasMovieKeyword, movieIndex, movieIndexRange, episodeIdx, movieIndexIdx); changed {
		episode, movieIndex, episodeIdx, movieIndexIdx = ep2, mi2, epIdx2, miIdx2
		title = trimMovieDashEpisodeTailFromTitle(rawTitle, title, episode, movieIndex)
		epslot = nil
		attrs = removeAttributeType(attrs, model.AttrEpisode)
		movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
		movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
		epRoute = computeEpRoute(tokens, episode, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
		contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	}
	templateID, templateConf, routeID := matchTemplate(p.cfg, epRoute, contentKind, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, attrs, tokens)
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
	epRoute = computeEpRoute(tokens, episode, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
	contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	if shouldDropEpisodeRangeForMovieRange(rawTitle, movieIndexRange, episode, episodeRange) {
		episode = ""
		episodeIdx = -1
		episodeList = ""
		episodeRange = ""
		episodeRangeIdx = -1
		epslot = nil
		attrs = removeAttributeType(attrs, model.AttrEpisode)
		attrs = removeAttributeType(attrs, model.AttrEpisodeList)
		attrs = removeAttributeType(attrs, model.AttrEpisodeRange)
		movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
		movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
		epRoute = computeEpRoute(tokens, episode, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
		contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	}
	if ep2, mi2, epIdx2, miIdx2, changed := normalizeMovieEpisodeSignals(rawTitle, contentKind, episode, episodeRange, season, seasonRange, packKind, hasMovieKeyword, movieIndex, movieIndexRange, episodeIdx, movieIndexIdx); changed {
		episode, movieIndex, episodeIdx, movieIndexIdx = ep2, mi2, epIdx2, miIdx2
		title = trimMovieDashEpisodeTailFromTitle(rawTitle, title, episode, movieIndex)
		epslot = nil
		attrs = removeAttributeType(attrs, model.AttrEpisode)
		movieRouteHardExcluded = hasMovieRouteHardExclusion(rawTitle, attrs, episodeRange, season, seasonRange, packKind)
		movieContentSignal = hasMovieContentSignal(attrs, hasMovieKeyword, movieIndex, spKind)
		epRoute = computeEpRoute(tokens, episode, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
		contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
	}
	templateID, templateConf, routeID = matchTemplate(p.cfg, epRoute, contentKind, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, attrs, tokens)
	if strings.HasPrefix(routeID, "streaming_release_") {
		p.applyStreamingReleaseRouteV1(tailParenBlock, &aliases, &attrs)
	}
	title = trimMovieRangeStartTailFromTitle(rawTitle, title, movieIndexRange, episode, episodeRange)
	origTitle := title
	title = sanitizePrimaryTitleForOutput(title)
	title = trimTitleTailByPackKind(title, packKind)
	aliases = p.sanitizeAliasesForOutput(aliases, title, episode, season, spKind)
	title, aliases = p.rebalanceGenericPrimaryTitleFromAliases(title, aliases)
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
			epRoute = computeEpRoute(tokens, episode, episodeRange, season, seasonRange, spKind, movieIndex, hasMovieKeyword, movieRouteHardExcluded)
			contentKind, contentConf = inferContentKind(p.cfg, episode, episodeRange, season, seasonRange, packKind, spKind, movieIndex, movieIndexRange, hasTech, hasMovieKeyword, hasYear, movieRouteHardExcluded, movieContentSignal)
			if tid, tconf, _ := matchTemplate(p.cfg, epRoute, contentKind, group, episode, episodeRange, season, packKind, spKind, movieIndex, hasTech, hasMovieKeyword, attrs, tokens); strings.TrimSpace(tid) != "" && tid != "TF_UNKNOWN_DEFAULT" {
				templateID = tid
				templateConf = tconf
			}
		}
	}
	if overrideApplied {
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
	title, aliases = p.rebalanceGenericPrimaryTitleFromAliases(title, aliases)
	packProfile := p.inferPackProfile(contentKind, packKind, season, seasonRange, attrs)
	attrs = upsertTitleAttribute(attrs, title, "canonical")
	attrs = upsertGroupAttribute(attrs, group, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrEpisode, episode, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrEpisodeList, episodeList, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrEpisodeRange, episodeRange, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrSeason, season, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrSeasonRange, seasonRange, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrVersion, version, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrPackKind, packKind, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrPackProfile, packProfile, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrSPKind, spKind, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrMovieIndex, movieIndex, "canonical")
	attrs = upsertScalarAttribute(attrs, model.AttrMovieIndexRange, movieIndexRange, "canonical")
	attrs = appendTitleAliasAttributes(attrs, aliases, title, "canonical")
	attrs = p.promoteNoteAttributes(attrs)
	attrs = dropRedundantNoteAttributes(attrs)
	p.normalizePlatformAttrs(attrs)
	p.normalizeSourceTypeAttrs(attrs)
	attrs = p.normalizeSubtitleAttrs(attrs)
	attrs = p.enrichBatchSourceTypeFromPackKind(attrs, contentKind)
	attrs = dedupeAttributes(attrs)
	attrs = collapseOverlappingSpecificAudioCodecs(attrs)
	ctx.AudioSlot = reconcileAudioSlotWithAttributes(ctx.AudioSlot, attrs)
	epslot = ensureSPPrimaryFromSPKind(epslot, season, episode, episodeRange, episodeList, spKind)
	epslot = adjudicateEPSlotPrimaryAxis(epslot, tokens, season)
	epslot = pruneBatchEPSlotForConservativeOutput(epslot, contentKind, episode, episodeRange, spKind)
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

	res := &model.ParseResult{
		RawTitle:           originalRawTitle,
		ContentKind:        contentKind,
		ContentConfidence:  contentConf,
		TemplateID:         templateID,
		TemplateConfidence: templateConf,
		EPSlot:             epslot,
		AudioSlot:          ctx.AudioSlot,
		Attributes:         attrs,
		Tokens:             tokens,
	}

	return res, nil
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
		return epslot
	}
	if strings.TrimSpace(episode) != "" || strings.TrimSpace(episodeRange) != "" || strings.TrimSpace(spKind) != "" {
		return epslot
	}
	return nil
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

func pruneUnknownCoreAttributes(attrs []model.Attribute) []model.Attribute {
	if len(attrs) == 0 {
		return attrs
	}
	out := make([]model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		switch a.Type {
		case model.AttrGroup, model.AttrTitle, model.AttrTitleAlias:
			if strings.TrimSpace(a.Normalized) == "" {
				continue
			}
			out = append(out, a)
		}
	}
	return dedupeAttributes(out)
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

func dropRedundantNoteAttributes(attrs []model.Attribute) []model.Attribute {
	if len(attrs) == 0 {
		return attrs
	}
	hasPlatform := map[string]struct{}{}
	hasResolution := map[string]struct{}{}
	hasSourceType := map[string]struct{}{}
	hasSeason := map[string]struct{}{}
	packKindPresent := false
	movieSignalPresent := false

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
		case model.AttrSeason:
			hasSeason[strings.TrimLeft(strings.TrimSpace(n), "0")] = struct{}{}
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
		if m := seasonNotePattern.FindStringSubmatch(n); len(m) == 2 {
			ss := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if ss == "" {
				ss = "0"
			}
			if _, ok := hasSeason[ss]; ok {
				continue
			}
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

func (p *Parser) applyGroupRouteOverride(
	rawTitle string,
	group string,
	episodeRange string,
	title string,
	aliases []string,
	templateID string,
	contentKind string,
	routeID string,
) (string, string, []string, string, string, string, bool) {
	if p == nil || len(p.groupRouteRules) == 0 {
		return group, title, aliases, templateID, contentKind, routeID, false
	}
	gk := strings.ToLower(strings.TrimSpace(group))
	rawLow := strings.ToLower(strings.TrimSpace(rawTitle))
	for _, r := range p.groupRouteRules {
		if len(r.groups) > 0 {
			if _, ok := r.groups[gk]; !ok {
				continue
			}
		}
		if r.requireEpisodeRange && strings.TrimSpace(episodeRange) == "" {
			continue
		}
		if r.requireEpisodeLike {
			if _, _, ok := extractEpisodeLikeFromThirdBracket(rawTitle); !ok {
				continue
			}
		}
		if r.requireEpisodeWithTitle {
			_, _, epTitle, epSep, ok := extractEpisodeLikeFromThirdBracketDetailedWithSep(rawTitle)
			if !ok || strings.TrimSpace(epTitle) == "" {
				continue
			}
			if len(r.episodeTitleSepIn) > 0 {
				if _, ok := r.episodeTitleSepIn[epSep]; !ok {
					continue
				}
			}
		}
		if len(r.rawMustContainAny) > 0 {
			hit := false
			for _, kw := range r.rawMustContainAny {
				if kw != "" && strings.Contains(rawLow, kw) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		outGroup := group
		outTitle := title
		outAliases := aliases
		outTemplateID := templateID
		outContentKind := contentKind
		outRouteID := routeID

		if strings.TrimSpace(r.forceGroup) != "" {
			outGroup = r.forceGroup
		}
		if strings.TrimSpace(r.forceTitle) != "" {
			outTitle = r.forceTitle
		}
		if r.clearAliases {
			outAliases = nil
		}
		if strings.TrimSpace(r.forceTemplateID) != "" {
			outTemplateID = r.forceTemplateID
		}
		if strings.TrimSpace(r.forceContentKind) != "" {
			outContentKind = r.forceContentKind
		}
		if strings.TrimSpace(r.forceRouteID) != "" {
			outRouteID = r.forceRouteID
		}
		if len(r.titleStripPrefixes) > 0 {
			outTitle = stripConfiguredPrefixes(outTitle, r.titleStripPrefixes)
		}
		if len(r.aliasStripPrefixes) > 0 && len(outAliases) > 0 {
			cleaned := make([]string, 0, len(outAliases))
			for _, a := range outAliases {
				c := stripConfiguredPrefixes(a, r.aliasStripPrefixes)
				if strings.TrimSpace(c) == "" {
					continue
				}
				cleaned = append(cleaned, c)
			}
			outAliases = cleaned
		}

		// Safe fallback: if override cannot produce required core fields, skip it.
		if strings.TrimSpace(outGroup) == "" || strings.TrimSpace(outTitle) == "" {
			continue
		}
		// If this override depends on range releases, keep general route when range is absent.
		if r.requireEpisodeRange && strings.TrimSpace(episodeRange) == "" {
			continue
		}
		return outGroup, outTitle, outAliases, outTemplateID, outContentKind, outRouteID, true
	}
	return group, title, aliases, templateID, contentKind, routeID, false
}

func stripConfiguredPrefixes(s string, prefixes []string) string {
	out := strings.TrimSpace(s)
	if out == "" || len(prefixes) == 0 {
		return out
	}
	for {
		changed := false
		for _, p := range prefixes {
			pp := strings.TrimSpace(p)
			if pp == "" {
				continue
			}
			rest, ok := trimPrefixFold(out, pp)
			if !ok {
				continue
			}
			out = strings.TrimSpace(strings.TrimLeft(rest, "-_—–:：|/·+~. "))
			changed = true
			break
		}
		if !changed {
			break
		}
	}
	return strings.TrimSpace(out)
}

func applyGroupTitleAliasSpecialization(rawTitle string, routeID string, title string, aliases []string) (string, []string) {
	rid := strings.ToLower(strings.TrimSpace(routeID))
	switch rid {
	case "group_epseg_fate_sf_wgjd_v1", "group_epseg_fate_sf_atlas_v1":
		return applyFateStrangeFakeTitleAliasSpecialization(rawTitle, title, aliases)
	default:
		return title, aliases
	}
}

func applyFateStrangeFakeTitleAliasSpecialization(rawTitle string, title string, aliases []string) (string, []string) {
	matches := bracketSegmentPattern.FindAllStringSubmatch(rawTitle, -1)
	if len(matches) < 2 {
		return title, aliases
	}
	seg2 := strings.TrimSpace(matches[1][1])
	if seg2 == "" {
		return title, aliases
	}
	parts := strings.SplitN(seg2, "_", 2)
	if len(parts) != 2 {
		return title, aliases
	}
	mainTitle := cleanTitleToken(parts[0])
	alias := cleanTitleToken(parts[1])
	if mainTitle == "" || alias == "" {
		return title, aliases
	}
	alias = strings.TrimSpace(strings.ReplaceAll(alias, "／", "/"))
	alias = strings.Join(strings.Fields(alias), " ")
	merged := mergeAliases([]string{alias}, aliases)
	merged = dropAliasFragmentsOfPrimary(alias, merged)
	return mainTitle, merged
}

func dropAliasFragmentsOfPrimary(primary string, aliases []string) []string {
	pk := aliasSemanticKey(primary)
	if pk == "" || len(aliases) == 0 {
		return aliases
	}
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		aa := strings.TrimSpace(a)
		if aa == "" {
			continue
		}
		ak := aliasSemanticKey(aa)
		if ak != "" && ak != pk && len(ak) >= 4 && strings.Contains(pk, ak) {
			continue
		}
		out = append(out, aa)
	}
	return out
}

func normalizeEpisodeTitleSepConstraint(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "_", "underscore":
		return "_"
	case "-", "dash":
		return "-"
	case ".", "dot":
		return "."
	case "&", "amp", "and":
		return "&"
	case "space", "whitespace":
		return " "
	}
	if raw == " " {
		return " "
	}
	return ""
}

func trimPrefixFold(s, prefix string) (string, bool) {
	if prefix == "" {
		return s, false
	}
	if len(s) < len(prefix) {
		return s, false
	}
	head := s[:len(prefix)]
	if !strings.EqualFold(head, prefix) {
		return s, false
	}
	return s[len(prefix):], true
}

var bracketSegmentPattern = regexp.MustCompile(`\[(.*?)\]`)
var episodeLikeThirdBracketPattern = regexp.MustCompile(`(?i)^\s*(\d{1,4})(?:\s*([_ .-]|&)\s*(.*))?\s*$`)
var spKindLeadPattern = regexp.MustCompile(`(?i)^(SP|OVA|OAD|ONA|NCOP|NCED|PV|CM)\b`)
var episodeLikeAliasPattern = regexp.MustCompile(`(?i)^\s*\d{1,4}(?:\s*([_ .-]|&)\s*.*)?$`)
var bracketDatePattern = regexp.MustCompile(`\[(\d{4}[./-]\d{2}[./-]\d{2})\]`)

func extractEpisodeLikeFromThirdBracket(rawTitle string) (episode string, spKind string, ok bool) {
	ep, sp, _, _, ok := extractEpisodeLikeFromThirdBracketDetailedWithSep(rawTitle)
	return ep, sp, ok
}

func extractEpisodeLikeFromThirdBracketDetailed(rawTitle string) (episode string, spKind string, epTitle string, ok bool) {
	ep, sp, title, _, ok := extractEpisodeLikeFromThirdBracketDetailedWithSep(rawTitle)
	return ep, sp, title, ok
}

func extractEpisodeLikeFromThirdBracketDetailedWithSep(rawTitle string) (episode string, spKind string, epTitle string, epSep string, ok bool) {
	if strings.TrimSpace(rawTitle) == "" {
		return "", "", "", "", false
	}
	matches := bracketSegmentPattern.FindAllStringSubmatch(rawTitle, -1)
	if len(matches) < 3 {
		return "", "", "", "", false
	}
	seg := strings.TrimSpace(matches[2][1])
	if seg == "" {
		return "", "", "", "", false
	}
	m := episodeLikeThirdBracketPattern.FindStringSubmatch(seg)
	if len(m) == 0 {
		return "", "", "", "", false
	}
	epRaw := strings.TrimSpace(m[1])
	epNum, err := strconv.Atoi(epRaw)
	if err != nil || epNum < 0 {
		return "", "", "", "", false
	}
	episode = strconv.Itoa(epNum)
	rest := ""
	sep := strings.TrimSpace(m[2])
	if m[2] == " " {
		sep = " "
	}
	if len(m) >= 4 {
		rest = strings.TrimSpace(m[3])
	}
	if strings.EqualFold(strings.TrimSpace(m[2]), "&") {
		if sm := spKindLeadPattern.FindStringSubmatch(rest); len(sm) > 1 {
			spKind = strings.ToUpper(strings.TrimSpace(sm[1]))
			epTitle = strings.TrimSpace(rest[len(strings.TrimSpace(sm[0])):])
			epTitle = strings.TrimSpace(strings.TrimLeft(epTitle, "_ .-"))
			return episode, spKind, epTitle, "&", true
		}
	}
	if sm := spKindLeadPattern.FindStringSubmatch(rest); len(sm) > 1 {
		spKind = strings.ToUpper(strings.TrimSpace(sm[1]))
		epTitle = strings.TrimSpace(rest[len(strings.TrimSpace(sm[0])):])
		epTitle = strings.TrimSpace(strings.TrimLeft(epTitle, "_ .-"))
		return episode, spKind, epTitle, sep, true
	}
	return episode, "", rest, sep, true
}

func extractTailBetweenThirdAndFourthBracket(rawTitle string) string {
	if strings.TrimSpace(rawTitle) == "" {
		return ""
	}
	closeCount := 0
	start := -1
	for i, r := range rawTitle {
		if r == ']' || r == '】' {
			closeCount++
			if closeCount == 3 {
				start = i + 1
				break
			}
		}
	}
	if start < 0 || start >= len(rawTitle) {
		return ""
	}
	end := len(rawTitle)
	for i, r := range rawTitle[start:] {
		if r == '[' || r == '【' {
			end = start + i
			break
		}
	}
	if end <= start {
		return ""
	}
	s := strings.TrimSpace(rawTitle[start:end])
	s = strings.TrimSpace(strings.Trim(s, "-_/ "))
	return s
}

func pruneEpisodeLikeAliases(aliases []string) []string {
	if len(aliases) == 0 {
		return aliases
	}
	out := make([]string, 0, len(aliases))
	for _, a := range aliases {
		aa := strings.TrimSpace(a)
		if aa == "" {
			continue
		}
		if episodeLikeAliasPattern.MatchString(aa) {
			continue
		}
		out = append(out, a)
	}
	return out
}

var rawMovieMixPattern = regexp.MustCompile(`(?i)\b(?:tv|ova|oad|ona|sp|special)\s*\+\s*(?:movie|movies|film|films)\b|\b(?:movie|movies|film|films)\s*\+\s*(?:tv|ova|oad|ona|sp|special)\b|\btv\b.{0,32}\+\s*(?:movie|movies|film|films)\b|\b(?:movie|movies|film|films)\b.{0,32}\+\s*tv\b|\b(?:movies|films)\s*\d{1,3}\s*[-~]\s*\d{1,3}\b|\bmovies\b|\bfilms\b`)
var explicitTVEpisodeRangeRawPattern = regexp.MustCompile(`(?i)\btv\s*0*\d{1,4}\s*[-~]\s*0*\d{1,4}\b`)

func hasMovieKeywordInRawTitle(raw string) bool {
	low := strings.ToLower(strings.TrimSpace(raw))
	if low == "" {
		return false
	}
	if strings.Contains(low, "movie") || strings.Contains(low, "movies") || strings.Contains(low, "film") || strings.Contains(low, "films") || strings.Contains(low, "gekijouban") || strings.Contains(low, "eiga") {
		return true
	}
	if strings.Contains(raw, "劇場版") || strings.Contains(raw, "剧场版") || strings.Contains(raw, "映画") || strings.Contains(raw, "大电影") || strings.Contains(raw, "大電影") {
		return true
	}
	return rawMovieTagPattern.MatchString(raw)
}

func shouldDropEpisodeRangeForMovieRange(rawTitle string, movieIndexRange string, episode string, episodeRange string) bool {
	mr := strings.TrimSpace(movieIndexRange)
	ep := strings.TrimSpace(episode)
	er := strings.TrimSpace(episodeRange)
	if mr == "" {
		return false
	}
	if ep == "" && er == "" {
		return false
	}
	// Keep TV range semantics for explicit mixed patterns like:
	// "TV 01-101 + Movie 01-04".
	if explicitTVEpisodeRangeRawPattern.MatchString(rawTitle) {
		return false
	}
	if ep != "" {
		return true
	}
	if er == mr {
		return true
	}
	return false
}

func shouldDemoteEpisodeToMovieIndex(rawTitle string, episode string, episodeRange string, season string, seasonRange string, packKind string, hasMovieKeyword bool, movieIndex string, movieIndexRange string) bool {
	if strings.TrimSpace(episode) == "" {
		return false
	}
	if strings.TrimSpace(movieIndexRange) != "" {
		return false
	}
	if strings.TrimSpace(episodeRange) != "" || strings.TrimSpace(season) != "" || strings.TrimSpace(seasonRange) != "" {
		return false
	}
	if isMovieExcludedByPackKind(packKind) {
		return false
	}
	hasMovieCtx := hasMovieKeyword || strings.TrimSpace(movieIndex) != "" || hasMovieKeywordInRawTitle(rawTitle)
	if !hasMovieCtx {
		return false
	}
	if strings.TrimSpace(movieIndex) != "" {
		return true
	}
	if moviePartSignalPattern.MatchString(rawTitle) {
		return true
	}
	if hasMovieKeywordInRawTitle(rawTitle) && movieBracketEpisodePattern.MatchString(rawTitle) {
		return true
	}
	return false
}

func normalizeMovieEpisodeSignals(rawTitle string, contentKind string, episode string, episodeRange string, season string, seasonRange string, packKind string, hasMovieKeyword bool, movieIndex string, movieIndexRange string, episodeIdx int, movieIndexIdx int) (string, string, int, int, bool) {
	if !strings.EqualFold(strings.TrimSpace(contentKind), "MOVIE") {
		return episode, movieIndex, episodeIdx, movieIndexIdx, false
	}
	if strings.TrimSpace(movieIndexRange) != "" {
		return episode, movieIndex, episodeIdx, movieIndexIdx, false
	}
	ep := strings.TrimSpace(episode)
	if ep == "" || strings.TrimSpace(episodeRange) != "" || strings.TrimSpace(season) != "" || strings.TrimSpace(seasonRange) != "" {
		return episode, movieIndex, episodeIdx, movieIndexIdx, false
	}
	if isMovieExcludedByPackKind(packKind) {
		return episode, movieIndex, episodeIdx, movieIndexIdx, false
	}
	hasMovieCtx := hasMovieKeyword || strings.TrimSpace(movieIndex) != "" || hasMovieKeywordInRawTitle(rawTitle)
	if !hasMovieCtx {
		return episode, movieIndex, episodeIdx, movieIndexIdx, false
	}
	newMovieIndex := strings.TrimSpace(movieIndex)
	if newMovieIndex == "" {
		base := ep
		if dot := strings.Index(base, "."); dot > 0 {
			base = base[:dot]
		}
		if n, err := strconv.Atoi(base); err == nil && n > 0 && n <= 120 {
			newMovieIndex = ep
			if movieIndexIdx < 0 {
				movieIndexIdx = episodeIdx
			}
		}
	}
	return "", newMovieIndex, -1, movieIndexIdx, true
}

func trimMovieDashEpisodeTailFromTitle(rawTitle string, title string, episode string, movieIndex string) string {
	if strings.TrimSpace(title) == "" {
		return title
	}
	if strings.TrimSpace(episode) != "" || strings.TrimSpace(movieIndex) == "" {
		return title
	}
	if !movieDashEpisodePattern.MatchString(rawTitle) {
		return title
	}
	out := strings.TrimSpace(title)
	out = regexp.MustCompile(`(?:\s*[-:：]\s*)?0*\d{1,3}\s*$`).ReplaceAllString(out, "")
	out = strings.TrimSpace(strings.Trim(out, "-:： "))
	if out != "" && !isMovieKeywordText(out) {
		base := strings.TrimSpace(movieIndex)
		if base != "" {
			movieWordPattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(out) + `\s+movie\s*0*` + regexp.QuoteMeta(base) + `\s*[-:：]\s*0*\d{1,3}\b`)
			if movieWordPattern.MatchString(rawTitle) {
				out = strings.TrimSpace(out + " Movie")
			}
		}
	}
	if out == "" {
		return title
	}
	return out
}

func trimMovieRangeStartTailFromTitle(rawTitle string, title string, movieIndexRange string, episode string, episodeRange string) string {
	if strings.TrimSpace(title) == "" {
		return title
	}
	if strings.TrimSpace(movieIndexRange) == "" {
		return title
	}
	if strings.TrimSpace(episode) != "" || strings.TrimSpace(episodeRange) != "" {
		return title
	}
	rng := strings.SplitN(strings.TrimSpace(movieIndexRange), "-", 2)
	if len(rng) != 2 {
		return title
	}
	start := strings.TrimLeft(strings.TrimSpace(rng[0]), "0")
	end := strings.TrimLeft(strings.TrimSpace(rng[1]), "0")
	if start == "" || end == "" {
		return title
	}
	if start == "0" || end == "0" {
		return title
	}
	ctxPattern := regexp.MustCompile(`(?i)(?:movie|film|劇場版|剧场版)\s*0*` + regexp.QuoteMeta(start) + `\s*[-~]\s*0*` + regexp.QuoteMeta(end) + `\b`)
	if !ctxPattern.MatchString(rawTitle) {
		return title
	}
	out := regexp.MustCompile(`(?i)((?:movie|film|劇場版|剧场版)\s*)0*`+regexp.QuoteMeta(start)+`\s*$`).ReplaceAllString(strings.TrimSpace(title), "$1")
	out = strings.TrimSpace(strings.Trim(out, "-:：_ "))
	if out == "" {
		return title
	}
	return out
}

func hasMovieRouteHardExclusion(rawTitle string, attrs []model.Attribute, episodeRange string, season string, seasonRange string, packKind string) bool {
	if strings.TrimSpace(episodeRange) != "" {
		return true
	}
	if strings.TrimSpace(seasonRange) != "" {
		return true
	}
	if strings.TrimSpace(season) != "" {
		return true
	}
	if isMovieExcludedByPackKind(packKind) {
		return true
	}
	if rawMovieMixPattern.MatchString(rawTitle) {
		return true
	}
	hasMovie := false
	hasTVLike := false
	for _, a := range attrs {
		if a.Type != model.AttrContents {
			continue
		}
		v := strings.ToUpper(strings.TrimSpace(a.Normalized))
		if v == "" {
			continue
		}
		switch v {
		case "MOVIE":
			hasMovie = true
		case "TV", "OVA", "OAD", "ONA", "SP", "EX":
			hasTVLike = true
		}
		if hasMovie && hasTVLike {
			return true
		}
	}
	return false
}

func isMovieExcludedByPackKind(packKind string) bool {
	pk := strings.ToLower(strings.TrimSpace(packKind))
	if pk == "" {
		return false
	}
	switch pk {
	case "batch", "complete", "collection", "boxset", "box set", "全集", "合集", "全话", "全話", "完结", "完結", "mixed_pack", "tv_pack":
		return true
	default:
		return false
	}
}

func hasMovieContentSignal(attrs []model.Attribute, hasMovieKeyword bool, movieIndex string, spKind string) bool {
	if hasMovieKeyword || strings.TrimSpace(movieIndex) != "" || strings.EqualFold(strings.TrimSpace(spKind), "MOVIE") {
		return true
	}
	for _, a := range attrs {
		if a.Type == model.AttrMovieFlag {
			if strings.TrimSpace(a.Normalized) != "" {
				return true
			}
			continue
		}
		if a.Type != model.AttrContents {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(a.Normalized), "MOVIE") {
			return true
		}
	}
	return false
}

func inferPackKindFromContentsAttrs(currentPackKind string, attrs []model.Attribute) string {
	if strings.TrimSpace(currentPackKind) != "" {
		return strings.TrimSpace(currentPackKind)
	}
	seen := map[string]struct{}{}
	hasTV := false
	hasOther := false
	for _, a := range attrs {
		if a.Type != model.AttrContents {
			continue
		}
		n := strings.ToUpper(strings.TrimSpace(a.Normalized))
		if n == "" {
			continue
		}
		seen[n] = struct{}{}
		if n == "TV" {
			hasTV = true
		} else {
			hasOther = true
		}
	}
	if hasTV && hasOther {
		return "mixed_contents"
	}
	// Require at least two distinct non-TV content markers to derive "contents",
	// so a single "(Movie)" marker does not force content_kind to BATCH.
	if len(seen) >= 2 {
		return "contents"
	}
	return ""
}

func inferSPKindFromContentsAttrs(attrs []model.Attribute) string {
	for _, a := range attrs {
		if a.Type != model.AttrContents {
			continue
		}
		if n, ok := normalizeSPKindFromContents(a.Normalized); ok {
			return n
		}
	}
	return ""
}

func normalizeSPKindFromContents(v string) (string, bool) {
	n := strings.ToUpper(strings.TrimSpace(v))
	switch n {
	case "SPECIAL", "SP":
		return "SP", true
	case "OVA", "OAD", "ONA", "NCOP", "NCED", "PV":
		return n, true
	default:
		return "", false
	}
}

func dropContentsAttrsInsideTitleSpan(attrs []model.Attribute, titleStart int, titleEnd int) []model.Attribute {
	if len(attrs) == 0 {
		return attrs
	}
	if titleStart < 0 || titleEnd <= titleStart {
		return attrs
	}
	out := make([]model.Attribute, 0, len(attrs))
	for _, a := range attrs {
		if a.Type == model.AttrContents && a.TokenStart >= titleStart && a.TokenStart < titleEnd {
			// Do not treat in-title markers (e.g. "剧场版" in canonical title span)
			// as contents/meta markers.
			continue
		}
		out = append(out, a)
	}
	return out
}

func (p *Parser) inferPackProfile(contentKind string, packKind string, season string, seasonRange string, attrs []model.Attribute) string {
	contentKindUp := strings.ToUpper(strings.TrimSpace(contentKind))
	packKindLow := strings.ToLower(strings.TrimSpace(packKind))
	hasSeason := strings.TrimSpace(season) != ""
	hasSeasonRange := strings.TrimSpace(seasonRange) != ""
	contentSet := collectContentsSet(attrs)

	for _, r := range p.packProfileRules {
		if !packProfileRuleMatches(r, contentKindUp, packKindLow, contentSet, hasSeason, hasSeasonRange) {
			continue
		}
		return r.emit
	}
	return ""
}

func packProfileRuleMatches(r packProfileRule, contentKind string, packKind string, contentSet map[string]struct{}, hasSeason bool, hasSeasonRange bool) bool {
	if len(r.contentKindIn) > 0 {
		if _, ok := r.contentKindIn[contentKind]; !ok {
			return false
		}
	}
	if len(r.packKindIn) > 0 {
		if _, ok := r.packKindIn[packKind]; !ok {
			return false
		}
	}
	if len(r.packKindNotIn) > 0 {
		if _, blocked := r.packKindNotIn[packKind]; blocked {
			return false
		}
	}
	if r.hasSeason != nil && *r.hasSeason != hasSeason {
		return false
	}
	if r.hasSeasonRange != nil && *r.hasSeasonRange != hasSeasonRange {
		return false
	}
	if r.minContentsDistinct > 0 && len(contentSet) < r.minContentsDistinct {
		return false
	}
	if len(r.requiresContentsAll) > 0 {
		for k := range r.requiresContentsAll {
			if _, ok := contentSet[k]; !ok {
				return false
			}
		}
	}
	if len(r.requiresContentsAny) > 0 {
		hit := false
		for k := range r.requiresContentsAny {
			if _, ok := contentSet[k]; ok {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if len(r.forbidContentsAny) > 0 {
		for k := range r.forbidContentsAny {
			if _, blocked := contentSet[k]; blocked {
				return false
			}
		}
	}
	return true
}

func collectContentsSet(attrs []model.Attribute) map[string]struct{} {
	out := map[string]struct{}{}
	for _, a := range attrs {
		if a.Type != model.AttrContents {
			continue
		}
		n := strings.ToUpper(strings.TrimSpace(a.Normalized))
		if n == "" {
			continue
		}
		out[n] = struct{}{}
	}
	return out
}

func computeEpRoute(tokens []model.Token, episode string, episodeRange string, season string, seasonRange string, spKind string, movieIndex string, hasMovieKeyword bool, movieRouteHardExcluded bool) string {
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
	if !movieRouteHardExcluded && (strings.TrimSpace(movieIndex) != "" || hasMovieKeyword) {
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

func (p *Parser) normalizePlatformAttrs(attrs []model.Attribute) {
	for i := range attrs {
		if attrs[i].Type != model.AttrPlatform {
			continue
		}
		attrs[i].Normalized = p.normalizePlatformCode(attrs[i].Normalized)
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

func (p *Parser) normalizeSourceTypeAttrs(attrs []model.Attribute) {
	for i := range attrs {
		if attrs[i].Type != model.AttrSourceType {
			continue
		}
		attrs[i].Normalized = p.normalizeSourceType(attrs[i].Normalized)
	}
}

func normalizeSubLangToken(v string) string {
	s := strings.ToUpper(strings.TrimSpace(v))
	switch s {
	case "CHS", "ZH-HANS", "ZH_CN", "ZH-CN", "SC":
		return "SC"
	case "CHT", "ZH-HANT", "ZH_TW", "ZH-TW", "BIG5", "TC":
		return "TC"
	case "CHS+JPN", "JPN+CHS", "JPSC":
		return "JPSC"
	case "CHT+JPN", "JPN+CHT", "JPTC":
		return "JPTC"
	case "RAW", "NOSUB", "NO_SUB", "NO-SUB":
		return "RAW"
	default:
		return s
	}
}

func normalizeSubLangGroupToken(v string) string {
	s := strings.ToUpper(strings.TrimSpace(v))
	switch s {
	case "CHS+JPN", "JPN+CHS", "JPSC":
		return "JPSC"
	case "CHT+JPN", "JPN+CHT", "JPTC":
		return "JPTC"
	default:
		s = strings.ReplaceAll(s, "CHS", "SC")
		s = strings.ReplaceAll(s, "CHT", "TC")
		return s
	}
}

func normalizeSubFlagToken(v string) (string, bool, bool) {
	s := strings.ToUpper(strings.TrimSpace(v))
	switch s {
	case "内嵌", "內嵌", "HARDSUB", "HARD":
		return "hard", false, false
	case "DUAL_SUBS", "MULTI_SUBS", "MULTI":
		return "soft", false, true
	case "BILINGUAL", "双语", "雙語":
		return "", false, true
	case "内封", "內封", "内挂", "內掛", "SOFTSUB", "SOFT", "SUBS", "SUBBED", "VOSTFR", "VOSTEN", "VF", "ENGSUB", "FANSUB", "字幕":
		return "soft", false, false
	case "外挂", "外掛", "EX", "EXTERNAL", "EXTERNAL_SUBS":
		return "ex", false, false
	case "RAW", "NOSUB", "NO-SUB", "NO_SUB":
		return "", true, false
	default:
		return "", false, false
	}
}

func (p *Parser) normalizeSubtitleAttrs(attrs []model.Attribute) []model.Attribute {
	if len(attrs) == 0 {
		return attrs
	}
	out := make([]model.Attribute, 0, len(attrs))
	langSet := map[string]bool{}
	groupSet := map[string]bool{}
	flagSet := map[string]bool{}
	rawMode := false
	multiHint := false
	for _, a := range attrs {
		switch a.Type {
		case model.AttrSubLang:
			n := normalizeSubLangToken(a.Normalized)
			if n == "" {
				continue
			}
			if n == "RAW" {
				rawMode = true
			}
			langSet[n] = true
		case model.AttrSubLangGroup:
			n := normalizeSubLangGroupToken(a.Normalized)
			if n == "" {
				continue
			}
			groupSet[n] = true
		case model.AttrSubFlag:
			n, isRaw, isMulti := normalizeSubFlagToken(a.Normalized)
			if isRaw {
				rawMode = true
				continue
			}
			if isMulti {
				multiHint = true
			}
			if n == "" {
				continue
			}
			flagSet[n] = true
		default:
			out = append(out, a)
		}
	}

	if groupSet["JPSC"] {
		langSet["JPSC"] = true
		delete(langSet, "SC")
		delete(langSet, "JPN")
	}
	if groupSet["JPTC"] {
		langSet["JPTC"] = true
		delete(langSet, "TC")
		delete(langSet, "JPN")
	}
	if groupSet["JPSC"] && groupSet["JPTC"] {
		delete(langSet, "SC")
		delete(langSet, "TC")
		delete(langSet, "JPN")
	}
	if rawMode {
		langSet = map[string]bool{"RAW": true}
		groupSet = map[string]bool{}
		flagSet = map[string]bool{}
	} else if len(langSet) == 0 && len(groupSet) == 0 && multiHint {
		langSet["MULTI"] = true
	}
	if len(langSet) > 1 {
		delete(langSet, "MULTI")
	}

	langOrder := []string{"RAW", "MULTI", "SC", "TC", "JPSC", "JPTC", "JPN", "ENG", "KOR", "FRA", "SPA", "CAT", "RUS", "DEU", "ITA", "POR"}
	for _, v := range langOrder {
		if !langSet[v] {
			continue
		}
		out = append(out, model.Attribute{
			Type:       model.AttrSubLang,
			Normalized: v,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.8,
			Source:     "subtitle_normalize",
		})
	}
	groupOrder := []string{"JPSC", "JPTC", "SC+ENG", "TC+ENG"}
	for _, v := range groupOrder {
		if !groupSet[v] || rawMode {
			continue
		}
		out = append(out, model.Attribute{
			Type:       model.AttrSubLangGroup,
			Normalized: v,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.75,
			Source:     "subtitle_normalize",
		})
	}
	flagOrder := []string{"hard", "soft", "ex"}
	for _, v := range flagOrder {
		if !flagSet[v] || rawMode {
			continue
		}
		out = append(out, model.Attribute{
			Type:       model.AttrSubFlag,
			Normalized: v,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.8,
			Source:     "subtitle_normalize",
		})
	}
	return out
}

func (p *Parser) enrichBatchSourceTypeFromPackKind(attrs []model.Attribute, contentKind string) []model.Attribute {
	if !strings.EqualFold(strings.TrimSpace(contentKind), "BATCH") {
		return attrs
	}
	hasSource := false
	packKind := ""
	for _, a := range attrs {
		if a.Type == model.AttrSourceType && strings.TrimSpace(a.Normalized) != "" {
			hasSource = true
			break
		}
		if packKind == "" && a.Type == model.AttrPackKind && strings.TrimSpace(a.Normalized) != "" {
			packKind = strings.ToLower(strings.TrimSpace(a.Normalized))
		}
	}
	if hasSource || packKind == "" {
		return attrs
	}
	sourceType := ""
	if p != nil && len(p.packKindSourceType) > 0 {
		sourceType = strings.TrimSpace(p.packKindSourceType[packKind])
	}
	if sourceType == "" {
		return attrs
	}
	out := make([]model.Attribute, 0, len(attrs)+1)
	out = append(out, attrs...)
	out = append(out, model.Attribute{
		Type:       model.AttrSourceType,
		Normalized: sourceType,
		TokenStart: -1,
		TokenEnd:   -1,
		Confidence: 0.65,
		Source:     "pack_kind_fallback",
	})
	return out
}

func (p *Parser) normalizeSourceType(s string) string {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return ""
	}
	if p != nil && len(p.sourceTypeCanonical) > 0 {
		if v, ok := p.sourceTypeCanonical[strings.ToLower(ss)]; ok {
			return v
		}
	}
	return ss
}

func (p *Parser) normalizeVideoCodecToken(s string) (string, bool) {
	ss := strings.TrimSpace(strings.ToLower(s))
	if ss == "" {
		return "", false
	}
	if p != nil && len(p.videoCodecCanonical) > 0 {
		if v, ok := p.videoCodecCanonical[ss]; ok {
			return v, true
		}
		dotless := strings.ReplaceAll(ss, ".", "")
		if v, ok := p.videoCodecCanonical[dotless]; ok {
			return v, true
		}
	}
	return "", false
}

func (p *Parser) normalizeAudioCodecToken(s string) (string, bool) {
	ss := strings.TrimSpace(strings.ToLower(s))
	if ss == "" {
		return "", false
	}
	if p != nil && len(p.audioCodecCanonical) > 0 {
		if v, ok := p.audioCodecCanonical[ss]; ok {
			return v, true
		}
		dashless := strings.ReplaceAll(ss, "-", "")
		if v, ok := p.audioCodecCanonical[dashless]; ok {
			return v, true
		}
	}
	return "", false
}

func (p *Parser) normalizeContainerToken(s string) (string, bool) {
	ss := strings.TrimSpace(strings.ToLower(s))
	if ss == "" || p == nil || len(p.containerCanonical) == 0 {
		return "", false
	}
	if v, ok := p.containerCanonical[ss]; ok {
		return v, true
	}
	return "", false
}

func (p *Parser) normalizeAudioFlagToken(s string) (string, bool) {
	ss := strings.TrimSpace(strings.ToLower(s))
	if ss == "" || p == nil || len(p.audioFlagCanonical) == 0 {
		return "", false
	}
	if v, ok := p.audioFlagCanonical[ss]; ok {
		return v, true
	}
	return "", false
}

func (p *Parser) isAliasMetaOnlyToken(s string) bool {
	ss := strings.ToLower(strings.TrimSpace(s))
	if ss == "" {
		return false
	}
	switch ss {
	case "tv", "audio", "sub", "subs", "srt", "ass", "pgs", "mkv", "mp4", "avi":
		return true
	}
	if reAliasResolutionToken.MatchString(ss) ||
		reAliasResolutionMetaPhrase.MatchString(ss) ||
		reAliasSourceResolutionPhrase.MatchString(ss) ||
		reAliasSourceMetaPhrase.MatchString(ss) ||
		reAliasSubtitleMetaPhrase.MatchString(ss) ||
		reAliasBitDepthMetaPhrase.MatchString(ss) ||
		reAliasAudioCountMetaPhrase.MatchString(ss) ||
		reAliasSubtitleCJKMetaPhrase.MatchString(strings.TrimSpace(s)) {
		return true
	}
	if p == nil {
		return false
	}
	if _, ok := p.platformCanonical[ss]; ok {
		return true
	}
	if _, ok := p.sourceTypeCanonical[ss]; ok {
		return true
	}
	if _, ok := p.normalizeVideoCodecToken(ss); ok {
		return true
	}
	if _, ok := p.normalizeAudioCodecToken(ss); ok {
		return true
	}
	if _, ok := p.normalizeContainerToken(ss); ok {
		return true
	}
	if _, ok := p.normalizeAudioFlagToken(ss); ok {
		return true
	}
	if _, ok := p.subFlagCanonical[ss]; ok {
		return true
	}
	if _, ok := p.subLangCanonical[ss]; ok {
		return true
	}
	if _, ok := p.audioLangCanonical[ss]; ok {
		return true
	}
	return false
}

func (p *Parser) isAliasUploaderAtPlatform(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	parts := strings.Split(ss, "@")
	if len(parts) != 2 {
		return false
	}
	uploader := strings.TrimSpace(parts[0])
	platform := strings.ToLower(strings.TrimSpace(parts[1]))
	if !reAliasUploaderName.MatchString(uploader) {
		return false
	}
	if platform == "" {
		return false
	}
	if strings.HasPrefix(platform, "b-global") || strings.HasPrefix(platform, "bglobal") {
		return true
	}
	if p == nil {
		return false
	}
	_, ok := p.platformCanonical[platform]
	return ok
}

func (p *Parser) normalizePlatformCode(s string) string {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return ""
	}
	if p != nil && len(p.platformCanonical) > 0 {
		if v, ok := p.platformCanonical[strings.ToLower(ss)]; ok {
			return v
		}
	}
	return strings.ToUpper(ss)
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

func hasAttrNormalized(attrs []model.Attribute, t model.AttrType, normalized string) bool {
	want := strings.TrimSpace(normalized)
	if want == "" {
		return false
	}
	for _, a := range attrs {
		if a.Type != t {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(a.Normalized), want) {
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

var reStreamMetaFlag = regexp.MustCompile(`(?i)\b(?:multi[-_ ]?subs?|multisub|dual[-_ ]?audio|multi[-_ ]?audio|english\s*dub|dub|vostfr|vosten|vf)\b`)

var reTailMetaToken = regexp.MustCompile(`(?i)^(?:weekly|batch|reseed|complete|fin|end|final\s+season|season|season\s*\d{1,2}|s\d{1,2}|\d{1,2}(?:st|nd|rd|th))$`)

var reHintPrefix = regexp.MustCompile(`(?i)^(检索用|检索|檢索用|檢索|索引|搜索用|搜尋用|搜索|搜尋|search)\s*[:：]\s*(.+)$`)
var reSearchHintKeyword = regexp.MustCompile(`(?i)(?:检索|檢索|索引|搜索|搜尋|search|网盘|網盤|下載|下载|download|招募|招校对|附\d+|下载地址|下載地址)`)
var reAliasMetaSemanticOnly = regexp.MustCompile(`(?i)^(?:全集|合集|特典(?:映像)?|正片(?:\s*[+＋]\s*特典(?:映像)?)?|第[一二三四五六七八九十0-9]+季|season\s*\d{1,2}|s\d{1,2}|movie|movies|specials?|sp(?:é|e)ciaux|tv\s*全集)$`)

var reAliasTrimTVSeasonTail = regexp.MustCompile(`(?i)(?:^|\s+)(?:tv|tb|тv|тв)\s*[-_ ]\s*\d{1,2}\s*$`)
var reAliasTrimPlatformTail = regexp.MustCompile(`(?i)\s+(?:b-?global(?:\s+donghua)?|baha|abema|cr|amzn|amazon|netflix|nf|unext|at-?x|bs11|bs-?ntv|bs-?fuji|france\s*tv|tv)\s*$`)
var reAliasTrimTVAnimePrefix = regexp.MustCompile(`(?i)^(?:tv\s*アニメ|tv\s*anime|tv\s*animation)\s+`)
var reAliasTrimSeasonWordTail = regexp.MustCompile(`(?i)\s+(?:season|saison)\s*$`)
var reAliasTrimEpisodeWordTail = regexp.MustCompile(`(?i)\s+(?:ep|episode)\s*$`)
var reAliasTVContentPhrase = regexp.MustCompile(`(?i)\btv\b.*(?:season|series|ver(?:sion)?|reproduction|release|ova|oad|ona|movie|sp|s\d{1,2}|剪辑版|剪輯版|版|系列)`)
var reAliasMovieIndexSingle = regexp.MustCompile(`(?i)^m\d{1,3}$`)
var reAliasResolutionToken = regexp.MustCompile(`(?i)^(?:\d{3,4}p|[248]k|fhd|uhd|hd|sd)$`)
var reAliasResolutionMetaPhrase = regexp.MustCompile(`(?i)^(?:[248]k|fhd|uhd|hd|sd)(?:\s*(?:ver(?:sion)?|reproduction|restore(?:d)?|remaster(?:ed)?|upscal(?:e|ed)|收藏级画质|收藏級畫質|修复版|修復版|畫質|画质))?$`)
var reAliasSourceResolutionPhrase = regexp.MustCompile(`(?i)^(?:ld|jdvd|dvd|bd|bdrip|webrip|web[- ]?dl)\s+\d{3,4}p$`)
var reAliasSourceMetaPhrase = regexp.MustCompile(`(?i)^(?:b[-_ ]?)?(?:j[-_ ]?dvd|dvd|bd|bdrip|bluray|blu[-_ ]?ray|webrip|web[-_ ]?dl)(?:\s+raw)?$`)
var reAliasSubtitleMetaPhrase = regexp.MustCompile(`(?i)^(?:srt|ass|pgs)(?:\b|[\s\-_/+].*)$`)
var reAliasBitDepthMetaPhrase = regexp.MustCompile(`(?i)^(?:ma(?:8|10|12)p|(?:8|10|12)\s*[-_ ]?bit)$`)
var reAliasAudioCountMetaPhrase = regexp.MustCompile(`(?i)^(?:\d+\s*audio|audio\s*\d+)$`)
var reAliasSubtitleCJKMetaPhrase = regexp.MustCompile(`(?i)^(?:[\p{Han}A-Za-z0-9&+／/\-_\s]*?(?:双语|雙語|中字|字幕|內封|内封|內嵌|内嵌|外挂|外掛))$`)
var reAliasTrailingEpisodeTail = regexp.MustCompile(`(?i)^(.*?)(?:\s*[-:：]\s*0*(\d{1,4}))$`)
var reTitleTrimTVSeasonTail = regexp.MustCompile(`(?i)\s+(?:tv|tb|тv|тв)\s*[-_ ]?\s*\d{1,3}\s*$`)
var reTitleTrimTVTail = regexp.MustCompile(`(?i)\s+(?:tv|tb|тv|тв)\s*$`)
var reTitleTrimTVSeriesTail = regexp.MustCompile(`(?i)\s+tv\s+series\s*$`)
var reTitleTrimTVEpisodeListTail = regexp.MustCompile(`(?i)\s+tv\s+[\d,，\-~\s]{2,}\s*$`)
var reTitleTrimSpecialTail = regexp.MustCompile(`(?i)\s+(?:ncop|nced|pv)(?:\d{1,3})?\s*$`)
var reTitleStripTVAnimeSegmentPrefix = regexp.MustCompile(`(?i)(^|/\s*)tv\s*(?:アニメ|anime|animation)\s+`)
var reTitleTrimEpisodeWordTail = regexp.MustCompile(`(?i)\s+(?:ep|episode)\s*$`)
var reTitleTrimPackSpecialTail = regexp.MustCompile(`(?i)\s+pack(?:\s*\+\s*(?:sp[eé]ciaux|specials?|sp|ova|oad|ona|ex))+(?:\s+(?:web|webrip|web[- ]?dl))?\s*$`)
var reTitleTrimPlainPackTail = regexp.MustCompile(`(?i)\s+pack\s*$`)
var reTitleTrimMetaTail = regexp.MustCompile(`(?i)\s+(?:hd|sd|fhd|uhd|retail|webrip|web[- ]?dl|bdrip|bluray|blu-ray|remux|dvd|hdtv|mkv|mp4|avi|x26[45]|h\.?26[45]|hevc|av1|aac|ac3|e-?ac-?3|flac|dts(?:-?hd|-?hdma)?|hdma|truehd|ma\d{1,2}p|10[- ]?bit|8[- ]?bit)\s*$`)
var reTitleTrimVolTail = regexp.MustCompile(`(?i)\s+(?:vol\.?(?:\s*\d{1,3})?|volume\s*\d{1,3})\s*$`)
var reTitleTrimDVDBundleTail = regexp.MustCompile(`(?i)\s+(?:dvd(?:\s*[- ]?\s*isos?)?|dvd\s*set)\s*$`)
var reTitleTrimDVDEpisodesTail = regexp.MustCompile(`(?i)\s+(?:(?:r\d+\s+)?dvd\s+)?episodes?\s*$`)
var reAliasTechSingle = regexp.MustCompile(`(?i)^(?:aac(?:×\d+)?|ac3|e-?ac-?3|ddp|flac|opus|mp3|dts(?:-?hd|-?hdma)?|truehd|atmos|avc|hevc|x26[45]|h\.?26[45]|mkv|mp4|dual)$`)
var reAliasTechList = regexp.MustCompile(`(?i)^(?:\.?\s*)?(?:aac(?:×\d+)?|ac3|e-?ac-?3|ddp|flac|opus|mp3|dts(?:-?hd|-?hdma)?|truehd|atmos|avc|hevc|x26[45]|h\.?26[45]|mkv|mp4|dual)(?:\s*[,/&+]\s*(?:aac(?:×\d+)?|ac3|e-?ac-?3|ddp|flac|opus|mp3|dts(?:-?hd|-?hdma)?|truehd|atmos|avc|hevc|x26[45]|h\.?26[45]|mkv|mp4|dual))+\.?$`)
var reAliasUploaderName = regexp.MustCompile(`(?i)^[a-z0-9._-]{2,40}$`)

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
		if strings.Contains(up, "MULTI") || strings.Contains(up, "BILINGUAL") {
			*attrs = append(*attrs, model.Attribute{Type: model.AttrSubLang, Normalized: "MULTI", Confidence: 0.6, Source: "route_streaming_release_v1"})
		}
		*attrs = append(*attrs, model.Attribute{Type: model.AttrSubFlag, Normalized: "soft", Confidence: 0.6, Source: "route_streaming_release_v1"})
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

func (p *Parser) applyStreamingReleaseRouteV1(tailParenBlock string, aliases *[]string, attrs *[]model.Attribute) {
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
		if !p.isValidAliasToken(it) {
			continue
		}
		aliasCands = append(aliasCands, it)
	}
	if len(aliasCands) == 0 {
		return
	}
	*aliases = mergeAliases(*aliases, aliasCands)
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
	out = regexp.MustCompile(`(?i)\bs\s*\d{1,2}\b`).ReplaceAllString(out, "")
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
	if regexp.MustCompile(`(?i)\bs\d{1,2}\b`).MatchString(s) {
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

var reTitlePartNumber = regexp.MustCompile(`(?i)\bpart\.?\s*([0-9]{1,2})\b`)

func restorePartSuffixFromOriginalTitle(originalTitleSpan string, main string, aliases []string) (string, []string) {
	main = strings.TrimSpace(main)
	if main == "" || len(aliases) == 0 {
		return main, aliases
	}
	hasPartAlias := false
	kept := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if strings.EqualFold(strings.TrimSpace(a), "part") {
			hasPartAlias = true
			continue
		}
		kept = append(kept, a)
	}
	if !hasPartAlias {
		return main, aliases
	}
	orig := strings.TrimSpace(originalTitleSpan)
	if orig == "" {
		// No structural evidence from titleslot; keep original aliases.
		return main, aliases
	}
	mm := reTitlePartNumber.FindStringSubmatch(orig)
	if len(mm) < 2 {
		// "Part" without numeric suffix is ambiguous; keep original aliases.
		return main, aliases
	}
	if !reTitlePartNumber.MatchString(main) {
		main = normalizeTitleMain(strings.TrimSpace(main + " Part " + mm[1]))
	}
	if len(kept) == 0 {
		return main, nil
	}
	return main, kept
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
		cleaned := regionHKMOTWRemovalPattern.ReplaceAllString(orig, "")
		cleaned = strings.TrimSpace(cleaned)
		cleaned = strings.Trim(cleaned, "（）()[]【】")
		cleaned = strings.TrimSpace(cleaned)
		tokenStart, tokenEnd := i, i+1
		// Embedded region-restriction phrase should not anchor to title span.
		// Keep positional indices only when this token is pure region meta.
		if cleaned != "" {
			tokenStart, tokenEnd = -1, -1
		}
		*out = append(*out, model.Attribute{
			Type:       model.AttrPlatformRegion,
			Normalized: "HK+MO+TW",
			TokenStart: tokenStart,
			TokenEnd:   tokenEnd,
			Confidence: 0.8,
			Source:     "region_restriction",
		})
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

func (p *Parser) sanitizeAliasesForOutput(aliases []string, title string, episode string, season string, spKind string) []string {
	if len(aliases) == 0 {
		return nil
	}
	titleKey := aliasSemanticKey(title)
	out := make([]string, 0, len(aliases))
	seen := map[string]bool{}
	for _, a := range aliases {
		a = normalizeAliasTrailingAbsoluteEpisode(a, episode, season)
		a = p.normalizeAliasCandidateForOutput(a)
		if stripped := stripExplicitSeasonSuffix(a); stripped != "" {
			a = p.normalizeAliasCandidateForOutput(stripped)
		}
		a = normalizeAliasTrailingAbsoluteEpisode(a, episode, season)
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
var reAliasTrimSPTail = regexp.MustCompile(`(?i)\s*(?:[-_/|:()\[\]{}【】（）]|\s)+(?:SP|SPECIAL)\s*$`)
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

func (p *Parser) extractParenAliasesAndRemoveFromTitle(tokens []model.Token, titleEnd int, episodeIdx int, episodeRangeIdx int, attrs *[]model.Attribute) []string {
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
		if !isSearchHint {
			if markers := extractParenContentMarkers(inner); len(markers) > 0 {
				emitContentsAttrs(attrs, markers, "paren_contents_marker")
				for k := openIdx; k <= closeIdx; k++ {
					tokens[k].Removed = true
				}
				i = closeIdx
				continue
			}
		}
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
			if c == "" {
				continue
			}
			if isSearchHint || isLikelySearchHintText(c) {
				appendSearchHintAttribute(attrs, c, "paren_search_hint")
				continue
			}
			if !p.isValidAliasToken(c) {
				continue
			}
			if stripped := stripExplicitSeasonSuffix(c); stripped != "" {
				c = stripped
				if !p.isValidAliasToken(c) {
					continue
				}
			}
			if reTailMetaToken.MatchString(c) || isAliasMetaSemanticOnly(c) {
				classifyAndEmitTailMetaNote(c, attrs)
				if markers := extractParenContentMarkers(c); len(markers) > 0 {
					emitContentsAttrs(attrs, markers, "alias_meta_demote")
				}
				continue
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

func (p *Parser) inferTailAliasesFromParens(tokens []model.Token, attrs *[]model.Attribute, episodeIdx int, episodeRangeIdx int, seasonIdx int) []string {
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
	scanStart := techStart
	epAnchor := episodeIdx
	if episodeRangeIdx > epAnchor {
		epAnchor = episodeRangeIdx
	}
	if seasonIdx > epAnchor {
		epAnchor = seasonIdx
	}
	if epAnchor >= 0 && epAnchor+1 < scanStart {
		scanStart = epAnchor + 1
	}
	if scanStart < 0 {
		scanStart = 0
	}
	if scanStart >= len(tokens) {
		return nil
	}

	aliases := make([]string, 0, 8)
	for i := scanStart; i < len(tokens); i++ {
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
		if !isSearchHint {
			if markers := extractParenContentMarkers(inner); len(markers) > 0 {
				emitContentsAttrs(attrs, markers, "paren_contents_marker")
				i = closeIdx
				continue
			}
		}
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
			if c == "" {
				continue
			}
			if isSearchHint || isLikelySearchHintText(c) {
				appendSearchHintAttribute(attrs, c, "tail_paren_search_hint")
				continue
			}
			if !p.isValidAliasToken(c) {
				continue
			}
			if stripped := stripExplicitSeasonSuffix(c); stripped != "" {
				c = stripped
				if !p.isValidAliasToken(c) {
					continue
				}
			}
			if isStreamMetaText(c) {
				classifyAndEmitStreamMeta(c, attrs)
				continue
			}
			if reTailMetaToken.MatchString(c) || isAliasMetaSemanticOnly(c) {
				classifyAndEmitTailMetaNote(c, attrs)
				if markers := extractParenContentMarkers(c); len(markers) > 0 {
					emitContentsAttrs(attrs, markers, "alias_meta_demote")
				}
				continue
			}
			if len([]rune(c)) > 80 {
				continue
			}
			if openRemoved && !isSearchHint {
				continue
			}
			aliases = append(aliases, c)
		}
		for k := i; k <= closeIdx && k < len(tokens); k++ {
			tokens[k].Removed = true
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
	if !allowComma && strings.Contains(norm, ",") {
		commaParts := strings.Split(norm, ",")
		validAliasParts := 0
		for _, p := range commaParts {
			pp := strings.TrimSpace(p)
			if pp == "" {
				continue
			}
			if isLikelyMetaBracketText(pp) || isStreamMetaText(pp) {
				validAliasParts = 0
				break
			}
			if len([]rune(pp)) >= 4 {
				validAliasParts++
			}
		}
		if validAliasParts >= 2 {
			allowComma = true
		}
	}
	parts := strings.FieldsFunc(norm, func(r rune) bool {
		if r == ',' {
			return allowComma
		}
		return r == '/' || r == '|' || r == ';' || r == '\\'
	})
	// Search-hint text: extract candidates for AttrSearchHint; do not keep as title_alias.
	if searchHint {
		if len(parts) > 0 {
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				out = append(out, p)
			}
			if len(out) > 0 {
				return out, searchHint, hintKeyword
			}
		}
		cand := strings.TrimSpace(norm)
		if cand == "" {
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

func appendSearchHintAttribute(attrs *[]model.Attribute, value string, source string) {
	if attrs == nil {
		return
	}
	v := strings.TrimSpace(value)
	if v == "" {
		return
	}
	key := aliasSemanticKey(v)
	for _, a := range *attrs {
		if a.Type != model.AttrSearchHint {
			continue
		}
		if aliasSemanticKey(a.Normalized) == key {
			return
		}
	}
	*attrs = append(*attrs, model.Attribute{
		Type:       model.AttrSearchHint,
		Normalized: v,
		TokenStart: -1,
		TokenEnd:   -1,
		Confidence: 0.65,
		Source:     source,
	})
}

func isLikelySearchHintText(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if reSearchHintKeyword.MatchString(ss) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(ss), "search:")
}

func isAliasMetaSemanticOnly(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if reAliasMetaSemanticOnly.MatchString(ss) {
		return true
	}
	// Content markers-only tokens should not remain as aliases.
	if markers := extractParenContentMarkers(ss); len(markers) > 0 {
		return true
	}
	return false
}

var reParenContentSeasonRange = regexp.MustCompile(`(?i)^season\s*\d{1,2}(?:\s*[-~]\s*\d{1,2})?$`)
var reParenContentNumericRange = regexp.MustCompile(`^\d{1,3}(?:\s*[-~]\s*\d{1,3})?$`)
var reParenContentMarkerSplit = regexp.MustCompile(`[+＋/&|,;，；、]+`)

func extractParenContentMarkers(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	norm := strings.NewReplacer(
		"＋", "+",
		"＆", "&",
		"｜", "|",
		"／", "/",
		"　", " ",
	).Replace(content)
	parts := reParenContentMarkerSplit.Split(norm, -1)
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	hasSeasonCtx := false
	for _, p := range parts {
		s := strings.TrimSpace(strings.Trim(p, "[](){}【】（）"))
		if s == "" {
			continue
		}
		ls := strings.ToLower(s)
		if strings.HasPrefix(ls, "season ") || strings.HasPrefix(ls, "saison ") {
			hasSeasonCtx = true
		}
		if reParenContentSeasonRange.MatchString(s) {
			hasSeasonCtx = true
			continue
		}
		if reParenContentNumericRange.MatchString(strings.ReplaceAll(s, " ", "")) {
			continue
		}
		marker, ok := normalizeParenContentMarkerToken(s)
		if !ok {
			return nil
		}
		if _, exists := seen[marker]; exists {
			continue
		}
		seen[marker] = struct{}{}
		out = append(out, marker)
	}
	if hasSeasonCtx && len(out) > 0 {
		if _, ok := seen["TV"]; !ok {
			out = append([]string{"TV"}, out...)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func extractContentsFromAliases(aliases []string) ([]string, []string) {
	if len(aliases) == 0 {
		return nil, nil
	}
	kept := make([]string, 0, len(aliases))
	collected := make([]string, 0, 4)
	seen := map[string]struct{}{}
	for _, a := range aliases {
		s := strings.TrimSpace(a)
		if s == "" {
			continue
		}
		markers := extractParenContentMarkers(s)
		if len(markers) == 0 {
			kept = append(kept, a)
			continue
		}
		for _, m := range markers {
			m = strings.ToUpper(strings.TrimSpace(m))
			if m == "" {
				continue
			}
			if _, ok := seen[m]; ok {
				continue
			}
			seen[m] = struct{}{}
			collected = append(collected, m)
		}
	}
	if len(kept) == 0 {
		kept = nil
	}
	if len(collected) == 0 {
		collected = nil
	}
	return kept, collected
}

var reRawParenContent = regexp.MustCompile(`[（(]([^（）()]{1,80})[）)]`)

func extractContentsFromRawParens(rawTitle string) []string {
	rawTitle = strings.TrimSpace(rawTitle)
	if rawTitle == "" {
		return nil
	}
	ms := reRawParenContent.FindAllStringSubmatch(rawTitle, -1)
	if len(ms) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, m := range ms {
		if len(m) < 2 {
			continue
		}
		for _, marker := range extractParenContentMarkers(m[1]) {
			marker = strings.ToUpper(strings.TrimSpace(marker))
			if marker == "" {
				continue
			}
			if _, ok := seen[marker]; ok {
				continue
			}
			seen[marker] = struct{}{}
			out = append(out, marker)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeParenContentMarkerToken(s string) (string, bool) {
	low := strings.ToLower(strings.TrimSpace(s))
	switch low {
	case "movie", "movies", "film", "films", "eiga", "gekijouban", "劇場版", "剧场版", "電影", "电影":
		return "MOVIE", true
	case "special", "specials", "sp", "spéciaux", "speciaux", "特典", "特典映像", "bonus", "bonuses", "bouns", "tokuten", "特别篇", "特別篇":
		return "SPECIAL", true
	case "ova":
		return "OVA", true
	case "oad":
		return "OAD", true
	case "ona":
		return "ONA", true
	case "ex":
		return "EX", true
	case "tv":
		return "TV", true
	default:
		return "", false
	}
}

func emitContentsAttrs(attrs *[]model.Attribute, values []string, source string) {
	if attrs == nil || len(values) == 0 {
		return
	}
	for _, v := range values {
		v = strings.TrimSpace(strings.ToUpper(v))
		if v == "" {
			continue
		}
		*attrs = append(*attrs, model.Attribute{
			Type:       model.AttrContents,
			Normalized: v,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: 0.8,
			Source:     source,
		})
	}
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

func (p *Parser) inferTitleTokenSpan(tokens []model.Token, episodeIdx int, episodeRangeIdx int, seasonCutIdx int) (int, int) {
	start := titleStartAfterLeadingEmptyBracketBlocks(tokens)

	end := len(tokens)
	if episodeRangeIdx >= 0 {
		end = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < end {
		if shouldUseEpisodeAsTitleCut(tokens, start, episodeIdx) {
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
				if p.looksLikeDotSeparatedMetaCutToken(t) {
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

func firstVisibleTitleTokenIndex(tokens []model.Token, start int, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	if start >= end {
		return -1
	}
	for i := start; i < end; i++ {
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
		return i
	}
	return -1
}

func shouldUseEpisodeAsTitleCut(tokens []model.Token, titleStart int, episodeIdx int) bool {
	if episodeIdx < 0 || episodeIdx >= len(tokens) {
		return false
	}
	first := firstVisibleTitleTokenIndex(tokens, titleStart, len(tokens))
	if first >= 0 && episodeIdx < first {
		return false
	}
	return true
}

var reDotSeparatedRes = regexp.MustCompile(`(?i)^\d{3,4}p$`)

func (p *Parser) looksLikeDotSeparatedMetaCutToken(s string) bool {
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
	if p != nil {
		if _, ok := p.sourceTypeCanonical[low]; ok {
			return true
		}
		if _, ok := p.platformCanonical[low]; ok {
			return true
		}
		if _, ok := p.normalizeVideoCodecToken(low); ok {
			return true
		}
		if _, ok := p.normalizeAudioCodecToken(low); ok {
			return true
		}
		if _, ok := p.normalizeContainerToken(low); ok {
			return true
		}
		if _, ok := p.normalizeAudioFlagToken(low); ok {
			return true
		}
	}
	if low == "webrip" || low == "web-dl" || low == "webdl" || low == "bluray" || low == "bdrip" || low == "dvdrip" {
		return true
	}
	if low == "hdr" {
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
			if t == "_" || t == "-" || t == "." || t == "/" || t == "／" || t == "|" || t == "｜" {
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
				if t == "_" || t == "-" || t == "." || t == "/" || t == "／" || t == "|" || t == "｜" {
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
var reMetaHDRWithKToken = regexp.MustCompile(`(?i)(?:4k|8k).*hdr|hdr.*(?:4k|8k)`)
var reEditionMetaText = regexp.MustCompile(`(?i)\b(?:uncut(?:\s*(?:ver|version)\.?)?|uncensored(?:\s*(?:ver|version)\.?)?|director'?s\s*cut|extended\s*cut|remaster(?:ed)?(?:\s*(?:ver|version)\.?)?|audio[_ -]?commentary(?:\s*(?:ver|version)\.?)?|encode\s*ver)\b`)

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
	start := 0
	for start < len(tokens) && tokens[start].Removed {
		start++
	}
	if start >= len(tokens) {
		return "", [2]int{0, 0}
	}
	open0, close0, ok0 := headGroupBracketPair(tokens[start].Text)
	if !ok0 {
		return "", [2]int{0, 0}
	}

	blocks := make([][3]int, 0, 4)
	i := start
	_ = open0
	_ = close0
	for i < len(tokens) {
		if tokens[i].Removed {
			i++
			continue
		}
		open, close, ok := headGroupBracketPair(tokens[i].Text)
		if !ok {
			break
		}
		end := -1
		depth := 0
		for j := i; j < len(tokens); j++ {
			if tokens[j].Removed {
				continue
			}
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
		for i < len(tokens) && tokens[i].Removed {
			i++
		}
	}
	if len(blocks) == 0 {
		return "", [2]int{0, 0}
	}

	// Prefer first bracket block. If first block is clear meta (e.g. [1080P], [BDRip]),
	// allow fallback to the first subsequent strong group candidate block.
	for bi, b := range blocks {
		if !headBracketBlockHasAliveToken(tokens, b) {
			continue
		}
		g := headBracketBlockText(tokens, b)
		if g == "" {
			continue
		}
		if g == "整理搬运" || g == "整理搬運" {
			continue
		}
		if !isGroupCandidateText(g) {
			continue
		}
		if (isLikelyMetaBracketText(g) && !hasStrongGroupKeyword(g)) || isEpisodeLikeBracketText(g) {
			continue
		}
		if bi == 0 {
			return g, [2]int{b[0], b[1]}
		}
		if areLeadingHeadBlocksMeta(tokens, blocks[:bi]) {
			return g, [2]int{b[0], b[1]}
		}
		break
	}
	return "", [2]int{0, 0}
}

func headBracketBlockText(tokens []model.Token, block [3]int) string {
	parts := make([]string, 0, block[1]-block[0])
	for k := block[0] + 1; k < block[1]-1; k++ {
		if strings.TrimSpace(tokens[k].Text) == "" {
			continue
		}
		parts = append(parts, tokens[k].Text)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func headBracketBlockHasAliveToken(tokens []model.Token, block [3]int) bool {
	for k := block[0] + 1; k < block[1]-1; k++ {
		if tokens[k].Removed {
			continue
		}
		if strings.TrimSpace(tokens[k].Text) == "" {
			continue
		}
		return true
	}
	return false
}

func areLeadingHeadBlocksMeta(tokens []model.Token, blocks [][3]int) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if !headBracketBlockHasAliveToken(tokens, b) {
			continue
		}
		text := headBracketBlockText(tokens, b)
		if text == "" {
			continue
		}
		if isLikelyMetaBracketText(text) || isEpisodeLikeBracketText(text) {
			continue
		}
		return false
	}
	return true
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

func extractHeadParenGroup(tokens []model.Token) (string, [2]int) {
	if len(tokens) < 3 || strings.TrimSpace(tokens[0].Text) != "(" {
		return "", [2]int{0, 0}
	}
	end := -1
	depth := 0
	for i := 0; i < len(tokens); i++ {
		t := strings.TrimSpace(tokens[i].Text)
		if t == "(" {
			depth++
		} else if t == ")" {
			if depth > 0 {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
	}
	if end <= 1 {
		return "", [2]int{0, 0}
	}
	parts := make([]string, 0, end-1)
	for i := 1; i < end; i++ {
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		parts = append(parts, t)
	}
	cand := strings.TrimSpace(strings.Join(parts, " "))
	if !isHeadParenGroupCandidate(cand) {
		return "", [2]int{0, 0}
	}
	return cand, [2]int{0, end + 1}
}

var headParenUpperTokenPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._&+\-]{1,23}$`)
var headParenVersionLikePattern = regexp.MustCompile(`(?i)^v(?:er)?\d+$`)

func isHeadParenGroupCandidate(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if isLikelyMetaBracketText(s) || isEpisodeLikeBracketText(s) {
		return false
	}
	if len([]rune(s)) > 24 {
		return false
	}
	if headParenVersionLikePattern.MatchString(s) {
		return false
	}
	if headParenUpperTokenPattern.MatchString(s) {
		return true
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "raw") || strings.Contains(low, "sub") || strings.Contains(low, "team") || strings.Contains(low, "studio") {
		return isGroupCandidateText(s)
	}
	return false
}

func extractHeadBracketGroupAfterLeadingParenMeta(tokens []model.Token) (string, [2]int) {
	if len(tokens) < 4 || strings.TrimSpace(tokens[0].Text) != "(" {
		return "", [2]int{0, 0}
	}
	offset := 0
	consumedMeta := false
	for offset < len(tokens) && strings.TrimSpace(tokens[offset].Text) == "(" {
		end := -1
		depth := 0
		for i := offset; i < len(tokens); i++ {
			t := strings.TrimSpace(tokens[i].Text)
			if t == "(" {
				depth++
			} else if t == ")" {
				if depth > 0 {
					depth--
					if depth == 0 {
						end = i
						break
					}
				}
			}
		}
		if end <= offset+1 {
			return "", [2]int{0, 0}
		}
		parts := make([]string, 0, end-offset-1)
		for i := offset + 1; i < end; i++ {
			t := strings.TrimSpace(tokens[i].Text)
			if t == "" {
				continue
			}
			parts = append(parts, t)
		}
		cand := strings.TrimSpace(strings.Join(parts, " "))
		if !isLeadingParenMetaText(cand) {
			return "", [2]int{0, 0}
		}
		consumedMeta = true
		offset = end + 1
	}
	if !consumedMeta || offset >= len(tokens) {
		return "", [2]int{0, 0}
	}
	g, span := extractHeadBracketGroup(tokens[offset:])
	if g == "" {
		return "", [2]int{0, 0}
	}
	return g, [2]int{span[0] + offset, span[1] + offset}
}

func isLeadingParenMetaText(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if headParenVersionLikePattern.MatchString(s) {
		return true
	}
	low := strings.ToLower(s)
	if low == "partial" || low == "reseed" || low == "fixed" {
		return true
	}
	return isLikelyMetaBracketText(s)
}

func isGroupCandidateText(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	runeLen := len([]rune(s))
	if runeLen > 64 {
		return false
	}
	if runeLen > 32 && !hasStrongGroupKeyword(s) {
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
		if r == ' ' || r == '_' || r == '-' || r == '.' || r == '&' || r == '＆' || r == '+' || r == '＋' || r == ',' || r == '，' || r == '@' {
			continue
		}
		if r > 127 {
			continue
		}
		return false
	}
	return true
}

func hasStrongGroupKeyword(s string) bool {
	ss := strings.ToLower(strings.TrimSpace(s))
	if ss == "" {
		return false
	}
	if strings.Contains(s, "字幕") || strings.Contains(s, "漢化") || strings.Contains(s, "汉化") {
		return true
	}
	if strings.Contains(ss, "sub") || strings.Contains(ss, "raw") || strings.Contains(ss, "fansub") || strings.Contains(ss, "team") || strings.Contains(ss, "studio") {
		return true
	}
	return false
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
	if trim == "先行版" || trim == "先行版本" {
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
	if low == "hdr" || low == "dovi" || low == "dolbyvision" {
		return true
	}
	if reMetaHDRWithKToken.MatchString(low) {
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
	if up == "BDBOX" || up == "BD-BOX" {
		return true
	}
	// Common combined language tags like "CHS&JPN" / "CHT&JPN" / "CHS&CHT&JPN".
	if regexp.MustCompile(`(?i)^(?:CHS|CHT|JPN|JP|ENG)(?:\s*(?:&|＆|\+|＋)\s*(?:CHS|CHT|JPN|JP|ENG))+$`).MatchString(trim) {
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
	if reEditionMetaText.MatchString(trim) {
		return true
	}
	return false
}

func isEpisodeLikeBracketText(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if regexp.MustCompile(`(?i)^0?\d{1,4}\s*[-~]\s*0?\d{1,4}(?:\.\d{1,2})?.*$`).MatchString(ss) {
		return true
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
var seasonWordLoosePattern = regexp.MustCompile(`(?i)^(season|saison)$`)
var seasonOrdinalPattern = regexp.MustCompile(`(?i)^(\d{1,2})(?:st|nd|rd|th)$`)
var seasonCJKPattern = regexp.MustCompile(`^第?([一二三四五六七八九十0-9]{1,3})季$`)

func detectSxxExx(tokens []model.Token) (season string, episode string, tokenIdx int) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		m := sxxExxPattern.FindStringSubmatch(tokens[i].Text)
		if len(m) == 3 {
			season = strings.TrimLeft(m[1], "0")
			if season == "" {
				season = "0"
			}
			episode = strings.TrimLeft(m[2], "0")
			if episode == "" {
				episode = "0"
			}
			return season, episode, i
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
		if seasonWordLoosePattern.MatchString(text) {
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
var episodeRangeLeftCJKPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})$`)
var episodeRangeRightCJKPattern = regexp.MustCompile(`^(\d{1,4})(?:集|话|話|回|章)?$`)
var epPrefixedDigitsPattern = regexp.MustCompile(`(?i)^e(?:p)?(\d{1,4})$`)
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
	// Pattern: Episode 27-40 / EP 01-12
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if isInBracket(tokens, i) {
			continue
		}
		kw := strings.TrimSpace(tokens[i].Text)
		if !(strings.EqualFold(kw, "episode") || strings.EqualFold(kw, "ep")) {
			continue
		}
		if i+1 >= len(tokens) {
			continue
		}
		if m := episodeRangePattern.FindStringSubmatch(strings.TrimSpace(tokens[i+1].Text)); len(m) == 3 {
			l := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if l == "" {
				l = "0"
			}
			r := strings.TrimLeft(strings.TrimSpace(m[2]), "0")
			if r == "" {
				r = "0"
			}
			return l + "-" + r, i + 1
		}
		if m := episodeRangeEmbeddedPattern.FindStringSubmatch(strings.TrimSpace(tokens[i+1].Text)); len(m) == 3 {
			l := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if l == "" {
				l = "0"
			}
			r := strings.TrimLeft(strings.TrimSpace(m[2]), "0")
			if r == "" {
				r = "0"
			}
			return l + "-" + r, i + 1
		}
		if i+3 < len(tokens) {
			lraw := strings.TrimSpace(tokens[i+1].Text)
			sep := strings.TrimSpace(tokens[i+2].Text)
			rraw := strings.TrimSpace(tokens[i+3].Text)
			if episodeOfTotalPattern.MatchString(lraw) && (sep == "-" || sep == "~") {
				if !episodeOfTotalPattern.MatchString(rraw) {
					if mm := leadingDigitsPattern.FindStringSubmatch(rraw); len(mm) == 2 {
						rraw = strings.TrimSpace(mm[1])
					}
				}
			}
			if episodeOfTotalPattern.MatchString(lraw) && (sep == "-" || sep == "~") && episodeOfTotalPattern.MatchString(rraw) {
				l := strings.TrimLeft(lraw, "0")
				if l == "" {
					l = "0"
				}
				r := strings.TrimLeft(rraw, "0")
				if r == "" {
					r = "0"
				}
				if l != "0" && r != "0" {
					return l + "-" + r, i + 1
				}
			}
		}
	}

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
		if rm := epPrefixedDigitsPattern.FindStringSubmatch(strings.ToLower(rawR)); len(rm) == 2 {
			rawR = strings.TrimSpace(rm[1])
		}
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
		if m := episodeCountPackSuffixPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(m) == 3 {
			n := strings.TrimLeft(strings.TrimSpace(m[1]), "0")
			if n == "" {
				n = "0"
			}
			if n != "0" {
				return "1-" + n, i
			}
		}
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
			leftRaw := strings.TrimSpace(tokens[i].Text)
			sep := strings.TrimSpace(tokens[i+1].Text)
			rightRaw := strings.TrimSpace(tokens[i+2].Text)
			if sep == "-" || sep == "~" {
				leftHasMarker := strings.HasPrefix(leftRaw, "第")
				rightHasMarker := strings.HasSuffix(rightRaw, "集") || strings.HasSuffix(rightRaw, "话") || strings.HasSuffix(rightRaw, "話") || strings.HasSuffix(rightRaw, "回") || strings.HasSuffix(rightRaw, "章")
				if leftHasMarker || rightHasMarker {
					lraw := leftRaw
					if lm := episodeRangeLeftCJKPattern.FindStringSubmatch(lraw); len(lm) == 2 {
						lraw = strings.TrimSpace(lm[1])
					}
					rraw := rightRaw
					if rm := episodeRangeRightCJKPattern.FindStringSubmatch(rraw); len(rm) == 2 {
						rraw = strings.TrimSpace(rm[1])
					} else if mm := leadingDigitsPattern.FindStringSubmatch(rraw); len(mm) == 2 {
						rraw = strings.TrimSpace(mm[1])
					}
					if episodeOfTotalPattern.MatchString(lraw) && episodeOfTotalPattern.MatchString(rraw) {
						l := strings.TrimLeft(lraw, "0")
						if l == "" {
							l = "0"
						}
						r := strings.TrimLeft(rraw, "0")
						if r == "" {
							r = "0"
						}
						if l != "0" && r != "0" {
							if len(lraw) == 4 && len(rraw) == 4 {
								if y1, err1 := strconv.Atoi(lraw); err1 == nil {
									if y2, err2 := strconv.Atoi(rraw); err2 == nil {
										if y1 >= 1900 && y1 <= 2099 && y2 >= 1900 && y2 <= 2099 {
											continue
										}
									}
								}
							}
							return l + "-" + r, i
						}
					}
				}
			}
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
var groupCommaSpacePattern = regexp.MustCompile(`\s*[,，]\s*`)

func normalizeGroupText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	s = groupHyphenSpacePattern.ReplaceAllString(s, `$1-$2`)
	// Normalize '&' / '＆' without surrounding spaces.
	s = groupAmpSpacePattern.ReplaceAllString(s, "&")
	// Normalize ',' / '，' without surrounding spaces.
	s = groupCommaSpacePattern.ReplaceAllString(s, ",")
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
	// Case A2: two-token group prefix before star.
	// Example: ["SFSub&", "六四位元字幕組", "★", "..."].
	if len(tokens) >= 3 {
		sep := strings.TrimSpace(tokens[2].Text)
		if sep == "★" || sep == "☆" {
			leftA := strings.TrimSpace(tokens[0].Text)
			leftB := strings.TrimSpace(tokens[1].Text)
			joined := strings.TrimSpace(leftA + " " + leftB)
			if validLeft(joined) || (strings.HasSuffix(leftA, "&") && hasStrongGroupKeyword(leftB)) {
				hasRight := false
				maxLookahead := 20
				if maxLookahead > len(tokens) {
					maxLookahead = len(tokens)
				}
				for j := 3; j < maxLookahead; j++ {
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
					tokens[2].Removed = true
					return joined
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
var episodeWithParenSuffixPattern = regexp.MustCompile(`(?i)^(?:(e|ep|第))?(\d{1,4})(?:\.(\d{1,2}))?[（(][^）)]{1,32}[）)]$`)
var episodeVersionSuffixPattern = regexp.MustCompile(`(?i)^(?:\d{1,4}(?:\.\d{1,2})?)[vV]\d{1,2}$`)
var chineseEpisodeSuffixPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})(?:话|集)$`)
var chineseEpisodeWithSuffixMarkerPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})(?:话|集|話)[①②③④⑤⑥⑦⑧⑨⑩⑪⑫⑬⑭⑮⑯⑰⑱⑲⑳]$`)
var chineseEpisodeLeadingPattern = regexp.MustCompile(`^(?:第)?(\d{1,4})(?:话|集|話)(?:$|(?:\s|[&＆(（\[【]).*)`)
var embeddedChineseEpisodeSuffixPattern = regexp.MustCompile(`第(\d{1,4})(?:集|话|話)$`)
var yearPattern = regexp.MustCompile(`^(?:19|20)\d{2}$`)
var starEpisodeEmbeddedPattern = regexp.MustCompile(`(?:★|☆)(\d{1,4})(?:★|☆)`)
var dateYMDPattern = regexp.MustCompile(`(?i)(?:\b(?:19|20)\d{2}[./-](?:0?[1-9]|1[0-2])[./-](?:0?[1-9]|[12]\d|3[01])\b)`)
var spKindEpisodePattern = regexp.MustCompile(`(?i)^(?:sp|ova|oad|ona|ex|ncop|nced|pv)0*(\d{1,3})$`)
var episodeWithVersionPattern = regexp.MustCompile(`(?i)^(?:(?:e|ep|第))?(\d{1,4})(?:\.(\d{1,2}))?[vV](\d{1,2})$`)
var episodeEndSuffixPattern = regexp.MustCompile(`(?i)^(\d{1,4})(end|fin|final)$`)
var standaloneVersionPattern = regexp.MustCompile(`(?i)^v(\d{1,2})$`)
var regionHKMOTWEmbeddedPattern = regexp.MustCompile(`(?i)(?:仅限港澳台地区|僅限港澳台地區)`)
var regionHKMOTWRemovalPattern = regexp.MustCompile(`(?i)[(（]?\s*(?:仅限港澳台地区|僅限港澳台地區)\s*[)）]?`)
var bracketScheduleNoticeDatePattern = regexp.MustCompile(`\d{1,2}[./]\d{1,2}`)

func isBracketScheduleNoticeText(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if !strings.Contains(ss, "停播") && !strings.Contains(ss, "停更") {
		return false
	}
	if bracketScheduleNoticeDatePattern.MatchString(ss) || dateYMDPattern.MatchString(ss) {
		return true
	}
	if strings.Contains(ss, "，") || strings.Contains(ss, ",") || strings.Contains(ss, "。") || strings.Contains(ss, ";") || strings.Contains(ss, "；") {
		return true
	}
	return false
}

func isEpisodeBoundaryRightToken(t string) bool {
	tt := strings.TrimSpace(t)
	if tt == "" {
		return false
	}
	switch tt {
	case "-", "[", "【", "(", "（":
		return true
	}
	if standaloneVersionPattern.MatchString(tt) {
		return true
	}
	up := strings.ToUpper(tt)
	return up == "END" || up == "FIN" || up == "FINAL"
}

func bracketBoundsAt(tokens []model.Token, idx int) (int, int, bool) {
	if idx < 0 || idx >= len(tokens) || !isInBracket(tokens, idx) {
		return -1, -1, false
	}
	openIdx := -1
	openTok := ""
	depth := 0
	for j := idx; j >= 0; j-- {
		t := strings.TrimSpace(tokens[j].Text)
		if t == "]" || t == "】" {
			depth++
			continue
		}
		if t == "[" || t == "【" {
			if depth == 0 {
				openIdx = j
				openTok = t
				break
			}
			depth--
		}
	}
	if openIdx < 0 {
		return -1, -1, false
	}
	closeTok := "]"
	if openTok == "【" {
		closeTok = "】"
	}
	d := 0
	for j := openIdx; j < len(tokens); j++ {
		t := strings.TrimSpace(tokens[j].Text)
		if t == openTok {
			d++
			continue
		}
		if t == closeTok {
			d--
			if d == 0 {
				return openIdx, j, true
			}
		}
	}
	return -1, -1, false
}

func firstMeaningfulTokenInRange(tokens []model.Token, start int, end int) (int, string) {
	if start < 0 {
		start = 0
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	for i := start; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "" {
			continue
		}
		return i, t
	}
	return -1, ""
}

func isLikelyBracketTitleWordToken(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" {
		return false
	}
	if t == "-" || t == "~" || t == ":" || t == "：" || t == "|" || t == "｜" || t == "/" || t == "／" {
		return false
	}
	if episodeOfTotalPattern.MatchString(t) {
		return false
	}
	if standaloneVersionPattern.MatchString(t) {
		return false
	}
	up := strings.ToUpper(t)
	if up == "END" || up == "FIN" || up == "FINAL" || up == "V2" || up == "V3" {
		return false
	}
	if looksLikeTitleCutMetaToken(t) || isLikelyMetaBracketText(t) {
		return false
	}
	return hasCJK(t) || hasLatin(t)
}

// Guard title-leading numeric tokens inside bracket title spans:
// [16 位元的輝煌 ...], [86-不存在的戰區] should stay in title, not become episode primary.
func isBracketLeadingNumericTitlePrefix(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	openIdx, closeIdx, ok := bracketBoundsAt(tokens, idx)
	if !ok {
		return false
	}
	for j := openIdx + 1; j < idx; j++ {
		if tokens[j].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[j].Text)
		if t == "" || t == "-" || t == "~" || t == ":" || t == "：" {
			continue
		}
		return false
	}
	n1Idx, n1 := firstMeaningfulTokenInRange(tokens, idx+1, closeIdx)
	if n1Idx < 0 {
		return false
	}
	if n1 == "-" || n1 == "~" {
		_, n2 := firstMeaningfulTokenInRange(tokens, n1Idx+1, closeIdx)
		if n2 == "" || episodeOfTotalPattern.MatchString(strings.TrimSpace(n2)) {
			return false
		}
		return isLikelyBracketTitleWordToken(n2)
	}
	return isLikelyBracketTitleWordToken(n1)
}

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

func looksLikeCodecPrefixToken(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return false
	}
	switch low {
	case "hevc", "avc", "h264", "h265", "h266", "x264", "x265", "av1", "mpeg2", "mpeg4", "xvid", "divx":
		return true
	default:
		return false
	}
}

func looksLikeBracketCodecNumber(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	if !isInBracket(tokens, idx) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	if idx-1 >= 0 {
		prev := strings.TrimSpace(tokens[idx-1].Text)
		if prev == "-" && idx-2 >= 0 {
			if looksLikeCodecPrefixToken(strings.TrimSpace(tokens[idx-2].Text)) {
				return true
			}
		}
	}
	return false
}

var episodeSPBracketCombinedPattern = regexp.MustCompile(`(?i)^0*(\d{1,3})\s*[_ .-]\s*(sp|ova|oad|ona|ex|pv|op|ed)\s*$`)
var partIndexLeadPattern = regexp.MustCompile(`(?i)^(?:part|pt|cour|cours)$`)
var seasonSignalTokenPattern = regexp.MustCompile(`(?i)^(?:s\d{1,2}|season|saison|cour|cours)$`)
var seasonSignalCJKTokenPattern = regexp.MustCompile(`^第?[一二三四五六七八九十0-9]{1,3}(?:季|期|部)$`)
var episodePrefixedTokenPattern = regexp.MustCompile(`(?i)^(?:e|ep)\d{1,4}(?:\.\d{1,2})?(?:[vV]\d{1,2})?$`)
var chineseEpisodeDecimalSuffixPattern = regexp.MustCompile(`^第?(\d{1,4})\.(\d{1,2})(?:集|话|話|回)$`)
var chineseEpisodeDecimalTailPattern = regexp.MustCompile(`^(\d{1,2})(?:集|话|話|回)$`)
var chineseEpisodeDecimalHeadPattern = regexp.MustCompile(`^第(\d{1,4})$`)
var bracketCatalogIDPrefixPattern = regexp.MustCompile(`^[A-Z]{3,}[A-Z0-9]*$`)

func hasDashSeparatorAroundToken(tokens []model.Token, idx int) bool {
	if idx-1 >= 0 {
		sep := strings.TrimSpace(tokens[idx-1].Text)
		if sep == "-" || sep == "~" {
			return true
		}
	}
	if idx+1 < len(tokens) {
		sep := strings.TrimSpace(tokens[idx+1].Text)
		if sep == "-" || sep == "~" {
			return true
		}
	}
	return false
}

func looksLikeVolumeContextNumberToken(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	if idx-1 >= 0 {
		prev := strings.TrimSpace(tokens[idx-1].Text)
		if volumeWordPattern.MatchString(prev) {
			return true
		}
	}
	if idx+1 < len(tokens) {
		next := strings.TrimSpace(tokens[idx+1].Text)
		if volumeWordPattern.MatchString(next) {
			return true
		}
	}
	if idx-2 >= 0 {
		prevSep := strings.TrimSpace(tokens[idx-1].Text)
		prevLead := strings.TrimSpace(tokens[idx-2].Text)
		if prevSep == "." && volumeWordPattern.MatchString(prevLead) {
			return true
		}
	}
	if idx+2 < len(tokens) {
		nextSep := strings.TrimSpace(tokens[idx+1].Text)
		nextLead := strings.TrimSpace(tokens[idx+2].Text)
		if nextSep == "." && volumeWordPattern.MatchString(nextLead) {
			return true
		}
	}
	return false
}

func detectEpisodeFromSPBracketSegment(tokens []model.Token) (string, int, bool) {
	for i := 0; i < len(tokens); i++ {
		if !isInBracket(tokens, i) {
			continue
		}
		cur := strings.TrimSpace(tokens[i].Text)
		if cur == "" {
			continue
		}
		if mm := episodeSPBracketCombinedPattern.FindStringSubmatch(cur); len(mm) == 3 {
			n := strings.TrimLeft(strings.TrimSpace(mm[1]), "0")
			if n == "" {
				n = "0"
			}
			return n, i, true
		}
		if !episodeOfTotalPattern.MatchString(cur) || len(cur) > 3 {
			continue
		}
		if i+2 >= len(tokens) {
			continue
		}
		if !isInBracket(tokens, i+1) || !isInBracket(tokens, i+2) {
			continue
		}
		sep := strings.TrimSpace(tokens[i+1].Text)
		if sep != "_" && sep != "-" && sep != "." && sep != "&" && sep != "＆" {
			continue
		}
		next := strings.TrimSpace(tokens[i+2].Text)
		if m := spKindPattern.FindStringSubmatch(next); len(m) < 2 {
			continue
		}
		n := strings.TrimLeft(cur, "0")
		if n == "" {
			n = "0"
		}
		return n, i, true
	}
	return "", -1, false
}

func detectEpisodeWithVersion(tokens []model.Token) (string, int, string, int) {
	bestIdx := -1
	best := ""
	bestScore := -1
	bestVer := ""
	bestKeywordIdx := -1
	bestKeywordDashIdx := -1

	// Pattern: "Episode 52" / "- Episode 52" (common in English TV rip naming).
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].Removed || tokens[i+1].Removed {
			continue
		}
		kw := strings.TrimSpace(tokens[i].Text)
		if !(strings.EqualFold(kw, "episode") || strings.EqualFold(kw, "ep")) {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		d := strings.TrimSpace(tokens[i+1].Text)
		if !episodeOfTotalPattern.MatchString(d) {
			continue
		}
		if yearPattern.MatchString(d) {
			continue
		}
		score := 74
		dashIdx := -1
		// Prefer the explicit "- Episode N" style, but do not require it.
		if i-1 >= 0 {
			prev := strings.TrimSpace(tokens[i-1].Text)
			if prev == "-" {
				score += 4
				dashIdx = i - 1
			}
		}
		n := strings.TrimLeft(d, "0")
		if n == "" {
			n = "0"
		}
		if score > bestScore {
			best = n
			bestIdx = i + 1
			bestScore = score
			bestKeywordIdx = i
			bestKeywordDashIdx = dashIdx
		}
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
				if isBracketScheduleNoticeText(txt) {
					continue
				}
				if isLikelyMetaBracketText(txt) && !isEpisodeLikeBracketText(txt) {
					continue
				}
			}
			if isBracketLeadingNumericTitlePrefix(tokens, i) {
				continue
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
		// Support episode tokens with trailing parenthetical marker:
		// e.g. "03（Onair）" / "03(Premium)".
		if mm := episodeWithParenSuffixPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(mm) >= 4 {
			if yearPattern.MatchString(strings.TrimSpace(mm[2])) {
				continue
			}
			n := strings.TrimLeft(mm[2], "0")
			if n == "" {
				n = "0"
			}
			if strings.TrimSpace(mm[3]) != "" {
				n = n + "." + strings.TrimSpace(mm[3])
			}
			if n != "0" {
				score := len(mm[2]) + 91
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = n
					bestVer = ""
				}
			}
			continue
		}
		// Compact end-flag episode token, e.g. "12END"/"12FIN".
		if mm := episodeEndSuffixPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(mm) == 3 {
			n := strings.TrimLeft(mm[1], "0")
			if n == "" {
				n = "0"
			}
			if n != "0" {
				score := len(mm[1]) + 96
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = n
					bestVer = ""
				}
			}
			continue
		}
		if inBracket {
			if looksLikeBracketCodecNumber(tokens, i) {
				continue
			}
			n := strings.TrimSpace(tokens[i].Text)
			if episodeOfTotalPattern.MatchString(n) {
				if looksLikeVolumeContextNumberToken(tokens, i) {
					continue
				}
				if hasMatchingSeasonNumberAroundToken(tokens, i, 12) && !hasDashSeparatorAroundToken(tokens, i) {
					continue
				}
				if i-2 >= 0 {
					prevSep := strings.TrimSpace(tokens[i-1].Text)
					prevLead := strings.TrimSpace(tokens[i-2].Text)
					if prevSep == "-" && bracketCatalogIDPrefixPattern.MatchString(prevLead) {
						continue
					}
				}
				nn := strings.TrimLeft(n, "0")
				if nn == "" {
					nn = "0"
				}
				if isBracketLeadingNumericTitlePrefix(tokens, i) {
					continue
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
		if mm := chineseEpisodeWithSuffixMarkerPattern.FindStringSubmatch(tokens[i].Text); len(mm) == 2 {
			n := strings.TrimLeft(mm[1], "0")
			if n == "" {
				n = "0"
			}
			score := len(mm[1]) + 90
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = n
			}
			continue
		}
		if mm := chineseEpisodeLeadingPattern.FindStringSubmatch(tokens[i].Text); len(mm) == 2 {
			n := strings.TrimLeft(mm[1], "0")
			if n == "" {
				n = "0"
			}
			score := len(mm[1]) + 86
			if score > bestScore {
				bestScore = score
				bestIdx = i
				best = n
			}
			continue
		}
		if mm := chineseEpisodeDecimalSuffixPattern.FindStringSubmatch(strings.TrimSpace(tokens[i].Text)); len(mm) == 3 {
			maj := strings.TrimLeft(mm[1], "0")
			if maj == "" {
				maj = "0"
			}
			dec := strings.TrimSpace(mm[2])
			if dec != "" {
				n := maj + "." + dec
				score := len(mm[1]) + 93
				if score > bestScore {
					bestScore = score
					bestIdx = i
					best = n
				}
				continue
			}
		}
		if i+2 < len(tokens) {
			left := strings.TrimSpace(tokens[i].Text)
			dot := strings.TrimSpace(tokens[i+1].Text)
			right := strings.TrimSpace(tokens[i+2].Text)
			if strings.HasPrefix(left, "第") && dot == "." {
				if lm := chineseEpisodeDecimalHeadPattern.FindStringSubmatch(left); len(lm) == 2 {
					if rm := chineseEpisodeDecimalTailPattern.FindStringSubmatch(right); len(rm) == 2 {
						maj := strings.TrimLeft(lm[1], "0")
						if maj == "" {
							maj = "0"
						}
						dec := strings.TrimSpace(rm[1])
						if dec != "" {
							n := maj + "." + dec
							score := len(lm[1]) + 93
							if score > bestScore {
								bestScore = score
								bestIdx = i
								best = n
							}
							continue
						}
					}
				}
			}
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
			// Guard: bracketed catalog IDs like "[KAXA-9023]" are product codes, not episodes.
			if prefix == "" && inBracket && i-2 >= 0 {
				prevSep := strings.TrimSpace(tokens[i-1].Text)
				prevLead := strings.TrimSpace(tokens[i-2].Text)
				if prevSep == "-" && bracketCatalogIDPrefixPattern.MatchString(prevLead) {
					continue
				}
			}
			if prefix == "" && looksLikeVolumeContextNumberToken(tokens, i) {
				continue
			}
			if prefix == "" && inBracket && hasMatchingSeasonNumberAroundToken(tokens, i, 12) && !hasDashSeparatorAroundToken(tokens, i) {
				continue
			}
			signalScore := 0
			if prefix == "" {
				if shouldSkipPartIndexEpisodeCandidate(tokens, i, inBracket) {
					continue
				}
				ok := false
				if inBracket {
					if isBracketLeadingNumericTitlePrefix(tokens, i) {
						continue
					}
					ok = true
					signalScore = 96
				}
				if i-1 >= 0 {
					sep := strings.TrimSpace(tokens[i-1].Text)
					if sep == "-" {
						ok = true
						if signalScore < 94 {
							signalScore = 94
						}
						rightBoundary := false
						if i+1 < len(tokens) {
							rightBoundary = isEpisodeBoundaryRightToken(tokens[i+1].Text)
						}
						if i-2 >= 0 {
							prev2 := strings.TrimSpace(tokens[i-2].Text)
							if looksLikeCodecPrefixToken(prev2) {
								ok = false
							}
							if inBracket && bracketCatalogIDPrefixPattern.MatchString(prev2) {
								ok = false
							}
						}
						// Guard: reject cases like "Gintama-3 nen" where the number is followed by a word.
						if !inBracket {
							if i+1 < len(tokens) {
								nextWord := strings.TrimSpace(tokens[i+1].Text)
								if !rightBoundary && (looksLikeLowercaseLatinWordToken(nextWord) || looksLikeBitDepthWordToken(nextWord)) {
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
									if !rightBoundary {
										ok = false
									}
								}
							}
						}
						// Guard: title season suffix before bracket episode, e.g.
						// "... Monogatari -2 [01] ...", should prefer [01].
						if ok && !inBracket && hasLaterBracketEpisodeCandidate(tokens, i+1) {
							if hasMatchingSeasonNumberAroundToken(tokens, i, 12) {
								ok = false
							}
						}
					} else if sep == "★" || sep == "☆" {
						ok = true
						if signalScore < 92 {
							signalScore = 92
						}
					}
				}
				if !ok {
					if i+1 < len(tokens) {
						next := strings.TrimSpace(tokens[i+1].Text)
						if next == "[" || next == "【" || next == "(" || next == "（" {
							if n != "0" {
								if n != "720" && n != "1080" && n != "2160" && n != "480" && n != "540" {
									if hasLaterBracketEpisodeCandidate(tokens, i+1) {
										if hasMatchingSeasonNumberAroundToken(tokens, i, 12) {
											continue
										}
										if i-1 >= 0 {
											prev := strings.TrimSpace(tokens[i-1].Text)
											if isLikelyBracketTitleWordToken(prev) && !strings.EqualFold(prev, "episode") && !strings.EqualFold(prev, "ep") {
												continue
											}
										}
									}
									ok = true
									if signalScore < 88 {
										signalScore = 88
									}
								}
							}
						}
					}
					if !ok {
						continue
					}
				}
				if n == "0" {
					isStrongZero := inBracket
					if !isStrongZero && i-1 >= 0 && strings.TrimSpace(tokens[i-1].Text) == "-" {
						if i+1 < len(tokens) && isEpisodeBoundaryRightToken(tokens[i+1].Text) {
							isStrongZero = true
						}
					}
					if !isStrongZero {
						continue
					}
				}
			}
			score := len(m[2])
			if prefix != "" {
				// Medium EP signal: EP12 / E12 / 第12
				score += 72
			} else if signalScore > 0 {
				// Strong EP signal: bracketed number / dash-number / star-number.
				score += signalScore
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
		if bestKeywordIdx >= 0 && bestKeywordIdx < len(tokens) {
			tokens[bestKeywordIdx].Removed = true
		}
		if bestKeywordDashIdx >= 0 && bestKeywordDashIdx < len(tokens) {
			tokens[bestKeywordDashIdx].Removed = true
		}
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

func shouldSkipPartIndexEpisodeCandidate(tokens []model.Token, idx int, inBracket bool) bool {
	if inBracket || idx <= 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimSpace(tokens[idx].Text)
	if !episodeOfTotalPattern.MatchString(cur) {
		return false
	}
	prev := strings.TrimSpace(tokens[idx-1].Text)
	if prev == "" {
		return false
	}
	if !partIndexLeadPattern.MatchString(prev) {
		return false
	}
	return hasSeasonSignalAroundToken(tokens, idx, 6)
}

func hasSeasonSignalAroundToken(tokens []model.Token, idx int, window int) bool {
	if len(tokens) == 0 || idx < 0 || idx >= len(tokens) {
		return false
	}
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := idx + window
	if end >= len(tokens) {
		end = len(tokens) - 1
	}
	for i := start; i <= end; i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if seasonSignalTokenPattern.MatchString(txt) || seasonSignalCJKTokenPattern.MatchString(txt) {
			return true
		}
	}
	return false
}

func hasMatchingSeasonNumberAroundToken(tokens []model.Token, idx int, window int) bool {
	if len(tokens) == 0 || idx < 0 || idx >= len(tokens) {
		return false
	}
	cur := strings.TrimLeft(strings.TrimSpace(tokens[idx].Text), "0")
	if cur == "" {
		cur = "0"
	}
	if !episodeOfTotalPattern.MatchString(cur) && cur != "0" {
		return false
	}
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := idx + window
	if end >= len(tokens) {
		end = len(tokens) - 1
	}
	matchSeasonVal := func(v string) bool {
		v = strings.TrimLeft(strings.TrimSpace(v), "0")
		if v == "" {
			v = "0"
		}
		return v == cur
	}
	for i := start; i <= end; i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if m := seasonOnlyPattern.FindStringSubmatch(txt); len(m) == 2 && matchSeasonVal(m[1]) {
			return true
		}
		if m := seasonEmbeddedPattern.FindStringSubmatch(txt); len(m) == 2 && matchSeasonVal(m[1]) {
			return true
		}
		if m := seasonCJKPattern.FindStringSubmatch(txt); len(m) == 2 {
			if v := parseSmallSeasonNumberToken(m[1]); v != "" && matchSeasonVal(v) {
				return true
			}
		}
		if m := seasonOrdinalPattern.FindStringSubmatch(txt); len(m) == 2 {
			if i+1 < len(tokens) && seasonWordLoosePattern.MatchString(strings.TrimSpace(tokens[i+1].Text)) && matchSeasonVal(m[1]) {
				return true
			}
		}
		if seasonWordLoosePattern.MatchString(txt) {
			if i+1 < len(tokens) {
				if v := parseSmallSeasonNumberToken(strings.TrimSpace(tokens[i+1].Text)); v != "" && matchSeasonVal(v) {
					return true
				}
			}
		}
	}
	return false
}

func hasLaterBracketEpisodeCandidate(tokens []model.Token, from int) bool {
	if from < 0 {
		from = 0
	}
	for i := from; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if !isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" || !episodeOfTotalPattern.MatchString(txt) {
			continue
		}
		if looksLikeVolumeContextNumberToken(tokens, i) || yearPattern.MatchString(txt) {
			continue
		}
		if isTitlePrefixedDigitRangeInBracket(tokens, i) {
			continue
		}
		if hasMatchingSeasonNumberAroundToken(tokens, i, 12) && !hasDashSeparatorAroundToken(tokens, i) {
			continue
		}
		return true
	}
	return false
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

func extractEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute, allowUnderscoreRoute bool) {
	if out == nil || len(tokens) == 0 {
		return
	}
	extractRestoreEditionAttrsAndRemoveFromTokens(tokens, out)
	extractWebVersionEditionAttrsAndRemoveFromTokens(tokens, out)
	extractAudioCommentaryEditionAttrsAndRemoveFromTokens(tokens, out, allowUnderscoreRoute)
	extractUncutEditionAttrsAndRemoveFromTokens(tokens, out)
	extractExtendedCutEditionAttrsAndRemoveFromTokens(tokens, out)
	extractUncensoredEditionAttrsAndRemoveFromTokens(tokens, out)
	extractDirectorsCutEditionAttrsAndRemoveFromTokens(tokens, out)
	extractRemasteredEditionAttrsAndRemoveFromTokens(tokens, out)
	extractEncodeVerEditionAttrsAndRemoveFromTokens(tokens, out)
}

func extractRestoreEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	findBlockEnd := func(start int, open, close string) int {
		depth := 0
		for j := start; j < len(tokens); j++ {
			if tokens[j].Removed {
				continue
			}
			cur := strings.TrimSpace(tokens[j].Text)
			if cur == open {
				depth++
				continue
			}
			if cur == close {
				depth--
				if depth == 0 {
					return j
				}
			}
		}
		return -1
	}
	reRestore := regexp.MustCompile(`(?i)(?:数字修复|數字修復|修复版|修復版|修复|修復|重制|重製|restore(?:d)?|remaster(?:ed)?)`)
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		open := strings.TrimSpace(tokens[i].Text)
		close := ""
		switch open {
		case "[":
			close = "]"
		case "【":
			close = "】"
		case "(":
			close = ")"
		case "（":
			close = "）"
		default:
			continue
		}
		end := findBlockEnd(i, open, close)
		if end <= i+1 {
			continue
		}
		inner := make([]string, 0, end-i-1)
		for k := i + 1; k < end; k++ {
			if tokens[k].Removed {
				continue
			}
			t := strings.TrimSpace(tokens[k].Text)
			if t == "" {
				continue
			}
			inner = append(inner, t)
		}
		if len(inner) == 0 {
			continue
		}
		joined := strings.ToLower(strings.Join(inner, ""))
		has4k := strings.Contains(joined, "4k")
		has2k := strings.Contains(joined, "2k")
		if !has4k || !reRestore.MatchString(strings.Join(inner, "")) {
			continue
		}
		norm := "RESTORED_EDITION"
		if has2k {
			norm = "RESTORE_4K_2K_EDITION"
		}
		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: norm,
			TokenStart: i,
			TokenEnd:   end + 1,
			Confidence: 0.74,
			Source:     "edition_token",
		})
		for k := i; k <= end; k++ {
			tokens[k].Removed = true
		}
		i = end
	}
}

func extractWebVersionEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	add := func(start, end int) {
		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: "WEB_VERSION",
			TokenStart: start,
			TokenEnd:   end,
			Confidence: 0.72,
			Source:     "edition_token",
		})
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		low := strings.ToLower(txt)
		compact := strings.ReplaceAll(strings.ReplaceAll(low, "_", ""), "-", "")
		compact = strings.Trim(compact, ".。· ")
		// WEB版 / WEB版本 / webver / webversion in one token.
		if compact == "web版" || compact == "web版本" || compact == "webver" || compact == "webversion" {
			add(i, i+1)
			tokens[i].Removed = true
			continue
		}
		if compact != "web" {
			continue
		}
		j := nextAliveToken(tokens, i+1)
		if j < 0 {
			continue
		}
		nextRaw := strings.TrimSpace(tokens[j].Text)
		nextLow := strings.ToLower(strings.Trim(nextRaw, ".。"))
		if nextRaw != "版" && nextLow != "ver" && nextLow != "version" {
			continue
		}
		end := j + 1
		if (nextLow == "ver" || nextLow == "version") && j+1 < len(tokens) {
			if d := strings.TrimSpace(tokens[j+1].Text); d == "." || d == "。" {
				end = j + 2
				tokens[j+1].Removed = true
			}
		}
		add(i, end)
		tokens[i].Removed = true
		tokens[j].Removed = true
	}
}

func extractAudioCommentaryEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute, allowUnderscoreRoute bool) {
	if out == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		curNorm := normalizeEditionToken(tokens[i].Text, false)
		if curNorm != "audio" {
			// Underscore / hyphen collapsed route is intentionally gated to specific groups
			// to avoid polluting generic parsing.
			if !(allowUnderscoreRoute && normalizeEditionToken(tokens[i].Text, true) == "audiocommentary") {
				continue
			}
		}

		j := -1
		if curNorm == "audio" {
			j = nextAliveToken(tokens, i+1)
			if j < 0 {
				continue
			}
			if normalizeEditionToken(tokens[j].Text, false) != "commentary" {
				if allowUnderscoreRoute {
					if txt := strings.TrimSpace(tokens[j].Text); txt == "_" || txt == "-" {
						j2 := nextAliveToken(tokens, j+1)
						if j2 >= 0 && normalizeEditionToken(tokens[j2].Text, false) == "commentary" {
							j = j2
						} else {
							continue
						}
					} else {
						continue
					}
				} else {
					continue
				}
			}
		} else {
			// audio_commentary / audio-commentary (group-routed only)
			j = i
		}

		k := nextAliveToken(tokens, j+1)
		if k < 0 {
			continue
		}
		ver := normalizeEditionToken(tokens[k].Text, false)
		if ver != "ver" && ver != "version" {
			continue
		}

		end := k
		if n := nextAliveToken(tokens, end+1); n >= 0 {
			norm := strings.TrimSpace(tokens[n].Text)
			if norm == "." || norm == "~" {
				end = n
			}
		}
		if n := nextAliveToken(tokens, end+1); n >= 0 {
			norm := strings.TrimSpace(tokens[n].Text)
			if norm == "~" {
				end = n
			}
		}

		start := i
		if i-1 >= 0 {
			prev := strings.TrimSpace(tokens[i-1].Text)
			if prev == "~" {
				start = i - 1
			}
		}

		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: "AUDIO_COMMENTARY",
			TokenStart: i,
			TokenEnd:   k + 1,
			Confidence: 0.75,
			Source:     "edition_token",
		})

		for idx := start; idx <= end; idx++ {
			tokens[idx].Removed = true
		}
	}
}

func extractEncodeVerEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		ver := normalizeEditionToken(tokens[i].Text, false)
		if ver != "ver" && ver != "version" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		prev := prevAliveToken(tokens, i-1)
		if prev < 0 {
			continue
		}
		prevNorm := normalizeEditionToken(tokens[prev].Text, true)
		if !isLikelyEncodeEditionAnchor(prevNorm) {
			continue
		}
		end := i
		if n := nextAliveToken(tokens, i+1); n >= 0 && strings.TrimSpace(tokens[n].Text) == "." {
			end = n
		}
		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: "ENCODE_VER",
			TokenStart: i,
			TokenEnd:   end + 1,
			Confidence: 0.7,
			Source:     "edition_token",
		})
		for idx := i; idx <= end; idx++ {
			tokens[idx].Removed = true
		}
	}
}

func extractUncensoredEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if compactEditionWord(tokens[i].Text) != "uncensored" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		j := nextAliveToken(tokens, i+1)
		if j < 0 {
			continue
		}
		ver := normalizeEditionToken(tokens[j].Text, false)
		if ver != "ver" && ver != "version" {
			continue
		}
		end := j
		if n := nextAliveToken(tokens, j+1); n >= 0 && strings.TrimSpace(tokens[n].Text) == "." {
			end = n
		}
		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: "UNCENSORED_VER",
			TokenStart: i,
			TokenEnd:   end + 1,
			Confidence: 0.72,
			Source:     "edition_token",
		})
		for k := i; k <= end; k++ {
			tokens[k].Removed = true
		}
	}
}

func extractUncutEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if compactEditionWord(tokens[i].Text) != "uncut" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		end := i
		norm := "UNCUT"
		if j := nextAliveToken(tokens, i+1); j >= 0 {
			ver := normalizeEditionToken(tokens[j].Text, false)
			if ver == "ver" || ver == "version" {
				end = j
				norm = "UNCUT_VER"
				if n := nextAliveToken(tokens, j+1); n >= 0 && strings.TrimSpace(tokens[n].Text) == "." {
					end = n
				}
			}
		}
		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: norm,
			TokenStart: i,
			TokenEnd:   end + 1,
			Confidence: 0.72,
			Source:     "edition_token",
		})
		for k := i; k <= end; k++ {
			tokens[k].Removed = true
		}
	}
}

func extractDirectorsCutEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		left := compactEditionWord(tokens[i].Text)
		if left != "directors" && left != "director" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		j := nextAliveToken(tokens, i+1)
		if j < 0 {
			continue
		}
		if compactEditionWord(tokens[j].Text) != "cut" {
			continue
		}
		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: "DIRECTORS_CUT",
			TokenStart: i,
			TokenEnd:   j + 1,
			Confidence: 0.72,
			Source:     "edition_token",
		})
		tokens[i].Removed = true
		tokens[j].Removed = true
	}
}

func extractExtendedCutEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if compactEditionWord(tokens[i].Text) != "extended" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		j := nextAliveToken(tokens, i+1)
		if j < 0 || compactEditionWord(tokens[j].Text) != "cut" {
			continue
		}
		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: "EXTENDED_CUT",
			TokenStart: i,
			TokenEnd:   j + 1,
			Confidence: 0.72,
			Source:     "edition_token",
		})
		tokens[i].Removed = true
		tokens[j].Removed = true
	}
}

func extractRemasteredEditionAttrsAndRemoveFromTokens(tokens []model.Token, out *[]model.Attribute) {
	if out == nil || len(tokens) == 0 {
		return
	}
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		cur := compactEditionWord(tokens[i].Text)
		if cur != "remaster" && cur != "remastered" {
			continue
		}
		if !isInBracket(tokens, i) && !isInParen(tokens, i) {
			continue
		}
		end := i
		norm := "REMASTERED"
		if j := nextAliveToken(tokens, i+1); j >= 0 {
			ver := normalizeEditionToken(tokens[j].Text, false)
			if ver == "ver" || ver == "version" {
				end = j
				norm = "REMASTERED_VER"
				if n := nextAliveToken(tokens, j+1); n >= 0 && strings.TrimSpace(tokens[n].Text) == "." {
					end = n
				}
			}
		}
		*out = append(*out, model.Attribute{
			Type:       model.AttrNote,
			Normalized: norm,
			TokenStart: i,
			TokenEnd:   end + 1,
			Confidence: 0.72,
			Source:     "edition_token",
		})
		for k := i; k <= end; k++ {
			tokens[k].Removed = true
		}
	}
}

func isLikelyEncodeEditionAnchor(norm string) bool {
	if norm == "" {
		return false
	}
	switch norm {
	case "x264", "x265", "h264", "h265", "hevc", "avc", "mpeg2", "av1":
		return true
	}
	if strings.HasSuffix(norm, "bit") {
		n := strings.TrimSuffix(norm, "bit")
		return n == "8" || n == "10" || n == "12"
	}
	return false
}

func normalizeEditionToken(raw string, foldConnectors bool) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimFunc(s, func(r rune) bool {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
		return true
	})
	s = strings.ToLower(s)
	if foldConnectors {
		s = strings.ReplaceAll(s, "_", "")
		s = strings.ReplaceAll(s, "-", "")
	}
	return s
}

func compactEditionWord(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func nextAliveToken(tokens []model.Token, from int) int {
	for i := from; i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if strings.TrimSpace(tokens[i].Text) == "" {
			continue
		}
		return i
	}
	return -1
}

func prevAliveToken(tokens []model.Token, from int) int {
	for i := from; i >= 0; i-- {
		if tokens[i].Removed {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if txt == "-" || txt == "_" || txt == "/" || txt == "." || txt == "|" || txt == "｜" {
			continue
		}
		return i
	}
	return -1
}

func shouldUseUnderscoreEditionRoute(rawTitle string) bool {
	toks := tokenize.Tokenize(rawTitle)
	if len(toks) < 3 || toks[0] != "[" || toks[2] != "]" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(toks[1]), "shincaps")
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
	// Guard: EP-like tokens such as E192 / EP07 are episode markers, not release groups.
	if episodePrefixedTokenPattern.MatchString(s) {
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
	if up == "NCOP" || up == "NCED" || up == "PV" || up == "CM" {
		return false
	}
	if tailGroupTechTokenPattern.MatchString(up) {
		return false
	}
	return true
}

func detectTailGroup(tokens []model.Token) (string, int) {
	return detectTailGroupWithMinIdx(tokens, -1)
}

func detectTailGroupAfter(tokens []model.Token, minIdx int) (string, int) {
	return detectTailGroupWithMinIdx(tokens, minIdx)
}

func detectTailGroupWithMinIdx(tokens []model.Token, minIdx int) (string, int) {
	// Prefer explicit "... X - Raws/Subs" pattern, even if tokens were marked removed by tech passes.
	for i := len(tokens) - 1; i >= 0; i-- {
		if i <= minIdx {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		text := strings.TrimSpace(tokens[i].Text)
		if !strings.EqualFold(text, "Raws") && !strings.EqualFold(text, "Subs") {
			continue
		}
		if i-2 <= minIdx {
			continue
		}
		if strings.TrimSpace(tokens[i-1].Text) != "-" {
			continue
		}
		left := strings.TrimSpace(tokens[i-2].Text)
		if !isTailGroupToken(left) {
			continue
		}
		return left + "-" + text, i - 2
	}

	lastTail := -1
	for j := len(tokens) - 1; j >= 0; j-- {
		if j <= minIdx {
			continue
		}
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
		if i <= minIdx {
			continue
		}
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

func (p *Parser) extractTitle(tokens []model.Token, episodeIdx int, episodeRangeIdx int, seasonCutIdx int) string {
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
		if !embeddedChineseEpisodeSuffixPattern.MatchString(strings.TrimSpace(tokens[episodeIdx].Text)) &&
			shouldUseEpisodeAsTitleCut(tokens, start, episodeIdx) {
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
				if p.looksLikeDotSeparatedMetaCutToken(t) {
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
var reUnderscoreRun = regexp.MustCompile(`_+`)
var reMultiSpace = regexp.MustCompile(`\s+`)

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
	s = reUnderscoreRun.ReplaceAllString(s, " ")
	s = reMultiSpace.ReplaceAllString(s, " ")
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

func aliasChainSemanticKey(s string) string {
	s = normalizeTitleMain(cleanAliasToken(strings.TrimSpace(s)))
	if s == "" {
		return ""
	}
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func aliasChainKeysContain(keys map[string]struct{}, key string) bool {
	if key == "" || len(keys) == 0 {
		return false
	}
	if _, ok := keys[key]; ok {
		return true
	}
	for k := range keys {
		if k == "" {
			continue
		}
		if k == key {
			return true
		}
		if len(k) >= 5 && len(key) >= 5 {
			if strings.Contains(k, key) || strings.Contains(key, k) {
				return true
			}
		}
	}
	return false
}

func finalTitleValueFromAttributes(attrs []model.Attribute, fallback string) string {
	for i := len(attrs) - 1; i >= 0; i-- {
		if attrs[i].Type != model.AttrTitle {
			continue
		}
		v := strings.TrimSpace(attrs[i].Normalized)
		if v != "" {
			return v
		}
	}
	return strings.TrimSpace(fallback)
}

func finalTitleAliasValuesFromAttributes(attrs []model.Attribute) []string {
	if len(attrs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	for _, a := range attrs {
		if a.Type != model.AttrTitleAlias {
			continue
		}
		v := strings.TrimSpace(a.Normalized)
		if v == "" {
			continue
		}
		key := aliasSemanticKey(v)
		if key == "" {
			key = strings.ToLower(v)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}

func (p *Parser) consumeAliasChainTokensByFinalAliases(tokens []model.Token, groupSpan [2]int, title string, aliases []string, episodeIdx int, episodeRangeIdx int, seasonIdx int, attrs []model.Attribute) {
	if len(tokens) == 0 || strings.TrimSpace(title) == "" || len(aliases) == 0 {
		return
	}
	scanStart := titleStartAfterLeadingEmptyBracketBlocks(tokens)
	if len(groupSpan) == 2 && groupSpan[1] > scanStart {
		scanStart = groupSpan[1]
	}
	if scanStart < 0 {
		scanStart = 0
	}
	scanEnd := len(tokens)
	if episodeRangeIdx >= scanStart && episodeRangeIdx < scanEnd {
		scanEnd = episodeRangeIdx
	}
	if episodeIdx >= scanStart && episodeIdx < scanEnd {
		scanEnd = episodeIdx
	}
	if techStart, ok := firstTechTokenIndexFromAttrs(attrs); ok && techStart >= 0 && techStart < scanEnd {
		if techStart > scanStart {
			scanEnd = techStart
		}
	}
	if scanStart >= scanEnd {
		return
	}

	firstDelim := -1
	lastDelim := -1
	for i := scanStart; i < scanEnd; i++ {
		if tokens[i].Removed {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		if !isAliasChainDelimiterToken(tokens[i].Text) {
			continue
		}
		if firstDelim < 0 {
			firstDelim = i
		}
		lastDelim = i
	}
	if firstDelim < 0 || lastDelim < firstDelim {
		return
	}

	type seg struct {
		from int
		to   int
		key  string
	}
	segments := make([]seg, 0, 4)
	cur := scanStart
	for i := scanStart; i <= scanEnd; i++ {
		isSplit := i == scanEnd
		if !isSplit && !tokens[i].Removed && !isInBracket(tokens, i) && !isInParen(tokens, i) && isAliasChainDelimiterToken(tokens[i].Text) {
			isSplit = true
		}
		if !isSplit {
			continue
		}
		if cur < i {
			txt := extractTitleFromSpanAllowRemoved(tokens, cur, i, false)
			key := aliasChainSemanticKey(txt)
			if key != "" {
				segments = append(segments, seg{from: cur, to: i, key: key})
			}
		}
		cur = i + 1
	}
	if len(segments) < 2 {
		return
	}

	titleKey := aliasChainSemanticKey(title)
	if titleKey == "" {
		return
	}
	aliasKeys := make(map[string]struct{}, len(aliases))
	for _, a := range aliases {
		k := aliasChainSemanticKey(a)
		if k == "" {
			continue
		}
		aliasKeys[k] = struct{}{}
	}
	if len(aliasKeys) == 0 {
		return
	}

	titleSegIdx := -1
	titleScore := -1
	for i, s := range segments {
		score := -1
		if s.key == titleKey {
			score = 100 + len(titleKey)
		} else if len(s.key) >= 5 && len(titleKey) >= 5 {
			if strings.Contains(s.key, titleKey) || strings.Contains(titleKey, s.key) {
				score = len(s.key)
			}
		}
		if score > titleScore {
			titleScore = score
			titleSegIdx = i
		}
	}

	matched := false
	for i, s := range segments {
		if i == titleSegIdx {
			continue
		}
		if !aliasChainKeysContain(aliasKeys, s.key) {
			continue
		}
		for k := s.from; k < s.to && k < len(tokens); k++ {
			tokens[k].Removed = true
		}
		matched = true
	}
	if !matched {
		if titleSegIdx < 0 {
			return
		}
		titleSegKey := segments[titleSegIdx].key
		if titleSegKey == "" {
			return
		}
		otherSegExists := false
		for i, s := range segments {
			if i == titleSegIdx {
				continue
			}
			if s.key == "" || s.key == titleSegKey {
				continue
			}
			otherSegExists = true
			break
		}
		if !otherSegExists {
			return
		}
		// Fallback: delimiter-only cleanup for bilingual title chains where
		// aliases are not promoted but title is already finalized from one side.
		matched = true
	}
	for i := firstDelim; i <= lastDelim && i < len(tokens); i++ {
		if tokens[i].Removed {
			continue
		}
		if isAliasChainDelimiterToken(tokens[i].Text) {
			tokens[i].Removed = true
		}
	}
}

func (p *Parser) consumeTitleGapSeasonBridgeMarkers(tokens []model.Token, titleEnd int, episodeIdx int, episodeRangeIdx int, attrs []model.Attribute) {
	if len(tokens) == 0 || titleEnd < 0 || titleEnd >= len(tokens) {
		return
	}
	scanStart := titleEnd
	scanEnd := len(tokens)
	if episodeRangeIdx >= scanStart && episodeRangeIdx < scanEnd {
		scanEnd = episodeRangeIdx
	}
	if episodeIdx >= scanStart && episodeIdx < scanEnd {
		scanEnd = episodeIdx
	}
	if techStart, ok := firstTechTokenIndexFromAttrs(attrs); ok && techStart > scanStart && techStart < scanEnd {
		scanEnd = techStart
	}
	if scanStart >= scanEnd {
		return
	}

	for i := scanStart; i < scanEnd; i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if isSingleSlashSeasonSignalToken(txt) || isSingleSlashBridgeToken(txt) {
			tokens[i].Removed = true
		}
	}
}

func (p *Parser) consumeTitleGapLatinAliasRuns(tokens []model.Token, title string, aliases []string, titleEnd int, episodeIdx int, episodeRangeIdx int, attrs []model.Attribute) {
	if len(tokens) == 0 || titleEnd < 0 || titleEnd >= len(tokens) {
		return
	}
	if !hasCJK(title) || hasLatin(title) {
		return
	}
	aliasKeys := map[string]bool{}
	for _, a := range aliases {
		aa := strings.TrimSpace(a)
		if aa == "" || hasCJK(aa) || !hasLatin(aa) {
			continue
		}
		if key := aliasSemanticKey(aa); key != "" {
			aliasKeys[key] = true
		}
	}
	if len(aliasKeys) == 0 {
		return
	}

	scanStart := titleEnd
	scanEnd := len(tokens)
	if episodeRangeIdx >= scanStart && episodeRangeIdx < scanEnd {
		scanEnd = episodeRangeIdx
	}
	if episodeIdx >= scanStart && episodeIdx < scanEnd {
		scanEnd = episodeIdx
	}
	if techStart, ok := firstTechTokenIndexFromAttrs(attrs); ok && techStart > scanStart && techStart < scanEnd {
		scanEnd = techStart
	}
	if scanStart >= scanEnd {
		return
	}

	for i := scanStart; i < scanEnd; i++ {
		if tokens[i].Removed || !isTitleGapLatinAliasToken(tokens[i].Text) {
			continue
		}
		start := i
		for i < scanEnd && !tokens[i].Removed && isTitleGapLatinAliasToken(tokens[i].Text) {
			i++
		}
		stop := i
		if stop-start < 2 {
			continue
		}
		cand := cleanAliasToken(extractTitleFromSpanAllowRemoved(tokens, start, stop, false))
		if cand == "" {
			continue
		}
		if aliasKeys[aliasSemanticKey(cand)] {
			for k := start; k < stop && k < len(tokens); k++ {
				tokens[k].Removed = true
			}
		}
	}
}

func (p *Parser) consumeTitleGapAliasSemanticTokens(tokens []model.Token, aliases []string, titleEnd int, episodeIdx int, episodeRangeIdx int, attrs []model.Attribute) {
	if len(tokens) == 0 || len(aliases) == 0 || titleEnd < 0 || titleEnd >= len(tokens) {
		return
	}
	aliasKeys := map[string]bool{}
	for _, a := range aliases {
		k := aliasSemanticKey(a)
		if k != "" {
			aliasKeys[k] = true
		}
	}
	if len(aliasKeys) == 0 {
		return
	}

	scanStart := titleEnd
	scanEnd := len(tokens)
	if episodeRangeIdx >= scanStart && episodeRangeIdx < scanEnd {
		scanEnd = episodeRangeIdx
	}
	if episodeIdx >= scanStart && episodeIdx < scanEnd {
		scanEnd = episodeIdx
	}
	if techStart, ok := firstTechTokenIndexFromAttrs(attrs); ok && techStart > scanStart && techStart < scanEnd {
		scanEnd = techStart
	}
	if scanStart >= scanEnd {
		return
	}

	for i := scanStart; i < scanEnd; i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" || isDashDelimiterToken(txt) || txt == "/" || txt == "／" {
			continue
		}
		switch txt {
		case "[", "]", "(", ")", "（", "）", "【", "】", "{", "}":
			continue
		}
		tk := aliasSemanticKey(txt)
		if tk == "" {
			continue
		}
		for ak := range aliasKeys {
			if tk == ak {
				tokens[i].Removed = true
				break
			}
			if strings.Contains(tk, ak) && hasEmbeddedSeasonMarkerToken(txt) {
				tokens[i].Removed = true
				break
			}
		}
	}
}

func (p *Parser) consumeTitleGapBatchTVMarkers(tokens []model.Token, titleEnd int, episodeIdx int, episodeRangeIdx int, attrs []model.Attribute) {
	if len(tokens) == 0 || titleEnd < 0 || titleEnd >= len(tokens) {
		return
	}
	// Scope to range-like outputs only; avoid touching regular TV title semantics.
	if episodeRangeIdx < 0 {
		return
	}
	scanStart := titleEnd
	scanEnd := len(tokens)
	if episodeRangeIdx >= scanStart && episodeRangeIdx < scanEnd {
		scanEnd = episodeRangeIdx
	}
	if episodeIdx >= scanStart && episodeIdx < scanEnd {
		scanEnd = episodeIdx
	}
	if techStart, ok := firstTechTokenIndexFromAttrs(attrs); ok && techStart > scanStart && techStart < scanEnd {
		scanEnd = techStart
	}
	if scanStart >= scanEnd {
		return
	}
	for i := scanStart; i < scanEnd; i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.ToLower(strings.TrimSpace(tokens[i].Text))
		if txt != "tv" && txt != "tb" && txt != "тв" && txt != "тv" {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) || hasSeasonSignalAroundToken(tokens, i, 3) {
			tokens[i].Removed = true
		}
	}
}

func hasEmbeddedSeasonMarkerToken(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if isSingleSlashSeasonSignalToken(ss) {
		return true
	}
	if reExplicitSeasonMarkerCJK.MatchString(ss) {
		return true
	}
	if titleGapEmbeddedSeasonTokenPattern.MatchString(ss) {
		return true
	}
	return false
}

func hasSemanticTokenAroundDelimiter(tokens []model.Token, start int, end int, step int) bool {
	if step == 0 {
		return false
	}
	for i := start; ; i += step {
		if step > 0 {
			if i >= end {
				break
			}
		} else {
			if i < end {
				break
			}
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" || isAliasChainDelimiterToken(txt) {
			continue
		}
		switch txt {
		case "-", "_", "~", ":", "：", "·", "•", "☆", "★", ".", "。":
			continue
		}
		return aliasChainSemanticKey(txt) != ""
	}
	return false
}

func (p *Parser) consumeGroupSpecificTitleChainDelimiters(tokens []model.Token, groupSpan [2]int, group string, episodeIdx int, episodeRangeIdx int, attrs []model.Attribute) {
	if len(tokens) == 0 {
		return
	}
	groupKey := strings.ToLower(strings.TrimSpace(group))
	if !strings.Contains(groupKey, "jibaketa") {
		return
	}
	scanStart := titleStartAfterLeadingEmptyBracketBlocks(tokens)
	if len(groupSpan) == 2 && groupSpan[1] > scanStart {
		scanStart = groupSpan[1]
	}
	if scanStart < 0 {
		scanStart = 0
	}
	scanEnd := len(tokens)
	if episodeRangeIdx >= scanStart && episodeRangeIdx < scanEnd {
		scanEnd = episodeRangeIdx
	}
	if episodeIdx >= scanStart && episodeIdx < scanEnd {
		scanEnd = episodeIdx
	}
	if techStart, ok := firstTechTokenIndexFromAttrs(attrs); ok && techStart >= scanStart && techStart < scanEnd {
		scanEnd = techStart
	}
	if scanStart >= scanEnd {
		return
	}
	for i := scanStart; i < scanEnd; i++ {
		if tokens[i].Removed || isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		if strings.TrimSpace(tokens[i].Text) != "/" && strings.TrimSpace(tokens[i].Text) != "／" {
			continue
		}
		leftOK := hasSemanticTokenAroundDelimiter(tokens, i-1, scanStart, -1)
		rightOK := hasSemanticTokenAroundDelimiter(tokens, i+1, scanEnd, 1)
		if leftOK && rightOK {
			tokens[i].Removed = true
		}
	}
}

func (p *Parser) extractPreEpisodePipeAliasesAndConsume(tokens []model.Token, start int, end int) (string, []string, bool) {
	if p == nil || len(tokens) == 0 {
		return "", nil, false
	}
	if start < 0 {
		start = 0
	}
	if end > len(tokens) {
		end = len(tokens)
	}
	if start >= end {
		return "", nil, false
	}

	firstDelim := -1
	pipeCount := 0
	slashCount := 0
	for i := start; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		if isInBracket(tokens, i) || isInParen(tokens, i) {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if isAliasChainDelimiterToken(t) {
			if firstDelim < 0 {
				firstDelim = i
			}
			if t == "|" || t == "｜" {
				pipeCount++
			} else {
				slashCount++
			}
			continue
		}
	}
	if firstDelim < 0 {
		return "", nil, false
	}
	// Safe trigger:
	// - pipe chain: single pipe is enough
	// - slash chain: require at least two separators to avoid treating regular "Fate/Zero" as alias split
	if pipeCount == 0 && slashCount < 2 {
		return "", nil, false
	}

	scanEnd := end
	for i := firstDelim + 1; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "[" || t == "【" {
			scanEnd = i
			break
		}
	}
	if scanEnd <= start {
		return "", nil, false
	}

	type seg struct {
		text      string
		tokenFrom int
		tokenTo   int
	}
	segments := make([]seg, 0, 4)
	delimCount := 0
	segStart := start
	for i := start; i <= scanEnd; i++ {
		isSplit := i == scanEnd
		if !isSplit {
			t := strings.TrimSpace(tokens[i].Text)
			if isAliasChainDelimiterToken(t) {
				isSplit = true
				delimCount++
			}
		}
		if !isSplit {
			continue
		}
		if segStart < i {
			txt := extractTitleFromSpanAllowRemoved(tokens, segStart, i, false)
			txt = strings.TrimSpace(txt)
			if txt != "" {
				segments = append(segments, seg{text: txt, tokenFrom: segStart, tokenTo: i})
			}
		}
		segStart = i + 1
	}
	if delimCount == 0 || len(segments) < 2 {
		return "", nil, false
	}

	rawParts := make([]string, 0, len(segments))
	for _, s := range segments {
		rawParts = append(rawParts, s.text)
	}
	main, aliases := inferTitleAndAliasesFromTitleCanonical(strings.Join(rawParts, " / "), p.isValidAliasToken)
	main = normalizeTitleMain(cleanTitleToken(main))
	if strings.TrimSpace(main) == "" || len(aliases) == 0 {
		return "", nil, false
	}
	mainKey := strings.ToLower(strings.TrimSpace(main))
	mainSegIdx := -1
	mainSegScore := -1
	for i, s := range segments {
		key := strings.ToLower(strings.TrimSpace(normalizeTitleMain(cleanTitleToken(s.text))))
		if key == "" {
			continue
		}
		score := -1
		if key == mainKey {
			score = len(key) + 100
		} else if strings.Contains(key, mainKey) {
			score = len(mainKey)
		} else if strings.Contains(mainKey, key) {
			score = len(key)
		}
		if score > mainSegScore {
			mainSegScore = score
			mainSegIdx = i
		}
	}

	for i := firstDelim; i < scanEnd; i++ {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if isAliasChainDelimiterToken(t) {
			tokens[i].Removed = true
		}
	}
	if mainSegIdx < 0 {
		return main, aliases, true
	}
	for i := 0; i < len(segments); i++ {
		if i == mainSegIdx {
			continue
		}
		from := segments[i].tokenFrom
		to := segments[i].tokenTo
		if from < 0 {
			from = 0
		}
		if to > len(tokens) {
			to = len(tokens)
		}
		for k := from; k < to; k++ {
			tokens[k].Removed = true
		}
	}

	return main, aliases, true
}

var singleSlashAliasSeasonSignalPattern = regexp.MustCompile(`(?i)^(?:s\d{1,2}|p\d{1,2}|season|saison|part|cour|\d{1,2}(?:st|nd|rd|th))$`)
var singleSlashAliasSeasonSignalCJKPattern = regexp.MustCompile(`^第?[一二三四五六七八九十0-9]{1,3}(?:季|期|部)$`)
var singleSlashAliasSeasonPhrasePattern = regexp.MustCompile(`(?i)^season\s*\d{1,2}$`)
var singleSlashAliasEmbeddedSeasonCJKPattern = regexp.MustCompile(`^第?[一二三四五六七八九十0-9]{1,3}(?:季|期|部).*$`)
var singleSlashAliasBridgeTokenPattern = regexp.MustCompile(`(?i)^(?:special|ova|oad|ona|sp|dub|audio|dual|sub|subs|subtitle|uncut|version|ver|vol|chapter|box|anime|mini|gekijou|theater)$`)
var titleGapLatinAliasTokenPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9'.’\-]*$`)
var titleGapEmbeddedSeasonTokenPattern = regexp.MustCompile(`(?i)\b(?:season|saison|part|cour)\s*\d{1,2}\b|\bs\d{1,2}\b`)

func (p *Parser) inferTitleGapSingleSlashAlias(tokens []model.Token, title string, titleEnd int, episodeIdx int, episodeRangeIdx int) []string {
	if len(tokens) == 0 || titleEnd < 0 || titleEnd >= len(tokens) {
		return nil
	}
	end := len(tokens)
	if episodeRangeIdx >= 0 && episodeRangeIdx < end {
		end = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < end {
		end = episodeIdx
	}
	if titleEnd >= end {
		return nil
	}
	slashIdx := -1
	for i := titleEnd; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "/" || t == "／" {
			if slashIdx >= 0 {
				// multi-slash chains are handled by existing alias chain path.
				return nil
			}
			slashIdx = i
		}
	}
	if slashIdx < 0 || slashIdx+1 >= end {
		return nil
	}
	if !hasSingleSlashBridgeSignal(tokens, titleEnd, end, slashIdx) {
		return nil
	}

	rightRaw := extractTitleFromSpanAllowRemoved(tokens, slashIdx+1, end, false)
	right := cleanAliasToken(rightRaw)
	if stripped := stripExplicitSeasonSuffix(right); stripped != "" {
		right = stripped
	}
	right = p.normalizeAliasCandidateForOutput(right)
	right = strings.TrimSpace(right)
	if right == "" || !p.isValidAliasToken(right) {
		return nil
	}
	titleKey := aliasSemanticKey(title)
	rightKey := aliasSemanticKey(right)
	if rightKey == "" || (titleKey != "" && rightKey == titleKey) {
		return nil
	}
	return []string{right}
}

func (p *Parser) inferTitleGapDashAlias(tokens []model.Token, title string, titleEnd int, episodeIdx int, episodeRangeIdx int) []string {
	if len(tokens) == 0 || titleEnd < 0 || titleEnd >= len(tokens) {
		return nil
	}
	end := len(tokens)
	if episodeRangeIdx >= 0 && episodeRangeIdx < end {
		end = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < end {
		end = episodeIdx
	}
	if titleEnd >= end {
		return nil
	}
	if !hasDashAliasBridgeSignal(tokens, titleEnd, end) {
		return nil
	}

	dashIdx := make([]int, 0, 3)
	for i := titleEnd; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		if isDashDelimiterToken(tokens[i].Text) {
			if isInlineAliasWordDash(tokens, i, titleEnd, end) {
				continue
			}
			dashIdx = append(dashIdx, i)
		}
	}
	if len(dashIdx) == 0 {
		return nil
	}

	segments := make([][2]int, 0, len(dashIdx))
	if len(dashIdx) == 1 {
		segments = append(segments, [2]int{dashIdx[0] + 1, end})
	} else {
		for i := 0; i+1 < len(dashIdx); i++ {
			segments = append(segments, [2]int{dashIdx[i] + 1, dashIdx[i+1]})
		}
	}

	titleKey := aliasSemanticKey(title)
	seen := map[string]bool{}
	out := make([]string, 0, 2)
	for _, seg := range segments {
		start, stop := seg[0], seg[1]
		if start < 0 {
			start = 0
		}
		if stop > end {
			stop = end
		}
		if start >= stop {
			continue
		}
		candidate := cleanAliasToken(extractTitleFromSpanAllowRemoved(tokens, start, stop, false))
		if stripped := stripExplicitSeasonSuffix(candidate); stripped != "" {
			candidate = stripped
		}
		candidate = p.normalizeAliasCandidateForOutput(candidate)
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || !p.isValidAliasToken(candidate) {
			continue
		}
		if !isBilingualDashAliasCandidate(title, candidate) {
			continue
		}
		key := aliasSemanticKey(candidate)
		if key == "" || key == titleKey || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p *Parser) inferTitleGapBilingualRunAlias(tokens []model.Token, title string, titleEnd int, episodeIdx int, episodeRangeIdx int) []string {
	if len(tokens) == 0 || titleEnd < 0 || titleEnd >= len(tokens) {
		return nil
	}
	// Conservative path: CJK primary title with latin run alias in title-gap.
	if !hasCJK(title) || hasLatin(title) {
		return nil
	}
	end := len(tokens)
	if episodeRangeIdx >= 0 && episodeRangeIdx < end {
		end = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < end {
		end = episodeIdx
	}
	if titleEnd >= end || !hasDashAliasBridgeSignal(tokens, titleEnd, end) {
		return nil
	}
	for i := titleEnd; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "/" || txt == "／" {
			// slash chains are handled by inferTitleGapSingleSlashAlias.
			return nil
		}
	}

	bestStart := -1
	bestEnd := -1
	for i := titleEnd; i < end; i++ {
		if tokens[i].Removed || !isTitleGapLatinAliasToken(tokens[i].Text) {
			continue
		}
		start := i
		for i < end && !tokens[i].Removed && isTitleGapLatinAliasToken(tokens[i].Text) {
			i++
		}
		stop := i
		if stop-start >= 2 && (bestStart < 0 || stop-start > bestEnd-bestStart) {
			bestStart = start
			bestEnd = stop
		}
	}
	if bestStart < 0 || bestEnd <= bestStart {
		return nil
	}

	cand := cleanAliasToken(extractTitleFromSpanAllowRemoved(tokens, bestStart, bestEnd, false))
	if stripped := stripExplicitSeasonSuffix(cand); stripped != "" {
		cand = stripped
	}
	cand = p.normalizeAliasCandidateForOutput(cand)
	cand = strings.TrimSpace(cand)
	if cand == "" || !p.isValidAliasToken(cand) {
		return nil
	}
	if !hasLatin(cand) || hasCJK(cand) {
		return nil
	}
	titleKey := aliasSemanticKey(title)
	candKey := aliasSemanticKey(cand)
	if candKey == "" || (titleKey != "" && candKey == titleKey) {
		return nil
	}
	return []string{cand}
}

func (p *Parser) promoteInlineBracketContinuationIntoTitle(tokens []model.Token, title string, aliases []string, titleEnd int, episodeIdx int, episodeRangeIdx int, attrs []model.Attribute) (string, []string) {
	title = strings.TrimSpace(title)
	if len(tokens) == 0 || title == "" || titleEnd < 0 || titleEnd >= len(tokens) {
		return title, aliases
	}
	// Conservative scope: latin primary title with inline bracket phrase + tail word run.
	if !hasLatin(title) || hasCJK(title) {
		return title, aliases
	}

	end := len(tokens)
	if episodeRangeIdx >= 0 && episodeRangeIdx < end {
		end = episodeRangeIdx
	}
	if episodeIdx >= 0 && episodeIdx < end {
		end = episodeIdx
	}
	if techStart, ok := firstTechTokenIndexFromAttrs(attrs); ok && techStart > titleEnd && techStart < end {
		end = techStart
	}
	if titleEnd >= end {
		return title, aliases
	}

	openIdx := -1
	for i := titleEnd; i < end; i++ {
		if tokens[i].Removed {
			continue
		}
		t := strings.TrimSpace(tokens[i].Text)
		if t == "[" || t == "【" || t == "(" || t == "（" {
			openIdx = i
			break
		}
		if isDashDelimiterToken(t) || t == "/" || t == "／" {
			return title, aliases
		}
	}
	if openIdx < 0 {
		return title, aliases
	}

	open, close, ok := headAliasBracketPair(strings.TrimSpace(tokens[openIdx].Text))
	if !ok {
		return title, aliases
	}
	closeIdx := -1
	depth := 0
	for i := openIdx; i < end; i++ {
		if strings.TrimSpace(tokens[i].Text) == open {
			depth++
			continue
		}
		if strings.TrimSpace(tokens[i].Text) == close {
			if depth > 0 {
				depth--
				if depth == 0 {
					closeIdx = i
					break
				}
			}
		}
	}
	if closeIdx <= openIdx+1 {
		return title, aliases
	}

	inner := cleanAliasToken(extractTitleFromSpanAllowRemoved(tokens, openIdx+1, closeIdx, false))
	inner = strings.Trim(inner, "\"'“”‘’")
	inner = strings.TrimSpace(inner)
	if inner == "" || isLikelyMetaBracketText(inner) || !p.isValidAliasToken(inner) {
		return title, aliases
	}

	tailStart := closeIdx + 1
	for tailStart < end {
		txt := strings.TrimSpace(tokens[tailStart].Text)
		if txt == "" || txt == "." || txt == "。" || txt == "!" || txt == "！" || txt == "?" || txt == "？" {
			tailStart++
			continue
		}
		break
	}
	if tailStart >= end {
		return title, aliases
	}
	tailRaw := cleanAliasToken(extractTitleFromSpanAllowRemoved(tokens, tailStart, end, false))
	tailRaw = strings.TrimSpace(strings.Trim(tailRaw, "-_/;:,+.。!！?？ "))
	if tailRaw == "" || isLikelyMetaBracketText(tailRaw) {
		return title, aliases
	}
	if isSingleSlashSeasonSignalToken(tailRaw) || isSingleSlashBridgeToken(tailRaw) {
		return title, aliases
	}

	merged := sanitizePrimaryTitleForOutput(strings.TrimSpace(title + " " + inner + " " + tailRaw))
	if merged == "" || aliasSemanticKey(merged) == aliasSemanticKey(title) {
		return title, aliases
	}

	aliases = removeAliasBySemanticKey(aliases, aliasSemanticKey(inner))
	return merged, aliases
}

func isDashDelimiterToken(t string) bool {
	switch strings.TrimSpace(t) {
	case "-", "–", "—", "－":
		return true
	default:
		return false
	}
}

func isInlineAliasWordDash(tokens []model.Token, idx int, start int, end int) bool {
	prev := lastVisibleTokenText(tokens, start, idx)
	next := nextVisibleTokenText(tokens, idx+1, end)
	if prev == "" || next == "" {
		return false
	}
	return isTitleGapLatinAliasToken(prev) && isTitleGapLatinAliasToken(next)
}

func hasDashAliasBridgeSignal(tokens []model.Token, start int, end int) bool {
	for i := start; i < end; i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if isSingleSlashSeasonSignalToken(txt) || isSingleSlashBridgeToken(txt) {
			return true
		}
	}
	return false
}

func isTitleGapLatinAliasToken(t string) bool {
	s := strings.TrimSpace(t)
	if s == "" {
		return false
	}
	if isSingleSlashSeasonSignalToken(s) || isDashDelimiterToken(s) {
		return false
	}
	switch s {
	case "[", "]", "(", ")", "（", "）", "【", "】", "{", "}":
		return false
	}
	s = strings.Trim(s, "[](){}【】（）.,;:!?\"'")
	if s == "" {
		return false
	}
	return titleGapLatinAliasTokenPattern.MatchString(s)
}

func isBilingualDashAliasCandidate(title string, candidate string) bool {
	titleHasCJK := hasCJK(title)
	titleHasLatin := hasLatin(title)
	candHasCJK := hasCJK(candidate)
	candHasLatin := hasLatin(candidate)
	if !candHasCJK && !candHasLatin {
		return false
	}
	if candHasCJK && !titleHasCJK {
		return true
	}
	if candHasLatin && !titleHasLatin {
		return true
	}
	if candHasCJK && candHasLatin {
		return true
	}
	return false
}

func hasSingleSlashBridgeSignal(tokens []model.Token, start int, end int, slashIdx int) bool {
	left := lastVisibleTokenText(tokens, start, slashIdx)
	right := nextVisibleTokenText(tokens, slashIdx+1, end)
	if isSingleSlashSeasonSignalToken(left) || isSingleSlashSeasonSignalToken(right) {
		return true
	}
	if isSingleSlashBridgeToken(left) || isSingleSlashBridgeToken(right) {
		return true
	}

	windowStart := slashIdx - 3
	if windowStart < start {
		windowStart = start
	}
	windowEnd := slashIdx + 4
	if windowEnd > end {
		windowEnd = end
	}
	for i := windowStart; i < windowEnd; i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if isSingleSlashSeasonSignalToken(txt) {
			return true
		}
		if isSingleSlashBridgeToken(txt) {
			return true
		}
	}
	return false
}

func isSingleSlashSeasonSignalToken(t string) bool {
	s := strings.TrimSpace(t)
	if s == "" {
		return false
	}
	if singleSlashAliasSeasonSignalPattern.MatchString(s) {
		return true
	}
	if singleSlashAliasSeasonSignalCJKPattern.MatchString(s) {
		return true
	}
	if singleSlashAliasSeasonPhrasePattern.MatchString(s) {
		return true
	}
	if singleSlashAliasEmbeddedSeasonCJKPattern.MatchString(s) {
		return true
	}
	return false
}

func isSingleSlashBridgeToken(t string) bool {
	s := strings.TrimSpace(t)
	if s == "" {
		return false
	}
	if singleSlashAliasBridgeTokenPattern.MatchString(s) {
		return true
	}
	ls := strings.ToLower(s)
	if strings.Contains(ls, "special") || strings.Contains(ls, "dub") || strings.Contains(ls, "audio") {
		return true
	}
	if strings.Contains(ls, "anime") || strings.Contains(ls, "gekijou") || strings.Contains(ls, "theater") {
		return true
	}
	if strings.Contains(s, "特別篇") || strings.Contains(s, "特别篇") || strings.Contains(s, "年齡限制版") || strings.Contains(s, "年龄限制版") || strings.Contains(s, "日配版") || strings.Contains(s, "中文配音") {
		return true
	}
	if strings.Contains(s, "小劇場") || strings.Contains(s, "小剧场") || strings.Contains(s, "劇場") || strings.Contains(s, "剧场") {
		return true
	}
	return false
}

func isAliasChainDelimiterToken(s string) bool {
	switch strings.TrimSpace(s) {
	case "|", "｜", "/", "／":
		return true
	default:
		return false
	}
}

func (p *Parser) inferTailAliasesFromPipes(tokens []model.Token, attrs []model.Attribute, group string) []string {
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
		if !p.isValidAliasToken(inner) {
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

var tvPackKeywordPattern = regexp.MustCompile(`(?i)tv\s*(全集|全话|全話)`)

func detectPackKind(cfg *config.Config, rawTitle string) string {
	lower := strings.ToLower(rawTitle)
	if tvPackKeywordPattern.MatchString(rawTitle) {
		return "TV_PACK"
	}

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

var spKindPattern = regexp.MustCompile(`(?i)^(sp|ova|oad|ona|ex|ncop|nced|pv)(\d{1,3})?$`)
var shortEpisodeNumberPattern = regexp.MustCompile(`^\d{1,3}$`)

func isShortEpisodeNumberToken(raw string) bool {
	txt := strings.TrimSpace(raw)
	if txt == "" {
		return false
	}
	if !shortEpisodeNumberPattern.MatchString(txt) {
		return false
	}
	n, err := strconv.Atoi(txt)
	if err != nil {
		return false
	}
	return n >= 0 && n <= 999
}

func isSPLeadToken(raw string) bool {
	up := strings.ToUpper(strings.TrimSpace(raw))
	switch up {
	case "SP", "SPECIAL", "特别篇", "特別篇", "OVA", "OAD", "ONA":
		return true
	default:
		return false
	}
}

func isSimpleBoundarySep(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "-", ":", "：", "&", "＆", "+", "＋", "/", "／":
		return true
	default:
		return false
	}
}

func isBracketOpen(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "[", "【", "(", "（":
		return true
	default:
		return false
	}
}

func isBracketClose(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "]", "】", ")", "）":
		return true
	default:
		return false
	}
}

func isLikelyDelimiterToken(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "", "-", "_", ".", ":", "：", "&", "＆", "+", "＋", "/", "／", "|", "｜":
		return true
	default:
		return false
	}
}

func isIsolatedInBracketOrParen(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	inBracket := isInBracket(tokens, idx)
	inParen := isInParen(tokens, idx)
	if !inBracket && !inParen {
		return false
	}
	if idx-1 >= 0 {
		prev := strings.TrimSpace(tokens[idx-1].Text)
		if !(isBracketOpen(prev) || isLikelyDelimiterToken(prev)) {
			return false
		}
	}
	if idx+1 < len(tokens) {
		next := strings.TrimSpace(tokens[idx+1].Text)
		if !(isBracketClose(next) || isLikelyDelimiterToken(next)) {
			return false
		}
	}
	return true
}

func shouldAcceptSPKindToken(tokens []model.Token, idx int, kind string, suffix string) bool {
	k := strings.ToUpper(strings.TrimSpace(kind))
	if k != "EX" && k != "OVA" && k != "OAD" && k != "ONA" && k != "SP" {
		return true
	}
	if strings.TrimSpace(suffix) != "" {
		return true
	}
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	if isInBracket(tokens, idx) || isInParen(tokens, idx) {
		if !isIsolatedInBracketOrParen(tokens, idx) {
			return false
		}
		return true
	}
	if k != "EX" {
		// OVA/OAD/ONA/SP without numeric suffix is ambiguous in plain title text.
		// Keep it as title word by default (e.g. "XXX OVA - 01"), and only accept
		// explicit standalone marker forms (bracket/paren handled above) or tail meta form.
		if hasSPTailMetaBoundary(tokens, idx) {
			return true
		}
		return false
	}
	prev := prevAliveToken(tokens, idx-1)
	if prev >= 0 && prev < idx {
		between := ""
		if idx-1 >= 0 {
			between = strings.TrimSpace(tokens[idx-1].Text)
		}
		if isShortEpisodeNumberToken(tokens[prev].Text) && isSimpleBoundarySep(between) {
			return true
		}
		if isSPLeadToken(tokens[prev].Text) && isSimpleBoundarySep(between) {
			return true
		}
	}
	next := nextAliveToken(tokens, idx+1)
	if next >= 0 && next > idx {
		between := ""
		if idx+1 < len(tokens) {
			between = strings.TrimSpace(tokens[idx+1].Text)
		}
		if isShortEpisodeNumberToken(tokens[next].Text) && isSimpleBoundarySep(between) {
			return true
		}
	}
	return false
}

func hasNearbyEpisodeNumberToken(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	if idx+1 < len(tokens) && !tokens[idx+1].Removed && isShortEpisodeNumberToken(tokens[idx+1].Text) {
		return true
	}
	if idx+2 < len(tokens) && !tokens[idx+2].Removed {
		sep := strings.TrimSpace(tokens[idx+1].Text)
		if isSimpleBoundarySep(sep) && isShortEpisodeNumberToken(tokens[idx+2].Text) {
			return true
		}
	}
	if idx-1 >= 0 && !tokens[idx-1].Removed && isShortEpisodeNumberToken(tokens[idx-1].Text) {
		return true
	}
	if idx-2 >= 0 && !tokens[idx-2].Removed {
		sep := strings.TrimSpace(tokens[idx-1].Text)
		if isSimpleBoundarySep(sep) && isShortEpisodeNumberToken(tokens[idx-2].Text) {
			return true
		}
	}
	return false
}

func hasSPTailMetaBoundary(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	// Immediate next alive token starts a meta block: "OVA (2015) [BD ...]"
	next := nextAliveToken(tokens, idx+1)
	if next >= 0 && next < len(tokens) {
		nt := strings.TrimSpace(tokens[next].Text)
		if isBracketOpen(nt) {
			return true
		}
	}
	// Allow one separator before the meta block.
	if idx+2 < len(tokens) {
		sep := strings.TrimSpace(tokens[idx+1].Text)
		open := strings.TrimSpace(tokens[idx+2].Text)
		if isSimpleBoundarySep(sep) && isBracketOpen(open) {
			return true
		}
	}
	return false
}

func detectSPKind(tokens []model.Token) string {
	for i := 0; i < len(tokens); i++ {
		txt := strings.TrimSpace(tokens[i].Text)
		if txt == "" {
			continue
		}
		if tokens[i].Removed {
			if m := spKindPattern.FindStringSubmatch(txt); len(m) >= 2 {
				kind := strings.ToUpper(strings.TrimSpace(m[1]))
				// Removed EX token is often part of channel/source markers (BS-EX/CS-EX1),
				// so we do not infer sp_kind=EX from removed tokens.
				if kind != "EX" {
					return kind
				}
			}
			continue
		}
		if m := regexp.MustCompile(`(?i)^0?\d{1,3}\s*(?:&|＆)\s*(sp|ova|oad|ona|ex|ncop|nced|pv)\b`).FindStringSubmatch(txt); len(m) == 2 {
			return strings.ToUpper(m[1])
		}
		m := spKindPattern.FindStringSubmatch(txt)
		if len(m) >= 2 {
			kind := strings.ToUpper(strings.TrimSpace(m[1]))
			suffix := ""
			if len(m) >= 3 {
				suffix = strings.TrimSpace(m[2])
			}
			if shouldAcceptSPKindToken(tokens, i, kind, suffix) {
				return kind
			}
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
var movieShortIndexPattern = regexp.MustCompile(`(?i)^m0*(\d{1,3})$`)
var movieShortRangePattern = regexp.MustCompile(`(?i)^m0*(\d{1,3})\s*[-~]\s*m?0*(\d{1,3})$`)
var movieRangeTokenPattern = regexp.MustCompile(`(?i)^(?:movie|film)\s*0*(\d{1,3})\s*[-~]\s*0*(\d{1,3})$`)
var movieRangeDigitPattern = regexp.MustCompile(`^0*(\d{1,3})$`)
var moviePartSignalPattern = regexp.MustCompile(`(?i)\b(?:part|day|chapter)\s*[-:：]?\s*0*(\d{1,3})\b|(?:第\s*0*(\d{1,3})\s*部)|(?:前篇|後篇|后篇|完结篇|完結篇)`)
var movieBracketEpisodePattern = regexp.MustCompile(`[\[【(（]\s*0*(\d{1,3})\s*[\]】)）]`)
var rawMovieTagPattern = regexp.MustCompile(`(?i)[\[【(（]\s*(?:movie|film|電影|电影)\s*[\]】)）]`)
var movieDashEpisodePattern = regexp.MustCompile(`(?i)\b(?:movie|film)\s*0*\d{1,3}\s*[-:：]\s*0*\d{1,3}\b`)

func isMovieKeywordText(text string) bool {
	lt := strings.ToLower(strings.TrimSpace(text))
	if lt == "movie" || lt == "themovie" || lt == "eiga" || lt == "gekijouban" || lt == "film" {
		return true
	}
	return strings.Contains(text, "劇場版") || strings.Contains(text, "剧场版") || strings.Contains(text, "映画") || strings.Contains(text, "大电影") || strings.Contains(text, "大電影")
}

func hasMovieContextAround(tokens []model.Token, idx int) bool {
	if idx < 0 || idx >= len(tokens) {
		return false
	}
	start := idx - 3
	if start < 0 {
		start = 0
	}
	end := idx + 3
	if end >= len(tokens) {
		end = len(tokens) - 1
	}
	for i := start; i <= end; i++ {
		if i == idx || tokens[i].Removed {
			continue
		}
		if isMovieKeywordText(strings.TrimSpace(tokens[i].Text)) {
			return true
		}
	}
	return false
}

func isMovieRangeSepToken(text string) bool {
	text = strings.TrimSpace(text)
	return text == "-" || text == "~"
}

func parseMovieRangeDigits(left string, right string) (string, bool) {
	lm := movieRangeDigitPattern.FindStringSubmatch(strings.TrimSpace(left))
	rm := movieRangeDigitPattern.FindStringSubmatch(strings.TrimSpace(right))
	if len(lm) != 2 || len(rm) != 2 {
		return "", false
	}
	ls := strings.TrimLeft(strings.TrimSpace(lm[1]), "0")
	rs := strings.TrimLeft(strings.TrimSpace(rm[1]), "0")
	if ls == "" {
		ls = "0"
	}
	if rs == "" {
		rs = "0"
	}
	if ls == "0" || rs == "0" {
		return "", false
	}
	li, lerr := strconv.Atoi(ls)
	ri, rerr := strconv.Atoi(rs)
	if lerr != nil || rerr != nil {
		return "", false
	}
	if li > ri {
		return "", false
	}
	return ls + "-" + rs, true
}

func detectMovieIndexRangeAndKeyword(tokens []model.Token) (movieIndexRange string, tokenIdx int, hasMovieKeyword bool) {
	for i := 0; i < len(tokens); i++ {
		text := strings.TrimSpace(tokens[i].Text)
		lt := strings.ToLower(text)
		if text == "" {
			continue
		}
		removed := tokens[i].Removed
		if removed && !isMovieKeywordText(text) && lt != "movie" && lt != "film" {
			continue
		}
		if m := movieRangeTokenPattern.FindStringSubmatch(text); len(m) == 3 {
			rng, ok := parseMovieRangeDigits(m[1], m[2])
			if ok {
				return rng, i, true
			}
		}
		if m := movieShortRangePattern.FindStringSubmatch(text); len(m) == 3 {
			if !removed && (hasMovieKeyword || hasMovieContextAround(tokens, i)) {
				rng, ok := parseMovieRangeDigits(m[1], m[2])
				if ok {
					return rng, i, true
				}
			}
		}
		if isMovieKeywordText(text) || lt == "movie" || lt == "film" {
			hasMovieKeyword = true
			j := i + 1
			for j < len(tokens) {
				next := strings.TrimSpace(tokens[j].Text)
				if next == "." || next == "_" || next == "-" {
					j++
					continue
				}
				break
			}
			if j+2 < len(tokens) && isMovieRangeSepToken(tokens[j+1].Text) {
				if rng, ok := parseMovieRangeDigits(tokens[j].Text, tokens[j+2].Text); ok {
					return rng, j, true
				}
			}
		}
	}
	return "", -1, hasMovieKeyword
}

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
		if m := movieShortIndexPattern.FindStringSubmatch(text); len(m) == 2 {
			if hasMovieKeyword || hasMovieContextAround(tokens, i) {
				return strings.TrimLeft(m[1], "0"), i, true
			}
		}
		if lt == "film" {
			hasMovieKeyword = true
			j := i + 1
			for j < len(tokens) {
				next := strings.TrimSpace(tokens[j].Text)
				if next == "." || next == "_" || next == "-" {
					j++
					continue
				}
				break
			}
			if j < len(tokens) {
				if j+2 < len(tokens) && isMovieRangeSepToken(tokens[j+1].Text) {
					if _, ok := parseMovieRangeDigits(tokens[j].Text, tokens[j+2].Text); ok {
						continue
					}
				}
				next := strings.TrimSpace(tokens[j].Text)
				m := movieIndexPattern.FindStringSubmatch(next)
				if len(m) == 2 {
					return strings.TrimLeft(m[1], "0"), j, true
				}
			}
		}
		if isMovieKeywordText(text) {
			hasMovieKeyword = true
			j := i + 1
			for j < len(tokens) {
				next := strings.TrimSpace(tokens[j].Text)
				if next == "." || next == "_" || next == "-" {
					j++
					continue
				}
				break
			}
			if j < len(tokens) {
				if j+2 < len(tokens) && isMovieRangeSepToken(tokens[j+1].Text) {
					if _, ok := parseMovieRangeDigits(tokens[j].Text, tokens[j+2].Text); ok {
						continue
					}
				}
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
