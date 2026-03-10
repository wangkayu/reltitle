package parser

import (
	"strings"
	"testing"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

func TestTechSubs_SubEspMarker_ShouldNotEmitSubFormatSUB(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[WZF]One_Punch_Man_(2025)_-_Ep_07~08[HI10p][1920x1080][X264-AAC][WEB][Sub_Esp]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrValueLocal(attrValuesLocal(res, model.AttrSubLang), "SPA") {
		t.Fatalf("expected sub_lang SPA, got=%v", attrValuesLocal(res, model.AttrSubLang))
	}
	if hasAttrValueLocal(attrValuesLocal(res, model.AttrSubFormat), "SUB") {
		t.Fatalf("unexpected sub_format=SUB for Sub_Esp marker, got=%v", attrValuesLocal(res, model.AttrSubFormat))
	}
	if !hasAttrValueLocal(attrValuesLocal(res, model.AttrSubFlag), "soft") {
		t.Fatalf("expected sub_flag soft for Sub_Esp marker, got=%v", attrValuesLocal(res, model.AttrSubFlag))
	}
}

func TestTechSubs_NoSubMarker_ShouldEmitRawLangOnly(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Group] Sample Show - 01 [无字幕][1080p][WEB-DL]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrValueLocal(attrValuesLocal(res, model.AttrSubLang), "RAW") {
		t.Fatalf("expected sub_lang RAW, got=%v", attrValuesLocal(res, model.AttrSubLang))
	}
	if flags := attrValuesLocal(res, model.AttrSubFlag); len(flags) != 0 {
		t.Fatalf("expected no sub_flag in RAW mode, got=%v", flags)
	}
}

func TestTechSubs_MultiSubsOnly_ShouldEmitMultiLang(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Group] Sample Show - 01 [MULTI_SUBS][1080p][WEB-DL]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrValueLocal(attrValuesLocal(res, model.AttrSubLang), "MULTI") {
		t.Fatalf("expected sub_lang MULTI, got=%v", attrValuesLocal(res, model.AttrSubLang))
	}
	if !hasAttrValueLocal(attrValuesLocal(res, model.AttrSubFlag), "soft") {
		t.Fatalf("expected sub_flag soft, got=%v", attrValuesLocal(res, model.AttrSubFlag))
	}
}

func TestTechSubs_ConcreteLangShouldOverrideMultiMarker(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Group] Sample Show - 01 [MULTI_SUBS][简繁内封][1080p][WEB-DL]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrValueLocal(attrValuesLocal(res, model.AttrSubLang), "SC") || !hasAttrValueLocal(attrValuesLocal(res, model.AttrSubLang), "TC") {
		t.Fatalf("expected sub_lang SC/TC, got=%v", attrValuesLocal(res, model.AttrSubLang))
	}
	if hasAttrValueLocal(attrValuesLocal(res, model.AttrSubLang), "MULTI") {
		t.Fatalf("unexpected sub_lang MULTI when concrete languages exist, got=%v", attrValuesLocal(res, model.AttrSubLang))
	}
}

func attrValuesLocal(res *model.ParseResult, t model.AttrType) []string {
	out := make([]string, 0)
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

func hasAttrValueLocal(got []string, want string) bool {
	for _, v := range got {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
