package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"title-parser/pkg/api"
	"title-parser/pkg/config"
	"title-parser/pkg/parser"
)

func main() {
	// 加载配置目录
	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "./configs"
	}

	log.Printf("Loading parser configuration from: %s", configDir)

	// 加载配置
	cfg, err := config.LoadAll(configDir)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 初始化 parser
	p := parser.New(cfg)

	log.Println("Parser initialized successfully")

	// 设置 Gin 模式
	ginMode := os.Getenv("GIN_MODE")
	if ginMode != "" {
		gin.SetMode(ginMode)
	}

	// 创建 HTTP 服务器
	r := gin.Default()
	if err := r.SetTrustedProxies(nil); err != nil {
		log.Fatalf("Failed to configure trusted proxies: %v", err)
	}

	// 注册路由
	api.RegisterRoutes(r, p)

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Title Parser Service on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
