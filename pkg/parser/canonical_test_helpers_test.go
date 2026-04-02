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
	if res == nil {
		return ""
	}
	return strings.TrimSpace(res.CanonicalEpisode())
}

func canonicalEpisodeRange(res *model.ParseResult) string {
	if res == nil {
		return ""
	}
	return strings.TrimSpace(res.CanonicalEpisodeRange())
}

func canonicalSeason(res *model.ParseResult) string {
	if res == nil {
		return ""
	}
	return strings.TrimSpace(res.CanonicalSeason())
}

func canonicalVersion(res *model.ParseResult) string {
	return firstAttrValue(res, model.AttrVersion)
}

func canonicalSPKind(res *model.ParseResult) string {
	if res == nil {
		return ""
	}
	return strings.TrimSpace(res.CanonicalSPKind())
}

func canonicalAliases(res *model.ParseResult) []string {
	return attrValues(res, model.AttrTitleAlias)
}

func canonicalPackKind(res *model.ParseResult) string {
	return firstAttrValue(res, model.AttrPackKind)
}

func canonicalPackProfile(res *model.ParseResult) string {
	return firstAttrValue(res, model.AttrPackProfile)
}
