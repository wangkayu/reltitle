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
