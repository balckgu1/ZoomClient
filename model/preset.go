package model

// Preset 表示一个模型预设配置。
type Preset struct {
	Name      string `yaml:"name" json:"name"`
	Type      string `yaml:"type" json:"type"` // "openai" | "ollama" | "anthropic" | "gemini"
	BaseURL   string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	APIKey    string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	ModelName string `yaml:"model_name" json:"model_name"` // 实际发送给 API 的模型名
}
