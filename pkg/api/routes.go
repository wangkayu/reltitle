package api

import (
	"github.com/gin-gonic/gin"
	"title-parser/pkg/parser"
)

// RegisterRoutes 注册所有 API 路由
func RegisterRoutes(r *gin.Engine, p *parser.Parser) {
	handler := NewHandler(p)

	// API v1
	v1 := r.Group("/api/v1")
	{
		v1.POST("/parse", handler.Parse)
		v1.POST("/parse/batch", handler.ParseBatch)
	}

	// Health check
	r.GET("/health", handler.Health)
}
