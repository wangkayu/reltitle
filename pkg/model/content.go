package model

import "strings"

// ContentResult is the compact public representation of a parsed title.
type ContentResult struct {
	Content  string          `json:"content,omitempty"`
	Aliases  []string        `json:"aliases,omitempty"`
	Groups   []string        `json:"groups,omitempty"`
	Versions []string        `json:"versions,omitempty"`
	Episode  *ContentEpisode `json:"episode,omitempty"`
	Movie    *ContentMovie   `json:"movie,omitempty"`
	Media    *ContentMedia   `json:"media,omitempty"`
}

type ContentMovie struct {
	Index string        `json:"index,omitempty"`
	Range *ContentRange `json:"range,omitempty"`
}

type ContentEpisode struct {
	Season      string                    `json:"season,omitempty"`
	SeasonRange string                    `json:"season_range,omitempty"`
	Number      string                    `json:"number,omitempty"`
	Numbers     []string                  `json:"numbers,omitempty"`
	Range       *ContentRange             `json:"range,omitempty"`
	Special     string                    `json:"special,omitempty"`
	Alternates  []ContentEpisodeAlternate `json:"alternates,omitempty"`
}

type ContentRange struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

type ContentEpisodeAlternate struct {
	Season      string        `json:"season,omitempty"`
	SeasonRange string        `json:"season_range,omitempty"`
	Number      string        `json:"number,omitempty"`
	Numbers     []string      `json:"numbers,omitempty"`
	Range       *ContentRange `json:"range,omitempty"`
	Special     string        `json:"special,omitempty"`
}

type ContentMedia struct {
	Platforms   []string            `json:"platforms,omitempty"`
	SourceTypes []string            `json:"source_types,omitempty"`
	Resolutions []string            `json:"resolutions,omitempty"`
	VideoCodecs []string            `json:"video_codecs,omitempty"`
	BitDepths   []string            `json:"bit_depths,omitempty"`
	Containers  []string            `json:"containers,omitempty"`
	HDR         []string            `json:"hdr,omitempty"`
	CRCs        []string            `json:"crcs,omitempty"`
	AudioFlags  []string            `json:"audio_flags,omitempty"`
	AudioTracks []ContentAudioTrack `json:"audio_tracks,omitempty"`
	Subtitles   *ContentSubtitles   `json:"subtitles,omitempty"`
}

type ContentAudioTrack struct {
	Language string `json:"language,omitempty"`
	Codec    string `json:"codec,omitempty"`
	Channels string `json:"channels,omitempty"`
	Object   string `json:"object,omitempty"`
}

type ContentSubtitles struct {
	Languages []string `json:"languages,omitempty"`
	Types     []string `json:"types,omitempty"`
	Formats   []string `json:"formats,omitempty"`
}

func ProjectContent(result *ParseResult) ContentResult {
	values := func(kind AttrType) []string {
		out := make([]string, 0, 2)
		for _, attr := range result.Attributes {
			if attr.Type == kind && strings.TrimSpace(attr.Normalized) != "" {
				out = appendUnique(out, strings.TrimSpace(attr.Normalized))
			}
		}
		return out
	}

	return ContentResult{
		Content:  result.CanonicalTitle(),
		Aliases:  values(AttrTitleAlias),
		Groups:   appendUnique(values(AttrGroup), values(AttrTailGroup)...),
		Versions: values(AttrVersion),
		Episode:  projectContentEpisode(result, values),
		Movie:    projectContentMovie(result),
		Media:    projectContentMedia(result, values),
	}
}

func projectContentMovie(result *ParseResult) *ContentMovie {
	index := result.CanonicalMovieIndex()
	rangeValue := result.CanonicalMovieIndexRange()
	if index == "" && rangeValue == "" {
		return nil
	}
	movie := &ContentMovie{Index: index}
	if rangeValue != "" {
		movie.Range = splitContentRange(rangeValue)
	}
	return movie
}

func projectContentEpisode(result *ParseResult, values func(AttrType) []string) *ContentEpisode {
	semantics := result.CanonicalEPSemantics()
	if semantics == nil && len(values(AttrEpisode)) == 0 && len(values(AttrSeason)) == 0 {
		return nil
	}
	ep := &ContentEpisode{
		Season: result.CanonicalSeason(), SeasonRange: result.CanonicalSeasonRange(),
		Number: result.CanonicalEpisode(), Numbers: values(AttrEpisodeList), Special: result.CanonicalSPKind(),
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
		if ranges := values(AttrEpisodeRange); len(ranges) > 0 {
			ep.Range = splitContentRange(ranges[0])
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

func projectContentMedia(result *ParseResult, values func(AttrType) []string) *ContentMedia {
	media := &ContentMedia{
		Platforms: values(AttrPlatform), SourceTypes: values(AttrSourceType), Resolutions: values(AttrResolution),
		VideoCodecs: values(AttrVideoCodec), BitDepths: values(AttrBitDepth), Containers: values(AttrContainer),
		HDR: values(AttrHDR), CRCs: values(AttrCRC), AudioFlags: values(AttrAudioFlag),
	}
	if result.AudioSlot != nil {
		for _, track := range result.AudioSlot.Tracks {
			media.AudioTracks = append(media.AudioTracks, ContentAudioTrack{
				Language: track.Language, Codec: track.Codec, Channels: track.Channels, Object: track.Object,
			})
		}
	}
	codecs, channels := values(AttrAudioCodec), values(AttrAudioChannels)
	languages, objects := values(AttrAudioLang), values(AttrAudioObject)
	trackCount := max(len(media.AudioTracks), len(codecs), len(channels), len(languages), len(objects))
	for len(media.AudioTracks) < trackCount {
		media.AudioTracks = append(media.AudioTracks, ContentAudioTrack{})
	}
	for i := range media.AudioTracks {
		track := &media.AudioTracks[i]
		if track.Codec == "" {
			track.Codec = contentValueAt(codecs, i)
		}
		if track.Channels == "" {
			track.Channels = contentValueAt(channels, i)
		}
		if track.Language == "" {
			track.Language = contentValueAt(languages, i)
		}
		if track.Object == "" {
			track.Object = contentValueAt(objects, i)
		}
	}
	media.Subtitles = projectContentSubtitles(values)
	if len(media.Platforms) == 0 && len(media.SourceTypes) == 0 && len(media.Resolutions) == 0 && len(media.VideoCodecs) == 0 && len(media.BitDepths) == 0 && len(media.Containers) == 0 && len(media.HDR) == 0 && len(media.CRCs) == 0 && len(media.AudioFlags) == 0 && len(media.AudioTracks) == 0 && media.Subtitles == nil {
		return nil
	}
	return media
}

func projectContentSubtitles(values func(AttrType) []string) *ContentSubtitles {
	subtitles := &ContentSubtitles{
		Languages: appendUnique(values(AttrSubLang), values(AttrSubLangGroup)...),
		Types:     values(AttrSubFlag), Formats: values(AttrSubFormat),
	}
	if len(subtitles.Languages) == 0 && len(subtitles.Types) == 0 && len(subtitles.Formats) == 0 {
		return nil
	}
	return subtitles
}

func splitContentRange(value string) *ContentRange {
	parts := strings.SplitN(value, "-", 2)
	result := &ContentRange{Start: strings.TrimSpace(parts[0])}
	if len(parts) == 2 {
		result.End = strings.TrimSpace(parts[1])
	}
	return result
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

func contentValueAt(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return ""
}
