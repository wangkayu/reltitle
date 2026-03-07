package parser

import (
	"strings"
	"testing"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

func TestEditionToken_AudioCommentaryVer(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[shincaps] Saikyou no Ousama, Nidome no Jinsei wa Nani wo Suru ~Audio Commentary ver.~ - 08 (AT-X 1440x1080 MPEG2 AAC).ts"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	title := canonicalTitle(res)
	if strings.Contains(strings.ToLower(title), "audio commentary") {
		t.Fatalf("edition token leaked into title: %q", title)
	}
	if canonicalEpisode(res) != "8" {
		t.Fatalf("episode=%q want=8", canonicalEpisode(res))
	}

	notes := attrValues(res, model.AttrNote)
	found := false
	for _, n := range notes {
		if strings.EqualFold(strings.TrimSpace(n), "AUDIO_COMMENTARY") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected note AUDIO_COMMENTARY, got=%v", notes)
	}
}

func TestEditionToken_AudioCommentaryVer_UnderscoreRoutedByGroup(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[shincaps] Saikyou no Ousama ~Audio_Commentary ver.~ - 08 (AT-X 1440x1080 MPEG2 AAC).ts"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if canonicalEpisode(res) != "8" {
		t.Fatalf("episode=%q want=8", canonicalEpisode(res))
	}
	if !hasParseAttrValue(res, model.AttrNote, "AUDIO_COMMENTARY") {
		t.Fatalf("expected note AUDIO_COMMENTARY")
	}
}

func TestEditionToken_AudioCommentaryVer_UnderscoreNotGeneric(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[other] Saikyou no Ousama ~Audio_Commentary ver.~ - 08 (AT-X 1440x1080 MPEG2 AAC).ts"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if hasParseAttrValue(res, model.AttrNote, "AUDIO_COMMENTARY") {
		t.Fatalf("unexpected note AUDIO_COMMENTARY for non-routed group")
	}
}

func TestEditionToken_EncodeVer(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Moozzi2] Clevatess BD-BOX [ x265-10Bit Ver. ] - TV + SP"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if canonicalTitle(res) != "Clevatess" {
		t.Fatalf("title=%q want=Clevatess", canonicalTitle(res))
	}
	if !hasParseAttrValue(res, model.AttrNote, "ENCODE_VER") {
		t.Fatalf("expected note ENCODE_VER")
	}
}

func TestEditionToken_NoUnderscorePollution_ForEpisodeSplit(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[阿特拉斯字幕组·雪原市出差所][命运-奇异赝品_Fate／strange Fake][02_群像 VS 虚像][简繁日内封PGS][Japanese Dub][Web-DL Remux][1080p AVC AAC][附：第2话先行上映后花泽香菜&诸星堇访谈节目熟肉（流媒体）]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if canonicalEpisode(res) != "2" {
		t.Fatalf("episode=%q want=2", canonicalEpisode(res))
	}
	if hasParseAttrValue(res, model.AttrNote, "AUDIO_COMMENTARY") {
		t.Fatalf("unexpected AUDIO_COMMENTARY note")
	}
}

func TestEditionToken_UncensoredVer(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Group] Example Show [Uncensored Ver.] - 01 [1080p AVC AAC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if canonicalEpisode(res) != "1" {
		t.Fatalf("episode=%q want=1", canonicalEpisode(res))
	}
	if !hasParseAttrValue(res, model.AttrNote, "UNCENSORED_VER") {
		t.Fatalf("expected note UNCENSORED_VER")
	}
}

func TestEditionToken_DirectorsCut(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Group] Example Show [Director's Cut] - 01 [1080p AVC AAC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if canonicalEpisode(res) != "1" {
		t.Fatalf("episode=%q want=1", canonicalEpisode(res))
	}
	if !hasParseAttrValue(res, model.AttrNote, "DIRECTORS_CUT") {
		t.Fatalf("expected note DIRECTORS_CUT")
	}
}

func TestEditionToken_UncutVer(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Group] Example Show [Uncut Ver.] - 01 [1080p AVC AAC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if canonicalEpisode(res) != "1" {
		t.Fatalf("episode=%q want=1", canonicalEpisode(res))
	}
	if !hasParseAttrValue(res, model.AttrNote, "UNCUT_VER") {
		t.Fatalf("expected note UNCUT_VER")
	}
}

func TestEditionToken_ExtendedCut(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Group] Example Show [Extended Cut] - 01 [1080p AVC AAC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if canonicalEpisode(res) != "1" {
		t.Fatalf("episode=%q want=1", canonicalEpisode(res))
	}
	if !hasParseAttrValue(res, model.AttrNote, "EXTENDED_CUT") {
		t.Fatalf("expected note EXTENDED_CUT")
	}
}

func TestEditionToken_RemasteredVer(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	raw := "[Group] Example Show [Remastered Ver.] - 01 [1080p AVC AAC]"
	res, err := p.Parse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if canonicalEpisode(res) != "1" {
		t.Fatalf("episode=%q want=1", canonicalEpisode(res))
	}
	if !hasParseAttrValue(res, model.AttrNote, "REMASTERED_VER") {
		t.Fatalf("expected note REMASTERED_VER")
	}
}

func hasParseAttrValue(res *model.ParseResult, typ model.AttrType, normalized string) bool {
	for _, a := range res.Attributes {
		if a.Type != typ {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(a.Normalized), normalized) {
			return true
		}
	}
	return false
}
