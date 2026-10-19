package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newRepoTestRouter(t *testing.T, handler http.Handler) (http.Handler, func()) {
	t.Helper()
	ghServer := httptest.NewServer(handler)
	cfg := Config{
		Owner:        "take-fish",
		Repos:        []string{"tf-gateway"},
		PollInterval: 30 * time.Second,
	}
	gh := NewGitHub(ghServer.URL, "")
	server := NewServer(cfg, NewSnapshotStore(), gh)
	return setupRouter(server), ghServer.Close
}

func TestRepoBranches(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/repos/take-fish/tf-gateway/branches", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"name":"master","protected":true,"commit":{"sha":"1234567890abcdef","url":"u1"}},
			{"name":"feature/portal","protected":false,"commit":{"sha":"abcdef1234567890","url":"u2"}}
		]`))
	})
	router, closeFn := newRepoTestRouter(t, handler)
	defer closeFn()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/repos/tf-gateway/branches", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Branches []BranchView `json:"branches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Branches) != 2 || body.Branches[0].Name != "master" {
		t.Fatalf("unexpected branches: %+v", body.Branches)
	}
	if body.Branches[1].ShortSHA != "abcdef1" {
		t.Fatalf("short sha = %q, want abcdef1", body.Branches[1].ShortSHA)
	}
}

func TestRepoCommits(t *testing.T) {
	var gotQuery string
	handler := http.NewServeMux()
	handler.HandleFunc("/repos/take-fish/tf-gateway/commits", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("sha")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"sha":"1234567890abcdef",
			"commit":{"message":"feat: portal\n\nbody","author":{"name":"Alice","date":"2026-10-19T01:00:00Z"}},
			"author":{"login":"alice","avatar_url":"https://example.com/a.png"},
			"html_url":"https://github.com/take-fish/tf-gateway/commit/123"
		}]`))
	})
	router, closeFn := newRepoTestRouter(t, handler)
	defer closeFn()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(
		http.MethodGet,
		"/api/v1/repos/tf-gateway/commits?branch=feature/portal&limit=5",
		nil,
	)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if gotQuery != "feature/portal" {
		t.Fatalf("sha query = %q, want feature/portal", gotQuery)
	}
	var body struct {
		Branch  string       `json:"branch"`
		Commits []CommitView `json:"commits"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Branch != "feature/portal" || len(body.Commits) != 1 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.Commits[0].Author != "alice" || body.Commits[0].Message != "feat: portal" {
		t.Fatalf("unexpected commit: %+v", body.Commits[0])
	}
}

func TestRepoUnknownAndUpstreamErrors(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/repos/take-fish/tf-gateway/branches", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream boom", http.StatusInternalServerError)
	})
	router, closeFn := newRepoTestRouter(t, handler)
	defer closeFn()

	unknown := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/repos/unknown/branches", nil)
	router.ServeHTTP(unknown, req)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown repo status = %d, want 404", unknown.Code)
	}

	failed := httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/repos/tf-gateway/branches", nil)
	router.ServeHTTP(failed, req)
	if failed.Code != http.StatusBadGateway {
		t.Fatalf("upstream failure status = %d, want 502; body=%s", failed.Code, failed.Body.String())
	}
}

func TestRepoInvalidBranchAndLimit(t *testing.T) {
	handler := http.NewServeMux()
	router, closeFn := newRepoTestRouter(t, handler)
	defer closeFn()

	longBranch := make([]byte, 300)
	for i := range longBranch {
		longBranch[i] = 'a'
	}
	for _, path := range []string{
		"/api/v1/repos/tf-gateway/commits?branch=" + string(longBranch),
		"/api/v1/repos/tf-gateway/commits?branch=master&limit=nope",
	} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", path, w.Code)
		}
	}
}
