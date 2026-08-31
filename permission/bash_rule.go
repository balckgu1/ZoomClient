package permission

import (
	"strings"

	"zoomClient/utils"
)

// dangerousBashSubstrings 内置默认危险命令模式
// 仅当配置未初始化或未配置任何 deny 规则时使用
var dangerousBashSubstrings = []string{
	"sudo ",
	"rm -rf /",
	"rm -rf /*",
	"mkfs",
	"shutdown",
	"reboot",
	"> /dev/sda",
	":(){:|:&};:", // Classic fork bomb
}

// DangerousBashPatterns 返回统一的危险 bash 命令模式集
//
// 优先从配置文件 permission.denyRules 中提取 tool 为 run_bash 的 content；
// 配置未初始化、未配置任何规则时，回退到内置默认集 dangerousBashSubstrings。
func DangerousBashPatterns() []string {
	if cfg := utils.GetConfigSafe(); cfg != nil {
		var patterns []string
		for _, rule := range cfg.Permission.DenyRules {
			if rule.Content == "" || strings.HasPrefix(rule.Content, "re:") {
				continue
			}
			if rule.Tool == "" || rule.Tool == "*" || rule.Tool == "run_bash" {
				patterns = append(patterns, rule.Content)
			}
		}
		if len(patterns) > 0 {
			return patterns
		}
	}
	return dangerousBashSubstrings
}

// isDangerousBash Determine whether a bash command should be rejected
func isDangerousBash(command string) (bool, string) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false, ""
	}
	lowered := strings.ToLower(cmd)

	for _, key := range DangerousBashPatterns() {
		if strings.Contains(lowered, key) {
			return true, "dangerous bash keyword: " + key
		}
	}
	return false, ""
}
