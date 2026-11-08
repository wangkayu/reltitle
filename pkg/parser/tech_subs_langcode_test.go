package parser

import (
	"reflect"
	"testing"

	"github.com/wangkayu/reltitle/pkg/model"
)

// The consumer wraps a subtitle file's own tag as "x [<tag>]" and reads only
// sub_lang from the result, so these cases mirror that carrier. A lone tag
// must not become an audio language or a title alias.
func TestSubtitleSuffixLanguageTags(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	cases := []struct {
		tag  string
		want []string
	}{
		{"zh", []string{"ZH"}},
		{"zho", []string{"ZH"}},
		{"chi", []string{"ZH"}},
		{"zh-cn", []string{"SC"}},
		{"zh_CN", []string{"SC"}},
		{"zh-Hans", []string{"SC"}},
		{"zh_hans", []string{"SC"}},
		{"zh-tw", []string{"TC"}},
		{"zh_TW", []string{"TC"}},
		{"zh-hk", []string{"TC"}},
		{"zh-hant", []string{"TC"}},
		{"zh-sg", []string{"SC"}},
		{"zh-my", []string{"SC"}},
		{"zh-mo", []string{"TC"}},
		{"zh-Hans-CN", []string{"SC"}},
		{"zh-Hant-HK", []string{"TC"}},
		{"zh-xx", []string{"ZH"}},
		{"ja", []string{"JPN"}},
		{"ja-JP", []string{"JPN"}},
		{"japanese", []string{"JPN"}},
		{"en", []string{"ENG"}},
		{"en-US", []string{"ENG"}},
		{"en-GB", []string{"ENG"}},
		{"english", []string{"ENG"}},
		{"ko", []string{"KOR"}},
		{"korean", []string{"KOR"}},
		{"track2", []string{"ENG"}},
		{"track3", []string{"TC"}},
		{"track4", []string{"SC"}},
		{"track5", []string{"JPN"}},
		{"chs_jpn", []string{"SC", "JPN"}},
		{"chs&jpn", []string{"SC", "JPN"}},
	}
	for _, tc := range cases {
		res, err := p.Parse("x [" + tc.tag + "]")
		if err != nil {
			t.Fatalf("parse x [%s]: %v", tc.tag, err)
		}
		if got := attrValues(res, model.AttrSubLang); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("x [%s] sub_lang = %v, want %v", tc.tag, got, tc.want)
		}
		if got := attrValues(res, model.AttrAudioLang); len(got) != 0 {
			t.Errorf("x [%s] audio_lang = %v, want none", tc.tag, got)
		}
	}
}

func TestSubtitleSuffixFormats(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	cases := map[string][]string{
		"ass":     {"ASS"},
		"srt":     {"SRT"},
		"ssa":     {"SSA"},
		"pgs":     {"PGS"},
		"sup":     {"PGS"},
		"vobsub":  {"VOBSUB"},
		"vtt":     {"VTT"},
		"chs.vtt": {"VTT"},
	}
	for tag, want := range cases {
		res, err := p.Parse("x [" + tag + "]")
		if err != nil {
			t.Fatalf("parse x [%s]: %v", tag, err)
		}
		if got := attrValues(res, model.AttrSubFormat); !reflect.DeepEqual(got, want) {
			t.Errorf("x [%s] sub_format = %v, want %v", tag, got, want)
		}
	}
}

// A bracketed title that happens to be a common English word must stay a
// title: only the streaming tags above may stand alone.
func TestSubtitleSuffixDoesNotClaimTitleWords(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault: %v", err)
	}
	for _, title := range []string{"x [It]", "x [My]", "x [No]"} {
		res, err := p.Parse(title)
		if err != nil {
			t.Fatalf("parse %s: %v", title, err)
		}
		if got := attrValues(res, model.AttrSubLang); len(got) != 0 {
			t.Errorf("%s sub_lang = %v, want none", title, got)
		}
	}
}
