package model

type AttrType string

type Attribute struct {
	Type       AttrType `json:"type"`
	Normalized string   `json:"normalized,omitempty"`
	TokenStart int      `json:"token_start,omitempty"`
	TokenEnd   int      `json:"token_end,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Source     string   `json:"source,omitempty"`
}

type Token struct {
	Text    string `json:"text"`
	Removed bool   `json:"removed,omitempty"`
}

type ParseResult struct {
	RawTitle           string      `json:"raw_title"`
	ContentKind        string      `json:"content_kind,omitempty"`
	ContentConfidence  float64     `json:"content_confidence,omitempty"`
	TemplateID         string      `json:"template_id,omitempty"`
	TemplateConfidence float64     `json:"template_confidence,omitempty"`
	EPSlot             *EPSlot     `json:"epslot,omitempty"`
	AudioSlot          *AudioSlot  `json:"audio_slot,omitempty"`
	Attributes         []Attribute `json:"attributes,omitempty"`
	Tokens             []Token     `json:"tokens,omitempty"`
}

type EPSlot struct {
	Primary    *EPEntity  `json:"primary,omitempty"`
	Candidates []EPEntity `json:"candidates,omitempty"`
}

// EPEntity represents one episode-like candidate. It is intentionally small and
// stable, so we can support composite epslot without encoding semantics into
// Attribute.Normalized strings.
type EPEntity struct {
	Kind         string `json:"kind,omitempty"` // EP | EP_LIST | RANGE | SP
	Axis         string `json:"axis,omitempty"` // SE | ABS | TV | OVA | SP | UNKNOWN
	Season       string `json:"season,omitempty"`
	Episode      string `json:"episode,omitempty"`
	EpisodeRange string `json:"episode_range,omitempty"`
	EpisodeList  string `json:"episode_list,omitempty"` // comma-separated list, e.g. "804,805"
	SPKind       string `json:"sp_kind,omitempty"`      // OVA/OAD/ONA/EX/SP
	TokenStart   int    `json:"token_start,omitempty"`
	TokenEnd     int    `json:"token_end,omitempty"`
	Source       string `json:"source,omitempty"` // sxxexx/episode/episode_range/sp_kind/etc
}

type AudioSlot struct {
	Tracks []AudioTrack `json:"tracks,omitempty"`
}

type AudioTrack struct {
	Language   string `json:"language,omitempty"`
	Codec      string `json:"codec,omitempty"`
	Channels   string `json:"channels,omitempty"`
	Object     string `json:"object,omitempty"` // ATMOS etc
	TokenStart int    `json:"token_start,omitempty"`
	TokenEnd   int    `json:"token_end,omitempty"`
	Source     string `json:"source,omitempty"` // tech_audio_slot/etc
}

const (
	AttrTitle        AttrType = "title"
	AttrTitleAlias   AttrType = "title_alias"
	AttrSearchHint   AttrType = "search_hint"
	AttrGroup        AttrType = "group"
	AttrEpisode      AttrType = "episode"
	AttrEpisodeList  AttrType = "episode_list"
	AttrEpisodeRange AttrType = "episode_range"
	AttrSeason       AttrType = "season"
	AttrSeasonRange  AttrType = "season_range"
	AttrVersion      AttrType = "version"
	AttrPackKind     AttrType = "pack_kind"
	AttrSPKind       AttrType = "sp_kind"
	AttrYear         AttrType = "year"
	AttrMovieIndex   AttrType = "movie_index"
	AttrMovieFlag    AttrType = "movie_flag"
	AttrSpecialFlag  AttrType = "special_flag"
	AttrSpecialIndex AttrType = "special_index"
	AttrContents     AttrType = "contents"

	AttrSourceType     AttrType = "source_type"
	AttrResolution     AttrType = "resolution"
	AttrVideoCodec     AttrType = "video_codec"
	AttrAudioCodec     AttrType = "audio_codec"
	AttrBitDepth       AttrType = "bit_depth"
	AttrContainer      AttrType = "container"
	AttrPlatform       AttrType = "platform"
	AttrPlatformRegion AttrType = "platform_region"
	AttrOriginRegion   AttrType = "origin_region"
	AttrCRC            AttrType = "crc"
	AttrTech           AttrType = "tech"
	AttrTailGroup      AttrType = "tail_group"

	AttrNote          AttrType = "note"
	AttrNoise         AttrType = "noise"
	AttrSubFlag       AttrType = "sub_flag"
	AttrSubLang       AttrType = "sub_lang"
	AttrSubFormat     AttrType = "sub_format"
	AttrSubLangGroup  AttrType = "sub_lang_group"
	AttrAudioFlag     AttrType = "audio_flag"
	AttrAudioLang     AttrType = "audio_lang"
	AttrAudioChannels AttrType = "audio_channels"
	AttrAudioObject   AttrType = "audio_object"
	AttrContentRating AttrType = "content_rating"
)
