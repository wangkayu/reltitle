package parser

import (
	"testing"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

func TestParserUnknownOutput_KeepOnlyGroupTitleAlias(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[AI-Raws] BEMANI SYMPHONY CONCERT 2022"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if res.ContentKind != "UNKNOWN" {
		t.Fatalf("content_kind=%q want UNKNOWN", res.ContentKind)
	}
	if res.TemplateID != "TF_UNKNOWN_DEFAULT" {
		t.Fatalf("template_id=%q want TF_UNKNOWN_DEFAULT", res.TemplateID)
	}
	if res.EPSlot != nil {
		t.Fatalf("epslot must be nil for UNKNOWN output, got=%+v", res.EPSlot)
	}
	if res.AudioSlot != nil {
		t.Fatalf("audio_slot must be nil for UNKNOWN output, got=%+v", res.AudioSlot)
	}
	for _, a := range res.Attributes {
		if a.Type != model.AttrGroup && a.Type != model.AttrTitle && a.Type != model.AttrTitleAlias {
			t.Fatalf("unknown output leaked non-core attr: %+v", a)
		}
	}
}

func TestParserUnknownOutput_KeepTitleAlias(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[AngelEcho]Charlotte [Epilogue]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if res.ContentKind != "UNKNOWN" {
		t.Fatalf("content_kind=%q want UNKNOWN", res.ContentKind)
	}
	if got := canonicalGroup(res); got != "AngelEcho" {
		t.Fatalf("group=%q want AngelEcho", got)
	}
	if got := canonicalTitle(res); got != "Charlotte" {
		t.Fatalf("title=%q want Charlotte", got)
	}
	aliases := canonicalAliases(res)
	if len(aliases) != 1 || aliases[0] != "Epilogue" {
		t.Fatalf("aliases=%v want [Epilogue]", aliases)
	}
	for _, a := range res.Attributes {
		if a.Type != model.AttrGroup && a.Type != model.AttrTitle && a.Type != model.AttrTitleAlias {
			t.Fatalf("unknown output leaked non-core attr: %+v", a)
		}
	}
}

