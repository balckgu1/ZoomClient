package tools

// 本文件实现 web_search 工具：通过搜索引擎 API 检索互联网信息。
// 支持 tavily / brave / serper / bocha 四家服务，由 config.yaml 的 websearch 段选择；
// 统一输出「标题 + 链接 + 摘要」，让模型可以直接引用来源而不必抓取整页。
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	webSearchDefaultMaxResults = 5
	webSearchMaxResultsLimit   = 10
	webSearchDefaultTimeout    = 20 * time.Second
)

// WebSearchConfig web_search 的运行配置，由 config.yaml 的 websearch 段映射而来。
type WebSearchConfig struct {
	Provider   string        // tavily | brave | serper | bocha，默认 tavily
	APIKey     string        // 服务商 API Key；为空时不注册该工具
	BaseURL    string        // 可选：覆盖默认 API 端点（私有代理或测试用）
	MaxResults int           // 默认返回条数，缺省 5，上限 10
	Timeout    time.Duration // 单次请求超时，缺省 20s
}

// WebSearchTool 调用搜索引擎 API 并返回结构化结果。
type WebSearchTool struct {
	cfg    WebSearchConfig
	client *http.Client
}

// NewWebSearchTool 归一化配置并创建工具实例。
func NewWebSearchTool(cfg WebSearchConfig) *WebSearchTool {
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = "tavily"
	}
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = webSearchDefaultMaxResults
	}
	if cfg.MaxResults > webSearchMaxResultsLimit {
		cfg.MaxResults = webSearchMaxResultsLimit
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = webSearchDefaultTimeout
	}
	return &WebSearchTool{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "Search the web through a search API and return the top results (title, URL, snippet). " +
		"Use it for current information, library versions or error messages you cannot determine locally, " +
		"then use web_fetch to read a specific result in full."
}

func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query, e.g. 'golang context cancellation best practice'.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Number of results to return, 1-10. Defaults to the configured value.",
			},
		},
		"required": []string{"query"},
	}
}

// webSearchResult 归一化后的单条搜索结果。
type webSearchResult struct {
	Title   string
	URL     string
	Snippet string
}

func (t *WebSearchTool) Call(args map[string]any, toolCtx *ToolContext) ToolResult {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return ToolResult{Ok: false, Content: "Error: missing query parameter or query parameter is not a string", IsError: true}
	}
	query = strings.TrimSpace(query)
	if t.cfg.APIKey == "" {
		return ToolResult{Ok: false, Content: "Error: web_search is not configured: set websearch.api_key in config.yaml", IsError: true}
	}

	maxResults := t.cfg.MaxResults
	if v, ok := args["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
		if maxResults > webSearchMaxResultsLimit {
			maxResults = webSearchMaxResultsLimit
		}
	}

	parent := context.Background()
	if toolCtx != nil && toolCtx.Ctx != nil {
		parent = toolCtx.Ctx
	}
	ctx, cancel := context.WithTimeout(parent, t.cfg.Timeout)
	defer cancel()

	var (
		results []webSearchResult
		err     error
	)
	switch t.cfg.Provider {
	case "tavily":
		results, err = t.searchTavily(ctx, query, maxResults)
	case "brave":
		results, err = t.searchBrave(ctx, query, maxResults)
	case "serper":
		results, err = t.searchSerper(ctx, query, maxResults)
	case "bocha":
		results, err = t.searchBocha(ctx, query, maxResults)
	default:
		return ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: unsupported websearch provider %q (supported: tavily, brave, serper, bocha)", t.cfg.Provider),
			IsError: true,
		}
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ToolResult{Ok: false, Content: fmt.Sprintf("Error: web search timed out after %s", t.cfg.Timeout), IsError: true}
		}
		return ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}

	return ToolResult{Ok: true, Content: formatWebSearchResults(query, t.cfg.Provider, results)}
}

// baseURL 返回服务商 API 根地址：配置了 base_url 时优先使用，并去掉尾部斜杠。
func (t *WebSearchTool) baseURL(fallback string) string {
	base := strings.TrimSpace(t.cfg.BaseURL)
	if base == "" {
		base = fallback
	}
	return strings.TrimRight(base, "/")
}

