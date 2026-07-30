package api

import (
	"github.com/gin-gonic/gin"
	"title-parser/pkg/parser"
)

// RegisterRoutes 注册所有 API 路由
func RegisterRoutes(r *gin.Engine, p *parser.Parser) {
	handler := NewHandler(p)
	r.GET("/", handler.Page)

	// Public API v1: only the compact business response is exposed.
	v1 := r.Group("/api/v1")
	{
		v1.POST("/content", handler.Content)
	}

	// Health check
	r.GET("/health", handler.Health)
}
