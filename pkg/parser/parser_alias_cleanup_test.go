package parser

import (
	"strings"
	"testing"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
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
	title := canonicalTitle(res)

	if strings.Contains(strings.ToLower(title), "tv アニメ") {
		t.Fatalf("title still contains tv anime prefix: %q", title)
	}
	if strings.Contains(strings.ToLower(title), "vol") {
		t.Fatalf("title still contains vol suffix: %q", title)
	}

	for _, a := range canonicalAliases(res) {
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
	title := canonicalTitle(res)

	lc := strings.ToLower(title)
	if strings.Contains(lc, "ma10p") || strings.Contains(lc, "flac") || strings.Contains(lc, "dts") || strings.Contains(lc, "hdma") {
		t.Fatalf("title still contains tech tail: %q", title)
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

	for _, a := range canonicalAliases(res) {
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
	title := canonicalTitle(res)
	aliases := canonicalAliases(res)

	if strings.Contains(title, "/") {
		t.Fatalf("slash title should not be merged: %q", title)
	}
	if title != "柔光魔女股份有限公司" {
		t.Fatalf("unexpected title: %q", title)
	}
	if !containsAlias(aliases, "株式会社マジルミエ") || !containsAlias(aliases, "Kabushikigaisha Magi Lumière") {
		t.Fatalf("unexpected aliases: %v", aliases)
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
	title := canonicalTitle(res)
	aliases := canonicalAliases(res)

	if strings.Contains(title, "/") {
		t.Fatalf("slash title should not be merged: %q", title)
	}
	if !containsAlias(aliases, "Delicos Nursery") && !containsAlias(aliases, "TRUMP 系列 Delico’s Nursery") {
		t.Fatalf("expected one clean alias, got: %v", aliases)
	}
}

func TestParserAliasCleanup_DropsResolutionMetaAliases(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []string{
		"(WHIM) Deltora Quest (2007) [480p JDVD] [H265] [Opus 2.0]",
		"[AI-Raws] 羅小黒戦記 ぼくが選ぶ未来 / 罗小黑战记 / The Legend of Hei [4K Reproduction] [MKV]",
		"[Moozzi2] Girls Band Cry [ 4K Ver. ]",
		"[Techmod Raws] Get Ride AMDriver (J-DVD RAW)",
		"[芝士动物朋友] 盒中之海 [B-WebDl][1716p(4K)+6ch][AVC+EAC3][简英内嵌]",
		"【合集】[bilibili]处刑少女的生存之道／処刑少女の生きる道[第一季—全][WebRip_1080P_AVC-HiL5+HEVC_MaL5_AAC_MKV][SRT-简繁内挂]",
	}
	for _, raw := range cases {
		res, err := p.Parse(raw)
		if err != nil {
			t.Fatalf("parse failed for %q: %v", raw, err)
		}
		for _, a := range canonicalAliases(res) {
			la := strings.ToLower(strings.TrimSpace(a))
			if strings.Contains(la, "4k") ||
				strings.Contains(la, "480p") ||
				strings.Contains(la, "fhd") ||
				strings.Contains(la, "srt") ||
				strings.Contains(la, "webdl") ||
				strings.Contains(la, "webrip") ||
				strings.Contains(la, "j-dvd") ||
				strings.Contains(la, "dvd raw") {
				t.Fatalf("meta alias leaked for %q: %v", raw, canonicalAliases(res))
			}
		}
	}
}

func TestParserSpecial_NCOPRouteAndTitleCleanup(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Happy World! NCOP DVD"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if res.ContentKind != "SPECIAL" {
		t.Fatalf("content_kind=%q want=SPECIAL", res.ContentKind)
	}
	if res.TemplateID != "TF_TV_SP_ONLY_04" {
		t.Fatalf("template_id=%q want=TF_TV_SP_ONLY_04", res.TemplateID)
	}
	if got := canonicalSPKind(res); got != "NCOP" {
		t.Fatalf("sp_kind=%q want=NCOP", got)
	}
	if got := canonicalTitle(res); strings.Contains(strings.ToUpper(got), "NCOP") {
		t.Fatalf("title should strip NCOP suffix, got=%q", got)
	}
}

func TestParserSpecial_NCOPNCEDNotAsTailGroup(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[sandoe41] Death March kara Hajimaru Isekai Kyousoukyoku Season One NCOP-NCED"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalGroup(res); got != "sandoe41" {
		t.Fatalf("group=%q want=sandoe41", got)
	}
	if got := canonicalSPKind(res); got != "NCOP" {
		t.Fatalf("sp_kind=%q want=NCOP", got)
	}
	if got := firstAttrValue(res, model.AttrTailGroup); got != "" {
		t.Fatalf("tail_group should be empty, got=%q", got)
	}
}

func TestParserSpecial_CMNotDetectedFromLengthUnit(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Lilith-Raws] 無論何時我們的戀情都是10厘米。 / Itsudatte Bokura no Koi wa 10 cm Datta [01-06][Baha][WEB-DL][1080p][AVC AAC][CHT][MP4]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalSPKind(res); got == "CM" {
		t.Fatalf("cm length unit must not be detected as sp_kind, raw=%q", raw)
	}
}

func TestParserLexicon_CommaLanguageTokensAreConsumed(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "A Couple of Cuckoos S02E07-09 MULTi 1080p WEB x264 AAC -Tsundere-Raws (CR) (VF, FRENCH, VOSTFR, Kakkou no Iinazuke Season 2)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	aliases := canonicalAliases(res)
	for _, a := range aliases {
		la := strings.ToLower(a)
		if strings.Contains(la, "vf,") || strings.Contains(la, "french,") || strings.Contains(la, "vostfr,") {
			t.Fatalf("comma language token leaked into alias: %v", aliases)
		}
	}
	if !hasAttrValue(attrValues(res, model.AttrSubFlag), "VOSTFR") {
		t.Fatalf("expected sub_flag=VOSTFR, attrs=%v", attrValues(res, model.AttrSubFlag))
	}
}

func TestParserTailParenAlias_CommaSeparatedListSplit(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Dragon Raja -The Blazing Dawn- (Japanese ver.) S02E01 VOSTFR 1080p WEB x264 AAC -Tsundere-Raws (CR) (Long Zu II: Daowangzhe Zhi Tong,Dragon Raja II: The Mourner's Eyes)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	aliases := canonicalAliases(res)
	if len(aliases) < 2 {
		t.Fatalf("expected split aliases, got=%v", aliases)
	}
	foundLongZu := false
	foundDragonRaja := false
	for _, a := range aliases {
		la := strings.ToLower(a)
		if strings.Contains(la, "long zu ii") {
			foundLongZu = true
		}
		if strings.Contains(la, "dragon raja ii") {
			foundDragonRaja = true
		}
		if strings.Contains(a, ",") {
			t.Fatalf("comma list should be split into separate aliases, got=%v", aliases)
		}
	}
	if !foundLongZu || !foundDragonRaja {
		t.Fatalf("missing expected split aliases, got=%v", aliases)
	}
}

func TestParserTailParenAlias_TokensConsumedAfterExtraction(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "A Star Brighter Than the Sun S01E01 MULTi 1080p WEB H.264 E-AC-3 -Tsundere-Raws (AMZN) (VF, FRENCH, VOSTFR, Taiyou yori mo Mabushii Hoshi)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	for _, tok := range res.Tokens {
		if strings.EqualFold(strings.TrimSpace(tok.Text), "yori") && !tok.Removed {
			t.Fatalf("tail paren alias token should be consumed, tokens=%v", res.Tokens)
		}
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioCodec), "EAC3") {
		t.Fatalf("expected audio_codec=EAC3 from dashed token, got=%v", attrValues(res, model.AttrAudioCodec))
	}
	for _, tok := range res.Tokens {
		txt := strings.ToUpper(strings.TrimSpace(tok.Text))
		if (txt == "WEB" || txt == "E" || txt == "AC" || txt == "3") && !tok.Removed {
			t.Fatalf("tech token should be consumed in post-process, token=%q", tok.Text)
		}
	}
}

