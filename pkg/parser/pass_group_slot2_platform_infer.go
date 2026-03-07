package parser

import (
	"regexp"
	"strings"

	"title-parser/pkg/model"
)

var (
	reJibaketaGroup             = regexp.MustCompile(`(?i)^jibaketa`)
	reYueToken                  = regexp.MustCompile(`(?i)(粤语|粵語|cantonese)`)
	reDistributorToken          = regexp.MustCompile(`(?i)(代理商|發行商|发行商)`)
	reLanguageMetaToken         = regexp.MustCompile(`(?i)(雙語|双语|字幕|內封|内封|中字|中文|繁體|简体|簡體|ch[st]|jpn|eng|\\+)`)
	reTrimYueSuffix             = regexp.MustCompile(`(?i)(粤语|粵語|cantonese)`)
	reCompactPlatformCharFilter = regexp.MustCompile(`(?i)[^a-z0-9\\p{Han}]`)
)

func passGroupSlot2PlatformInfer(tokens []model.Token, attrs *[]model.Attribute, ctx *parseContext) {
	if attrs == nil || ctx == nil {
		return
	}
	g := strings.TrimSpace(ctx.Group)
	if g == "" || !reJibaketaGroup.MatchString(strings.ToLower(g)) {
		return
	}

	slot2 := extractSecondBracketSlot(ctx.RawTitle)
	if slot2 == "" {
		return
	}

	slotLower := strings.ToLower(slot2)
	hasYue := reYueToken.MatchString(slot2)

	addAttr := func(t model.AttrType, val string, conf float64, src string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		*attrs = append(*attrs, model.Attribute{
			Type:       t,
			Normalized: val,
			TokenStart: -1,
			TokenEnd:   -1,
			Confidence: conf,
			Source:     src,
		})
	}

	if strings.Contains(slotLower, "tvb") {
		addAttr(model.AttrPlatform, "TVB", 0.9, "group_slot2_platform")
		if hasYue {
			addAttr(model.AttrAudioLang, "YUE", 0.9, "group_slot2_platform")
		}
		return
	}
	if strings.Contains(slotLower, "viutv") {
		addAttr(model.AttrPlatform, "VIUTV", 0.9, "group_slot2_platform")
		if hasYue {
			addAttr(model.AttrAudioLang, "YUE", 0.9, "group_slot2_platform")
		}
		return
	}
	if strings.Contains(slotLower, "hoy") {
		addAttr(model.AttrPlatform, "HOY", 0.9, "group_slot2_platform")
		if hasYue {
			addAttr(model.AttrAudioLang, "YUE", 0.9, "group_slot2_platform")
		}
		return
	}
	if strings.Contains(slotLower, "rthk") {
		addAttr(model.AttrPlatform, "RTHK", 0.9, "group_slot2_platform")
		if hasYue {
			addAttr(model.AttrAudioLang, "YUE", 0.9, "group_slot2_platform")
		}
		return
	}

	if hasYue {
		addAttr(model.AttrAudioLang, "YUE", 0.85, "group_slot2_platform")
	}
	if reDistributorToken.MatchString(slot2) {
		addAttr(model.AttrNote, "CHANNEL_DISTRIBUTOR", 0.75, "group_slot2_platform")
		return
	}
	if !hasYue {
		return
	}
	if reLanguageMetaToken.MatchString(slot2) {
		return
	}

	compact := strings.TrimSpace(reTrimYueSuffix.ReplaceAllString(slot2, ""))
	compact = strings.TrimSpace(reCompactPlatformCharFilter.ReplaceAllString(compact, ""))
	if compact == "" {
		return
	}
	addAttr(model.AttrPlatform, strings.ToUpper(compact), 0.6, "group_slot2_platform_infer_unknown")
}

func extractSecondBracketSlot(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	ms := bracketSegmentPattern.FindAllStringSubmatch(raw, -1)
	if len(ms) < 2 {
		return ""
	}
	return strings.TrimSpace(ms[1][1])
}

