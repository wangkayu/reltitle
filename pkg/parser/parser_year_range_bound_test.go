package parser

import (
	"testing"

	"github.com/wangkayu/reltitle/pkg/config"
)

// A four-digit year is metadata, never an episode-range bound. The reported
// file name put the year directly before " - <ep>", and the year token had
// already been removed by year extraction, so the range scanner reused it as
// the left bound and the episode was lost.
func TestParserYearIsNotEpisodeRangeBound(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []struct {
		name string
		raw  string
		ep   string
	}{
		{
			name: "reported_filename",
			raw:  "[Nekomoe kissaten&LoliHouse] Ranma ½ 2024 - 27 [WebRip 1080p HEVC-10bit AAC ASSx2].mkv",
			ep:   "27",
		},
		{
			name: "spaced_year_dash_episode",
			raw:  "Ranma 2024 - 28.mkv",
			ep:   "28",
		},
		{
			name: "compact_year_dash_episode",
			raw:  "Ranma 2024-27.mkv",
			ep:   "27",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := canonicalEPKind(res); got != "EP" {
				t.Fatalf("ep_semantics.kind=%q want=%q", got, "EP")
			}
			if got := canonicalEpisodeRange(res); got != "" {
				t.Fatalf("episode_range=%q want empty", got)
			}
			if got := canonicalEpisode(res); got != tc.ep {
				t.Fatalf("episode=%q want=%q", got, tc.ep)
			}
		})
	}
}

// The year guard must not swallow real ranges or change year–year handling.
func TestParserRealEpisodeRangesSurviveYearGuard(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	t.Run("plain_range", func(t *testing.T) {
		res, err := p.Parse("One Piece 1080 - 1085.mkv")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if got := canonicalEpisodeRange(res); got != "1080-1085" {
			t.Fatalf("episode_range=%q want=%q", got, "1080-1085")
		}
	})

	t.Run("spaced_tilde_batch", func(t *testing.T) {
		res, err := p.Parse("[Erai-raws] Prism Rondo - 01 ~ 20 [1080p NF WEBRip HEVC EAC3][MultiSub] [BATCH]")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if got := canonicalEpisodeRange(res); got != "1-20" {
			t.Fatalf("episode_range=%q want=%q", got, "1-20")
		}
	})

	t.Run("year_year_stays_out_of_range", func(t *testing.T) {
		res, err := p.Parse("Show 2024 - 2025.mkv")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if got := canonicalEpisodeRange(res); got != "" {
			t.Fatalf("episode_range=%q want empty", got)
		}
		if got := canonicalEpisode(res); got != "" {
			t.Fatalf("episode=%q want empty", got)
		}
	})
}