func TestParserStandaloneWebSource_InferredAndConsumed(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "100 METERS (2025) MULTi 1080p WEB AV1 E-AC-3 -Tsundere-Raws (NF) (VF, FRENCH, VOSTFR, Hyakuemu.)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrSourceType), "WEBRip") {
		t.Fatalf("expected source_type=WEBRip, got=%v", attrValues(res, model.AttrSourceType))
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioCodec), "EAC3") {
		t.Fatalf("expected audio_codec=EAC3, got=%v", attrValues(res, model.AttrAudioCodec))
	}
	for _, tok := range res.Tokens {
		if strings.EqualFold(strings.TrimSpace(tok.Text), "WEB") && !tok.Removed {
			t.Fatalf("standalone WEB token should be consumed, tokens=%v", res.Tokens)
		}
	}
}

func TestParserPreEpisodePipeAliases_AreExtractedAndConsumed(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Ansatsusha de Aru Ore no Status ga Yuusha yori mo Akiraka ni Tsuyoi Nodaga | My Status as an Assassin Obviously Exceeds the Hero's | Мoй стaтус убийцы oчeвиднo прeвoсхoдит гeрoйский [2025] [01 of 12] [WEBRip] [1080p] [RUS + JAP]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	aliases := canonicalAliases(res)
	if len(aliases) < 2 {
		t.Fatalf("expected pipe aliases extracted, got=%v", aliases)
	}
	if !containsAlias(aliases, "My Status as an Assassin Obviously Exceeds the Hero's") {
		t.Fatalf("missing english alias, got=%v", aliases)
	}
	hasNonLatinAlias := false
	for _, a := range aliases {
		for _, r := range a {
			if (r >= 0x0400 && r <= 0x052F) || (r >= 0x2DE0 && r <= 0x2DFF) || (r >= 0xA640 && r <= 0xA69F) {
				hasNonLatinAlias = true
				break
			}
		}
		if hasNonLatinAlias {
			break
		}
	}
	if !hasNonLatinAlias {
		t.Fatalf("missing non-latin alias, got=%v", aliases)
	}
	for _, tok := range res.Tokens {
		if strings.TrimSpace(tok.Text) == "|" && !tok.Removed {
			t.Fatalf("pipe delimiters should be consumed after alias extraction")
		}
	}
}

func TestParserLexicon_RecruitmentAndBonusMetaTokensAreConsumed(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[1080P][DBD-Raws][龙珠GT/Dragon Ball GT][01-12TV全集+SP+特典映像][HEVC-10bit][招募翻譯]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	aliases := canonicalAliases(res)
	for _, a := range aliases {
		if strings.Contains(a, "招募") || strings.Contains(a, "特典映像") {
			t.Fatalf("meta token leaked into alias: %v", aliases)
		}
	}
	if !hasAttrValue(attrValues(res, model.AttrNote), "RECRUITMENT") {
		t.Fatalf("expected note=RECRUITMENT")
	}
	if !hasAttrValue(attrValues(res, model.AttrNote), "BONUS_VIDEO") {
		t.Fatalf("expected note=BONUS_VIDEO")
	}
}

