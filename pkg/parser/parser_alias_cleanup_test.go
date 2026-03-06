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

func TestParserAliasCleanup_BracketTitleFallbackWhenMainTitleEmpty(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "【喵萌Production】★07月新番★[LoveLive! 超級明星!! 2期 / Love Live! Superstar!! S2][04][WebRip][1080p][繁日雙語][招募翻譯]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if strings.TrimSpace(res.Title) == "" {
		t.Fatalf("title should not be empty, aliases=%v", res.Aliases)
	}
	if strings.Contains(res.Title, "/") {
		t.Fatalf("title should not keep slash-merged aliases: %q", res.Title)
	}
	if !containsAlias(res.Aliases, "Love Live! Superstar!!") && !containsAlias(res.Aliases, "Love Live! Superstar!! S2") {
		t.Fatalf("expected alias from bracket title split, got=%v", res.Aliases)
	}
}

func TestParserGroupRouteOverride_OnePaceFixedTitle(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[One Pace][23-41] Syrup Village [1080p]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if res.Group != "One Pace" {
		t.Fatalf("group=%q want=%q", res.Group, "One Pace")
	}
	if res.Title != "One Piece" {
		t.Fatalf("title=%q want=%q", res.Title, "One Piece")
	}
	if len(res.Aliases) != 0 {
		t.Fatalf("aliases should be empty, got=%v", res.Aliases)
	}
	if res.EpisodeRange != "23-41" {
		t.Fatalf("episode_range=%q want=%q", res.EpisodeRange, "23-41")
	}
	if res.TemplateID != "TF_BATCH_RANGE_ONEPACE_01" {
		t.Fatalf("template_id=%q want=%q", res.TemplateID, "TF_BATCH_RANGE_ONEPACE_01")
	}
}

func TestParserEpisodePriority_BracketTitleLeadingNumberSweetSub(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[SweetSub][16 位元的輝煌 Another Layer][16bit Sensation: Another Layer][01][WebRip][1080P][AVC 8bit][繁日雙語][v2]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if res.Episode != "1" {
		t.Fatalf("episode=%q want=%q", res.Episode, "1")
	}
	if strings.TrimSpace(res.Title) == "" {
		t.Fatalf("title should not be empty")
	}
}

func TestParserEpisodePriority_BracketTitleLeadingNumberCCAnime(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[c.c動漫][10月新番][86-不存在的戰區][12][BIG5][1080P][MP4]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if res.Episode != "12" {
		t.Fatalf("episode=%q want=%q", res.Episode, "12")
	}
	if !strings.Contains(res.Title, "不存在的戰區") {
		t.Fatalf("title=%q should contain 86 title body", res.Title)
	}
}

func TestParserNoiseWrapper_UnwrapTransportNestedBracket(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[搬運[ANi] You and I Are Polar Opposites / 相反的你和我 - 01 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if strings.TrimSpace(res.Title) == "" {
		t.Fatalf("title should not be empty")
	}
	if !strings.EqualFold(strings.TrimSpace(res.Group), "ANi") {
		t.Fatalf("group=%q want=%q", res.Group, "ANi")
	}
}

func TestParserSingleVersionToken_Extracted(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[v1]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if res.Version != "v1" {
		t.Fatalf("version=%q want=%q", res.Version, "v1")
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

func TestNormalizeAliasCandidateForOutput_StripsInlineLangMetaWords(t *testing.T) {
	p := &Parser{
		aliasInlineStripWords: map[string]struct{}{
			"vostfr": {},
			"vosten": {},
		},
	}
	in := "Maria Watches Over Us Printemps VOSTFR VOSTEN マリア様がみてる ～春～"
	got := p.normalizeAliasCandidateForOutput(in)
	if strings.Contains(strings.ToLower(got), "vostfr") || strings.Contains(strings.ToLower(got), "vosten") {
		t.Fatalf("inline lang markers should be stripped, got=%q", got)
	}
	if !strings.Contains(got, "Maria Watches Over Us Printemps") {
		t.Fatalf("core alias text should be kept, got=%q", got)
	}
	if !strings.Contains(got, "マリア様がみてる") {
		t.Fatalf("japanese alias text should be kept, got=%q", got)
	}
}
