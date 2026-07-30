package api

type ParseRequest struct {
	Title string `json:"title" binding:"required"`
}

// ContentResponse is the stable, user-facing projection of a parse result.
// Internal parser strategy, confidence, source and token data are intentionally omitted.
type ContentResponse struct {
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
