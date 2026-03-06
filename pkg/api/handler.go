package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
		Success: true,
		Result:  result,
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
				Result:  result,
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
