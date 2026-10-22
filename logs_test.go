package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

var queryIDPattern = regexp.MustCompile(`(?:trace_id|request_id)="([0-9a-f]+)"`)

type fakeLoki struct {
	t             *testing.T
	baseNS        int64
	prodStream    map[string]string
	testStream    map[string]string
	onlyRequestID bool

	mu       sync.Mutex
	requests []url.Values
}

func newFakeLoki(t *testing.T) (*fakeLoki, *httptest.Server) {
	t.Helper()
	fake := &fakeLoki{
		t:      t,
		baseNS: time.Now().Add(-time.Hour).UnixNano(),
		prodStream: map[string]string{
			"stack": "fishmatrix", "container": "fm-user", "service": "tf-user", "env": "production",
			"detected_level": "info",
		},
		testStream: map[string]string{
			"stack": "fishmatrix", "container": "fm-test-user", "service": "tf-user", "env": "test",
		},
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server
}

func (f *fakeLoki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ready" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
		return
	}
	if r.URL.Path == "/loki/api/v1/labels" {
		// 真实流标签只有 Alloy 写入的这几个；env、service 等来自 `| json`。
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   []string{"compose_service", "container", "service_name", "stack", "stream"},
		})
		return
	}
	if r.URL.Path != "/loki/api/v1/query_range" {
		http.NotFound(w, r)
		return
	}

	query := r.URL.Query()
	f.mu.Lock()
	f.requests = append(f.requests, query)
	f.mu.Unlock()

	q := query.Get("query")
	if strings.Contains(q, "| json") {
		match := queryIDPattern.FindStringSubmatch(q)
		if len(match) != 2 {
			f.t.Errorf("查询语句缺少编号：%s", q)
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		id := match[1]
		streams := []lokiStream{}
		if strings.Contains(q, "trace_id=") && !f.onlyRequestID {
			streams = append(streams, lokiStream{
				Stream: f.prodStream,
				Values: [][]string{
					{strconv.FormatInt(f.baseNS+1000, 10), testLogLine(id, "production", "tf-user", "第 107 行", "")},
				},
			})
			// 测试环境的日志来自不同容器，单独成一个 stream。
			streams = append(streams, lokiStream{
				Stream: f.testStream,
				Values: [][]string{
					{strconv.FormatInt(f.baseNS+2000, 10), testLogLine(id, "test", "tf-user", "第 109 行", "")},
				},
			})
		}
		if strings.Contains(q, "request_id=") {
			streams = append(streams, lokiStream{
				Stream: f.prodStream,
				Values: [][]string{
					{strconv.FormatInt(f.baseNS+3000, 10), testLogLine("", "production", "tf-user", "仅 request_id", id)},
				},
			})
		}
		writeLokiResponse(w, streams)
		return
	}

	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	direction := query.Get("direction")
	startNS, _ := strconv.ParseInt(query.Get("start"), 10, 64)
	endNS, _ := strconv.ParseInt(query.Get("end"), 10, 64)

	// 模拟真实 Loki：selector 里出现非流标签时查不到数据。
	for _, bad := range []string{`detected_level="`, `env="`, `service="`} {
		if strings.Contains(q, bad) {
			writeLokiResponse(w, nil)
			return
		}
	}

	stream := f.prodStream
	env := "production"
	if strings.Contains(q, "fm-test-user") {
		stream = f.testStream
		env = "test"
	}
	values := make([][]string, 0, limit)
	for i := 0; i < limit; i++ {
		var ts int64
		if direction == "backward" {
			ts = endNS - int64(limit-i)*int64(time.Millisecond)
		} else {
			ts = startNS + int64(i+1)*int64(time.Millisecond)
		}
		values = append(values, []string{
			strconv.FormatInt(ts, 10),
			testLogLine("ctx", env, "tf-user", fmt.Sprintf("上下文-%s-%d", direction, i), ""),
		})
	}
	writeLokiResponse(w, []lokiStream{{Stream: stream, Values: values}})
}

func (f *fakeLoki) captured() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]url.Values, len(f.requests))
	copy(out, f.requests)
	return out
}

