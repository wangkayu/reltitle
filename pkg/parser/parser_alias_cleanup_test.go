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
	version := canonicalVersion(res)
	if version != "v1" {
		t.Fatalf("version=%q want=%q", version, "v1")
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