func TestParserEpisodeRange_TVPackSuffixSplit(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[1080P][DBD-Raws][极限女孩/UG*Ultimate Girls/UG☆アルティメットガール][01-12TV全集][HEVC-10bit][简繁外挂][FLAC][MKV]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisodeRange(res); got != "1-12" {
		t.Fatalf("episode_range=%q want=1-12", got)
	}
	if !hasAttrValue(attrValues(res, model.AttrNote), "TV_PACK") {
		t.Fatalf("expected note=TV_PACK")
	}
	if got := canonicalPackKind(res); got != "TV_PACK" {
		t.Fatalf("pack_kind=%q want=TV_PACK", got)
	}
	for _, tok := range res.Tokens {
		if strings.EqualFold(strings.TrimSpace(tok.Text), "12TV") && !tok.Removed {
			t.Fatalf("12TV should be consumed, tokens=%v", res.Tokens)
		}
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
	title := canonicalTitle(res)
	aliases := canonicalAliases(res)

	if strings.TrimSpace(title) == "" {
		t.Fatalf("title should not be empty, aliases=%v", aliases)
	}
	if strings.Contains(title, "/") {
		t.Fatalf("title should not keep slash-merged aliases: %q", title)
	}
	if !containsAlias(aliases, "Love Live! Superstar!!") && !containsAlias(aliases, "Love Live! Superstar!! S2") {
		t.Fatalf("expected alias from bracket title split, got=%v", aliases)
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
	group := canonicalGroup(res)
	title := canonicalTitle(res)
	aliases := canonicalAliases(res)
	episodeRange := canonicalEpisodeRange(res)
	if group != "One Pace" {
		t.Fatalf("group=%q want=%q", group, "One Pace")
	}
	if title != "One Piece" {
		t.Fatalf("title=%q want=%q", title, "One Piece")
	}
	if len(aliases) != 0 {
		t.Fatalf("aliases should be empty, got=%v", aliases)
	}
	if episodeRange != "23-41" {
		t.Fatalf("episode_range=%q want=%q", episodeRange, "23-41")
	}
	if res.TemplateID != "TF_BATCH_RANGE_ONEPACE_01" {
		t.Fatalf("template_id=%q want=%q", res.TemplateID, "TF_BATCH_RANGE_ONEPACE_01")
	}
}

func TestParserGroupRouteOverride_GroupEPRouteWGDL(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[晚街与灯][命运-奇异赝品_Fate strange Fake][09 噩梦与伦敦的黎明一同到来][WebRip][1080P_AVC_AAC][简日双语内嵌]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "9" {
		t.Fatalf("episode=%q want=%q", got, "9")
	}
	if got := canonicalGroup(res); got != "晚街与灯" {
		t.Fatalf("group=%q want=%q", got, "晚街与灯")
	}
	if got := canonicalTitle(res); got == "" {
		t.Fatalf("title should not be empty")
	}
	for _, a := range canonicalAliases(res) {
		if strings.Contains(a, "09 ") || strings.HasPrefix(strings.TrimSpace(a), "09_") {
			t.Fatalf("episode-like alias should be pruned, aliases=%v", canonicalAliases(res))
		}
	}
	if res.ContentKind == "MOVIE" {
		t.Fatalf("content_kind should not remain MOVIE, got=%q", res.ContentKind)
	}
}

func TestParserGroupRouteOverride_GroupEPRouteAtlasWithSP(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[阿特拉斯字幕组][命运-奇异赝品 -黎明低语-_Fate／strange Fake -Whispers of Dawn-][00_SP][简繁英内封PGS][English Dub][Web-DL Remux][1080p HEVC AAC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "0" {
		t.Fatalf("episode=%q want=%q", got, "0")
	}
	if got := canonicalSPKind(res); got != "SP" {
		t.Fatalf("sp_kind=%q want=%q", got, "SP")
	}
	if res.ContentKind == "MOVIE" {
		t.Fatalf("content_kind should not remain MOVIE, got=%q", res.ContentKind)
	}
	for _, a := range canonicalAliases(res) {
		if strings.EqualFold(strings.TrimSpace(a), "00_SP") {
			t.Fatalf("episode-like alias should be pruned, aliases=%v", canonicalAliases(res))
		}
	}
}

func TestParserGroupRouteOverride_GroupEPRouteMarukoNotMovie(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[丸子家族][海螺小姐(Sazae-san)][2712]鲑仔和电线杆叔叔&不要责怪我把自己当女儿&喜欢恐怖电影吗[2023.08.06][GB][1080P][MP4]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalGroup(res); got != "丸子家族" {
		t.Fatalf("group=%q want=%q", got, "丸子家族")
	}
	if got := canonicalEpisode(res); got != "2712" {
		t.Fatalf("episode=%q want=%q", got, "2712")
	}
	if got := res.ContentKind; got != "TV_EP" {
		t.Fatalf("content_kind=%q want=%q", got, "TV_EP")
	}
	if notes := attrValues(res, model.AttrNote); !containsAlias(notes, "鲑仔和电线杆叔叔&不要责怪我把自己当女儿&喜欢恐怖电影吗") {
		t.Fatalf("missing maruko eptitle note, notes=%v", notes)
	}
	if notes := attrValues(res, model.AttrNote); !containsAlias(notes, "2023.08.06") {
		t.Fatalf("missing maruko airdate note, notes=%v", notes)
	}
	if res.EPSlot == nil {
		t.Fatalf("epslot should not be nil")
	}
	for _, c := range res.EPSlot.Candidates {
		if c.Episode == "8" || c.Episode == "6" {
			t.Fatalf("date fragments leaked into epslot candidates: %+v", res.EPSlot.Candidates)
		}
	}
}

func TestParserGroupRouteOverride_FurretarDubPrefixCleanup(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Furretar] 台配 - 入间同学入魔了！ (Mairimashita! Iruma-kun) (Season 1) (Mandarin Chinese Dub) (Matching Soft Subs) [國語][国语][中文配音][中配版] (入間同學入魔了！)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	title := canonicalTitle(res)
	if strings.HasPrefix(title, "台配") || strings.HasPrefix(title, "中台配") {
		t.Fatalf("furretar title prefix should be cleaned, got=%q", title)
	}
	if title != "入间同学入魔了！" {
		t.Fatalf("title=%q want=%q", title, "入间同学入魔了！")
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "CHN") {
		t.Fatalf("expected audio_lang CHN, got=%v", attrValues(res, model.AttrAudioLang))
	}
}

func TestParserGroupRouteOverride_FurretarCleanupNoGlobalDrift(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[TestGroup] 台配 - 入间同学入魔了！ (Season 1)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	title := canonicalTitle(res)
	if !strings.HasPrefix(title, "台配") {
		t.Fatalf("non-furretar title should keep prefix, got=%q", title)
	}
}

func TestParserAliasCleanup_FurretarDropsMovieMetaAlias(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []struct {
		name     string
		raw      string
		notAlias []string
		contents []string
		packKind string
		profile  string
	}{
		{
			name: "[Movie] should not leak",
			raw:  "[Furretar] 台配 - 阿基拉 (AKIRA) (Movie) (Mandarin Chinese Dub) (Matching Soft Subs) [國語][国语][中文配音][中配版]",
			notAlias: []string{
				"Movie",
			},
			contents: []string{"MOVIE"},
			profile:  "movie_only",
		},
		{
			name: "(Movies + Specials) should not leak",
			raw:  "[Furretar] 台配 - 名侦探柯南 (Detective Conan) (Movies + Specials) (Mandarin Chinese Dub) (Matching Soft Subs) [國語][国语][中文配音][中配版] (Case Closed) (Meitantei Conan)",
			notAlias: []string{
				"Movies + Specials",
			},
			contents: []string{"MOVIE", "SPECIAL"},
			packKind: "contents",
			profile:  "movie_special_mix",
		},
		{
			name: "(Season x + Movie) fragment should not leak",
			raw:  "[Furretar] 台配 - 关于我转生变成史莱姆这档事 (Tensei Shitara Slime Datta Ken) (Season 1-3 + Movie) (Mandarin Chinese Dub) (Matching Soft Subs) [國語][国语][中文配音][中配版] (關於我轉生變成史萊姆這檔事)",
			notAlias: []string{
				"3 + Movie",
				"Season 1-3 + Movie",
			},
			contents: []string{"TV", "MOVIE"},
			profile:  "movie_series_mix",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			aliases := canonicalAliases(res)
			for _, ban := range tc.notAlias {
				if containsAlias(aliases, ban) {
					t.Fatalf("forbidden alias leaked: %q aliases=%v", ban, aliases)
				}
			}
			gotContents := attrValues(res, model.AttrContents)
			for _, want := range tc.contents {
				if !hasAttrValue(gotContents, want) {
					t.Fatalf("missing contents marker %q, got=%v", want, gotContents)
				}
			}
			if tc.packKind != "" {
				if got := canonicalPackKind(res); !strings.EqualFold(got, tc.packKind) {
					t.Fatalf("pack_kind=%q want=%q", got, tc.packKind)
				}
			}
			if tc.profile != "" {
				if got := canonicalPackProfile(res); !strings.EqualFold(got, tc.profile) {
					t.Fatalf("pack_profile=%q want=%q", got, tc.profile)
				}
			}
		})
	}
}

func TestParserStableTechSpanCombo_CommaSeparatedCodecBlock(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[CicakRumah] 2200-nen Neko no Kuni Nippon - S01E09 (2200年ねこの国ニッポン) [ABEMA.WEB-DL 1080P AVC, AAC, DUAL][001EC424]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	foundVideo := false
	foundAudio := false
	foundDual := false
	for _, a := range res.Attributes {
		if a.Type == model.AttrVideoCodec && strings.EqualFold(strings.TrimSpace(a.Normalized), "AVC") {
			foundVideo = true
			if a.Source == "template_float_infer" {
				t.Fatalf("video_codec should be resolved before template float infer, got source=%q", a.Source)
			}
		}
		if a.Type == model.AttrAudioCodec && strings.EqualFold(strings.TrimSpace(a.Normalized), "AAC") {
			foundAudio = true
			if a.Source == "template_float_infer" {
				t.Fatalf("audio_codec should be resolved before template float infer, got source=%q", a.Source)
			}
		}
		if a.Type == model.AttrAudioFlag && strings.EqualFold(strings.TrimSpace(a.Normalized), "DUAL_AUDIO") {
			foundDual = true
		}
	}
	if !foundVideo || !foundAudio {
		t.Fatalf("expected AVC/AAC from stable tech span combo, got attrs=%v", res.Attributes)
	}
	if !foundDual {
		t.Fatalf("expected DUAL_AUDIO from stable tech span combo, got attrs=%v", res.Attributes)
	}
}

func TestParserStableTechSpanCombo_GluedCodecBitDepth(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[诸神字幕组][NHK][古罗马街道 穿越时空的旅行 阿庇亚大道 贯穿意大利580公里][1080P HEVC10 MKV]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrVideoCodec), "HEVC") {
		t.Fatalf("expected video_codec=HEVC, got=%v", attrValues(res, model.AttrVideoCodec))
	}
	if !hasAttrValue(attrValues(res, model.AttrBitDepth), "10bit") {
		t.Fatalf("expected bit_depth=10bit, got=%v", attrValues(res, model.AttrBitDepth))
	}
	for _, tok := range res.Tokens {
		if strings.EqualFold(strings.TrimSpace(tok.Text), "HEVC10") && !tok.Removed {
			t.Fatalf("HEVC10 should be consumed, tokens=%v", res.Tokens)
		}
	}
}