func testLogLine(traceID, env, service, message, requestID string) string {
	fields := map[string]string{
		"@timestamp": "2026-10-22T10:00:00Z",
		"message":    message,
		"level":      "INFO",
		"service":    service,
		"env":        env,
		"trace_id":   traceID,
	}
	if requestID != "" {
		fields["request_id"] = requestID
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func writeLokiResponse(w http.ResponseWriter, streams []lokiStream) {
	if streams == nil {
		streams = []lokiStream{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "streams",
			"result":     streams,
		},
	})
}

func newLogSearchOptions(lines int) logSearchOptions {
	return logSearchOptions{
		ID:           testTraceID,
		Env:          "production",
		EnvLabel:     "prod",
		ContextLines: lines,
		Window:       24 * time.Hour,
		WindowLabel:  "24h",
	}
}

func TestSearchLogsAddsContextAroundMatches(t *testing.T) {
	for _, lines := range []int{30, 50, 100} {
		t.Run(strconv.Itoa(lines), func(t *testing.T) {
			fake, server := newFakeLoki(t)
			client := newLokiClient(server.URL)
			end := time.Now().UTC()
			start := end.Add(-24 * time.Hour)

			resp, err := searchLogs(context.Background(), client, newLogSearchOptions(lines), start, end)
			if err != nil {
				t.Fatalf("searchLogs: %v", err)
			}
			if resp.Matched != 1 {
				t.Fatalf("生产环境命中数 = %d, want 1", resp.Matched)
			}
			if len(resp.Entries) != 1+2*lines {
				t.Fatalf("日志条数 = %d, want %d", len(resp.Entries), 1+2*lines)
			}
			if resp.Env != "prod" || resp.ContextLines != lines {
				t.Errorf("响应参数 = env %q lines %d", resp.Env, resp.ContextLines)
			}

			contextQueries := 0
			for _, query := range fake.captured() {
				if strings.Contains(query.Get("query"), "| json") {
					continue
				}
				if strings.Contains(query.Get("query"), "detected_level") {
					t.Errorf("上下文查询不应包含结构化元数据标签：%s", query.Get("query"))
				}
				contextQueries++
				if query.Get("limit") != strconv.Itoa(lines) {
					t.Errorf("上下文 limit = %s, want %d", query.Get("limit"), lines)
				}
			}
			if contextQueries != 2 {
				t.Errorf("上下文查询次数 = %d, want 2（前后各一次）", contextQueries)
			}

			// 输出必须按时间升序，且包含上下文字段。
			prev := int64(-1)
			for _, entry := range resp.Entries {
				ns, _ := strconv.ParseInt(entry.TimestampNs, 10, 64)
				if ns < prev {
					t.Fatalf("日志未按时间升序：%d < %d", ns, prev)
				}
				prev = ns
			}
		})
	}
}

func TestLogStreamSelectorIgnoresStructuredMetadata(t *testing.T) {
	// 真实流标签集合由 Loki /labels 决定。
	allowed := map[string]bool{
		"stack": true, "container": true, "compose_service": true,
		"service_name": true, "stream": true,
	}
	selector := logStreamSelector(map[string]string{
		"stack":          "fishmatrix",
		"container":      "fm-user",
		"detected_level": "info",
		"service":        "tf-user",
		"env":            "production",
		"service_name":   "fm-user",
		"empty":          "",
	}, allowed)
	want := `{container="fm-user",service_name="fm-user",stack="fishmatrix"}`
	if selector != want {
		t.Fatalf("selector = %s, want %s", selector, want)
	}
}

func TestSearchLogsFiltersEnvironment(t *testing.T) {
	cases := []struct {
		name        string
		env         string
		wantMatched int
		wantStreams int
		wantTest    bool
	}{
		{name: "生产", env: "production", wantMatched: 1, wantStreams: 1},
		{name: "全部", env: "", wantMatched: 2, wantStreams: 2, wantTest: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, server := newFakeLoki(t)
			opts := newLogSearchOptions(30)
			opts.Env = tc.env
			end := time.Now().UTC()
			resp, err := searchLogs(context.Background(), newLokiClient(server.URL), opts, end.Add(-24*time.Hour), end)
			if err != nil {
				t.Fatalf("searchLogs: %v", err)
			}
			if resp.Matched != tc.wantMatched || resp.Streams != tc.wantStreams {
				t.Fatalf("matched/streams = %d/%d, want %d/%d",
					resp.Matched, resp.Streams, tc.wantMatched, tc.wantStreams)
			}
			hasTest := false
			for _, entry := range resp.Entries {
				if entry.Env == "test" {
					hasTest = true
				}
			}
			if hasTest != tc.wantTest {
				t.Errorf("包含 test 日志 = %v, want %v", hasTest, tc.wantTest)
			}
		})
	}
}

