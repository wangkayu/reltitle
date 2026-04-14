package parser

import (
	"testing"

	"title-parser/pkg/config"
	"title-parser/pkg/model"
)

func TestParserPlatformPriority_BSTXBeatsTX(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []string{
		"[shincaps] LIAR GAME - 01 (BS-TX 1440x1080 MPEG2 AAC).ts",
		"[shincaps] Isekai Nonbiri Nouka 2 - 01 (BS-TX 1440x1080 MPEG2 AAC).ts",
	}

	for _, raw := range cases {
		res, err := p.Parse(raw)
		if err != nil {
			t.Fatalf("parse failed for %q: %v", raw, err)
		}
		if got := firstAttrValue(res, model.AttrPlatform); got != "BS-TX" {
			t.Fatalf("platform=%q want=%q for %q", got, "BS-TX", raw)
		}
	}
}
