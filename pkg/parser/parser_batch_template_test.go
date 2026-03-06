package parser

import (
	"testing"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

func TestParserBatchSourceType_BDRipPack(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "(五一劳动节快乐)[肥猫压制][FatCatRAW][假面骑士Stronger][BDBOX][BDRIP][1080P]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := firstAttrValue(res, model.AttrSourceType); got != "BDRip" {
		t.Fatalf("source_type=%q want=%q", got, "BDRip")
	}
	if got := firstAttrValue(res, model.AttrPackKind); got != "bdrip" {
		t.Fatalf("pack_kind=%q want=%q", got, "bdrip")
	}
}

func TestParserBatchSourceType_WEBRipRange(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "29-sai Dokushin Chuuken Boukensha no Nichijou | An Adventurer`s Daily Grind at Age 29 | Повседневная жизнь одинокого 29-летнего авантюриста [2026] [01 of 12] [WEBRip] [1080p] [RUS + JAP]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := firstAttrValue(res, model.AttrSourceType); got != "WEBRip" {
		t.Fatalf("source_type=%q want=%q", got, "WEBRip")
	}
	if got := canonicalEpisodeRange(res); got != "1-1" {
		t.Fatalf("episode_range=%q want=%q", got, "1-1")
	}
}

func TestParserBatchSourceType_DVDRipPack(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "City Hunter 1-3 and 91 and films dvdrip (JPN audio ENG sub)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := firstAttrValue(res, model.AttrSourceType); got != "DVDRip" {
		t.Fatalf("source_type=%q want=%q", got, "DVDRip")
	}
	if got := firstAttrValue(res, model.AttrPackKind); got != "dvdrip" {
		t.Fatalf("pack_kind=%q want=%q", got, "dvdrip")
	}
}

func TestParserBatchMovieKeyword_TVMoviePackStaysBatch(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[AI-Raws] SPACE WARRIOR BALDIOS 宇宙戦士バルディオス TV+Movie BDRip 1080p MKV"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	contents := attrValues(res, model.AttrContents)
	if len(contents) == 0 {
		t.Fatalf("expected contents attrs for TV+Movie pack")
	}
}
