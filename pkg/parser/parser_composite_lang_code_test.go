package parser

import (
	"testing"

	"github.com/wangkayu/reltitle/pkg/model"
)

// The composite codes JPSC and JPTC are also written literally, most often as
// a subtitle file's own tag ("Movie.JPSC.ass"). A bracketed literal reads as
// that one sub_lang, like the SC/TC literals.
func TestCompositeLangCodeLiteral(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw      string
		wantLang []string
	}{
		{raw: "x [JPSC]", wantLang: []string{"JPSC"}},
		{raw: "x [JPTC]", wantLang: []string{"JPTC"}},
		{raw: "x [jpsc]", wantLang: []string{"JPSC"}},
		{raw: "[G][Show][01][1080p HEVC][JPSC]", wantLang: []string{"JPSC"}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got := attrValues(res, model.AttrSubLang); !equalAttrValues(got, tc.wantLang) {
				t.Errorf("sub_lang=%v want %v", got, tc.wantLang)
			}
		})
	}
}