// searchTavily 调用 Tavily Search API（POST /search，Bearer 鉴权）。
func (t *WebSearchTool) searchTavily(ctx context.Context, query string, maxResults int) ([]webSearchResult, error) {
	body, err := t.doJSON(ctx, http.MethodPost, t.baseURL("https://api.tavily.com")+"/search",
		map[string]string{"Authorization": "Bearer " + t.cfg.APIKey},
		map[string]any{"query": query, "max_results": maxResults, "search_depth": "basic"})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("tavily response parsing failed: %w", err)
	}
	results := make([]webSearchResult, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		results = append(results, webSearchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return results, nil
}

// searchBrave 调用 Brave Web Search API（GET /res/v1/web/search）。
func (t *WebSearchTool) searchBrave(ctx context.Context, query string, maxResults int) ([]webSearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", fmt.Sprintf("%d", maxResults))
	endpoint := t.baseURL("https://api.search.brave.com") + "/res/v1/web/search?" + params.Encode()
	body, err := t.doJSON(ctx, http.MethodGet, endpoint,
		map[string]string{"X-Subscription-Token": t.cfg.APIKey}, nil)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("brave response parsing failed: %w", err)
	}
	results := make([]webSearchResult, 0, len(parsed.Web.Results))
	for _, r := range parsed.Web.Results {
		results = append(results, webSearchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return results, nil
}

// searchSerper 调用 Serper.dev Google 搜索 API（POST /search，X-API-KEY 鉴权）。
func (t *WebSearchTool) searchSerper(ctx context.Context, query string, maxResults int) ([]webSearchResult, error) {
	body, err := t.doJSON(ctx, http.MethodPost, t.baseURL("https://google.serper.dev")+"/search",
		map[string]string{"X-API-KEY": t.cfg.APIKey},
		map[string]any{"q": query, "num": maxResults})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("serper response parsing failed: %w", err)
	}
	results := make([]webSearchResult, 0, len(parsed.Organic))
	for _, r := range parsed.Organic {
		results = append(results, webSearchResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return results, nil
}

// searchBocha 调用博查 Web Search API（POST /v1/web-search，Bearer 鉴权）。
func (t *WebSearchTool) searchBocha(ctx context.Context, query string, maxResults int) ([]webSearchResult, error) {
	body, err := t.doJSON(ctx, http.MethodPost, t.baseURL("https://api.bochaai.com")+"/v1/web-search",
		map[string]string{"Authorization": "Bearer " + t.cfg.APIKey},
		map[string]any{"query": query, "count": maxResults, "summary": false})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data struct {
			WebPages struct {
				Value []struct {
					Name    string `json:"name"`
					URL     string `json:"url"`
					Snippet string `json:"snippet"`
				} `json:"value"`
			} `json:"webPages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("bocha response parsing failed: %w", err)
	}
	results := make([]webSearchResult, 0, len(parsed.Data.WebPages.Value))
	for _, r := range parsed.Data.WebPages.Value {
		results = append(results, webSearchResult{Title: r.Name, URL: r.URL, Snippet: r.Snippet})
	}
	return results, nil
}

// doJSON 发送 JSON 请求并读取响应体；非 2xx 时返回带状态码与正文片段的错误。
func (t *WebSearchTool) doJSON(ctx context.Context, method, endpoint string, headers map[string]string, payload any) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("request encoding failed: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("request creation failed: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", webFetchUserAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("reading response failed: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("closing response failed: %w", closeErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("search API returned HTTP %d: %s",
			resp.StatusCode, truncateRunes(strings.TrimSpace(string(body)), 500))
	}
	return body, nil
}

// formatWebSearchResults 把归一化结果格式化为带编号的文本，便于模型引用和后续 web_fetch。
func formatWebSearchResults(query, provider string, results []webSearchResult) string {
	if len(results) == 0 {
		return fmt.Sprintf("No results found for %q via %s.", query, provider)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Search results for %q (%s):\n", query, provider)
	for i, r := range results {
		title := strings.TrimSpace(r.Title)
		if title == "" {
			title = "(untitled)"
		}
		snippet := strings.TrimSpace(r.Snippet)
		// 摘要里可能夹带换行，压成单行避免破坏编号结构
		snippet = strings.Join(strings.Fields(snippet), " ")
		fmt.Fprintf(&b, "\n%d. %s\n   %s\n", i+1, title, r.URL)
		if snippet != "" {
			fmt.Fprintf(&b, "   %s\n", snippet)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
