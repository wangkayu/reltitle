package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"title-parser/pkg/model"
	"title-parser/pkg/parser"
)

type Handler struct {
	parser *parser.Parser
}

func NewHandler(p *parser.Parser) *Handler {
	return &Handler{parser: p}
}

// POST /api/v1/parse
func (h *Handler) Parse(c *gin.Context) {
	var req ParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	startTime := time.Now()

	// 调用 parser
	result, err := h.parser.Parse(req.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	parseTime := time.Since(startTime).Milliseconds()

	c.JSON(http.StatusOK, ParseResponse{
		Success:  true,
		Result:   convertToAPIResult(result),
		Metadata: &ParseMetadata{
			ParseTimeMs:     parseTime,
			TemplateMatched: result.TemplateID,
			ContentType:     result.ContentKind,
		},
	})
}

// POST /api/v1/parse/batch
func (h *Handler) ParseBatch(c *gin.Context) {
	var req BatchParseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	startTime := time.Now()
	results := make([]BatchParseResult, len(req.Titles))
	successCount := 0

	for i, title := range req.Titles {
		result, err := h.parser.Parse(title)
		if err != nil {
			results[i] = BatchParseResult{
				Index:   i,
				Success: false,
				Error:   err.Error(),
			}
		} else {
			results[i] = BatchParseResult{
				Index:   i,
				Success: true,
				Result:  convertToAPIResult(result),
			}
			successCount++
		}
	}

	totalTime := time.Since(startTime).Milliseconds()
	avgTime := int64(0)
	if len(req.Titles) > 0 {
		avgTime = totalTime / int64(len(req.Titles))
	}

	c.JSON(http.StatusOK, BatchParseResponse{
		Success: true,
		Results: results,
		Summary: &BatchSummary{
			Total:       len(req.Titles),
			Success:     successCount,
			Failed:      len(req.Titles) - successCount,
			TotalTimeMs: totalTime,
			AvgTimeMs:   avgTime,
		},
	})
}

// GET /health
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":        "healthy",
		"version":       "2.0.0",
		"config_loaded": true,
	})
}

// 辅助函数：转换 parser 结果到 API 结果
func convertToAPIResult(result *model.ParseResult) *ParseResult {
	if result == nil {
		return nil
	}

	apiResult := &ParseResult{
		Title:        result.Title,
		TitleRaw:     result.RawTitle,
		Group:        result.Group,
		Episode:      result.Episode,
		EpisodeRange: result.EpisodeRange,
		SeasonRange:  result.SeasonRange,
		Version:      result.Version,
		SPKind:       result.SPKind,
		PackKind:     result.PackKind,
		ContentKind:  result.ContentKind,
		Confidence:   result.ContentConfidence,
	}

	// 转换 Season (string -> *int)
	if result.Season != "" {
		if seasonInt, err := strconv.Atoi(result.Season); err == nil {
			apiResult.Season = &seasonInt
		}
	}

	// 从 Attributes 中提取技术信息
	for _, attr := range result.Attributes {
		switch attr.Type {
		case model.AttrResolution:
			apiResult.Resolution = attr.Normalized
		case model.AttrVideoCodec:
			apiResult.VideoCodec = attr.Normalized
		case model.AttrAudioCodec:
			apiResult.AudioCodec = attr.Normalized
		case model.AttrSourceType:
			apiResult.Source = attr.Normalized
		case model.AttrSubLang:
			apiResult.SubtitleLang = attr.Normalized
		}
	}

	// 从 AudioSlot 中提取音频信息
	if result.AudioSlot != nil && len(result.AudioSlot.Tracks) > 0 {
		var tracks []AudioTrack
		for _, track := range result.AudioSlot.Tracks {
			tracks = append(tracks, AudioTrack{
				Lang:     track.Language,
				Codec:    track.Codec,
				Channels: track.Channels,
			})
		}
		apiResult.AudioInfo = &AudioInfo{Tracks: tracks}
		// 如果 AudioCodec 为空，也从 AudioSlot 填充
		if apiResult.AudioCodec == "" {
			apiResult.AudioCodec = result.AudioSlot.Tracks[0].Codec
		}
	}

	// 从 Attributes 中提取字幕信息，构建结构化 SubtitleInfo
	var subFlag string
	var subLangs []string
	var subGroups []string
	var subFormat string
	for _, attr := range result.Attributes {
		switch attr.Type {
		case model.AttrSubFlag:
			if attr.Normalized != "" {
				subFlag = attr.Normalized
			}
		case model.AttrSubLang:
			if attr.Normalized != "" {
				subLangs = append(subLangs, attr.Normalized)
			}
		case model.AttrSubLangGroup:
			if attr.Normalized != "" {
				subGroups = append(subGroups, attr.Normalized)
			}
		case model.AttrSubFormat:
			if attr.Normalized != "" && subFormat == "" {
				subFormat = attr.Normalized
			}
		}
	}
	// 如果有字幕信息，构建 SubtitleInfo
	if subFlag != "" || len(subLangs) > 0 || len(subGroups) > 0 {
		subInfo := &SubtitleInfo{}
		// 转换 flag 到 type
		switch subFlag {
		case "内封":
			subInfo.Type = "soft"
		case "内嵌":
			subInfo.Type = "hard"
		case "外挂":
			subInfo.Type = "external"
		case "RAW", "NOSUB":
			subInfo.Type = "raw"
		case "MULTI_SUBS":
			subInfo.Type = "multi"
		}
		subInfo.Langs = subLangs
		subInfo.Groups = subGroups
		subInfo.Format = subFormat
		apiResult.SubtitleInfo = subInfo
	}

	return apiResult
}

// JoinStrings joins a slice of strings with a separator
func JoinStrings(parts []string, sep string) string {
	return strings.Join(parts, sep)
}