func TestSearchLogsFallsBackToRequestID(t *testing.T) {
	fake, server := newFakeLoki(t)
	fake.onlyRequestID = true
	end := time.Now().UTC()
	resp, err := searchLogs(context.Background(), newLokiClient(server.URL), newLogSearchOptions(30), end.Add(-24*time.Hour), end)
	if err != nil {
		t.Fatalf("searchLogs: %v", err)
	}
	if resp.Matched != 1 {
		t.Fatalf("request_id 命中数 = %d, want 1", resp.Matched)
	}
	queries := fake.captured()
	if len(queries) == 0 || !strings.Contains(queries[0].Get("query"), "trace_id=") {
		t.Fatal("第一次查询应优先使用 trace_id")
	}
	if len(queries) < 2 || !strings.Contains(queries[1].Get("query"), "request_id=") {
		t.Fatal("trace_id 无结果时应回退到 request_id")
	}
}

func TestParseLogSearchParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name       string
		query      string
		wantErr    bool
		wantID     string
		wantEnv    string
		wantLines  int
		wantRange  string
		wantWindow time.Duration
	}{
		{
			name: "默认值", query: "?id=" + strings.ToUpper(testTraceID),
			wantID: testTraceID, wantEnv: "all", wantLines: 30, wantRange: "24h", wantWindow: 24 * time.Hour,
		},
		{
			name: "完整参数", query: "?id=" + testTraceID + "&env=test&lines=100&range=14d",
			wantID: testTraceID, wantEnv: "test", wantLines: 100, wantRange: "14d", wantWindow: 14 * 24 * time.Hour,
		},
		{name: "编号非法", query: "?id=not-hex", wantErr: true},
		{name: "编号过短", query: "?id=abcd", wantErr: true},
		{name: "行数非法", query: "?id=" + testTraceID + "&lines=40", wantErr: true},
		{name: "环境非法", query: "?id=" + testTraceID + "&env=staging", wantErr: true},
		{name: "范围非法", query: "?id=" + testTraceID + "&range=30d", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/logs/search"+tc.query, nil)
			opts, err := parseLogSearchParams(c)
			if tc.wantErr {
				if err == nil {
					t.Fatal("希望参数校验失败，实际通过")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLogSearchParams: %v", err)
			}
			if opts.ID != tc.wantID || opts.EnvLabel != tc.wantEnv ||
				opts.ContextLines != tc.wantLines || opts.WindowLabel != tc.wantRange ||
				opts.Window != tc.wantWindow {
				t.Errorf("解析结果 = %+v", opts)
			}
		})
	}
}

func TestLogSearchHandlerRequiresToken(t *testing.T) {
	_, lokiServer := newFakeLoki(t)
	router := setupLogTestRouter(t, lokiServer.URL, "log-secret")

	cases := []struct {
		name   string
		token  string
		status int
	}{
		{name: "缺少口令", status: http.StatusUnauthorized},
		{name: "口令错误", token: "wrong", status: http.StatusUnauthorized},
		{name: "口令正确", token: "log-secret", status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/logs/search?id="+testTraceID+"&env=prod&lines=30", nil)
			if tc.token != "" {
				req.Header.Set("X-Log-Token", tc.token)
			}
			router.ServeHTTP(recorder, req)
			if recorder.Code != tc.status {
				t.Fatalf("状态码 = %d, want %d, body=%s", recorder.Code, tc.status, recorder.Body.String())
			}
		})
	}
}

func TestLogSearchHandlerRejectsInvalidParams(t *testing.T) {
	_, lokiServer := newFakeLoki(t)
	router := setupLogTestRouter(t, lokiServer.URL, "")
	cases := []string{
		"?id=xyz",
		"?id=" + testTraceID + "&lines=40",
		"?id=" + testTraceID + "&env=staging",
		"?id=" + testTraceID + "&range=30d",
	}
	for _, suffix := range cases {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/logs/search"+suffix, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s 状态码 = %d, want 400", suffix, recorder.Code)
		}
	}
}

func TestLogMeta(t *testing.T) {
	_, lokiServer := newFakeLoki(t)
	router := setupLogTestRouter(t, lokiServer.URL, "log-secret")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/logs/meta", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", recorder.Code)
	}
	var meta logMetaResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &meta); err != nil {
		t.Fatalf("解析响应: %v", err)
	}
	if !meta.AuthRequired || !meta.LokiAvailable || meta.RetentionDays != 14 {
		t.Fatalf("meta = %+v", meta)
	}
	if len(meta.ContextOptions) != 3 || meta.ContextOptions[0] != 30 || meta.ContextOptions[2] != 100 {
		t.Errorf("ContextOptions = %v", meta.ContextOptions)
	}
}

func setupLogTestRouter(t *testing.T, lokiURL, token string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	server := &Server{
		cfg:  Config{LogToken: token},
		loki: newLokiClient(lokiURL),
	}
	router := gin.New()
	router.GET("/api/v1/logs/meta", server.handleLogMeta)
	router.GET("/api/v1/logs/search", server.handleLogSearch)
	return router
}
