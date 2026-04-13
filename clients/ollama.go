package clients

import (
	"net/http"
	"zoomClient/tools"
)

// OllamaClient Ollama客户端
type OllamaClient struct {
	BaseURL string
	Client  *http.Client
}

// OllamaTool 定义发送给Ollama的工具格式
type OllamaTool struct {
	Type     string         `json:"type"`
	Function OllamaFunction `json:"function"`
}

// OllamaFunction 工具函数定义
type OllamaFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// NewOllamaClient 创建新的Ollama客户端
func NewOllamaClient(baseURL string) *OllamaClient {
	return &OllamaClient{
		BaseURL: baseURL,
		Client:  &http.Client{},
	}
}

// BuildOllamaTools 将 Tool 接口列表转换为 Ollama API 格式的工具定义
func BuildOllamaTools(toolList []tools.Tool) []OllamaTool {
	result := make([]OllamaTool, 0, len(toolList))
	for _, t := range toolList {
		result = append(result, OllamaTool{
			Type: "function",
			Function: OllamaFunction{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return result
}
