package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// distFS 内嵌前端构建产物。
// dist/.gitkeep 是提交进仓库的占位（go:embed 在编译期要求目录存在），
// 真实产物由 CI / Dockerfile 的 node 阶段生成；本地纯后端开发时页面会提示"前端未构建"。
//
//go:embed all:dist
var distFS embed.FS

// Server 提供 API 与前端静态页
type Server struct {
	cfg     Config
	store   *SnapshotStore
	started time.Time
}

func NewServer(cfg Config, store *SnapshotStore) *Server {
	return &Server{cfg: cfg, store: store, started: time.Now()}
}

func setupRouter(s *Server) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "tf-console",
			"version": version,
			"uptime":  time.Since(s.started).Round(time.Second).String(),
		})
	})

	api := r.Group("/api/v1")
	{
		api.GET("/pipeline", func(c *gin.Context) {
			c.JSON(http.StatusOK, s.store.Get())
		})
	}

	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	// SPA fallback：/api/ 之外的未命中路径返回前端页面
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "not found"})
			return
		}
		// 真实静态文件（assets/*.js|css 等）直接返回，长缓存
		if p != "/" {
			if f, err := sub.Open(strings.TrimPrefix(p, "/")); err == nil {
				st, _ := f.Stat()
				_ = f.Close()
				if st != nil && !st.IsDir() {
					if strings.HasPrefix(p, "/assets/") {
						c.Header("Cache-Control", "public, max-age=31536000, immutable")
					}
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
			}
		}
		// 其余路径回落 index.html（前端路由由 SPA 自理）
		c.Header("Cache-Control", "no-cache")
		index, err := fs.ReadFile(distFS, "dist/index.html")
		if err != nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8",
				[]byte("<!doctype html><meta charset=utf-8><title>tf-console</title><p>tf-console 前端未构建：请先执行 <code>npm install && npm run build-only</code></p>"))
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})

	return r
}
