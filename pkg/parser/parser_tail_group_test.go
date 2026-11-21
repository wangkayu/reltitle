package parser

import (
	"testing"

	"github.com/wangkayu/reltitle/pkg/config"
	"github.com/wangkayu/reltitle/pkg/model"
)

func TestParserTailGroupWithAtSign(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	for _, tc := range []struct {
		name      string
		raw       string
		group     string
		title     string
		container string
		episode   string
	}{
		{
			name:  "season release",
			raw:   "Death.Note.S01.2006.1080p.BluRay.x265.FLAC.2.0-AnimeF@ADE",
			group: "AnimeF@ADE",
			title: "Death.Note",
		},
		{
			name:      "episode filename",
			raw:       "Death.Note.S01E02.2006.1080p.BluRay.x265.FLAC.2.0-AnimeF@ADE.mkv",
			group:     "AnimeF@ADE",
			title:     "Death.Note",
			container: "MKV",
			episode:   "2",
		},
		{
			name: "at sign in the title",
			raw:  "Show@10th.S01E02.1080p",
		},
		{
			name: "frame rate suffix",
			raw:  "Show.S01E02.1080p-1080p@60fps",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := canonicalGroup(res); got != tc.group {
				t.Errorf("group = %q, want %q", got, tc.group)
			}
			if tc.title != "" && canonicalTitle(res) != tc.title {
				t.Errorf("title = %q, want %q", canonicalTitle(res), tc.title)
			}
			if tc.container != "" && firstAttrValue(res, model.AttrContainer) != tc.container {
				t.Errorf("container = %q, want %q", firstAttrValue(res, model.AttrContainer), tc.container)
			}
			if tc.episode != "" && (res.EPSemantics == nil || res.EPSemantics.Primary == nil || res.EPSemantics.Primary.Episode != tc.episode) {
				t.Errorf("episode semantics = %+v, want episode %s", res.EPSemantics, tc.episode)
			}
		})
	}
}
