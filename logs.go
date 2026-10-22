package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	logDefaultRange      = 24 * time.Hour
	logRetention         = 14 * 24 * time.Hour
	logMaxMatchEntries   = 200
	logMaxContextStreams = 10
	logMaxEntries        = 3000
	logMaxLineRunes      = 6000
	logSearchTimeout     = 15 * time.Second
	logMetaProbeTimeout  = 1500 * time.Millisecond
)

var (
	logIDPattern           = regexp.MustCompile(`^[0-9a-f]{8,64}$`)
	logAllowedContextLines = map[int]bool{30: true, 50: true, 100: true}
	// fallbackStreamLabelKeys 仅作为 Loki /labels 不可用时的兜底。
	// 注意不能把 env、service 等 JSON 日志字段放进来：`| json` 解析出的字段
	// 会随流一起返回，但它们不是真实流标签，拼进 selector 会查不到上下文。
	fallbackStreamLabelKeys = map[string]bool{
		"stack":           true,
		"container":       true,
		"compose_service": true,
		"service_name":    true,
		"stream":          true,
		"job":             true,
		"instance":        true,
	}
)

// ── Loki 客户端 ──

type lokiClient struct {
	baseURL string
	http    *http.Client
}

func newLokiClient(baseURL string) *lokiClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	return &lokiClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *lokiClient) available(ctx context.Context) bool {
	if c == nil || c.baseURL == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/ready", nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode == http.StatusOK
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

type lokiResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string       `json:"resultType"`
		Result     []lokiStream `json:"result"`
	} `json:"data"`
}

