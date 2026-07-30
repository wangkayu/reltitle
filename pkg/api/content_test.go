package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"title-parser/pkg/config"
	"title-parser/pkg/model"
	"title-parser/pkg/parser"
)

func TestProjectContentPreservesCompositeBusinessFields(t *testing.T) {
	result := &model.ParseResult{
		EPSemantics: &model.EPSemantics{
			Primary:    &model.EPReference{Season: "2", EpisodeList: []string{"3", "4"}, RangeStart: "3", RangeEnd: "4"},
			Alternates: []model.EPReference{{Season: "1", Episode: "15", Source: "internal_strategy"}},
		},
		AudioSlot: &model.AudioSlot{Tracks: []model.AudioTrack{
			{Language: "JPN", Codec: "AAC", Channels: "2.0", Source: "internal_strategy"},
			{Language: "CHN", Codec: "AAC", Channels: "2.0", Source: "internal_strategy"},
		}},
		Attributes: []model.Attribute{
			{Type: model.AttrTitle, Normalized: "Main"},
			{Type: model.AttrTitleAlias, Normalized: "Alias One"},
			{Type: model.AttrTitleAlias, Normalized: "Alias Two"},
			{Type: model.AttrGroup, Normalized: "Group A"},
			{Type: model.AttrResolution, Normalized: "1080p"},
			{Type: model.AttrResolution, Normalized: "2160p"},
			{Type: model.AttrSubLang, Normalized: "CHS"},
			{Type: model.AttrSubLang, Normalized: "JPN"},
			{Type: model.AttrVersion, Normalized: "v2"},
			{Type: model.AttrMovieIndexRange, Normalized: "1-3"},
			{Type: model.AttrTech, Normalized: "internal tech boundary"},
			{Type: model.AttrNote, Normalized: "internal note"},
			{Type: model.AttrSearchHint, Normalized: "internal hint"},
		},
	}

	got := projectContent(result)
	if len(got.Aliases) != 2 || got.Aliases[1] != "Alias Two" {
		t.Fatalf("aliases=%v", got.Aliases)
	}
	if got.Episode == nil || len(got.Episode.Numbers) != 2 || len(got.Episode.Alternates) != 1 {
		t.Fatalf("episode=%+v", got.Episode)
	}
	if got.Media == nil || len(got.Media.AudioTracks) != 2 || len(got.Media.Resolutions) != 2 {
		t.Fatalf("media=%+v", got.Media)
	}
	if got.Media.AudioTracks[0].Codec != "AAC" || got.Media.AudioTracks[0].Language != "JPN" {
		t.Fatalf("audio_tracks=%+v", got.Media.AudioTracks)
	}
	if len(got.Versions) != 1 || got.Versions[0] != "v2" {
		t.Fatalf("versions=%v", got.Versions)
	}
	if got.Movie == nil || got.Movie.Range == nil || got.Movie.Range.Start != "1" || got.Movie.Range.End != "3" {
		t.Fatalf("movie=%+v", got.Movie)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "extras") || strings.Contains(string(encoded), "internal note") || strings.Contains(string(encoded), "internal hint") || strings.Contains(string(encoded), "internal tech boundary") {
		t.Fatalf("internal attributes leaked: %s", encoded)
	}
}

func TestPublicRoutesExposeOnlyCompactResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := gin.New()
	RegisterRoutes(router, parser.New(cfg))

	oldRoute := httptest.NewRecorder()
	router.ServeHTTP(oldRoute, httptest.NewRequest(http.MethodPost, "/api/v1/parse", bytes.NewBufferString(`{"title":"Example"}`)))
	if oldRoute.Code != http.StatusNotFound {
		t.Fatalf("legacy parse route status=%d", oldRoute.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/content", bytes.NewBufferString(`{"title":"[LoliHouse] Sample Title - 03 [1080p][HEVC][AAC]"}`))
	request.Header.Set("content-type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("content route status=%d body=%s", response.Code, response.Body.String())
	}
	for _, internal := range []string{"confidence", "source", "tokens", "template", "internal", "note", "extras"} {
		if strings.Contains(strings.ToLower(response.Body.String()), internal) {
			t.Fatalf("public response leaked %q: %s", internal, response.Body.String())
		}
	}
}

func TestPublicRoutePreservesKnowledgeBasedCompositeTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := gin.New()
	RegisterRoutes(router, parser.New(cfg))

	body := `{"title":"【黑白映画字幕社】★7月新番[No.6] [未来都市][第10话][GB][848x480][RMVB]"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/content", bytes.NewBufferString(body))
	request.Header.Set("content-type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("content route status=%d body=%s", response.Code, response.Body.String())
	}

	var got ContentResponse
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Content != "No.6 未来都市" {
		t.Fatalf("content=%q", got.Content)
	}
	if len(got.Aliases) != 0 {
		t.Fatalf("compound title component leaked as alias: %v", got.Aliases)
	}
	if got.Episode == nil || got.Episode.Number != "10" {
		t.Fatalf("episode=%+v", got.Episode)
	}
}

func TestPublicRouteReturnsGenericValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg, err := config.LoadAll("../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	router := gin.New()
	RegisterRoutes(router, parser.New(cfg))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/content", bytes.NewBufferString(`{}`))
	request.Header.Set("content-type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || strings.TrimSpace(response.Body.String()) != `{"error":"invalid request"}` {
		t.Fatalf("validation response status=%d body=%s", response.Code, response.Body.String())
	}
}
