package tools

// 本文件实现 web_fetch 工具：抓取 HTTP(S) 页面并转换为可读文本。
// HTML 通过下方的 htmlToText 转换成纯文本 / 轻量 Markdown，
import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	webFetchDefaultMaxChars = 30000            // 默认输出字符上限
	webFetchMinMaxChars     = 1000             // max_length 下限
	webFetchMaxMaxChars     = 100000           // max_length 上限
	webFetchMaxBytes        = 5 << 20          // 响应体读取上限（5MB）
	webFetchTimeout         = 30 * time.Second // 单次抓取超时
	webFetchUserAgent       = "ZoomClient/1.0 (+web_fetch)"
)

// WebFetchTool 抓取单个 URL 并返回文本内容。
type WebFetchTool struct{}

func (t WebFetchTool) Name() string { return "web_fetch" }

func (t WebFetchTool) Description() string {
	return "Fetch a URL over HTTP(S) and return its readable text (HTML is converted to plain text). " +
		"Use it to read documentation, release notes or error pages instead of guessing APIs. " +
		"Only http/https URLs are allowed; responses are truncated to max_length characters."
}

func (t WebFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Absolute http(s) URL to fetch.",
			},
			"max_length": map[string]any{
				"type":        "integer",
				"description": "Maximum characters of extracted text to return. Defaults to 20000, clamped to [1000, 100000].",
			},
		},
		"required": []string{"url"},
	}
}

func (t WebFetchTool) Call(args map[string]any, toolCtx *ToolContext) ToolResult {
	rawURL, ok := args["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return ToolResult{Ok: false, Content: "Error: missing url parameter or url parameter is not a string", IsError: true}
	}
	rawURL = strings.TrimSpace(rawURL)

	// 只允许 http/https，拒绝 file:// 等可能读取本地内容的协议
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: invalid URL: %v", err), IsError: true}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ToolResult{Ok: false, Content: "Error: only http and https URLs are supported", IsError: true}
	}
	if parsed.Host == "" {
		return ToolResult{Ok: false, Content: "Error: URL must include a host", IsError: true}
	}
	// 拒绝链路本地地址：云厂商的实例元数据服务（169.254.169.254）位于该网段
	if isLinkLocalHost(parsed.Hostname()) {
		return ToolResult{Ok: false, Content: "Error: link-local / metadata addresses are not allowed", IsError: true}
	}

	maxChars := webFetchDefaultMaxChars
	if v, ok := args["max_length"].(float64); ok && v > 0 {
		maxChars = int(v)
		if maxChars < webFetchMinMaxChars {
			maxChars = webFetchMinMaxChars
		}
		if maxChars > webFetchMaxMaxChars {
			maxChars = webFetchMaxMaxChars
		}
	}

	// 继承会话 context 并叠加单次抓取超时，用户中断时能立即退出
	parent := context.Background()
	if toolCtx != nil && toolCtx.Ctx != nil {
		parent = toolCtx.Ctx
	}
	ctx, cancel := context.WithTimeout(parent, webFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: request creation failed: %v", err), IsError: true}
	}
	req.Header.Set("User-Agent", webFetchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,text/plain;q=0.8,*/*;q=0.5")

	client := &http.Client{Timeout: webFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ToolResult{Ok: false, Content: fmt.Sprintf("Error: request timed out after %s", webFetchTimeout), IsError: true}
		}
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: request failed: %v", err), IsError: true}
	}
	// 多读 1 字节用于判断是否超限
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: reading response failed: %v", readErr), IsError: true}
	}
	if closeErr != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: closing response failed: %v", closeErr), IsError: true}
	}

	sizeTruncated := false
	if len(body) > webFetchMaxBytes {
		body = body[:webFetchMaxBytes]
		sizeTruncated = true
	}

	// 非 2xx 通常是模型拼错 URL 或接口失效：返回状态码与正文片段，便于自我修正
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := truncateRunes(strings.TrimSpace(string(body)), 2000)
		return ToolResult{
			Ok:      false,
			IsError: true,
			Content: fmt.Sprintf("Error: HTTP %d %s\n%s", resp.StatusCode, http.StatusText(resp.StatusCode), snippet),
		}
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	title := ""
	var text string
	if isHTMLContentType(contentType, body) {
		title, text = htmlToText(body)
	} else {
		text = strings.TrimSpace(string(body))
	}

	var out strings.Builder
	fmt.Fprintf(&out, "URL: %s\n", rawURL)
	fmt.Fprintf(&out, "Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
	if contentType != "" {
		fmt.Fprintf(&out, "Content-Type: %s\n", contentType)
	}
	if title != "" {
		fmt.Fprintf(&out, "Title: %s\n", title)
	}
	out.WriteString("\n")

	var truncated bool
	text, truncated = truncateRunesWithFlag(text, maxChars)
	if truncated {
		fmt.Fprintf(&out, "%s\n\n[truncated to %d characters]\n", text, maxChars)
	} else {
		out.WriteString(text)
	}
	if sizeTruncated {
		fmt.Fprintf(&out, "\n[response body truncated at %d bytes]", webFetchMaxBytes)
	}
	return ToolResult{Ok: true, Content: strings.TrimRight(out.String(), "\n")}
}

// isHTMLContentType 判断响应是否需要做 HTML → 文本转换。
// Content-Type 缺失时按正文首个非空白字符是否为 '<' 兜底判断。
func isHTMLContentType(contentType string, body []byte) bool {
	if strings.Contains(contentType, "html") {
		return true
	}
	if contentType != "" {
		return false
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '<'
}

// isLinkLocalHost 判断主机名是否指向 IPv4 链路本地网段 169.254.0.0/16。
func isLinkLocalHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 169 && ip4[1] == 254
}

// truncateRunes 按字符（而非字节）截断，避免切断多字节 UTF-8。
func truncateRunes(s string, max int) string {
	out, _ := truncateRunesWithFlag(s, max)
	return out
}

// truncateRunesWithFlag 截断并返回是否发生截断。
func truncateRunesWithFlag(s string, max int) (string, bool) {
	if max <= 0 {
		return "", len(s) > 0
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s, false
	}
	return string(runes[:max]), true
}
