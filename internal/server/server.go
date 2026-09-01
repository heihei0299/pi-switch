package server

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/config"
)

func configPath() string {
	if p := os.Getenv("PI_SWITCH_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/pi-switch-config.json"
	}
	return home + "/.pi-switch/config.json"
}

func NewProxyRouter() *gin.Engine {
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	return r
}

func NewMgmtRouter() *gin.Engine {
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(`<html><body style="font-family:system-ui;padding:32px"><h1>pi-switch Go</h1><p>embed.FS placeholder for webui/dist</p><p style="color:#888">PROTOTYPE — will be replaced by webui/dist</p></body></html>`))
	})
	r.GET("/api/config", func(c *gin.Context) {
		cfg, src, _ := config.LoadConfigAtPath(configPath())
		c.JSON(200, gin.H{"source": src, "config": cfg})
	})
	r.GET("/api/profiles", func(c *gin.Context) {
		cfg, src, _ := config.LoadConfigAtPath(configPath())
		c.JSON(200, gin.H{"source": src, "config": cfg})
	})
	r.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"error": "not found"})
	})
	return r
}
