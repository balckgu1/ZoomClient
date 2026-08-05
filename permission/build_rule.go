package permission

import (
	"zoomClient/utils"
)

// Rule 一条权限规则。
//
//   - Tool     ：针对哪个工具（"" 或 "*" 表示任意工具）
//   - Behavior ：命中后的处理方式（allow / deny / ask）
//   - Path     ：可选；命中工具的 filename / path / file 参数子串
//   - Content  ：可选；命中工具的 command / content / prompt 参数子串
//
// Path / Content 支持两种写法：
//   - 普通子串：直接 Contains 匹配（默认）
//   - 正则：以 "re:" 开头，例如 "re:^git\\s+push"
type Rule struct {
	Tool     string   `mapstructure:"tool"     yaml:"tool"`
	Behavior Behavior `mapstructure:"behavior" yaml:"behavior"`
	Path     string   `mapstructure:"path"     yaml:"path"`
	Content  string   `mapstructure:"content"  yaml:"content"`
}

// BuildPermissionRules converts the PermissionRuleConfig list in config to permission.Rule.
func BuildPermissionRules(rawRules []utils.PermissionRuleConfig) []Rule {
	rules := make([]Rule, 0, len(rawRules))

	for _, raw := range rawRules {
		rules = append(
			rules, Rule{
				Tool:     raw.Tool,
				Behavior: Behavior(raw.Behavior),
				Path:     raw.Path,
				Content:  raw.Content,
			},
		)
	}
	return rules
}