func TestParserStableTechSpanCombo_GluedCodecBitDepthWithP60(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[诸神字幕组][NHK纪录片][历史秘话之EVA][2020-04-14][1080P60 HEVC10 MKV]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrVideoCodec), "HEVC") {
		t.Fatalf("expected video_codec=HEVC, got=%v", attrValues(res, model.AttrVideoCodec))
	}
	if !hasAttrValue(attrValues(res, model.AttrBitDepth), "10bit") {
		t.Fatalf("expected bit_depth=10bit, got=%v", attrValues(res, model.AttrBitDepth))
	}
	for _, tok := range res.Tokens {
		if strings.EqualFold(strings.TrimSpace(tok.Text), "HEVC10") && !tok.Removed {
			t.Fatalf("HEVC10 should be consumed, tokens=%v", res.Tokens)
		}
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
	episode := canonicalEpisode(res)
	title := canonicalTitle(res)
	if episode != "1" {
		t.Fatalf("episode=%q want=%q", episode, "1")
	}
	if strings.TrimSpace(title) == "" {
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
	episode := canonicalEpisode(res)
	title := canonicalTitle(res)
	if episode != "12" {
		t.Fatalf("episode=%q want=%q", episode, "12")
	}
	if !strings.Contains(title, "不存在的戰區") {
		t.Fatalf("title=%q should contain 86 title body", title)
	}
}

func TestParserEPSlotPrimary_SxxExxHasPrimary(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "9nine- Ruler's Crown S01E11 VOSTFR 1080p WEB h264 AAC - Yangire-Raws (YUZUKITV)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "11" {
		t.Fatalf("episode=%q want=%q", got, "11")
	}
	if res.EPSlot == nil || res.EPSlot.Primary == nil {
		t.Fatalf("epslot.primary should not be nil for sxxexx title")
	}
	if got := strings.TrimSpace(res.EPSlot.Primary.Source); got != "sxxexx_final" {
		t.Fatalf("epslot.primary.source=%q want=%q", got, "sxxexx_final")
	}
	if got := strings.TrimSpace(res.EPSlot.Primary.Season); got != "1" {
		t.Fatalf("epslot.primary.season=%q want=%q", got, "1")
	}
}

