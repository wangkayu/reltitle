package parser

import (
	"testing"

	"github.com/wangkayu/reltitle/pkg/config"
	"github.com/wangkayu/reltitle/pkg/model"
)

// "乱马1/2" is protected by the title dictionary. The season marker after it is consumed, so
// the episode in "第二季 - 05" became the first visible token after the group and was
// protected as a "leading title digit". The result was BATCH with no episode.
func TestParserKnownTitleFollowedBySeasonKeepsEpisode(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	res, err := New(cfg).Parse("[G] 乱马1/2 第二季 - 05 [1080P]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.ContentKind != model.ContentKind("TV_EP") {
		t.Errorf("content_kind=%q want TV_EP", res.ContentKind)
	}
	if got := canonicalTitle(res); got != "乱马1/2" {
		t.Errorf("title=%q want 乱马1/2", got)
	}
	if got := canonicalSeason(res); got != "2" {
		t.Errorf("season=%q want 2", got)
	}
	if got := canonicalEpisode(res); got != "5" {
		t.Errorf("episode=%q want 5", got)
	}
}

// In "Season 3 - 01" the 3 is the season value. It was offered as an episode candidate and,
// next to the protected alias "Ranma 1-2", won over the real episode 01.
func TestParserSeasonValueIsNotAnEpisodeCandidate(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	res, err := New(cfg).Parse("[ToonsHub] 乱马1/2 第三季 / Ranma 1-2 (2024) Season 3 - 01 [1080P][DUAL][MKV]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := canonicalSeason(res); got != "3" {
		t.Errorf("season=%q want 3", got)
	}
	if got := canonicalEpisode(res); got != "1" {
		t.Errorf("episode=%q want 1", got)
	}
}

// Protected title content before a digit means the digit is not the leading title digit.
func TestProtectTitleLeadingDigitsIgnoresDigitAfterProtectedTitle(t *testing.T) {
	tokens := []model.Token{
		{Text: "乱马1", Protected: true}, {Text: "/", Protected: true}, {Text: "2", Protected: true},
		{Text: "第二季", Removed: true}, {Text: "-", Removed: true}, {Text: "05"}, {Text: "[", Removed: true},
	}
	protectTitleLeadingDigits(tokens, 0)
	if tokens[5].Protected {
		t.Fatal("the episode digit after protected title content was protected")
	}

	// The documented "86" case keeps working.
	lead := []model.Token{{Text: "86"}, {Text: "-"}, {Text: "03"}}
	protectTitleLeadingDigits(lead, 0)
	if !lead[0].Protected || lead[2].Protected {
		t.Fatalf("leading title digit: protected=%v/%v, want true/false", lead[0].Protected, lead[2].Protected)
	}
}

// "乱马1/2 第三季 / Ranma 1-2 ..." has two slashes: the one inside the known title and the
// bilingual separator. The slash alias chain needs two delimiters, so the protected "/" of
// "乱马1/2" used to trigger it and produce the noisy alias "Ranma 1-3".
func TestParserProtectedSlashIsNotAnAliasDelimiter(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	res, err := New(cfg).Parse("[ToonsHub] 乱马1/2 第三季 / Ranma 1-2 (2024) Season 3 - 01 [1080P][DUAL][CHS&CHT&JP&EN&KO][MKV]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := canonicalTitle(res); got != "乱马1/2" {
		t.Errorf("title=%q want 乱马1/2", got)
	}
	want := []string{"Ranma 1-2", "Ranma 1"}
	got := attrValues(res, model.AttrTitleAlias)
	if len(got) != len(want) {
		t.Fatalf("aliases=%q want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("aliases=%q want %q", got, want)
		}
	}
	if canonicalSeason(res) != "3" || canonicalEpisode(res) != "1" {
		t.Errorf("season/episode=%q/%q want 3/1", canonicalSeason(res), canonicalEpisode(res))
	}
}

// A plain bilingual title (one separator slash) must still split into title and alias.
func TestParserBilingualSlashStillSplitsAliases(t *testing.T) {
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	res, err := New(cfg).Parse("[ToonsHub] 青之箱 第二季 / Blue Box Season 2 - 01 [1080P][MKV]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := canonicalTitle(res); got != "青之箱" {
		t.Errorf("title=%q want 青之箱", got)
	}
	if got := attrValues(res, model.AttrTitleAlias); len(got) != 1 || got[0] != "Blue Box" {
		t.Errorf("aliases=%q want [Blue Box]", got)
	}
}
