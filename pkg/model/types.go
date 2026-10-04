package model

// AttrType names the category of an Attribute, e.g. "title", "group",
// "resolution". See the Attr* constants for the full set.
type AttrType string

// Attribute is one recognized piece of a release title: a typed, normalized
// value with the token span it came from, a heuristic confidence, and the
// pipeline stage that produced it (Source, e.g. "lexicon", "canonical",
// "heuristic"). TokenStart/TokenEnd are -1 when no span applies.
type Attribute struct {
	Type       AttrType `json:"type"`
	Normalized string   `json:"normalized,omitempty"`
	TokenStart int      `json:"token_start,omitempty"`
	TokenEnd   int      `json:"token_end,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Source     string   `json:"source,omitempty"`
}

// Token is one lexical piece of the raw title produced by tokenize.Tokenize.
// Removed means some extractor has claimed the token; it does NOT mean later
// stages can ignore it (several stages intentionally read consumed tokens).
type Token struct {
	Text    string `json:"text"`
	Removed bool   `json:"removed,omitempty"`

	// Protected marks tokens that belong to a known title span (from title_knowledge.yaml).
	// Episode/season/range detection must never reinterpret them.
	Protected bool `json:"-"`
}

// ParseResult is the full parse output for one release title. The CLI
// serializes it as indented JSON; empty fields are omitted. Episode-related
// attributes are not serialized into Attributes — they live in EPSemantics.
type ParseResult struct {
	RawTitle           string       `json:"raw_title"`
	ContentKind        string       `json:"content_kind,omitempty"`
	ContentConfidence  float64      `json:"content_confidence,omitempty"`
	TemplateID         string       `json:"template_id,omitempty"`
	TemplateConfidence float64      `json:"template_confidence,omitempty"`
	EPSemantics        *EPSemantics `json:"ep_semantics,omitempty"`
	AudioSlot          *AudioSlot   `json:"audio_slot,omitempty"`
	Attributes         []Attribute  `json:"attributes,omitempty"`
	Tokens             []Token      `json:"tokens,omitempty"`
}

// EPReference is one episode/season/range conclusion: the primary parse
// result or an alternate candidate. TokenStart/TokenEnd are internal only.
type EPReference struct {
	Kind        string   `json:"kind,omitempty"` // NONE | SEASON | EP | EP_LIST | RANGE | SEASON_RANGE | SP
	Axis        string   `json:"axis,omitempty"` // SE | ABS | TV | OVA | SP | UNKNOWN
	Season      string   `json:"season,omitempty"`
	SeasonRange string   `json:"season_range,omitempty"` // e.g. S1-S3
	Episode     string   `json:"episode,omitempty"`
	EpisodeList []string `json:"episode_list,omitempty"`
	RangeStart  string   `json:"range_start,omitempty"`
	RangeEnd    string   `json:"range_end,omitempty"`
	SPKind      string   `json:"sp_kind,omitempty"`
	Relation    string   `json:"relation,omitempty"` // overall | catalog | alt_cut | etc
	Source      string   `json:"source,omitempty"`

	TokenStart int `json:"-"`
	TokenEnd   int `json:"-"`
}

// EPSemantics carries the episode-related outcome of a parse: Primary is the
// chosen conclusion, Alternates the rejected candidates.
type EPSemantics struct {
	Primary    *EPReference  `json:"primary,omitempty"`
	Alternates []EPReference `json:"alternates,omitempty"`

	// Legacy mirrors remain available to Go callers for now, but are no longer
	// serialized. Canonical accessors should prefer Primary.
	Kind        string   `json:"-"`
	Axis        string   `json:"-"`
	Season      string   `json:"-"`
	SeasonRange string   `json:"-"`
	Episode     string   `json:"-"`
	EpisodeList []string `json:"-"`
	RangeStart  string   `json:"-"`
	RangeEnd    string   `json:"-"`
	SPKind      string   `json:"-"`
	Source      string   `json:"-"`
}

// EPSlot is the internal, pre-output form of EPSemantics used while the
// parser adjudicates between episode candidates.
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

// AudioSlot is the recognized set of audio tracks on the release.
type AudioSlot struct {
	Tracks []AudioTrack `json:"tracks,omitempty"`
}

// AudioTrack describes one audio track: language, codec, channel layout and
// object audio (e.g. ATMOS), with the token span and producing stage.
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
	AttrTitle           AttrType = "title"
	AttrTitleAlias      AttrType = "title_alias"
	AttrSearchHint      AttrType = "search_hint"
	AttrGroup           AttrType = "group"
	AttrEpisode         AttrType = "episode"
	AttrEpisodeList     AttrType = "episode_list"
	AttrEpisodeRange    AttrType = "episode_range"
	AttrSeason          AttrType = "season"
	AttrSeasonRange     AttrType = "season_range"
	AttrVersion         AttrType = "version"
	AttrEdition         AttrType = "edition"
	AttrReleaseStatus   AttrType = "release_status"
	AttrPackKind        AttrType = "pack_kind"
	AttrPackProfile     AttrType = "pack_profile"
	AttrSPKind          AttrType = "sp_kind"
	AttrYear            AttrType = "year"
	AttrMovieIndex      AttrType = "movie_index"
	AttrMovieIndexRange AttrType = "movie_index_range"
	AttrMovieFlag       AttrType = "movie_flag"
	AttrSpecialFlag     AttrType = "special_flag"
	AttrSpecialIndex    AttrType = "special_index"
	AttrContents        AttrType = "contents"

	AttrSourceType     AttrType = "source_type"
	AttrResolution     AttrType = "resolution"
	AttrVideoCodec     AttrType = "video_codec"
	AttrAudioCodec     AttrType = "audio_codec"
	AttrBitDepth       AttrType = "bit_depth"
	AttrContainer      AttrType = "container"
	AttrPlatform       AttrType = "platform"
	AttrPlatformRegion AttrType = "platform_region"
	AttrOriginRegion   AttrType = "origin_region"
	AttrHDR            AttrType = "hdr"
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
