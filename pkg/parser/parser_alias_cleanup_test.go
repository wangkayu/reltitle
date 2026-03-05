package parser

import (
	"strings"
	"testing"

	"title-parser/pkg/config"
)

func TestParserAliasCleanup_StripsTVAnimePrefixAndTVSeasonTail(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[BDMV] 赛马娘/TVアニメ「ウマ娘 プリティーダービー Season 3」Vol.1 (Umamusume Season3)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if strings.Contains(strings.ToLower(res.Title), "tv アニメ") {
		t.Fatalf("title still contains tv anime prefix: %q", res.Title)
	}
	if strings.Contains(strings.ToLower(res.Title), "vol") {
		t.Fatalf("title still contains vol suffix: %q", res.Title)
	}

	for _, a := range res.Aliases {
		la := strings.ToLower(a)
		if strings.Contains(la, "tv アニメ") || strings.Contains(la, "tv anime") {
			t.Fatalf("alias still contains tv anime prefix: %q", a)
		}
		if strings.Contains(la, "tv - 2") || strings.Contains(la, "тв - 2") {
			t.Fatalf("alias still contains tv season tail: %q", a)
		}
	}
}

func TestParserAliasCleanup_StripsTechTailFromTitle(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Niconeiko Works] Colorful Stage! The Movie A Miku Who Can`t Sing Ma10p FLAC DTS-HDMA"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	lc := strings.ToLower(res.Title)
	if strings.Contains(lc, "ma10p") || strings.Contains(lc, "flac") || strings.Contains(lc, "dts") || strings.Contains(lc, "hdma") {
		t.Fatalf("title still contains tech tail: %q", res.Title)
	}
}
