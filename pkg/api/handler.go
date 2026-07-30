package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"title-parser/pkg/model"
	"title-parser/pkg/parser"
)

type Handler struct {
	parser *parser.Parser
}

// Content returns the stable user-facing projection without parser internals.
func (h *Handler) Content(c *gin.Context) {
	var req ParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	result, err := h.parser.Parse(req.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parse failed"})
		return
	}
	c.JSON(http.StatusOK, projectContent(result))
}

func projectContent(result *model.ParseResult) ContentResponse {
	values := func(kind model.AttrType) []string {
		out := make([]string, 0, 2)
		for _, attr := range result.Attributes {
			if attr.Type == kind && strings.TrimSpace(attr.Normalized) != "" {
				out = appendUnique(out, strings.TrimSpace(attr.Normalized))
			}
		}
		return out
	}

	groups := appendUnique(values(model.AttrGroup), values(model.AttrTailGroup)...)
	episode := projectEpisode(result, values)
	movie := projectMovie(result)
	media := projectMedia(result, values)
	response := ContentResponse{
		Content: result.CanonicalTitle(), Aliases: values(model.AttrTitleAlias), Groups: groups,
		Versions: values(model.AttrVersion),
		Episode:  episode, Movie: movie, Media: media,
	}
	return response
}

func projectMovie(result *model.ParseResult) *ContentMovie {
	index := result.CanonicalMovieIndex()
	rangeValue := result.CanonicalMovieIndexRange()
	if index == "" && rangeValue == "" {
		return nil
	}
	movie := &ContentMovie{Index: index}
	if rangeValue != "" {
		parts := strings.SplitN(rangeValue, "-", 2)
		movie.Range = &ContentRange{Start: strings.TrimSpace(parts[0])}
		if len(parts) == 2 {
			movie.Range.End = strings.TrimSpace(parts[1])
		}
	}
	return movie
}

func appendUnique(dst []string, values ...string) []string {
	for _, value := range values {
		if value == "" {
			continue
		}
		found := false
		for _, existing := range dst {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, value)
		}
	}
	return dst
}

func projectEpisode(result *model.ParseResult, values func(model.AttrType) []string) *ContentEpisode {
	semantics := result.CanonicalEPSemantics()
	if semantics == nil && len(values(model.AttrEpisode)) == 0 && len(values(model.AttrSeason)) == 0 {
		return nil
	}
	ep := &ContentEpisode{
		Season: result.CanonicalSeason(), SeasonRange: result.CanonicalSeasonRange(),
		Number: result.CanonicalEpisode(), Numbers: values(model.AttrEpisodeList),
		Special: result.CanonicalSPKind(),
	}
	if semantics != nil && semantics.Primary != nil {
		primary := semantics.Primary
		if len(primary.EpisodeList) > 0 {
			ep.Numbers = appendUnique(nil, primary.EpisodeList...)
		}
		if primary.RangeStart != "" || primary.RangeEnd != "" {
			ep.Range = &ContentRange{Start: primary.RangeStart, End: primary.RangeEnd}
		}
	}
	if ep.Range == nil {
		rangeValues := values(model.AttrEpisodeRange)
		if len(rangeValues) > 0 {
			parts := strings.SplitN(rangeValues[0], "-", 2)
			ep.Range = &ContentRange{Start: parts[0]}
			if len(parts) == 2 {
				ep.Range.End = parts[1]
			}
		}
	}
	if semantics != nil {
		for _, alternate := range semantics.Alternates {
			item := ContentEpisodeAlternate{
				Season: alternate.Season, SeasonRange: alternate.SeasonRange,
				Number: alternate.Episode, Numbers: appendUnique(nil, alternate.EpisodeList...), Special: alternate.SPKind,
			}
			if alternate.RangeStart != "" || alternate.RangeEnd != "" {
				item.Range = &ContentRange{Start: alternate.RangeStart, End: alternate.RangeEnd}
			}
			if item.Season != "" || item.SeasonRange != "" || item.Number != "" || len(item.Numbers) > 0 || item.Range != nil || item.Special != "" {
				ep.Alternates = append(ep.Alternates, item)
			}
		}
	}
	return ep
}

func projectMedia(result *model.ParseResult, values func(model.AttrType) []string) *ContentMedia {
	media := &ContentMedia{
		Platforms: values(model.AttrPlatform), SourceTypes: values(model.AttrSourceType),
		Resolutions: values(model.AttrResolution), VideoCodecs: values(model.AttrVideoCodec),
		BitDepths: values(model.AttrBitDepth), Containers: values(model.AttrContainer), HDR: values(model.AttrHDR),
		CRCs: values(model.AttrCRC), AudioFlags: values(model.AttrAudioFlag),
	}
	if result.AudioSlot != nil {
		for _, track := range result.AudioSlot.Tracks {
			media.AudioTracks = append(media.AudioTracks, ContentAudioTrack{
				Language: track.Language, Codec: track.Codec, Channels: track.Channels, Object: track.Object,
			})
		}
	}
	codecs := values(model.AttrAudioCodec)
	channels := values(model.AttrAudioChannels)
	languages := values(model.AttrAudioLang)
	objects := values(model.AttrAudioObject)
	trackCount := max(len(media.AudioTracks), len(codecs), len(channels), len(languages), len(objects))
	for len(media.AudioTracks) < trackCount {
		media.AudioTracks = append(media.AudioTracks, ContentAudioTrack{})
	}
	for i := range media.AudioTracks {
		track := &media.AudioTracks[i]
		if track.Codec == "" {
			track.Codec = valueAt(codecs, i)
		}
		if track.Channels == "" {
			track.Channels = valueAt(channels, i)
		}
		if track.Language == "" {
			track.Language = valueAt(languages, i)
		}
		if track.Object == "" {
			track.Object = valueAt(objects, i)
		}
	}
	subtitles := &ContentSubtitles{
		Languages: appendUnique(values(model.AttrSubLang), values(model.AttrSubLangGroup)...),
		Types:     values(model.AttrSubFlag), Formats: values(model.AttrSubFormat),
	}
	if len(subtitles.Languages) > 0 || len(subtitles.Types) > 0 || len(subtitles.Formats) > 0 {
		media.Subtitles = subtitles
	}
	if len(media.Platforms) == 0 && len(media.SourceTypes) == 0 && len(media.Resolutions) == 0 && len(media.VideoCodecs) == 0 && len(media.BitDepths) == 0 && len(media.Containers) == 0 && len(media.HDR) == 0 && len(media.CRCs) == 0 && len(media.AudioFlags) == 0 && len(media.AudioTracks) == 0 && media.Subtitles == nil {
		return nil
	}
	return media
}

func valueAt(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return ""
}

func NewHandler(p *parser.Parser) *Handler {
	return &Handler{parser: p}
}

// GET /health
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":        "healthy",
		"version":       "2.5.0",
		"config_loaded": true,
	})
}
