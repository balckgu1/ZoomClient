package utils

import (
	"log"
	"sync"

	"github.com/spf13/viper"
)

// Config config struct
type Config struct {
	ApiKey   ApiKeyConfig   `mapstructure:"api_key"`
	Subagent SubagentConfig `mapstructure:"subagent"`
}

// ApiKeyConfig api_key config
type ApiKeyConfig struct {
	Deepseek string `mapstructure:"deepseek"`
	Openai   string `mapstructure:"openai"`
	Qwen     string `mapstructure:"qwen"`
}

// SubagentConfig subagent config
type SubagentConfig struct {
	DefaultMaxTurns         int    `mapstructure:"defaultMaxTurns"`
	DefaultSystemPrompt     string `mapstructure:"defaultSystemPrompt"`
	ForkSubtaskPromptPrefix string `mapstructure:"forkSubtaskPromptPrefix"`
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
