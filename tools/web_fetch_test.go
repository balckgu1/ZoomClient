package tools

// 本文件覆盖 web_fetch 的 HTTP 交互、HTML 转换、截断与协议限制。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// writeTestBody 写入测试响应体并校验错误，避免测试里出现未处理的写错误。
func writeTestBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response failed: %v", err)
	}
}

func TestWebFetch_HTML转文本(t *testing.T) {
	page := `<!doctype html>
<html><head><title>Example Domain</title>
<script>var secret = "should not appear";</script>
<style>.hidden{display:none}</style>
</head>
<body>
<h1>Welcome</h1>
<p>First paragraph &amp; entity.</p>
<ul><li>alpha</li><li>beta</li></ul>
<a href="https://example.com/docs">Documentation</a>
<img alt="diagram"/>
</body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		writeTestBody(t, w, page)
	}))
	defer server.Close()

	res := WebFetchTool{}.Call(map[string]any{"url": server.URL}, &ToolContext{})
	if res.IsError || !res.Ok {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	checks := []string{
		"Title: Example Domain",
		"# Welcome",
		"First paragraph & entity.",
		"- alpha",
		"- beta",
		"Documentation (https://example.com/docs)",
		"[image: diagram]",
	}
	for _, want := range checks {
		if !strings.Contains(res.Content, want) {
			t.Errorf("输出缺少 %q：\n%s", want, res.Content)
		}
	}
	if strings.Contains(res.Content, "should not appear") || strings.Contains(res.Content, "display:none") {
		t.Errorf("script/style 内容不应出现在输出中：\n%s", res.Content)
	}
}

func TestWebFetch_纯文本与JSON原样返回(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeTestBody(t, w, `{"status":"ok","count":2}`)
	}))
	defer server.Close()

	res := WebFetchTool{}.Call(map[string]any{"url": server.URL}, &ToolContext{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, `{"status":"ok","count":2}`) {
		t.Errorf("JSON 应原样返回：\n%s", res.Content)
	}
}

func TestWebFetch_无ContentType时按尖括号判定HTML(t *testing.T) {
	// httptest 默认会补 Content-Type；显式删除后验证兜底判定逻辑
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil
		writeTestBody(t, w, "<html><body><p>plain html body</p></body></html>")
	}))
	defer server.Close()

	res := WebFetchTool{}.Call(map[string]any{"url": server.URL}, &ToolContext{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if strings.Contains(res.Content, "<p>") {
		t.Errorf("应被识别为 HTML 并去除标签：\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "plain html body") {
		t.Errorf("正文应保留：\n%s", res.Content)
	}
}

func TestWebFetch_非2xx返回错误(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "page not found", http.StatusNotFound)
	}))
	defer server.Close()

	res := WebFetchTool{}.Call(map[string]any{"url": server.URL}, &ToolContext{})
	if !res.IsError {
		t.Fatalf("404 应返回错误，实际：%s", res.Content)
	}
	if !strings.Contains(res.Content, "HTTP 404") {
		t.Errorf("错误信息应包含状态码：%s", res.Content)
	}
}

func TestWebFetch_协议与参数校验(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"missing url", map[string]any{}, "missing url"},
		{"file scheme", map[string]any{"url": "file:///etc/passwd"}, "only http and https"},
		{"no host", map[string]any{"url": "http:///path"}, "must include a host"},
		{"link local", map[string]any{"url": "http://169.254.169.254/latest/meta-data/"}, "link-local"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := WebFetchTool{}.Call(tc.args, &ToolContext{})
			if !res.IsError || !strings.Contains(res.Content, tc.want) {
				t.Errorf("期望错误包含 %q，实际：%s", tc.want, res.Content)
			}
		})
	}
}

func TestWebFetch_max_length截断(t *testing.T) {
	longBody := strings.Repeat("abcdefghij", 500) // 5000 字符
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		writeTestBody(t, w, longBody)
	}))
	defer server.Close()

	res := WebFetchTool{}.Call(map[string]any{"url": server.URL, "max_length": float64(1500)}, &ToolContext{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "[truncated to 1500 characters]") {
		t.Errorf("应提示截断信息：\n%s", res.Content)
	}
	if strings.Count(res.Content, "abcdefghij") > 200 {
		t.Errorf("正文长度应被限制，实际输出长度 %d", len(res.Content))
	}
}

func TestHTMLToText_嵌套结构与空白压缩(t *testing.T) {
	html := `<html><head><title> T </title></head><body>
	<div>   hello
		<span>world</span>   </div>
	<h2>Section</h2>
	<p>line&nbsp;one   two</p>
	</body></html>`
	title, text := htmlToText([]byte(html))
	if title != "T" {
		t.Errorf("标题解析错误：%q", title)
	}
	if !strings.Contains(text, "hello world") {
		t.Errorf("行内空白应被压缩为单个空格：\n%s", text)
	}
	if !strings.Contains(text, "## Section") {
		t.Errorf("h2 应转为 ## 前缀：\n%s", text)
	}
	if !strings.Contains(text, "line one two") {
		t.Errorf("nbsp 实体应解码并压缩空白：\n%s", text)
	}
	if strings.Contains(text, "\n\n\n") {
		t.Errorf("不应出现连续多个空行：\n%s", text)
	}
}
