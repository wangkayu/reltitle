package parser

import (
	"github.com/wangkayu/reltitle/pkg/model"
	"testing"
)

func TestConsumeEpisodeRangeSpan(t *testing.T) {
	tokens := []model.Token{{Text: "Show"}, {Text: "-"}, {Text: "01"}, {Text: "-"}, {Text: "12"}, {Text: "["}, {Text: "1080p"}, {Text: "]"}}
	slot := &model.EPSlot{Primary: &model.EPEntity{Kind: model.EPKindRange, EpisodeRange: "1-12", TokenStart: 2, TokenEnd: 3}}
	consumeTokensByEPSlotPrimarySpan(tokens, slot)
	if slot.Primary.TokenEnd != 5 {
		t.Errorf("range end=%d want 5", slot.Primary.TokenEnd)
	}
	for i := 1; i < 5; i++ {
		if !tokens[i].Removed {
			t.Errorf("token %d not consumed", i)
		}
	}
	if tokens[0].Removed || tokens[5].Removed {
		t.Fatal("consumed tokens outside episode span")
	}
}

func TestParserEpisodeSeparatorsConsumed(t *testing.T) {
	p, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"[Gecko] Show - S01E01-E75 [1080p][BATCH]",
		"[ToonsHub] 东京复仇者 三天战争篇 - 01 [1080P][CHT&EN][MKV]",
		"[ToonsHub] 某作品 / Some Show - 01-12 [1080P][CHT&EN][MKV]",
		"[G] 新網球王子 U-17 WORLD CUP - 01 [1080p]",
	} {
		res, err := p.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		for i, tok := range res.Tokens {
			if tok.Text == "-" && i > 0 && res.Tokens[i-1].Text == "U" {
				if tok.Removed {
					t.Error("title hyphen was consumed")
				}
				continue
			}
			if (tok.Text == "-" || tok.Text == "01" || tok.Text == "12" || tok.Text == "E75") && !tok.Removed {
				t.Errorf("%q: unconsumed episode token %q", raw, tok.Text)
			}
		}
	}
}
