package parser

import (
	"testing"

	"github.com/wangkayu/reltitle/pkg/config"
	"github.com/wangkayu/reltitle/pkg/model"
)

// 无修版 and 无修正 are the same content rating written two ways. The lexicon
// has always emitted content_rating for 无修正 and only a note for 无修版, so a
// consumer filtering on content_rating missed the 无修版 spelling. The note
// promotion now lifts both spellings to content_rating.
func TestParserUncensoredVersionIsContentRating(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []string{
		"[北宇治字幕组] 学生会也有洞！ / 脑洞学生会 / 學生會也有洞！ / Seitokai ni mo Ana wa Aru! [03][无修版][WebRip][HEVC_AAC][简繁内封]",
		"[Group] 示例标题 / Example Title [03][無修版][WebRip][HEVC_AAC][简繁内封]",
		"[Group] 示例标题 / Example Title [03][完全无修版][WebRip][HEVC_AAC][简繁内封]",
		"[Group] 示例标题 / Example Title [03][完全無修版][WebRip][HEVC_AAC][简繁内封]",
	}
	for _, raw := range cases {
		res, err := p.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		ratings := attrValues(res, model.AttrContentRating)
		if len(ratings) != 1 || ratings[0] != "UNCENSORED" {
			t.Fatalf("%q: content_rating=%v want=[UNCENSORED]", raw, ratings)
		}
		if hasParseAttrValue(res, model.AttrNote, "UNCENSORED_VERSION") {
			t.Fatalf("%q: note UNCENSORED_VERSION should have been promoted", raw)
		}
		if canonicalTitle(res) == "" {
			t.Fatalf("%q: title is empty", raw)
		}
	}

	// The same fact in two spellings collapses to one attribute.
	res, err := p.Parse("[Group] 示例标题 [03][无修正][无修版][WebRip][1080p]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ratings := attrValues(res, model.AttrContentRating); len(ratings) != 1 || ratings[0] != "UNCENSORED" {
		t.Fatalf("co-occurring spellings: content_rating=%v want exactly one UNCENSORED", ratings)
	}
}
