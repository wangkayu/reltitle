package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectContentPreservesPublicFieldsAndOmitsInternals(t *testing.T) {
	result := &ParseResult{
		EPSemantics: &EPSemantics{Primary: &EPReference{Season: "2", EpisodeList: []string{"3", "4"}}},
		AudioSlot:   &AudioSlot{Tracks: []AudioTrack{{Language: "JPN", Codec: "AAC", Channels: "2.0", Source: "internal"}}},
		Attributes: []Attribute{
			{Type: AttrTitle, Normalized: "Main"},
			{Type: AttrTitleAlias, Normalized: "Alias"},
			{Type: AttrGroup, Normalized: "Group"},
			{Type: AttrResolution, Normalized: "1080p"},
			{Type: AttrVersion, Normalized: "v2"},
			{Type: AttrNote, Normalized: "internal note"},
			{Type: AttrTech, Normalized: "internal tech"},
		},
	}

	got := ProjectContent(result)
	if got.Content != "Main" || len(got.Aliases) != 1 || len(got.Groups) != 1 {
		t.Fatalf("content=%+v", got)
	}
	if got.Episode == nil || len(got.Episode.Numbers) != 2 {
		t.Fatalf("episode=%+v", got.Episode)
	}
	if got.Media == nil || len(got.Media.AudioTracks) != 1 || got.Media.AudioTracks[0].Codec != "AAC" {
		t.Fatalf("media=%+v", got.Media)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, internal := range []string{"confidence", "source", "tokens", "template", "internal note", "internal tech"} {
		if strings.Contains(strings.ToLower(string(encoded)), internal) {
			t.Fatalf("public output leaked %q: %s", internal, encoded)
		}
	}
}
