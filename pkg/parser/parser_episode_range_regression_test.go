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