func TestParserEPSlotPrimary_SxxExxE00AsEpisodeZero(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Towa.No.Yuugure.S01E00.VOSTFR.1080p.WEBRiP.AV1-KAF"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "0" {
		t.Fatalf("episode=%q want=%q", got, "0")
	}
	if res.EPSlot == nil || res.EPSlot.Primary == nil {
		t.Fatalf("epslot.primary should not be nil for sxxexx e00 title")
	}
	if got := strings.TrimSpace(res.EPSlot.Primary.Source); got != "sxxexx_final" {
		t.Fatalf("epslot.primary.source=%q want=%q", got, "sxxexx_final")
	}
}

func TestParserEpisodeDashBoundary_BeybladeStyle(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Beyblade X - 70 - Dread Dragon [English Dub][1080p][MW]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "70" {
		t.Fatalf("episode=%q want=%q", got, "70")
	}
}

func TestParserEpisodeDashBoundary_AllowsVersionAndEndFlag(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []struct {
		name string
		raw  string
		ep   string
		ver  string
	}{
		{
			name: "dash_ep_with_version",
			raw:  "[TestGroup] Sample Show - 01 v2 [1080p][WEB-DL]",
			ep:   "1",
			ver:  "v2",
		},
		{
			name: "dash_ep_with_endflag",
			raw:  "[TestGroup] Sample Show - 12 END [1080p][WEB-DL]",
			ep:   "12",
			ver:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := canonicalEpisode(res); got != tc.ep {
				t.Fatalf("episode=%q want=%q", got, tc.ep)
			}
			if got := canonicalVersion(res); got != tc.ver {
				t.Fatalf("version=%q want=%q", got, tc.ver)
			}
		})
	}
}

func TestParserEpisodePriority_StrongDashBeatsEpisodeKeyword(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Fansub] Sample Show Episode 16 Preview - 09 [1080p][WEB]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "9" {
		t.Fatalf("episode=%q want=%q", got, "9")
	}
}

func TestParserEpisodePriority_KeywordEpisodeStillWorks(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Fansub] Sample Show Episode 52 [1080p][WEB]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "52" {
		t.Fatalf("episode=%q want=%q", got, "52")
	}
}

func TestParserEpisodePriority_DashBitDepthTokenNotEpisode(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Fansub] Sample Show - 24 Bit Remaster - 01 [1080p][WEB]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "1" {
		t.Fatalf("episode=%q want=%q", got, "1")
	}
}

func TestParserEpisodeZero_DashZeroStyle(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[ANi] KamitsubakiShi Kensetsuchū / 神椿市建設中。 - 00 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "0" {
		t.Fatalf("episode=%q want=%q", got, "0")
	}
}

func TestParserEpisodeRange_EpisodeKeywordRange(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Furretar] 台配 - BLEACH 千年血战篇 -相克谭- (Season 17 Episode 27-40)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisodeRange(res); got != "27-40" {
		t.Fatalf("episode_range=%q want=%q", got, "27-40")
	}
}

func TestParserEpisodeChineseLeadingVariants(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []struct {
		name string
		raw  string
		ep   string
	}{
		{
			name: "bracket_ch_ep_with_marker",
			raw:  "[AngelEcho]辺境都市の育成者 边境都市的培养者 [第01话②]",
			ep:   "1",
		},
		{
			name: "ch_ep_with_tail_text",
			raw:  "[AngelEcho]とじのみこ四コマ計画 刀使之巫女四格计划 第14话&最终话",
			ep:   "14",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := canonicalEpisode(res); got != tc.ep {
				t.Fatalf("episode=%q want=%q", got, tc.ep)
			}
		})
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
	title := canonicalTitle(res)
	group := canonicalGroup(res)
	if strings.TrimSpace(title) == "" {
		t.Fatalf("title should not be empty")
	}
	if !strings.EqualFold(strings.TrimSpace(group), "ANi") {
		t.Fatalf("group=%q want=%q", group, "ANi")
	}
}

func TestParserSingleVersionToken_UnknownCoreOnly(t *testing.T) {
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
	if res.ContentKind != "UNKNOWN" {
		t.Fatalf("content_kind=%q want UNKNOWN", res.ContentKind)
	}
	for _, a := range res.Attributes {
		if a.Type != model.AttrGroup && a.Type != model.AttrTitle && a.Type != model.AttrTitleAlias {
			t.Fatalf("unknown output leaked non-core attr: %+v", a)
		}
	}
}

func TestTemplateSignalTechStrict_InferPostEPCoreTechFromFloat(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Fansub] Example Show - 09 VOSTFR 720p WEB x.265 AAC -Tsundere-Raws (CR).mkv"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if canonicalEpisode(res) != "9" {
		t.Fatalf("episode=%q want=9", canonicalEpisode(res))
	}
	vcodecs := attrValues(res, model.AttrVideoCodec)
	if len(vcodecs) == 0 {
		t.Fatalf("expected video codec inferred from post-ep float token, attrs=%v", res.Attributes)
	}
	found := false
	for _, v := range vcodecs {
		low := strings.ToLower(v)
		if strings.Contains(low, "x265") || strings.Contains(low, "hevc") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected x265/hevc codec inferred from post-ep float token, got=%v", vcodecs)
	}
}

func TestTemplateSignalTechStrict_DoesNotInferPreTitleTechAsCodec(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Fansub] x.265 Example Show - 09 VOSTFR 720p WEB AAC -Tsundere-Raws (CR).mkv"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if canonicalEpisode(res) != "9" {
		t.Fatalf("episode=%q want=9", canonicalEpisode(res))
	}
	vcodecs := attrValues(res, model.AttrVideoCodec)
	for _, v := range vcodecs {
		if strings.Contains(strings.ToLower(v), "x265") {
			t.Fatalf("pre-title x.265 should not be inferred as video codec, got=%v", vcodecs)
		}
	}
}

