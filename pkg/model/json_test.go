package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseResultMarshalJSON_HardCutsLegacyEPSemanticsFields(t *testing.T) {
	res := ParseResult{
		RawTitle:    "[A] Sample - 01",
		ContentKind: "TV_EP",
		EPSemantics: &EPSemantics{
			Kind:    "EP",
			Axis:    "SE",
			Season:  "2",
			Episode: "1",
			Source:  "episode_final",
		},
		EPSlot: &EPSlot{
			Primary: &EPEntity{
				Kind:    "EP",
				Axis:    "SE",
				Season:  "2",
				Episode: "1",
				Source:  "episode_final",
			},
		},
		Attributes: []Attribute{
			{Type: AttrTitle, Normalized: "Sample"},
			{Type: AttrSeason, Normalized: "2"},
			{Type: AttrEpisode, Normalized: "1"},
		},
	}

	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"ep_semantics"`) {
		t.Fatalf("expected ep_semantics in json: %s", text)
	}
	if strings.Contains(text, `"epslot"`) {
		t.Fatalf("legacy epslot should not be serialized: %s", text)
	}
	if strings.Contains(text, `"type":"episode"`) || strings.Contains(text, `"type":"season"`) {
		t.Fatalf("legacy episode attrs should be filtered from serialized attributes: %s", text)
	}
	if !strings.Contains(text, `"type":"title"`) {
		t.Fatalf("non-ep attrs should remain serialized: %s", text)
	}
}

func TestCanonicalEPSemantics_FallbacksFromLegacyFields(t *testing.T) {
	res := &ParseResult{
		Attributes: []Attribute{
			{Type: AttrSeasonRange, Normalized: "S1-S3"},
			{Type: AttrSeason, Normalized: "1"},
		},
	}
	sem := res.CanonicalEPSemantics()
	if sem == nil {
		t.Fatalf("semantics should not be nil")
	}
	if sem.Kind != "SEASON_RANGE" {
		t.Fatalf("kind=%q want=%q", sem.Kind, "SEASON_RANGE")
	}
	if sem.SeasonRange != "S1-S3" {
		t.Fatalf("season_range=%q want=%q", sem.SeasonRange, "S1-S3")
	}
}
