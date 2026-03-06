package parser

import (
	"strings"

	"title-parser/pkg/model"
)

func attrValues(res *model.ParseResult, t model.AttrType) []string {
	if res == nil {
		return nil
	}
	out := make([]string, 0, 4)
	for _, a := range res.Attributes {
		if a.Type != t {
			continue
		}
		v := strings.TrimSpace(a.Normalized)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func firstAttrValue(res *model.ParseResult, t model.AttrType) string {
	for _, v := range attrValues(res, t) {
		return v
	}
	return ""
}

func canonicalTitle(res *model.ParseResult) string {
	return firstAttrValue(res, model.AttrTitle)
}

func canonicalGroup(res *model.ParseResult) string {
	return firstAttrValue(res, model.AttrGroup)
}

func canonicalEpisode(res *model.ParseResult) string {
	if res != nil && res.EPSlot != nil && res.EPSlot.Primary != nil {
		v := strings.TrimSpace(res.EPSlot.Primary.Episode)
		if v != "" {
			return v
		}
	}
	return firstAttrValue(res, model.AttrEpisode)
}

func canonicalEpisodeRange(res *model.ParseResult) string {
	if res != nil && res.EPSlot != nil && res.EPSlot.Primary != nil {
		v := strings.TrimSpace(res.EPSlot.Primary.EpisodeRange)
		if v != "" {
			return v
		}
	}
	return firstAttrValue(res, model.AttrEpisodeRange)
}

func canonicalVersion(res *model.ParseResult) string {
	return firstAttrValue(res, model.AttrVersion)
}

func canonicalSPKind(res *model.ParseResult) string {
	if res != nil && res.EPSlot != nil && res.EPSlot.Primary != nil {
		v := strings.TrimSpace(res.EPSlot.Primary.SPKind)
		if v != "" {
			return v
		}
	}
	return firstAttrValue(res, model.AttrSPKind)
}

func canonicalAliases(res *model.ParseResult) []string {
	return attrValues(res, model.AttrTitleAlias)
}
