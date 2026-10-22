package main

import (
	"os"
	"strings"
	"time"
)

// DefaultRepos 流水线页默认展示的仓库列表（顺序即展示顺序）
var DefaultRepos = []string{
	"tf-gateway", "tf-edge-node",
	"tf-user", "tf-chat", "tf-reader", "tf-game", "tf-music",
	"tf-agent", "tf-web",
	"tf-deploy", "tf-console",
}

// Config 服务运行配置（全部来自环境变量，默认值适配集中机部署）
type Config struct {
	Port         string
	Owner        string
	Token        string
	APIBase      string
	Repos        []string
	PollInterval time.Duration
	TestCI       bool // 是否展示 test 分支的 CI 状态（TF_CONSOLE_TEST_CI=0 可关）
	LokiURL      string
	LogToken     string // 非空时日志检索必须携带 X-Log-Token
}

// LoadConfig 从环境变量装载配置
func LoadConfig() Config {
	cfg := Config{
		Port:         getenv("PORT", "8301"),
		Owner:        getenv("GITHUB_OWNER", "take-fish"),
		Token:        os.Getenv("GITHUB_TOKEN"),
		APIBase:      getenv("GITHUB_API_BASE", "https://api.github.com"),
		PollInterval: 30 * time.Second,
		TestCI:       os.Getenv("TF_CONSOLE_TEST_CI") != "0",
		LokiURL:      strings.TrimSpace(os.Getenv("LOKI_URL")),
		LogToken:     os.Getenv("TF_CONSOLE_LOG_TOKEN"),
	}
	if v := os.Getenv("TF_CONSOLE_REPOS"); v != "" {
		cfg.Repos = splitTrim(v)
	} else {
		cfg.Repos = DefaultRepos
	}
	// POLL_INTERVAL 仅作为下限参考；实际轮询间隔由配额自适应（见 poller.go）
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= 10*time.Second {
			cfg.PollInterval = d
		}
	}
	return cfg
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
