package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(snap Snapshot) *Server {
	cfg := Config{
		Owner:        "take-fish",
		Repos:        []string{"tf-gateway"},
		PollInterval: 30 * time.Second,
	}
	store := NewSnapshotStore()
	store.Set(snap)
	return NewServer(cfg, store)
}

func TestHealthz(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	setupRouter(newTestServer(Snapshot{})).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if body["service"] != "tf-console" {
		t.Fatalf("service 字段错误: %v", body["service"])
	}
}

func TestPipelineEndpoint(t *testing.T) {
	snap := Snapshot{
		GeneratedAt: time.Now().UTC(),
		Repos:       []RepoStatus{{Name: "tf-gateway", HTMLURL: "https://github.com/take-fish/tf-gateway"}},
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/pipeline", nil)
	setupRouter(newTestServer(snap)).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/pipeline = %d, want 200", w.Code)
	}
	var got Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("快照反序列化失败: %v", err)
	}
	if len(got.Repos) != 1 || got.Repos[0].Name != "tf-gateway" {
		t.Fatalf("快照内容错误: %+v", got.Repos)
	}
}

func TestSPAFallback(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	setupRouter(newTestServer(Snapshot{})).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html（dist 未构建时应返回提示页）", ct)
	}
}

func TestAPINotFound(t *testing.T) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/nope", nil)
	setupRouter(newTestServer(Snapshot{})).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/nope = %d, want 404", w.Code)
	}
}
