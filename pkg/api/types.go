package api

import "title-parser/pkg/model"

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

// ParseResult 解析结果（canonical 输出）
type ParseResult = model.ParseResult

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
