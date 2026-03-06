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

func TestParserMovieRoute_DayIndexDemotedToMovieIndex(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[GJ.Y] IDOLiSH7 Movie: LIVE 4bit - BEYOND THE PERiOD - Day 1 (CR 1920x1080 AVC AAC MKV)"
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
	if got := firstAttrValue(res, model.AttrMovieIndex); got != "1" {
		t.Fatalf("movie_index=%q want=%q", got, "1")
	}
}

func TestParserMovieRoute_SeasonPartTvEpisodeNotDemoted(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[LoliHouse] 关于我转生变成史莱姆这档事 第二季 第2部分 / Tensei Shitara Slime Datta Ken 2nd Season Part 2 - 41 [WebRip 1080p HEVC-10bit AAC][简繁英内封]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got == "MOVIE" {
		t.Fatalf("content_kind=%q should not be MOVIE", got)
	}
	if got := firstAttrValue(res, model.AttrEpisode); got != "41" {
		t.Fatalf("episode=%q want=%q", got, "41")
	}
}

func TestParserMovieRoute_OVAWithYearTech_PrefersSpecial(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Blasphemboys] Sanzoku Diary / Bandit's Diary / 山賊ダイアリー OVA (2015) [DVD AVC AC3]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "SPECIAL" {
		t.Fatalf("content_kind=%q want=%q", got, "SPECIAL")
	}
	if got := firstAttrValue(res, model.AttrSPKind); got != "OVA" {
		t.Fatalf("sp_kind=%q want=%q", got, "OVA")
	}
}

func TestParserMovieRoute_MovieYearTech_NoSpKindStillMovie(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Colorful (2010) MULTi 1080p WEB x265 Opus -Tsundere-Raws (VF, FRENCH, VOSTFR, Colorful (Movie))"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "MOVIE" {
		t.Fatalf("content_kind=%q want=%q", got, "MOVIE")
	}
}