func TestLexiconDict_ConfigFirst_WEB1080PAndCNSubFlags(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[TestGroup] Sample Show - 01 [WEB1080P][简繁内封][AAC][MKV]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrValue(attrValues(res, model.AttrSourceType), "WEBRip") {
		t.Fatalf("expected source_type WEBRip, got=%v", attrValues(res, model.AttrSourceType))
	}
	if !hasAttrValue(attrValues(res, model.AttrResolution), "1080p") {
		t.Fatalf("expected resolution 1080p, got=%v", attrValues(res, model.AttrResolution))
	}
	if !hasAttrValue(attrValues(res, model.AttrSubFlag), "内封") {
		t.Fatalf("expected sub_flag 内封, got=%v", attrValues(res, model.AttrSubFlag))
	}
	if !hasAttrValue(attrValues(res, model.AttrSubLang), "CHS") || !hasAttrValue(attrValues(res, model.AttrSubLang), "CHT") {
		t.Fatalf("expected sub_lang CHS/CHT, got=%v", attrValues(res, model.AttrSubLang))
	}
}

func TestLexiconDict_ConfigFirst_VCDRipAndYueSubs(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[TestGroup] Sample Show - 01 [VCDRip][粤语中字][MP4]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrValue(attrValues(res, model.AttrSourceType), "VCDRip") {
		t.Fatalf("expected source_type VCDRip, got=%v", attrValues(res, model.AttrSourceType))
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
		t.Fatalf("expected audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
	}
	if !hasAttrValue(attrValues(res, model.AttrSubFlag), "中字") {
		t.Fatalf("expected sub_flag 中字, got=%v", attrValues(res, model.AttrSubFlag))
	}
}

func TestLexiconDict_ConfigFirst_StandaloneYueAudioLang(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[TestGroup] Sample Show - 01 [粤语][MP4]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
		t.Fatalf("expected audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
	}
}

func TestLexiconDict_ConfigFirst_YuePhraseVariants(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []struct {
		name string
		raw  string
	}{
		{name: "yue_dub_cn", raw: "[TestGroup] Sample Show - 01 [粤语配音][MP4]"},
		{name: "yue_no_sub_cn", raw: "[TestGroup] Sample Show - 01 [粤语无字][MP4]"},
		{name: "yue_version_tw", raw: "[TestGroup] Sample Show - 01 [粵語版][MP4]"},
		{name: "cantonese_dub_en", raw: "[TestGroup] Sample Show - 01 [CantoneseDub][WEB-DL][1080p]"},
		{name: "yue_eng_dual_cn", raw: "[TestGroup] Sample Show - 01 [粤英双语中字][MP4]"},
		{name: "yue_tag_tvb_cn", raw: "[TestGroup] Sample Show - 01 [TVB+發行商粵語][MP4]"},
		{name: "yue_reissue_cn", raw: "[TestGroup] Sample Show - 01 [代理商再版粵語][MP4]"},
		{name: "yue_agency_cn", raw: "[TestGroup] Sample Show - 01 [代理商粵語][MP4]"},
		{name: "yue_audio_track_cn", raw: "[TestGroup] Sample Show - 01 [日语_国语_粤语音轨][BDRip][1080P][MKV]"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
				t.Fatalf("expected audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
			}
		})
	}
}

func TestTagCodeAudioLang_CAUsesYUE(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Erai-raws] Sample Show - 01 (CA) [WEB-DL][1080p]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
		t.Fatalf("expected tag code CA => audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
	}
}

func TestGroupSlot2PlatformInfer_JibaketaTVBYue(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[jibaketa合成&音頻壓制][TVB粵語] Sample Show - 01 [WEB-DL][1080p]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrPlatform), "TVB") {
		t.Fatalf("expected platform TVB, got=%v", attrValues(res, model.AttrPlatform))
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
		t.Fatalf("expected audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
	}
}

func TestGroupRouteOverride_JibaketaStripsSlot2PrefixFromTitle(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[jibaketa合成&音頻壓制][TVB粵語]閃躍吧！星夢☆頻道 / 美妙☆频道 / Kiratto Pri-chan - 01 [粵日雙語+內封繁體中文字幕][WEB 1920x1080 x264 AACx2 SRT TVB CHT]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrValue(attrValues(res, model.AttrPlatform), "TVB") {
		t.Fatalf("expected platform TVB, got=%v", attrValues(res, model.AttrPlatform))
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
		t.Fatalf("expected audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
	}
	title := canonicalTitle(res)
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(title)), "TVB") {
		t.Fatalf("title should strip TVB slot2 prefix, got=%q", title)
	}
	if !strings.Contains(title, "閃躍吧！星夢") {
		t.Fatalf("title core should be kept, got=%q", title)
	}
	if !containsAlias(canonicalAliases(res), "Kiratto Pri-chan") {
		t.Fatalf("expected alias Kiratto Pri-chan, got=%v", canonicalAliases(res))
	}
	for _, tok := range res.Tokens {
		txt := strings.TrimSpace(tok.Text)
		if (txt == "/" || strings.EqualFold(txt, "Kiratto") || strings.EqualFold(txt, "Pri") || strings.EqualFold(txt, "美妙")) && !tok.Removed {
			t.Fatalf("alias-chain residual token should be consumed, token=%q", txt)
		}
	}
}

func TestConsumeAliasChainTokensByFinalAliases_Jibaketa(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)
	raw := "[jibaketa合成&音頻壓制][TVB粵語]閃躍吧！星夢☆頻道 / 美妙☆频道 / Kiratto Pri-chan - 01 [粵日雙語+內封繁體中文字幕][WEB 1920x1080 x264 AACx2 SRT TVB CHT]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	cp := make([]model.Token, len(res.Tokens))
	copy(cp, res.Tokens)
	epIdx := -1
	seasonIdx := -1
	epRangeIdx := -1
	for _, a := range res.Attributes {
		if a.Type == model.AttrEpisode && a.TokenStart >= 0 {
			epIdx = a.TokenStart
		}
		if a.Type == model.AttrSeason && a.TokenStart >= 0 {
			seasonIdx = a.TokenStart
		}
		if a.Type == model.AttrEpisodeRange && a.TokenStart >= 0 {
			epRangeIdx = a.TokenStart
		}
	}
	p.consumeAliasChainTokensByFinalAliases(cp, [2]int{0, 4}, canonicalTitle(res), canonicalAliases(res), epIdx, epRangeIdx, seasonIdx, res.Attributes)
	removedSlash := 0
	for _, tok := range cp {
		if strings.TrimSpace(tok.Text) == "/" && tok.Removed {
			removedSlash++
		}
	}
	if removedSlash == 0 {
		t.Fatalf("expected slash tokens consumed by post cleanup")
	}
}

