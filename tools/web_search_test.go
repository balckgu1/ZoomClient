package tools

// 本文件覆盖 web_search 的多服务商适配、参数校验与错误路径。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearch_Tavily成功解析并携带鉴权(t *testing.T) {
	var gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" {
			t.Errorf("Tavily 应请求 POST /search，实际 %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body failed: %v", err)
			return
		}
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		writeTestBody(t, w, `{"results":[
			{"title":"Go Docs","url":"https://go.dev/doc","content":"The Go programming language docs"},
			{"title":"Context","url":"https://pkg.go.dev/context","content":"Package context"}
		]}`)
	}))
	defer server.Close()

	tool := NewWebSearchTool(WebSearchConfig{Provider: "tavily", APIKey: "key123", BaseURL: server.URL})
	res := tool.Call(map[string]any{"query": "golang docs"}, &ToolContext{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if gotAuth != "Bearer key123" {
		t.Errorf("鉴权头错误：%q", gotAuth)
	}
	if !strings.Contains(gotBody, `"query":"golang docs"`) {
		t.Errorf("请求体缺少 query：%s", gotBody)
	}
	for _, want := range []string{"1. Go Docs", "https://go.dev/doc", "2. Context", "Package context"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("输出缺少 %q：\n%s", want, res.Content)
		}
	}
}

func TestWebSearch_BraveGET与订阅头(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/res/v1/web/search" {
			t.Errorf("Brave 应请求 GET /res/v1/web/search，实际 %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("q") != "rust async" {
			t.Errorf("查询参数错误：%q", r.URL.Query().Get("q"))
		}
		if r.Header.Get("X-Subscription-Token") != "brave-key" {
			t.Errorf("缺少订阅头")
		}
		writeTestBody(t, w, `{"web":{"results":[{"title":"Async Book","url":"https://rust-lang.github.io/async-book","description":"Asynchronous programming in Rust"}]}}`)
	}))
	defer server.Close()

	tool := NewWebSearchTool(WebSearchConfig{Provider: "brave", APIKey: "brave-key", BaseURL: server.URL})
	res := tool.Call(map[string]any{"query": "rust async"}, &ToolContext{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "Async Book") || !strings.Contains(res.Content, "Asynchronous programming") {
		t.Errorf("解析结果不完整：\n%s", res.Content)
	}
}

func TestWebSearch_Serper与Bocha解析(t *testing.T) {
	cases := []struct {
		provider string
		path     string
		header   string
		response string
		want     string
	}{
		{
			provider: "serper",
			path:     "/search",
			header:   "X-API-KEY",
			response: `{"organic":[{"title":"Serper Result","link":"https://serper.dev","snippet":"fast google search"}]}`,
			want:     "Serper Result",
		},
		{
			provider: "bocha",
			path:     "/v1/web-search",
			header:   "Authorization",
			response: `{"data":{"webPages":{"value":[{"name":"博查结果","url":"https://bochaai.com","snippet":"中文搜索摘要"}]}}}`,
			want:     "博查结果",
		},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("路径错误：%s", r.URL.Path)
				}
				if r.Header.Get(tc.header) == "" {
					t.Errorf("缺少鉴权头 %s", tc.header)
				}
				writeTestBody(t, w, tc.response)
			}))
			defer server.Close()

			tool := NewWebSearchTool(WebSearchConfig{Provider: tc.provider, APIKey: "k", BaseURL: server.URL})
			res := tool.Call(map[string]any{"query": "q"}, &ToolContext{})
			if res.IsError || !strings.Contains(res.Content, tc.want) {
				t.Errorf("解析结果缺少 %q：%s", tc.want, res.Content)
			}
		})
	}
}

func TestWebSearch_参数与配置错误(t *testing.T) {
	tool := NewWebSearchTool(WebSearchConfig{Provider: "tavily", APIKey: "k"})

	if res := tool.Call(map[string]any{}, &ToolContext{}); !res.IsError || !strings.Contains(res.Content, "missing query") {
		t.Errorf("缺少 query 应报错：%s", res.Content)
	}

	noKey := NewWebSearchTool(WebSearchConfig{Provider: "tavily"})
	if res := noKey.Call(map[string]any{"query": "x"}, &ToolContext{}); !res.IsError || !strings.Contains(res.Content, "not configured") {
		t.Errorf("缺少 API Key 应报错：%s", res.Content)
	}

	unknown := NewWebSearchTool(WebSearchConfig{Provider: "nope", APIKey: "k"})
	if res := unknown.Call(map[string]any{"query": "x"}, &ToolContext{}); !res.IsError || !strings.Contains(res.Content, "unsupported") {
		t.Errorf("未知服务商应报错：%s", res.Content)
	}
}

func TestWebSearch_API错误状态与空结果(t *testing.T) {
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer failServer.Close()
	failTool := NewWebSearchTool(WebSearchConfig{Provider: "tavily", APIKey: "k", BaseURL: failServer.URL})
	if res := failTool.Call(map[string]any{"query": "x"}, &ToolContext{}); !res.IsError || !strings.Contains(res.Content, "HTTP 429") {
		t.Errorf("非 2xx 应返回状态码错误：%s", res.Content)
	}

	emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTestBody(t, w, `{"results":[]}`)
	}))
	defer emptyServer.Close()
	emptyTool := NewWebSearchTool(WebSearchConfig{Provider: "tavily", APIKey: "k", BaseURL: emptyServer.URL})
	if res := emptyTool.Call(map[string]any{"query": "x"}, &ToolContext{}); res.IsError || !strings.Contains(res.Content, "No results found") {
		t.Errorf("空结果应给出明确提示：%s", res.Content)
	}
}

func TestWebSearch_max_results上限收敛(t *testing.T) {
	var gotMax float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body failed: %v", err)
			return
		}
		if uerr := json.Unmarshal(data, &payload); uerr == nil {
			if v, ok := payload["max_results"].(float64); ok {
				gotMax = v
			}
		}
		writeTestBody(t, w, `{"results":[]}`)
	}))
	defer server.Close()

	tool := NewWebSearchTool(WebSearchConfig{Provider: "tavily", APIKey: "k", BaseURL: server.URL})
	_ = tool.Call(map[string]any{"query": "x", "max_results": float64(99)}, &ToolContext{})
	if gotMax != webSearchMaxResultsLimit {
		t.Errorf("max_results 应被收敛到 %d，实际 %v", webSearchMaxResultsLimit, gotMax)
	}
}