// streamLabelNames 返回 Loki 中真实存在的流标签名。
// `| json` 解析出的字段会混在查询结果的 stream 对象里，只有这些名字才是合法标签。
func (c *lokiClient) streamLabelNames(ctx context.Context) (map[string]bool, error) {
	u, err := url.Parse(c.baseURL + "/loki/api/v1/labels")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", `{stack="fishmatrix"}`)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var body struct {
		Status string   `json:"status"`
		Error  string   `json:"error"`
		Data   []string `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("解析 Loki 标签响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK || body.Status != "success" {
		message := body.Error
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("loki 标签查询失败: %s", message)
	}
	names := make(map[string]bool, len(body.Data))
	for _, name := range body.Data {
		names[name] = true
	}
	return names, nil
}

func (c *lokiClient) queryRange(
	ctx context.Context,
	query string,
	start, end time.Time,
	limit int,
	direction string,
) ([]lokiStream, error) {
	u, err := url.Parse(c.baseURL + "/loki/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("limit", strconv.Itoa(limit))
	q.Set("direction", direction)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var body lokiResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("解析 Loki 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK || body.Status != "success" {
		message := body.Error
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("loki 查询失败: %s", message)
	}
	return body.Data.Result, nil
}

// ── 请求与响应 ──

type logSearchOptions struct {
	ID           string
	Env          string // 空表示不按环境过滤
	EnvLabel     string // all / prod / test
	ContextLines int
	Window       time.Duration
	WindowLabel  string
}

type logMetaResponse struct {
	AuthRequired   bool     `json:"authRequired"`
	LokiAvailable  bool     `json:"lokiAvailable"`
	RetentionDays  int      `json:"retentionDays"`
	ContextOptions []int    `json:"contextOptions"`
	RangeOptions   []string `json:"rangeOptions"`
}

type logEntry struct {
	Timestamp   string `json:"timestamp"`
	TimestampNs string `json:"timestampNs"`
	Level       string `json:"level"`
	Service     string `json:"service"`
	Env         string `json:"env"`
	Logger      string `json:"logger,omitempty"`
	Message     string `json:"message"`
	Line        string `json:"line"`
	Matched     bool   `json:"matched"`

	sortNs int64
}

type logSearchResponse struct {
	ID           string     `json:"id"`
	Env          string     `json:"env"`
	ContextLines int        `json:"contextLines"`
	Range        string     `json:"range"`
	Start        string     `json:"start"`
	End          string     `json:"end"`
	Matched      int        `json:"matched"`
	Streams      int        `json:"streams"`
	Truncated    bool       `json:"truncated"`
	Warnings     []string   `json:"warnings,omitempty"`
	Entries      []logEntry `json:"entries"`
}

type logMatchGroup struct {
	Selector string
	Stream   map[string]string
	Matches  []logRawEntry
	MinTs    int64
	MaxTs    int64
	HasRange bool
}

type logRawEntry struct {
	TimestampNs int64
	Line        string
}

type parsedLog struct {
	Timestamp string
	Level     string
	Service   string
	Env       string
	Logger    string
	Message   string
}

// ── HTTP 处理 ──

func (s *Server) authorizeLogAccess(c *gin.Context) bool {
	if s.cfg.LogToken == "" {
		return true
	}
	provided := c.GetHeader("X-Log-Token")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.LogToken)) == 1 {
		return true
	}
	c.JSON(http.StatusUnauthorized, gin.H{
		"error":   "UNAUTHORIZED",
		"message": "日志检索口令无效",
	})
	return false
}

func (s *Server) handleLogMeta(c *gin.Context) {
	available := false
	if s.loki != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), logMetaProbeTimeout)
		available = s.loki.available(ctx)
		cancel()
	}
	c.JSON(http.StatusOK, logMetaResponse{
		AuthRequired:   s.cfg.LogToken != "",
		LokiAvailable:  available,
		RetentionDays:  int(logRetention / (24 * time.Hour)),
		ContextOptions: []int{30, 50, 100},
		RangeOptions:   []string{"1h", "6h", "24h", "7d", "14d"},
	})
}

func (s *Server) handleLogSearch(c *gin.Context) {
	if !s.authorizeLogAccess(c) {
		return
	}
	opts, err := parseLogSearchParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "INVALID_PARAMETER",
			"message": err.Error(),
		})
		return
	}
	if s.loki == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "LOKI_UNAVAILABLE",
			"message": "日志服务未配置",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), logSearchTimeout)
	defer cancel()

	end := time.Now().UTC()
	start := end.Add(-opts.Window)
	resp, err := searchLogs(ctx, s.loki, opts, start, end)
	if err != nil {
		log.Printf("日志检索失败 id=%s: %v", opts.ID, err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "LOG_QUERY_FAILED",
			"message": "日志服务查询失败，请稍后重试",
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func parseLogSearchParams(c *gin.Context) (logSearchOptions, error) {
	var opts logSearchOptions

	opts.ID = strings.ToLower(strings.TrimSpace(c.Query("id")))
	if !logIDPattern.MatchString(opts.ID) {
		return opts, fmt.Errorf("编号必须是 8-64 位十六进制字符")
	}

	envLabel := strings.ToLower(strings.TrimSpace(c.DefaultQuery("env", "all")))
	switch envLabel {
	case "", "all":
		opts.EnvLabel = "all"
	case "prod", "production":
		opts.Env = "production"
		opts.EnvLabel = "prod"
	case "test", "testing":
		opts.Env = "test"
		opts.EnvLabel = "test"
	default:
		return opts, fmt.Errorf("环境只能是 prod、test 或 all")
	}

	opts.ContextLines = 30
	if raw := strings.TrimSpace(c.Query("lines")); raw != "" {
		lines, err := strconv.Atoi(raw)
		if err != nil || !logAllowedContextLines[lines] {
			return opts, fmt.Errorf("上下文行数只能是 30、50 或 100")
		}
		opts.ContextLines = lines
	}

	opts.Window = logDefaultRange
	opts.WindowLabel = "24h"
	switch strings.ToLower(strings.TrimSpace(c.Query("range"))) {
	case "", "24h":
	case "1h":
		opts.Window, opts.WindowLabel = time.Hour, "1h"
	case "6h":
		opts.Window, opts.WindowLabel = 6*time.Hour, "6h"
	case "7d":
		opts.Window, opts.WindowLabel = 7*24*time.Hour, "7d"
	case "14d":
		opts.Window, opts.WindowLabel = logRetention, "14d"
	default:
		return opts, fmt.Errorf("时间范围只能是 1h、6h、24h、7d 或 14d")
	}

	return opts, nil
}

// ── 查询与合并 ──

func searchLogs(
	ctx context.Context,
	client *lokiClient,
	opts logSearchOptions,
	start, end time.Time,
) (logSearchResponse, error) {
	resp := logSearchResponse{
		ID:           opts.ID,
		Env:          opts.EnvLabel,
		ContextLines: opts.ContextLines,
		Range:        opts.WindowLabel,
		Start:        start.Format(time.RFC3339),
		End:          end.Format(time.RFC3339),
		Entries:      []logEntry{},
	}

	streams, err := client.queryRange(ctx, logMatchQuery(opts.ID, "trace_id"), start, end, logMaxMatchEntries, "backward")
	if err != nil {
		return resp, err
	}
	matchCount := countLogValues(streams)
	// trace_id 是主键；兼容只带自定义 X-Request-ID 的旧日志。
	if matchCount == 0 {
		streams, err = client.queryRange(ctx, logMatchQuery(opts.ID, "request_id"), start, end, logMaxMatchEntries, "backward")
		if err != nil {
			return resp, err
		}
		matchCount = countLogValues(streams)
	}
	if matchCount >= logMaxMatchEntries {
		resp.Truncated = true
	}

	labelKeys := fallbackStreamLabelKeys
	if names, err := client.streamLabelNames(ctx); err == nil {
		labelKeys = names
	} else {
		log.Printf("获取 Loki 流标签失败，使用兜底白名单：%v", err)
	}

	groups := make(map[string]*logMatchGroup)
	for _, stream := range streams {
		selector := logStreamSelector(stream.Stream, labelKeys)
		if selector == "" {
			continue
		}
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}
			ts, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil {
				continue
			}
			if opts.Env != "" {
				line := parseLogLine(value[1], stream.Stream, ts)
				if line.Env != opts.Env {
					continue
				}
			}
			group := groups[selector]
			if group == nil {
				group = &logMatchGroup{Selector: selector, Stream: stream.Stream}
				groups[selector] = group
			}
			group.Matches = append(group.Matches, logRawEntry{TimestampNs: ts, Line: value[1]})
			if !group.HasRange || ts < group.MinTs {
				group.MinTs = ts
			}
			if !group.HasRange || ts > group.MaxTs {
				group.MaxTs = ts
			}
			group.HasRange = true
		}
	}
	if len(groups) == 0 {
		return resp, nil
	}

	selectors := make([]string, 0, len(groups))
	for selector := range groups {
		selectors = append(selectors, selector)
	}
	sort.Strings(selectors)
	resp.Streams = len(selectors)
	if len(selectors) > logMaxContextStreams {
		selectors = selectors[:logMaxContextStreams]
		resp.Truncated = true
	}

	entries := make(map[string]*logEntry)
	add := func(ts int64, line string, matched bool, streamLabels map[string]string) {
		key := strconv.FormatInt(ts, 10) + "\x00" + line
		if entry, ok := entries[key]; ok {
			entry.Matched = entry.Matched || matched
			return
		}
		parsed := parseLogLine(line, streamLabels, ts)
		entries[key] = &logEntry{
			Timestamp:   parsed.Timestamp,
			TimestampNs: strconv.FormatInt(ts, 10),
			Level:       parsed.Level,
			Service:     parsed.Service,
			Env:         parsed.Env,
			Logger:      parsed.Logger,
			Message:     truncateLogText(parsed.Message),
			Line:        truncateLogText(line),
			Matched:     matched,
			sortNs:      ts,
		}
	}

	// 命中行全部保留。
	for _, selector := range selectors {
		group := groups[selector]
		for _, match := range group.Matches {
			add(match.TimestampNs, match.Line, isMatchedLogLine(match.Line, opts.ID), group.Stream)
		}
	}

	// 再按同一 stream 取命中时间点前后的上下文；与命中行重复时会自动合并。
	for _, selector := range selectors {
		group := groups[selector]

		before, err := client.queryRange(
			ctx, group.Selector, start, time.Unix(0, group.MinTs),
			opts.ContextLines, "backward",
		)
		if err != nil {
			resp.Warnings = append(resp.Warnings, "部分日志上下文获取失败")
		} else {
			addLokiValues(before, group.Stream, opts.ID, add)
		}

		after, err := client.queryRange(
			ctx, group.Selector, time.Unix(0, group.MaxTs), end,
			opts.ContextLines, "forward",
		)
		if err != nil {
			resp.Warnings = append(resp.Warnings, "部分日志上下文获取失败")
		} else {
			addLokiValues(after, group.Stream, opts.ID, add)
		}
	}

	list := make([]logEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Matched {
			resp.Matched++
		}
		list = append(list, *entry)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].sortNs != list[j].sortNs {
			return list[i].sortNs < list[j].sortNs
		}
		return list[i].Line < list[j].Line
	})
	if len(list) > logMaxEntries {
		list = list[:logMaxEntries]
		resp.Truncated = true
	}
	resp.Entries = list
	return resp, nil
}

func addLokiValues(
	streams []lokiStream,
	streamLabels map[string]string,
	id string,
	add func(ts int64, line string, matched bool, labels map[string]string),
) {
	for _, stream := range streams {
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}
			ts, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil {
				continue
			}
			add(ts, value[1], isMatchedLogLine(value[1], id), streamLabels)
		}
	}
}

func countLogValues(streams []lokiStream) int {
	count := 0
	for _, stream := range streams {
		count += len(stream.Values)
	}
	return count
}

func logMatchQuery(id, field string) string {
	return `{stack="fishmatrix"} | json | ` + field + `=` + strconv.Quote(id)
}

func logStreamSelector(stream map[string]string, allowed map[string]bool) string {
	keys := make([]string, 0, len(stream))
	for key, value := range stream {
		if value != "" && allowed[key] {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Quote(stream[key]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func parseLogLine(line string, stream map[string]string, ts int64) parsedLog {
	var fields map[string]any
	_ = json.Unmarshal([]byte(line), &fields)

	get := func(keys ...string) string {
		for _, key := range keys {
			value, ok := fields[key]
			if !ok {
				continue
			}
			switch typed := value.(type) {
			case string:
				if typed != "" {
					return typed
				}
			case float64:
				return strconv.FormatFloat(typed, 'f', -1, 64)
			case bool:
				return strconv.FormatBool(typed)
			}
		}
		return ""
	}

	message := get("message", "msg")
	if message == "" {
		message = line
	}
	level := strings.ToLower(get("level", "severity"))
	if level == "" {
		level = "info"
	}
	env := get("env")
	if env == "" {
		env = stream["env"]
	}
	service := get("service")
	if service == "" {
		service = stream["service"]
	}
	timestamp := get("@timestamp", "timestamp", "time")
	if timestamp == "" {
		timestamp = time.Unix(0, ts).UTC().Format(time.RFC3339Nano)
	}

	return parsedLog{
		Timestamp: timestamp,
		Level:     level,
		Service:   service,
		Env:       env,
		Logger:    get("logger", "logger_name"),
		Message:   message,
	}
}

func isMatchedLogLine(line, id string) bool {
	var fields map[string]any
	if err := json.Unmarshal([]byte(line), &fields); err == nil {
		for _, key := range []string{"trace_id", "request_id"} {
			if value, ok := fields[key].(string); ok && strings.EqualFold(value, id) {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(line), strings.ToLower(id))
}

func truncateLogText(text string) string {
	runes := []rune(text)
	if len(runes) <= logMaxLineRunes {
		return text
	}
	return string(runes[:logMaxLineRunes]) + "\n…（日志过长，已截断）"
}
