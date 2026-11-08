package parser

import (
	"testing"

	"github.com/wangkayu/reltitle/pkg/config"
	"github.com/wangkayu/reltitle/pkg/model"
)

// The resolution dimension names a quality tier by vertical lines, so a
// WxH token normalizes by its height: anamorphic 1440x1080 and letterboxed
// 2600x1080 are 1080p, 848x480 is 480p, and a height outside the canonical
// tiers keeps the raw WxH.
func TestResolutionWxHNormalizesByHeight(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	for _, tc := range []struct {
		token string
		want  string
	}{
		{"1440x1080", "1080p"},
		{"2600x1080", "1080p"},
		{"848x480", "480p"},
		{"1920x1080", "1080p"},
		{"1024x576", "576p"},
		{"3840x1600", "3840x1600"},
	} {
		res, err := p.Parse("[Group] Show - 01 (AT-X " + tc.token + " MPEG2 AAC).ts")
		if err != nil {
			t.Fatalf("%s: parse failed: %v", tc.token, err)
		}
		got := ""
		for _, attr := range res.Attributes {
			if attr.Type == model.AttrResolution {
				got = attr.Normalized
			}
		}
		if got != tc.want {
			t.Fatalf("%s: resolution=%q want=%q", tc.token, got, tc.want)
		}
	}
}
