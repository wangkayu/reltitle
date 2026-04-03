package parser

import (
	"strings"
	"testing"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

func TestLexiconNewFormat_SeasonVersionCompactS01V2(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "The Fragrant Flower Blooms with Dignity S01v2 MULTi 1080p WEB x264 AAC -Tsundere-Raws (NF)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if got := canonicalSeason(res); got != "1" {
		t.Fatalf("expected season=1 from S01v2, got=%q", got)
	}
	if !hasAttrNormalized(res.Attributes, model.AttrVersion, "V2") {
		t.Fatalf("expected version=V2 from S01v2, attrs=%v", res.Attributes)
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(res.CanonicalTitle())), "s01v2") {
		t.Fatalf("compact season/version token should not stay in title, title=%q", res.CanonicalTitle())
	}
}

func TestLexiconNewFormat_MultiAudioSubDottedBlock(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Shingeki.no.Kyojin.The.Last.Attack.Movie.2024.MULTI.Audio.Sub[FRE][ENG][GER][ITA][SPA][POR][CHI][ARA].1080p.BDRip.CUSTOM.x264.TrueHD.AC3.PCM.EAC3-KuroNeko VOSTFR Attack on Titan: The Last Attack 劇場版 進撃の巨人 完結編 THE LAST ATTACK"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrNormalized(res.Attributes, model.AttrAudioFlag, "MULTI_AUDIO") {
		t.Fatalf("expected audio_flag=MULTI_AUDIO, attrs=%v", res.Attributes)
	}
	if !hasAttrNormalized(res.Attributes, model.AttrSubFlag, "soft") {
		t.Fatalf("expected sub_flag=soft, attrs=%v", res.Attributes)
	}
	for _, lang := range []string{"FRA", "ENG", "DEU", "ITA", "SPA", "POR"} {
		if !hasAttrNormalized(res.Attributes, model.AttrSubLang, lang) {
			t.Fatalf("expected sub_lang=%s from bracket language bundle, attrs=%v", lang, res.Attributes)
		}
	}
	for _, tok := range []string{"CHI", "ARA"} {
		seenRemoved := false
		for _, tk := range res.Tokens {
			if strings.EqualFold(strings.TrimSpace(tk.Text), tok) && tk.Removed {
				seenRemoved = true
				break
			}
		}
		if !seenRemoved {
			t.Fatalf("expected %s token to be consumed as language marker", tok)
		}
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(res.CanonicalTitle())), "audio sub") {
		t.Fatalf("dotted MULTI.Audio.Sub marker should not pollute title, title=%q", res.CanonicalTitle())
	}
}

func TestLexiconNewFormat_SubDotEspanolParen(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[GSF] Hitoribocchi no Marumaru Seikatsu [BD 810p] (sub. español)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrNormalized(res.Attributes, model.AttrSubLang, "SPA") {
		t.Fatalf("expected sub_lang=SPA for sub. español, attrs=%v", res.Attributes)
	}
	if !hasAttrNormalized(res.Attributes, model.AttrSubFlag, "soft") {
		t.Fatalf("expected sub_flag=soft for sub. español, attrs=%v", res.Attributes)
	}
}

func TestLexiconNewFormat_PlatformUNEXTVariants(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Duel Masters LOST: Boukyaku no Taiyou S01E04 1080p U.NEXT WEB-DL AAC2.0 H.264 (Duel Masters LOST ～忘却の太陽～)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrNormalized(res.Attributes, model.AttrPlatform, "UNEXT") {
		t.Fatalf("expected platform=UNEXT for U.NEXT variant, attrs=%v", res.Attributes)
	}
	if sem := canonicalEPSemantics(res); sem == nil {
		t.Fatalf("expected ep semantics for S01E04 title")
	}
}

func TestLexiconNewFormat_VOSTFRSeasonBatchKeepsFrenchSubLang(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "Ave Mujica - The Die is Cast S01 VOSTFR 1080p WEB x264 AAC -Tsundere-Raws (CR) (Multi-Subs, BanG Dream! Ave Mujica)"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrNormalized(res.Attributes, model.AttrSubLang, "FRA") {
		t.Fatalf("expected sub_lang=FRA for VOSTFR marker, attrs=%v", res.Attributes)
	}
	if !hasAttrNormalized(res.Attributes, model.AttrSubFlag, "soft") {
		t.Fatalf("expected sub_flag=soft for VOSTFR marker, attrs=%v", res.Attributes)
	}
}