func TestConsumeAliasChainTokensByFinalAliases_JibaketaBilingualSlashOnly(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)
	raw := "[jibaketa合成&二次壓制][TVB粵語]MIX 第二季~第二個夏天，邁向晴空~ / Mix - Meisei Story 2nd Season - 01 [粵語+內封繁體中文字幕] [BD 1920x1080 x264 AAC SRT TVB YUE]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	epIdx := -1
	if res.EPSlot != nil && res.EPSlot.Primary != nil && res.EPSlot.Primary.TokenStart >= 0 {
		epIdx = res.EPSlot.Primary.TokenStart
	}
	if epIdx < 0 {
		t.Fatalf("expected episode token index")
	}
	for i := 0; i < epIdx && i < len(res.Tokens); i++ {
		if strings.TrimSpace(res.Tokens[i].Text) != "/" {
			continue
		}
		if !res.Tokens[i].Removed {
			t.Fatalf("expected pre-episode bilingual slash consumed, idx=%d", i)
		}
	}
}

func TestGroupSpecificTitleChainDelimiterCleanup_JibaketaViuTVSlash(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)
	raw := "[jibaketa合成&壓制][ViuTV粵語]约定的夢幻島 S2 / Yakusoku no Neverland 2nd Season - 01 [粵日雙語+內封繁體中文雙字幕] (BD 1920x1080 x264 AACx2 PGS+SRT ViuTV CHT)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	epIdx := -1
	if res.EPSlot != nil && res.EPSlot.Primary != nil && res.EPSlot.Primary.TokenStart >= 0 {
		epIdx = res.EPSlot.Primary.TokenStart
	}
	if epIdx < 0 {
		t.Fatalf("expected episode token index")
	}
	for i := 0; i < epIdx && i < len(res.Tokens); i++ {
		if strings.TrimSpace(res.Tokens[i].Text) != "/" {
			continue
		}
		if !res.Tokens[i].Removed {
			t.Fatalf("expected jibaketa bilingual title-chain slash consumed, idx=%d", i)
		}
	}
}

func TestGroupExtract_JibaketaPlusCompoundGroup(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)
	raw := "[jibaketa合成&音頻壓制+部份二次壓制][ViuTV粵語]捉捉！迷你寶 / 奇妙！萌可 第1季 / Catch! Teenieping [粵語+無字幕] (WEB 1920x1080 x264 AAC YUE)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	title := canonicalTitle(res)
	if strings.Contains(title, "jibaketa") || strings.Contains(title, "合成") {
		t.Fatalf("title should not collapse to group payload, got=%q", title)
	}
	groupVals := attrValues(res, model.AttrGroup)
	if len(groupVals) == 0 || !strings.Contains(strings.ToLower(groupVals[0]), "jibaketa") {
		t.Fatalf("expected jibaketa group extracted, got=%v", groupVals)
	}
}

func TestGroupSlot2PlatformInfer_JibaketaDistributorNoPlatform(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[jibaketa合成&音頻壓制][代理商粵語] Sample Show - 01 [WEB-DL][1080p]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if hasAttrValue(attrValues(res, model.AttrPlatform), "代理商粵語") {
		t.Fatalf("distributor slot must not be emitted as platform, got=%v", attrValues(res, model.AttrPlatform))
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
		t.Fatalf("expected audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
	}
}

func TestGroupSlot2PlatformInfer_JibaketaUnknownPlatformInfer(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[jibaketa合成&音頻壓制][MyTV粵語] Sample Show - 01 [WEB-DL][1080p]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrPlatform), "MYTV") {
		t.Fatalf("expected inferred platform MYTV, got=%v", attrValues(res, model.AttrPlatform))
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
		t.Fatalf("expected audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
	}
}

func TestLexiconDict_ConfigFirst_GluedSourceResolutionVariants(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []struct {
		name       string
		raw        string
		wantSource string
		wantRes    string
	}{
		{
			name:       "web720p",
			raw:        "[TestGroup] Sample Show - 01 [WEB720P][AAC][MKV]",
			wantSource: "WEBRip",
			wantRes:    "720p",
		},
		{
			name:       "web2160p",
			raw:        "[TestGroup] Sample Show - 01 [WEB2160P][AAC][MKV]",
			wantSource: "WEBRip",
			wantRes:    "2160p",
		},
		{
			name:       "bd720p",
			raw:        "[TestGroup] Sample Show - 01 [BD720P][AAC][MKV]",
			wantSource: "BD",
			wantRes:    "720p",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if !hasAttrValue(attrValues(res, model.AttrSourceType), tc.wantSource) {
				t.Fatalf("source_type got=%v want=%q", attrValues(res, model.AttrSourceType), tc.wantSource)
			}
			if !hasAttrValue(attrValues(res, model.AttrResolution), tc.wantRes) {
				t.Fatalf("resolution got=%v want=%q", attrValues(res, model.AttrResolution), tc.wantRes)
			}
		})
	}
}

func TestParserTechCombo_BitDepthWithTrailingComma(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Gecko] Kaijyu Sekai Seifuku - S01E47 [ABEMA.WEB-DL 1920x1080 AVC 8bit, AAC 2.0][819BADDB].mkv"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrBitDepth), "8bit") {
		t.Fatalf("expected bit_depth=8bit, got=%v", attrValues(res, model.AttrBitDepth))
	}
}

func TestLexiconDict_ConfigFirst_ITABDCompound(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Mahou Shoujo Madoka Magica S1 ITA-BD x265 10bit"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if canonicalTitle(res) != "Mahou Shoujo Madoka Magica" {
		t.Fatalf("title polluted, got=%q", canonicalTitle(res))
	}
	if !hasAttrValue(attrValues(res, model.AttrPlatformRegion), "ITA") {
		t.Fatalf("expected platform_region=ITA, got=%v", attrValues(res, model.AttrPlatformRegion))
	}
	if !hasAttrValue(attrValues(res, model.AttrSourceType), "BD") {
		t.Fatalf("expected source_type includes BD, got=%v", attrValues(res, model.AttrSourceType))
	}
}

