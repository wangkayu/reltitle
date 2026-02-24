package api

// ParseOptions 解析选项
type ParseOptions struct {
	ExtractTitle     bool `json:"extract_title"`
	ExtractEpisode   bool `json:"extract_episode"`
	ExtractTechnical bool `json:"extract_technical"`
	ExtractGroup     bool `json:"extract_group"`
}

// ParseRequest 解析请求
type ParseRequest struct {
	Title       string        `json:"title" binding:"required"`
	Options     *ParseOptions `json:"options"`
	KnownGroups []KnownGroup  `json:"known_groups,omitempty"`
}

// KnownGroup represents a known subtitle group with its aliases
type KnownGroup struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
}

// AudioInfo 结构化音频信息
type AudioInfo struct {
	Tracks []AudioTrack `json:"tracks,omitempty"`
}

// AudioTrack 单个音轨
type AudioTrack struct {
	Lang     string `json:"lang,omitempty"`     // JPN, YUE, ENG
	Codec    string `json:"codec,omitempty"`    // AAC, FLAC, OPUS
	Channels string `json:"channels,omitempty"` // 2.0, 5.1, 7.1
}

// SubtitleInfo 结构化字幕信息
type SubtitleInfo struct {
	Type   string   `json:"type,omitempty"`   // soft, hard, external, raw, multi
	Langs  []string `json:"langs,omitempty"`  // ["CHS", "JPN"]
	Groups []string `json:"groups,omitempty"` // ["CHS+JPN", "CHT+JPN"]
	Format string   `json:"format,omitempty"` // ASS, PGS, SRT
}

// ParseResult 解析结果
type ParseResult struct {
	Title        string         `json:"title,omitempty"`
	TitleRaw     string         `json:"title_raw,omitempty"`
	Group        string         `json:"group,omitempty"`
	Episode      string         `json:"episode,omitempty"`
	EpisodeRange string         `json:"episode_range,omitempty"`
	Season       *int           `json:"season,omitempty"`
	SeasonRange  string         `json:"season_range,omitempty"`
	Version      string         `json:"version,omitempty"`
	SPKind       string         `json:"sp_kind,omitempty"`
	PackKind     string         `json:"pack_kind,omitempty"`
	ContentKind  string         `json:"content_kind,omitempty"`
	Resolution   string         `json:"resolution,omitempty"`
	VideoCodec   string         `json:"video_codec,omitempty"`
	AudioCodec   string         `json:"audio_codec,omitempty"`
	AudioInfo    *AudioInfo     `json:"audio_info,omitempty"`
	SubtitleLang string         `json:"subtitle_lang,omitempty"`
	SubtitleInfo *SubtitleInfo  `json:"subtitle_info,omitempty"`
	Source       string         `json:"source,omitempty"`
	Confidence   float64        `json:"confidence,omitempty"`
}

// ParseMetadata 解析元数据
type ParseMetadata struct {
	ParseTimeMs     int64  `json:"parse_time_ms"`
	TemplateMatched string `json:"template_matched,omitempty"`
	ContentType     string `json:"content_type,omitempty"`
}

// ParseResponse 解析响应
type ParseResponse struct {
	Success  bool           `json:"success"`
	Result   *ParseResult   `json:"result,omitempty"`
	Error    string         `json:"error,omitempty"`
	Metadata *ParseMetadata `json:"metadata,omitempty"`
}

// BatchParseRequest 批量解析请求
type BatchParseRequest struct {
	Titles  []string      `json:"titles" binding:"required"`
	Options *ParseOptions `json:"options"`
}

// BatchParseResult 批量解析单个结果
type BatchParseResult struct {
	Index   int          `json:"index"`
	Success bool         `json:"success"`
	Result  *ParseResult `json:"result,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// BatchSummary 批量解析摘要
type BatchSummary struct {
	Total       int   `json:"total"`
	Success     int   `json:"success"`
	Failed      int   `json:"failed"`
	TotalTimeMs int64 `json:"total_time_ms"`
	AvgTimeMs   int64 `json:"avg_time_ms"`
}

// BatchParseResponse 批量解析响应
type BatchParseResponse struct {
	Success bool               `json:"success"`
	Results []BatchParseResult `json:"results"`
	Summary *BatchSummary      `json:"summary,omitempty"`
}
