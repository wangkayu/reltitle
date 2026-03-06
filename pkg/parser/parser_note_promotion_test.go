package parser

import (
	"testing"

	"title-parser/pkg/model"
)

func TestPromoteNoteAttributes(t *testing.T) {
	makeRule := func(vals []string, typ model.AttrType, norm string, normFromToken bool) notePromotionRule {
		m := make(map[string]struct{}, len(vals))
		for _, v := range vals {
			m[v] = struct{}{}
		}
		return notePromotionRule{
			values:              m,
			targetType:          typ,
			normalized:          norm,
			normalizedFromToken: normFromToken,
		}
	}
	p := &Parser{
		notePromotionRules: []notePromotionRule{
			makeRule([]string{"web"}, model.AttrSourceType, "WEBRip", false),
			makeRule([]string{"bdrip"}, model.AttrSourceType, "BDRip", false),
			makeRule([]string{"nf"}, model.AttrPlatform, "", true),
			makeRule([]string{"multi"}, model.AttrSubFlag, "MULTI_SUBS", false),
			makeRule([]string{"subs", "sub"}, model.AttrSubFlag, "SUBS", false),
			makeRule([]string{"vostfr"}, model.AttrSubFlag, "", true),
		},
	}
	in := []model.Attribute{
		{Type: model.AttrNote, Normalized: "WEB"},
		{Type: model.AttrNote, Normalized: "BDRIP"},
		{Type: model.AttrNote, Normalized: "NF"},
		{Type: model.AttrNote, Normalized: "MULTI"},
		{Type: model.AttrNote, Normalized: "Subs"},
		{Type: model.AttrNote, Normalized: "VOSTFR"},
		{Type: model.AttrNote, Normalized: "WEEKLY"},
	}

	out := p.promoteNoteAttributes(in)

	assertAttr := func(idx int, typ model.AttrType, norm string) {
		if out[idx].Type != typ || out[idx].Normalized != norm {
			t.Fatalf("idx=%d got=(%s,%q) want=(%s,%q)", idx, out[idx].Type, out[idx].Normalized, typ, norm)
		}
	}

	assertAttr(0, model.AttrSourceType, "WEBRip")
	assertAttr(1, model.AttrSourceType, "BDRip")
	assertAttr(2, model.AttrPlatform, "NETFLIX")
	assertAttr(3, model.AttrSubFlag, "MULTI_SUBS")
	assertAttr(4, model.AttrSubFlag, "SUBS")
	assertAttr(5, model.AttrSubFlag, "VOSTFR")
	// non-promoted value should remain note
	assertAttr(6, model.AttrNote, "WEEKLY")
}