func TestLexiconDict_ConfigFirst_BDBoxAtomicNoAliasPollution(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[VCB-Studio] Some Title [BD-BOX][1080p][x265][10bit]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrSourceType), "BD") {
		t.Fatalf("expected source_type includes BD, got=%v", attrValues(res, model.AttrSourceType))
	}
	for _, a := range canonicalAliases(res) {
		if strings.EqualFold(strings.TrimSpace(a), "BOX") {
			t.Fatalf("bd-box token leaked as alias: %v", canonicalAliases(res))
		}
	}
}

func TestParserAliasCleanup_TrimTailAbsoluteEpisodeWhenSeasonRelativePrimary(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[WF] Sousou no Frieren 2nd Season - 04 [WEBRip 1080p x265 AAC][DUAL][PT-BR] (Frieren: Beyond Journey's End - 32)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := canonicalEpisode(res); got != "4" {
		t.Fatalf("episode=%q want=4", got)
	}
	aliases := canonicalAliases(res)
	if !containsAlias(aliases, "Frieren Beyond Journey's End") {
		t.Fatalf("expected cleaned alias, got=%v", aliases)
	}
	for _, a := range aliases {
		if strings.Contains(a, "32") {
			t.Fatalf("tail absolute episode should be trimmed from alias, got=%v", aliases)
		}
	}
}

func TestJibaketaYueSample_SubtitleAndPlatformCleanup(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[jibaketa合成][代理商粵語]SPY×FAMILY間諜家家酒 / 间谍过家家Season 3 - 09 [粵日雙語+內封繁體中文字幕](WEB 1920x1080 AVC AACx2 SRT MUSE CHT)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioLang), "YUE") {
		t.Fatalf("expected audio_lang YUE, got=%v", attrValues(res, model.AttrAudioLang))
	}
	if !hasAttrValue(attrValues(res, model.AttrSubLang), "CHT") {
		t.Fatalf("expected sub_lang CHT, got=%v", attrValues(res, model.AttrSubLang))
	}
	if hasAttrValue(attrValues(res, model.AttrSubLang), "JPN") {
		t.Fatalf("unexpected sub_lang JPN from 粵日雙語, got=%v", attrValues(res, model.AttrSubLang))
	}
	if !hasAttrValue(attrValues(res, model.AttrPlatform), "MUSE") {
		t.Fatalf("expected platform MUSE, got=%v", attrValues(res, model.AttrPlatform))
	}
	for _, a := range canonicalAliases(res) {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(a)), "season") {
			t.Fatalf("season residue leaked in alias: %v", canonicalAliases(res))
		}
	}
}

func TestGeckoGTETX_ParenAliasBetweenEpisodeAndTech(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Gecko] Kaijyu Sekai Seifuku - S01E47 (かいじゅうせかいせいふく; Kaijuu Sekai Seifuku; Kaijuu Sekai Seifuku (TV)) [ABEMA.WEB-DL 1920x1080 AVC 8bit, AAC 2.0][819BADDB]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if canonicalTitle(res) != "Kaijyu Sekai Seifuku" {
		t.Fatalf("unexpected title=%q", canonicalTitle(res))
	}
	aliases := canonicalAliases(res)
	if !containsAlias(aliases, "かいじゅうせかいせいふく") {
		t.Fatalf("expected kana alias, got=%v", aliases)
	}
	if !containsAlias(aliases, "Kaijuu Sekai Seifuku") {
		t.Fatalf("expected latin alias, got=%v", aliases)
	}
}

func TestUnderscoreSceneStyle_NoRouteNeeded(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[WZF]One_Punch_Man_(2025)_-_Ep_07~08[HI10p][1920x1080][X264-AAC][WEB][Sub_Esp]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if canonicalTitle(res) != "One Punch Man" {
		t.Fatalf("unexpected title=%q", canonicalTitle(res))
	}
	if got := canonicalEpisodeRange(res); got != "7-8" {
		t.Fatalf("episode_range=%q want=7-8", got)
	}
	if !hasAttrValue(attrValues(res, model.AttrSubLang), "SPA") {
		t.Fatalf("expected sub_lang SPA, got=%v", attrValues(res, model.AttrSubLang))
	}
}

func TestLexiconDict_4KHDRComboShouldNotPolluteTitle(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[4K影视屋][4K_HDR][007：无暇赴死No.Time.to.Die.2021][2160P][WEB-DL][杜比全景声][HEVC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if strings.EqualFold(strings.TrimSpace(canonicalTitle(res)), "4K HDR") {
		t.Fatalf("4k_hdr block leaked to title: %q", canonicalTitle(res))
	}
	if !hasAttrValue(attrValues(res, model.AttrResolution), "2160p") {
		t.Fatalf("expected resolution 2160p, got=%v", attrValues(res, model.AttrResolution))
	}
}

func TestLexiconDict_DVTokenToDolbyVisionHDR(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[TestGroup] Sample Show - 01 [DV][WEB-DL][2160p][HEVC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasAttrValue(attrValues(res, model.AttrType("hdr")), "DOLBY_VISION") {
		t.Fatalf("expected hdr DOLBY_VISION, got=%v", attrValues(res, model.AttrType("hdr")))
	}
}

func hasAttrValue(got []string, want string) bool {
	for _, v := range got {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
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

func TestExtractParenContentMarkers(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{in: "Movie", want: []string{"MOVIE"}},
		{in: "Movies + Specials", want: []string{"MOVIE", "SPECIAL"}},
		{in: "Season 1-3 + Movie", want: []string{"TV", "MOVIE"}},
		{in: "3 + Movie", want: []string{"MOVIE"}},
	}
	for _, tc := range cases {
		got := extractParenContentMarkers(tc.in)
		for _, w := range tc.want {
			if !containsAlias(got, w) {
				t.Fatalf("input=%q markers=%v want include=%q", tc.in, got, w)
			}
		}
	}
}
