package parser

import (
	"testing"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

func TestParserMovieRoute_MixedTVMovie_NoPackKindStillBatch(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[AI-Raws] Sample Franchise TV+Movie 1080p MKV"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	contents := attrValues(res, model.AttrContents)
	if len(contents) == 0 {
		t.Fatalf("expected contents attrs for TV+Movie mixed title")
	}
}

func TestParserMovieRoute_ShortMovieIndexWithContext(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Atlas] Fate strange Fake 劇場版 M1 [1080p][WEB-DL][AAC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "MOVIE" {
		t.Fatalf("content_kind=%q want=%q", got, "MOVIE")
	}
	if got := firstAttrValue(res, model.AttrMovieIndex); got != "1" {
		t.Fatalf("movie_index=%q want=%q", got, "1")
	}
}

func TestParserMovieRoute_ChineseMovieKeywordInTitleStaysMovie(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "【MCE汉化组】[剧场版 少女与战车 最终章][Girls und Panzer Saishuushou][04][冒头映像][Trial Version][简体][1080P][x264 AAC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "MOVIE" {
		t.Fatalf("content_kind=%q want=%q", got, "MOVIE")
	}
}

func TestParserMovieRoute_MoviesRangePlusOVA_StaysBatch(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[SoM] Dragon Ball / DBZ Movies 1-17 + OVAs - Color Corrected [1080p][x264]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
}

func TestParserMovieRoute_DashCodecNumberNotEpisode(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[OneStar][福音战士新剧场版：终 / 福音戰士新劇場版：終(2021)][1080P][WEB][Amazon][HEVC-265 10bit][MKV]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "MOVIE" {
		t.Fatalf("content_kind=%q want=%q", got, "MOVIE")
	}
	if got := firstAttrValue(res, model.AttrEpisode); got != "" {
		t.Fatalf("episode=%q want empty", got)
	}
	if res.EPSlot != nil && res.EPSlot.Primary != nil && res.EPSlot.Primary.Episode != "" {
		t.Fatalf("epslot.primary.episode=%q want empty", res.EPSlot.Primary.Episode)
	}
}
