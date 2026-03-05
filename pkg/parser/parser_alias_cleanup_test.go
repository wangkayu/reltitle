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

func TestParserAliasCleanup_DropsPreviewVersionAlias(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[天月搬運組] 不死不運 / UNDEAD UNLUCK - 01 [NetFilx][WEB-RIP][1080p][AVC AAC][CHT][SRT][MKV](先行版本)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for _, a := range res.Aliases {
		if strings.Contains(a, "先行版") || strings.Contains(a, "先行版本") {
			t.Fatalf("preview tag leaked into alias list: %q", a)
		}
	}
}

func TestParserAliasCleanup_SlashTitleNotMerged(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[VCB-Studio] 柔光魔女股份有限公司 / Kabushikigaisha Magi-Lumière / 株式会社マジルミエ 10-bit 1080p HEVC BDRip [Fin]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if strings.Contains(res.Title, "/") {
		t.Fatalf("slash title should not be merged: %q", res.Title)
	}
	if res.Title != "柔光魔女股份有限公司" {
		t.Fatalf("unexpected title: %q", res.Title)
	}
	if !containsAlias(res.Aliases, "株式会社マジルミエ") || !containsAlias(res.Aliases, "Kabushikigaisha Magi Lumière") {
		t.Fatalf("unexpected aliases: %v", res.Aliases)
	}
}

func TestParserAliasCleanup_DelicosSlashTitleNotMerged(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[ANi] Delicos Nursery / TRUMP 系列 Delico’s Nursery - 08 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if strings.Contains(res.Title, "/") {
		t.Fatalf("slash title should not be merged: %q", res.Title)
	}
	if !containsAlias(res.Aliases, "Delicos Nursery") && !containsAlias(res.Aliases, "TRUMP 系列 Delico’s Nursery") {
		t.Fatalf("expected one clean alias, got: %v", res.Aliases)
	}
}

func containsAlias(aliases []string, want string) bool {
	for _, a := range aliases {
		if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
