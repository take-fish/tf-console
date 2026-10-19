package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // distroless 等精简镜像无时区库，内嵌以确保 TZ=Asia/Shanghai 生效

	"github.com/gin-gonic/gin"
)

// version 由构建时通过 -ldflags "-X main.version=..." 注入
var version = "dev"

func main() {
	cfg := LoadConfig()
	gh := NewGitHub(cfg.APIBase, cfg.Token)
	store := NewSnapshotStore()
	poller := NewPoller(cfg, gh, store)
	server := NewServer(cfg, store, gh)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go poller.Run(ctx)

	gin.SetMode(gin.ReleaseMode)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: setupRouter(server)}
	go func() {
		log.Printf("tf-console %s listening on :%s（%d 个仓库, token=%v）",
			version, cfg.Port, len(cfg.Repos), cfg.Token != "")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