func TestLexiconNewFormat_YTBPlatformAndZhHansSubtitle(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Gecko] Dog's Tea - S01 (犬之茗; Cat's Tea; Quan Zhi Ming; The Dog's Tea) [YTB.WEB-DL 1080P AVC, Opus, SUB, HARD SUB (ZH-HANS)][BATCH]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrNormalized(res.Attributes, model.AttrPlatform, "YTB") {
		t.Fatalf("expected platform=YTB for YTB marker, attrs=%v", res.Attributes)
	}
	if !hasAttrNormalized(res.Attributes, model.AttrSubLang, "SC") {
		t.Fatalf("expected sub_lang=SC for ZH-HANS marker, attrs=%v", res.Attributes)
	}
	if hasAttrNormalized(res.Attributes, model.AttrSubLang, "ZH") {
		t.Fatalf("ZH-HANS should keep SC precision and not collapse to ZH, attrs=%v", res.Attributes)
	}
	if !hasAttrNormalized(res.Attributes, model.AttrSubFlag, "hard") {
		t.Fatalf("expected sub_flag=hard for HARD SUB marker, attrs=%v", res.Attributes)
	}
}

func TestLexiconNewFormat_ZhHantSubtitleKeepsTC(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Gecko] Dog's Tea - S01 [WEB-DL 1080P AVC, SUB, HARD SUB (ZH-HANT)][BATCH]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrNormalized(res.Attributes, model.AttrSubLang, "TC") {
		t.Fatalf("expected sub_lang=TC for ZH-HANT marker, attrs=%v", res.Attributes)
	}
	if hasAttrNormalized(res.Attributes, model.AttrSubLang, "ZH") {
		t.Fatalf("ZH-HANT should keep TC precision and not collapse to ZH, attrs=%v", res.Attributes)
	}
}

func TestLexiconNewFormat_HiResConsumedAsNoteNotTailGroup(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "【TSDM字幕组】[超时空辉夜姬][Cosmic Princess Kaguya][HEVC-10bit 1080p AAC][MKV][简日内封字幕]付『超かぐや姫！』劇中歌集10首自购Hi-Res"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !hasAttrNormalized(res.Attributes, model.AttrGroup, "TSDM 字幕组") {
		t.Fatalf("expected group=TSDM 字幕组, attrs=%v", res.Attributes)
	}
	if !hasAttrNormalized(res.Attributes, model.AttrTitle, "超时空辉夜姬") {
		t.Fatalf("expected title=超时空辉夜姬, attrs=%v", res.Attributes)
	}
	if hasAttrNormalized(res.Attributes, model.AttrGroup, "Res") {
		t.Fatalf("Hi-Res tail should not be parsed as group=Res, attrs=%v", res.Attributes)
	}
	for _, a := range res.Attributes {
		if a.Type == model.AttrTitleAlias && strings.EqualFold(strings.TrimSpace(a.Normalized), "Hi Res") {
			t.Fatalf("Hi-Res should not leak into title_alias, attrs=%v", res.Attributes)
		}
	}
	for _, tk := range res.Tokens {
		if (strings.EqualFold(strings.TrimSpace(tk.Text), "Hi") || strings.EqualFold(strings.TrimSpace(tk.Text), "Res")) && !tk.Removed {
			t.Fatalf("Hi-Res tokens should be consumed, tokens=%v", res.Tokens)
		}
	}
}

func TestLexiconNewFormat_TailCustomNotParsedAsTailGroup(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[EssAirTé] JoJo's Bizarre Adventure STEEL BALL RUN - EP01 VOSTFR-custom"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if hasAttrNormalized(res.Attributes, model.AttrGroup, "custom") {
		t.Fatalf("tail custom should not be parsed as group, attrs=%v", res.Attributes)
	}
	if hasAttrNormalized(res.Attributes, model.AttrTailGroup, "custom") {
		t.Fatalf("tail custom should not be parsed as tail_group, attrs=%v", res.Attributes)
	}
	if got := canonicalEpisode(res); got != "1" {
		t.Fatalf("episode=%q want=%q", got, "1")
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(canonicalTitle(res))), "custom") {
		t.Fatalf("tail custom should not pollute title, title=%q", canonicalTitle(res))
	}
}
