package parser

import (
	"testing"

	"github.com/wangkayu/reltitle/pkg/config"
)

// A bracketed calendar date must be one fact, not a year plus the leftover
// month–day range. Reported from bansoko: [2020-04-14] parsed as RANGE.
func TestParserBracketedDateIsNotAnEpisodeRange(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	cases := []struct {
		name string
		raw  string
		date string
	}{
		{
			name: "dash_separated_documentary",
			raw:  "[诸神字幕组][NHK纪录片][历史秘话之EVA][2020-04-14][1080P60 HEVC10 MKV]",
			date: "2020-04-14",
		},
		{
			name: "dot_separated_chibi_maruko",
			raw:  "[丸子家族][樱桃小丸子(Chibi Maruko-chan)]樱花盛开 祝大家幸福！春季1小时特别篇[2026.03.29][GB][1080P][MP4]",
			date: "2026-03-29",
		},
		{
			name: "dot_separated_chibi_maruko_ii_episode",
			raw:  "[丸子家族][櫻桃小丸子第二期(Chibi Maruko-chan II)][1553]小丸子想被說聲謝謝&友藏躲著中野先生[2026.10.25][BIG5][1080P][MP4]",
			date: "2026-10-25",
		},
		{
			name: "slash_separated",
			raw:  "[2020/04/14] Show 01 [1080p]",
			date: "2020-04-14",
		},
		{
			name: "cjk_separated",
			raw:  "[2020年4月14日] Show 01 [1080p]",
			date: "2020-04-14",
		},
		{
			name: "compact",
			raw:  "[20200414] Show 01 [1080p]",
			date: "2020-04-14",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.Parse(tc.raw)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := res.CanonicalDate(); got != tc.date {
				t.Fatalf("date=%q want=%q", got, tc.date)
			}
			if got := canonicalEpisodeRange(res); got != "" {
				t.Fatalf("episode_range=%q want empty", got)
			}
		})
	}

	t.Run("episode_survives_with_date", func(t *testing.T) {
		res, err := p.Parse(cases[2].raw)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if got := canonicalEpisode(res); got != "1553" {
			t.Fatalf("episode=%q want=%q", got, "1553")
		}
	})

	t.Run("plain_text_date", func(t *testing.T) {
		res, err := p.Parse("Chibi Maruko-chan 2020-04-14")
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if got := res.CanonicalDate(); got != "2020-04-14" {
			t.Fatalf("date=%q want=%q", got, "2020-04-14")
		}
		if got := canonicalEpisodeRange(res); got != "" {
			t.Fatalf("episode_range=%q want empty", got)
		}
	})
}

func TestParserDateShapesDoNotMatchRealRanges(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	for _, raw := range []string{
		"One Piece 1080 - 1085.mkv",
		"Show 2024 - 2025.mkv",
		"Show 2024-13-01 [1080p]",
		"[Nekomoe kissaten&LoliHouse] Ranma ½ 2024 - 27 [WebRip 1080p HEVC-10bit AAC ASSx2].mkv",
	} {
		res, err := p.Parse(raw)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if got := res.CanonicalDate(); got != "" {
			t.Fatalf("date=%q want empty for %q", got, raw)
		}
	}
}

func TestParserDateYearStillReported(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := New(cfg)

	res, err := p.Parse("[2020-04-14] Show 01 [1080p]")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := res.CanonicalYear(); got != "2020" {
		t.Fatalf("year=%q want=%q", got, "2020")
	}
}
