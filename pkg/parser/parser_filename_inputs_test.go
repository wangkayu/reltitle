package parser

import (
	"strings"
	"testing"

	"github.com/wangkayu/reltitle/pkg/model"
)

func TestParserDualAudioSeparatorVariants(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	titles := []string{
		"Chainsaw.Man.The.Movie.Reze.Arc.2025.1080p.AMZN.WEBRip.Dual.Audio.AAC5.1.10bits.x265-Rapta",
		"100 Meters 2025 1080p NF WEB-DL DDP5.1 H 264-VARYG (Hyakuemu., Dual-Audio, Multi-Subs)",
		"[MirrorNova] Doraemon - Nobita and the Steel Troops (1986) [1080p WEB-DL, Dual-Audio, FLAC, AVC]",
	}
	for _, title := range titles {
		res, err := p.Parse(title)
		if err != nil {
			t.Fatalf("parse %q: %v", title, err)
		}
		if !hasAttrValue(attrValues(res, model.AttrAudioFlag), "DUAL_AUDIO") {
			t.Errorf("%q audio_flag = %v, want DUAL_AUDIO", title, attrValues(res, model.AttrAudioFlag))
		}
	}
}

func TestParserChineseAtmosSpelling(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	title := "[4K影视屋][4K_HDR][007：无暇赴死No.Time.to.Die.2021][2160P][WEB-DL][杜比全景声][HEVC]"
	res, err := p.Parse(title)
	if err != nil {
		t.Fatalf("parse %q: %v", title, err)
	}
	if !hasAttrValue(attrValues(res, model.AttrAudioObject), "ATMOS") {
		t.Errorf("audio_object = %v, want ATMOS", attrValues(res, model.AttrAudioObject))
	}
}

// 1080P60 is the 60 fps spelling of 1080p in the same bracket. It reports the
// resolution and keeps the frame rate as a note, as 1080p@60fps already does.
func TestParserResolution1080p60(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	title := "[诸神字幕组][NHK纪录片][历史秘话之EVA][2020-04-14][1080P60 HEVC10 MKV]"
	res, err := p.Parse(title)
	if err != nil {
		t.Fatalf("parse %q: %v", title, err)
	}
	if got := firstAttrValue(res, model.AttrResolution); got != "1080p" {
		t.Errorf("resolution = %q, want 1080p", got)
	}
	if !hasAttrValue(attrValues(res, model.AttrNote), "60FPS") {
		t.Errorf("note = %v, want 60FPS", attrValues(res, model.AttrNote))
	}
	for _, tok := range res.Tokens {
		if tok.Text == "1080P60" && !tok.Removed {
			t.Errorf("1080P60 token was not consumed")
		}
	}
}

func TestParserContainerRMVB(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	title := "【黑白映画字幕社】★7月新番[No.6] [未来都市][第10话][GB][848x480][RMVB]"
	res, err := p.Parse(title)
	if err != nil {
		t.Fatalf("parse %q: %v", title, err)
	}
	if got := firstAttrValue(res, model.AttrContainer); got != "RMVB" {
		t.Errorf("container = %q, want RMVB", got)
	}
	if got := firstAttrValue(res, model.AttrResolution); got != "480p" {
		t.Errorf("resolution = %q, want 480p", got)
	}
}

func TestParserEditionDeluxe(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	title := "Dragon Ball DAIMA 2024 S01 Deluxe Edition JPN 2160p UHD Blu-ray 1080p Blu-ray BOX-GWXHBL [BDMV]"
	res, err := p.Parse(title)
	if err != nil {
		t.Fatalf("parse %q: %v", title, err)
	}
	if !hasAttrValue(attrValues(res, model.AttrEdition), "DELUXE") {
		t.Errorf("edition = %v, want DELUXE", attrValues(res, model.AttrEdition))
	}
}

func TestParserChineseSubtitleWords(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	cases := map[string]string{
		"x [简体字幕]": "SC",
		"x [繁字]":   "TC",
		"[超次元ゲイム ネプテューヌ][简体字幕]OVA3 沐浴阳光的紫茄子花 陽だまりの茄花[超次元游戏海王星][Hyperdimension Neptunia][CHS]": "SC",
		"[Furretar] 台配 - 进击的巨人 The Final Season 完结篇 [繁字][CHT][國語][中文配音]":                      "TC",
	}
	for title, want := range cases {
		res, err := p.Parse(title)
		if err != nil {
			t.Fatalf("parse %q: %v", title, err)
		}
		if !hasAttrValue(attrValues(res, model.AttrSubLang), want) {
			t.Errorf("%q sub_lang = %v, want %s", title, attrValues(res, model.AttrSubLang), want)
		}
	}
}

// Erai-raws marks the audio language in a trailing parenthesis: (CA) is
// Chinese audio and (JA) is Japanese audio. The raw code is not a note.
func TestEraiTagCodeAudioLanguage(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	cases := []struct {
		title string
		want  string
	}{
		{"[Erai-raws] Sample Show - 01 (CA) [WEB-DL][1080p]", "CMN"},
		{"[Erai-raws] Sample Show - 02 (JA) [WEB-DL][1080p]", "JPN"},
	}
	for _, tc := range cases {
		res, err := p.Parse(tc.title)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.title, err)
		}
		langs := attrValues(res, model.AttrAudioLang)
		if !hasAttrValue(langs, tc.want) {
			t.Errorf("%q audio_lang = %v, want %s", tc.title, langs, tc.want)
		}
		if hasAttrValue(langs, "YUE") {
			t.Errorf("%q audio_lang = %v, must not contain YUE", tc.title, langs)
		}
		for _, note := range attrValues(res, model.AttrNote) {
			if strings.HasPrefix(note, "tag_code:") {
				t.Errorf("%q note = %q, raw tag code must not be a note", tc.title, note)
			}
		}
	}
}

// The Maruko route used to add the bracketed date as a note even though the
// date pass already reported it.
func TestParserMarukoDateNotDuplicatedAsNote(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	title := "[丸子家族][海螺小姐(Sazae-san)][2712]鲑仔和电线杆叔叔&不要责怪我把自己当女儿&喜欢恐怖电影吗[2023.08.06][GB][1080P][MP4]"
	res, err := p.Parse(title)
	if err != nil {
		t.Fatalf("parse %q: %v", title, err)
	}
	if got := firstAttrValue(res, model.AttrDate); got != "2023-08-06" {
		t.Errorf("date = %q, want 2023-08-06", got)
	}
	if hasAttrValue(attrValues(res, model.AttrNote), "2023.08.06") {
		t.Errorf("note = %v, bracketed date must not repeat as a note", attrValues(res, model.AttrNote))
	}
}
