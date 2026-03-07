package parser

import (
	"strings"
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
	if got := canonicalGroup(res); got != "FatCatRAW" {
		t.Fatalf("group=%q want=%q", got, "FatCatRAW")
	}
	for _, a := range canonicalAliases(res) {
		la := strings.ToLower(strings.TrimSpace(a))
		if strings.Contains(a, "节快乐") || strings.Contains(a, "壓制") || strings.Contains(a, "压制") || strings.Contains(la, "fatcatraw") || strings.EqualFold(strings.TrimSpace(a), "BDBOX") {
			t.Fatalf("alias should drop obvious noise token, got=%q aliases=%v", a, canonicalAliases(res))
		}
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
	if res.EPSlot == nil || res.EPSlot.Primary == nil {
		t.Fatalf("expected epslot.primary for explicit WEBRip range case")
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
	if res.EPSlot != nil {
		t.Fatalf("ambiguous batch digits should not keep candidate-only epslot, got=%+v", res.EPSlot)
	}
}

func TestParserBatchSeasonRange_FromSeasonKeywordAndConnector(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[AI-Raws] Tomorrow's Joe あしたのジョー TV Season 1&2 BDRip 1080p MKV"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := firstAttrValue(res, model.AttrSeasonRange); got != "S1-S2" {
		t.Fatalf("season_range=%q want=%q", got, "S1-S2")
	}
	if got := firstAttrValue(res, model.AttrEpisode); got != "" {
		t.Fatalf("episode=%q want empty", got)
	}
	if got := firstAttrValue(res, model.AttrEpisodeList); got != "" {
		t.Fatalf("episode_list=%q want empty", got)
	}
}

func TestParserBatchSeasonRange_FrenchSaisonRange(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[MECHAPOULPE] Solo Leveling Henshū saison 1 & 2"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := firstAttrValue(res, model.AttrSeasonRange); got != "S1-S2" {
		t.Fatalf("season_range=%q want=%q", got, "S1-S2")
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

func TestParserBatchSourceType_FallbackFromPackKindDVDISO(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Dofus.livre.1.Julith.2016.DVDISO.FRENCH-MaitreAnonyme.iso"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := firstAttrValue(res, model.AttrPackKind); got != "dvdiso" {
		t.Fatalf("pack_kind=%q want=%q", got, "dvdiso")
	}
	if got := firstAttrValue(res, model.AttrSourceType); got != "DVD" {
		t.Fatalf("source_type=%q want=%q", got, "DVD")
	}
}

func TestParserBatchTemplateRangePack_WithTech(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[LoliHouse] 2.5-jigen no Ririsa / 2.5次元的诱惑 [01-24 合集][WebRip 1080p HEVC-10bit AAC][简繁内封字幕][Fin]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := res.TemplateID; got != "TF_BATCH_RANGE_PACK_01" {
		t.Fatalf("template_id=%q want=%q", got, "TF_BATCH_RANGE_PACK_01")
	}
	if got := canonicalEpisodeRange(res); got != "1-24" {
		t.Fatalf("episode_range=%q want=%q", got, "1-24")
	}
	if got := firstAttrValue(res, model.AttrPackKind); got != "合集" {
		t.Fatalf("pack_kind=%q want=%q", got, "合集")
	}
}

func TestParserBatchTemplateRangePack_NoTechWithCollectionKeyword(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[LoliHouse] Sample Title [01-12][Collection]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := res.TemplateID; got != "TF_BATCH_RANGE_PACK_02" {
		t.Fatalf("template_id=%q want=%q", got, "TF_BATCH_RANGE_PACK_02")
	}
	if got := canonicalEpisodeRange(res); got != "1-12" {
		t.Fatalf("episode_range=%q want=%q", got, "1-12")
	}
	if got := firstAttrValue(res, model.AttrPackKind); !strings.EqualFold(got, "collection") {
		t.Fatalf("pack_kind=%q want=%q", got, "collection")
	}
}

func TestParserBatchTemplateRangePack_TVQuanJiUsesPackKindNotContents(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[DBD-Raws][2.5次元的诱惑/2.5 Jigen no Ririsa][01-24TV全集][1080P][BDRip][HEVC-10bit][简繁外挂][FLAC][MKV](2.5次元の誘惑)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := canonicalEpisodeRange(res); got != "1-24" {
		t.Fatalf("episode_range=%q want=%q", got, "1-24")
	}
	if got := firstAttrValue(res, model.AttrPackKind); got != "全集" {
		t.Fatalf("pack_kind=%q want=%q", got, "全集")
	}
	if vals := attrValues(res, model.AttrContents); len(vals) > 0 {
		t.Fatalf("contents should be empty for TV全集 batch flag, got=%v", vals)
	}
	for _, a := range canonicalAliases(res) {
		if strings.Contains(a, "01 24TV") || strings.Contains(a, "24TV") {
			t.Fatalf("episode range marker leaked into alias: %q aliases=%v", a, canonicalAliases(res))
		}
	}
}

func TestParserEpisodeCompactEndFlag_NotBatch(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[TestGroup][Sample Title][12END][1080p][WEB-DL][AAC][MKV]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "TV_EP" {
		t.Fatalf("content_kind=%q want=%q", got, "TV_EP")
	}
	if got := canonicalEpisode(res); got != "12" {
		t.Fatalf("episode=%q want=%q", got, "12")
	}
	if got := firstAttrValue(res, model.AttrPackKind); got != "" {
		t.Fatalf("pack_kind=%q want empty", got)
	}
	if got := firstAttrValue(res, model.AttrEpisodeRange); got != "" {
		t.Fatalf("episode_range=%q want empty", got)
	}
	notes := attrValues(res, model.AttrNote)
	hasEndFlag := false
	for _, n := range notes {
		up := strings.ToUpper(strings.TrimSpace(n))
		if up == "END" || up == "FIN" || up == "FINAL" {
			hasEndFlag = true
			break
		}
	}
	if !hasEndFlag {
		t.Fatalf("expected endflag note from compact episode token, notes=%v", notes)
	}
}

func TestParserBatchTemplateRangePack_TVQuanJiWithExtrasStillKeepsRange(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[DBD-Raws][AIR/青空][01-12TV全集+SP+剧场版+特典映像][1080P][BDRip][HEVC-10bit][简繁外挂][FLAC][MKV]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "BATCH" {
		t.Fatalf("content_kind=%q want=%q", got, "BATCH")
	}
	if got := canonicalEpisodeRange(res); got != "1-12" {
		t.Fatalf("episode_range=%q want=%q", got, "1-12")
	}
	if got := firstAttrValue(res, model.AttrPackKind); got != "全集" {
		t.Fatalf("pack_kind=%q want=%q", got, "全集")
	}
}

func TestParserEpisodeList_ConanDoubleEpisodeBracket(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[DBD-Raws][名侦探柯南/Detective Conan/名探偵コナン][804&805][柯南与海老藏 歌舞伎十八番之谜/コナンと海老蔵 歌舞伎十八番ミステリー][1080P][BDRip][HEVC-10bit][FLAC][MKV]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.ContentKind; got != "TV_EP" {
		t.Fatalf("content_kind=%q want=%q", got, "TV_EP")
	}
	if got := canonicalEpisode(res); got != "804" {
		t.Fatalf("episode=%q want=%q", got, "804")
	}
	if got := firstAttrValue(res, model.AttrEpisodeList); got != "804,805" {
		t.Fatalf("episode_list=%q want=%q", got, "804,805")
	}
	if got := firstAttrValue(res, model.AttrEpisodeRange); got != "" {
		t.Fatalf("episode_range=%q want empty", got)
	}
	if res.EPSlot == nil || res.EPSlot.Primary == nil {
		t.Fatalf("epslot.primary should exist")
	}
	if got := strings.ToUpper(strings.TrimSpace(res.EPSlot.Primary.Kind)); got != "EP_LIST" {
		t.Fatalf("epslot.primary.kind=%q want=%q", got, "EP_LIST")
	}
	if got := strings.TrimSpace(res.EPSlot.Primary.EpisodeList); got != "804,805" {
		t.Fatalf("epslot.primary.episode_list=%q want=%q", got, "804,805")
	}
}
