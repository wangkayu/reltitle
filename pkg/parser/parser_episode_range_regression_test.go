package parser

import (
	"testing"

	"title-parser/pkg/config"
)

func TestParserEpisodeRange_PlainDashAfterTitleDelimiter(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[shincaps] Crayon Shin-chan - 241-248 (CS-EX2 1440x1080 MPEG2 AAC).ts"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got := canonicalEpisodeRange(res); got != "241-248" {
		t.Fatalf("episode_range=%q want=%q", got, "241-248")
	}
	if got := canonicalTitle(res); got != "Crayon Shin-chan" {
		t.Fatalf("title=%q want=%q", got, "Crayon Shin-chan")
	}
	if got := canonicalGroup(res); got != "shincaps" {
		t.Fatalf("group=%q want=%q", got, "shincaps")
	}
}

func TestParserEpisodeRange_SpacedTildeBatchAfterEpisodePreconsume(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Erai-raws] Prism Rondo - 01 ~ 20 [1080p NF WEBRip HEVC EAC3][MultiSub] [BATCH]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got := canonicalEpisodeRange(res); got != "1-20" {
		t.Fatalf("episode_range=%q want=%q", got, "1-20")
	}
	if got := canonicalEpisode(res); got != "" {
		t.Fatalf("episode=%q want empty when range detected", got)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
}
