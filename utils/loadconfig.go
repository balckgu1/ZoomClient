package utils

import (
	"log"
	"sync"

	"github.com/spf13/viper"
)

// Config config struct
type Config struct {
	ApiKey     ApiKeyConfig     `mapstructure:"api_key"`
	OpenAI     OpenAIConfig     `mapstructure:"openai"`
	Subagent   SubagentConfig   `mapstructure:"subagent"`
	Skills     SkillsConfig     `mapstructure:"skills"`
	Memory     MemoryConfig     `mapstructure:"memory"`
	AgentLoop  AgentLoopConfig  `mapstructure:"agentloop"`
	Compact    CompactConfig    `mapstructure:"compact"`
	Permission PermissionConfig `mapstructure:"permission"`
	Tools      ToolsConfig      `mapstructure:"tools"`
}

// ApiKeyConfig api_key config
type ApiKeyConfig struct {
	Anthropic string `mapstructure:"anthropic"`
	Gemini    string `mapstructure:"gemini"`
}

// OpenAIConfig OpenAI 兼容后端配置
type OpenAIConfig struct {
	ApiKey    string `mapstructure:"api_key"`
	BaseURL   string `mapstructure:"base_url"`
	ModelName string `mapstructure:"model_name"`
}

// SubagentConfig subagent config
type SubagentConfig struct {
	DefaultMaxTurns         int    `mapstructure:"defaultMaxTurns"`
	DefaultSystemPrompt     string `mapstructure:"defaultSystemPrompt"`
	ForkSubtaskPromptPrefix string `mapstructure:"forkSubtaskPromptPrefix"`
}

type SkillsConfig struct {
	Dir string `mapstructure:"dir"`
}

type MemoryConfig struct {
	Dir string `mapstructure:"dir"`
}

type ToolsConfig struct {
	DefaultBashTimeout int `mapstructure:"defaultBashTimeout"`
}

type AgentLoopConfig struct {
	MaxTurns            int      `mapstructure:"maxTurns"`
	TodoRoundsThreshold int      `mapstructure:"todoRoundsThreshold"`
	MaxTools            int      `mapstructure:"maxTools"`
	SensitiveFiles      []string `mapstructure:"sensitiveFiles"`
}

type CompactConfig struct {
	PersistThreshold      int    `mapstructure:"persistThreshold"`
	PreviewBytes          int    `mapstructure:"previewBytes"`
	KeepRecentToolResults int    `mapstructure:"keepRecentToolResults"`
	ContextLimit          int    `mapstructure:"contextLimit"`
	PersistDir            string `mapstructure:"persistDir"`
}

// PermissionConfig 权限系统配置
type PermissionConfig struct {
	Mode        string                 `mapstructure:"mode"`        // default | plan | auto，未配置或非法值时回退到 default
	Interactive bool                   `mapstructure:"interactive"` // 命中 ask 时是否从 stdin 询问用户
	DenyRules   []PermissionRuleConfig `mapstructure:"denyRules"`   // 命中即拒绝的命令
	AllowRules  []PermissionRuleConfig `mapstructure:"allowRules"`  // 命中即放行的命令
}

// PermissionRuleConfig 单条权限规则的 yaml 表达。
//   - Tool    ：针对哪个工具（"" 或 "*" 表示任意工具）
//   - Behavior：allow / deny / ask
//   - Path    ：可选，匹配 filename / path / file 参数（"re:" 前缀视为正则）
//   - Content ：可选，匹配 command / content / prompt 参数（"re:" 前缀视为正则）
type PermissionRuleConfig struct {
	Tool     string `mapstructure:"tool"`
	Behavior string `mapstructure:"behavior"`
	Path     string `mapstructure:"path"`
	Content  string `mapstructure:"content"`
}

// global config variable
var (
	globalConfig *Config
	configOnce   sync.Once
)

// InitConfig 初始化配置文件
func InitConfig() {
	configOnce.Do(func() {
		// Config file name
		viper.SetConfigName("config")
		// Config file type
		viper.SetConfigType("yaml")
		// config file path
		viper.AddConfigPath("./config")
		// find config file and read it
		err := viper.ReadInConfig()
		if err != nil {
			log.Fatalf("Fatal error config file: %s \n", err)
		}

		// parse config to struct `globalConfig` and save it to global variable
		globalConfig = &Config{}
		if err := viper.Unmarshal(globalConfig); err != nil {
			log.Fatalf("Fatal error unmarshaling config: %s \n", err)
		}
	})
}

// GetConfig 获取全局配置实例
func GetConfig() *Config {
	if globalConfig == nil {
		log.Fatal("Config not initialized. Call InitConfig() first.")
	}
	return globalConfig
}
