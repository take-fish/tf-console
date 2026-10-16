package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGetJSONConditionalRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Reset", "2000000000")
		if atomic.AddInt32(&hits, 1) == 1 {
			w.Header().Set("ETag", `"abc"`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"tf-gateway"}`))
			return
		}
		if got := r.Header.Get("If-None-Match"); got != `"abc"` {
			t.Errorf("第二次请求未携带 If-None-Match: %q", got)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	gh := NewGitHub(srv.URL, "tkn")
	ctx := context.Background()
	var out struct {
		Name string `json:"name"`
	}

	if err := gh.getJSON(ctx, "/repos/x/y", &out); err != nil {
		t.Fatalf("第一轮请求失败: %v", err)
	}
	if out.Name != "tf-gateway" {
		t.Fatalf("第一轮解码错误: %+v", out)
	}

	out.Name = ""
	if err := gh.getJSON(ctx, "/repos/x/y", &out); err != nil {
		t.Fatalf("第二轮（304 复用缓存）失败: %v", err)
	}
	if out.Name != "tf-gateway" {
		t.Fatalf("304 后未用缓存回填: %+v", out)
	}
	if n := atomic.LoadInt32(&hits); n != 2 {
		t.Fatalf("请求次数 = %d, want 2", n)
	}
	if r := gh.RateLimit(); r.Limit != 5000 || r.Remaining != 4999 {
		t.Fatalf("配额解析错误: %+v", r)
	}
}

func TestGetJSONNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	gh := NewGitHub(srv.URL, "")
	var out any
	if err := gh.getJSON(context.Background(), "/x", &out); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 应返回 ErrNotFound, 得到 %v", err)
	}
}

func TestGetJSONErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer srv.Close()

	gh := NewGitHub(srv.URL, "")
	err := gh.getJSON(context.Background(), "/x", &struct{}{})
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("应返回 apiError, 得到 %v", err)
	}
	if apiErr.Status != http.StatusForbidden {
		t.Fatalf("Status = %d, want 403", apiErr.Status)
	}
	if apiErr.RetryAfter.Seconds() != 7 {
		t.Fatalf("RetryAfter = %v, want 7s", apiErr.RetryAfter)
	}
}
