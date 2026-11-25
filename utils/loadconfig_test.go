package utils

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// TestUnmarshalOllamaBaseURL 回归测试：OllamaConfig.BaseURL 曾因结构体标签拼写错误
// （mapstructre）而无法反序列化，导致 `-m ollama` 启动时误报 "BaseURL is empty"。
func TestUnmarshalOllamaBaseURL(t *testing.T) {
	const raw = `
ollama:
  base_url: "http://127.0.0.1:11434"
  model_name: "qwen3:8b"
`
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(raw)); err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if cfg.Ollama.BaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("Ollama.BaseURL = %q，期望 http://127.0.0.1:11434", cfg.Ollama.BaseURL)
	}
	if cfg.Ollama.ModelName != "qwen3:8b" {
		t.Fatalf("Ollama.ModelName = %q，期望 qwen3:8b", cfg.Ollama.ModelName)
	}
}

// TestConfigMapstructureTags 遍历 Config 及其嵌套结构体，校验 mapstructure 标签：
// 标签名拼写错误或缺失都会让对应配置项静默失效。
func TestConfigMapstructureTags(t *testing.T) {
	const want = "mapstructure"

	var walk func(tp reflect.Type)
	seen := make(map[reflect.Type]bool)
	walk = func(tp reflect.Type) {
		for tp.Kind() == reflect.Ptr || tp.Kind() == reflect.Slice || tp.Kind() == reflect.Array {
			tp = tp.Elem()
		}
		if tp.Kind() != reflect.Struct || seen[tp] {
			return
		}
		seen[tp] = true

		for i := 0; i < tp.NumField(); i++ {
			field := tp.Field(i)
			if field.PkgPath != "" { // 跳过未导出字段
				continue
			}
			keys := tagKeys(field.Tag)
			for _, key := range keys {
				// mapstructure 的常见拼写错误（mapstructre 等）都以 "mapstruct" 开头。
				if key != want && strings.HasPrefix(key, "mapstruct") {
					t.Errorf("%s.%s 的标签名拼写错误: %q，应为 %q", tp.Name(), field.Name, key, want)
				}
			}
			hasWant := false
			for _, key := range keys {
				if key == want {
					hasWant = true
				}
			}
			// 结构体类型的配置字段必须带标签，否则 viper 不会填充。
			if !hasWant && isStructKind(field.Type) {
				t.Errorf("%s.%s 缺少 %q 标签，配置项会静默失效", tp.Name(), field.Name, want)
			}
			walk(field.Type)
		}
	}

	walk(reflect.TypeOf(Config{}))
}

// tagKeys 解析结构体标签中的键名列表（标准库未提供枚举接口）。
func tagKeys(tag reflect.StructTag) []string {
	var keys []string
	s := string(tag)
	for {
		s = strings.TrimLeft(s, " ")
		if s == "" {
			return keys
		}
		i := strings.Index(s, ":")
		if i <= 0 {
			return keys
		}
		keys = append(keys, s[:i])
		rest := s[i+1:]
		if !strings.HasPrefix(rest, `"`) {
			return keys
		}
		// 跳过带引号的值，处理反斜杠转义。
		j := 1
		for j < len(rest) {
			if rest[j] == '\\' {
				j += 2
				continue
			}
			if rest[j] == '"' {
				break
			}
			j++
		}
		if j >= len(rest) {
			return keys
		}
		s = rest[j+1:]
	}
}

// isStructKind 判断类型（含指针/切片元素）是否为结构体。
func isStructKind(tp reflect.Type) bool {
	for tp.Kind() == reflect.Ptr || tp.Kind() == reflect.Slice || tp.Kind() == reflect.Array {
		tp = tp.Elem()
	}
	return tp.Kind() == reflect.Struct
}
