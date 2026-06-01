package permission

import (
	"regexp"
	"strings"
)

// - ModeDefault：未命中规则时一律问用户
// - ModePlan   ：只允许读，不允许任何写/执行
// - ModeAuto   ：只读工具自动放行，写/执行类问用户
type Mode string

const (
	ModeDefault Mode = "default"
	ModePlan    Mode = "plan"
	ModeAuto    Mode = "auto"
)

// Behavior 单条规则或一次决策的行为。
type Behavior string

const (
	BehaviorAllow Behavior = "allow"
	BehaviorDeny  Behavior = "deny"
	BehaviorAsk   Behavior = "ask"
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

// Decision 一次权限检查的结果。reason 用于日志和给用户的解释。
type Decision struct {
	Behavior Behavior
	Reason   string
}

// readOnlyTools 被视作"只读、安全"的工具白名单。
var readOnlyTools = map[string]bool{
	"read_file":   true,
	"load_skill":  true,
	"todo":        true,
	"compact":     true,
	"glob_search": true,
}

// writeTools 被视作"会写文件 / 会跑命令 / 会跨上下文"的工具。
var writeTools = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"run_bash":   true,
	"sub_task":   true,
}

// IsReadOnly 报告某个工具是否被视作只读。
func IsReadOnly(toolName string) bool {
	return readOnlyTools[toolName]
}

// IsWrite 报告某个工具是否被视作写/执行类。
func IsWrite(toolName string) bool {
	return writeTools[toolName]
}

// Manager 权限管理器
type Manager struct {
	mode       Mode
	DenyRules  []Rule // 命中即拒绝
	AllowRules []Rule // 命中即放行
	Asker      Asker  // 命中 ask 时如何与用户交互
}

// NewManager 构造一个权限管理器，asker 为 nil 时使用 DenyAsker
func NewManager(mode Mode, denyRules, allowRules []Rule, asker Asker) *Manager {
	m := &Manager{
		DenyRules:  denyRules,
		AllowRules: allowRules,
		Asker:      asker,
	}
	m.SetMode(mode)
	if m.Asker == nil {
		m.Asker = DenyAsker{}
	}
	return m
}

// SetMode 切换当前模式。不合法的取值会回退到 ModeDefault
func (m *Manager) SetMode(mode Mode) {
	switch mode {
	case ModeDefault, ModePlan, ModeAuto:
		m.mode = mode
	default:
		m.mode = ModeDefault
	}
}

// Mode 返回当前模式
func (m *Manager) GetMode() Mode { return m.mode }

// Check 执行权限检查
func (m *Manager) Check(toolName string, args map[string]any) Decision {
	// 1. deny rules 最高优先级
	for _, rule := range m.DenyRules {
		if matchesRule(rule, toolName, args) {
			return Decision{
				Behavior: BehaviorDeny,
				Reason:   "matched deny rule: " + describeRule(rule),
			}
		}
	}

	// 2. 模式硬约束
	if m.mode == ModePlan && IsWrite(toolName) {
		return Decision{
			Behavior: BehaviorDeny,
			Reason:   "plan mode blocks write tool: " + toolName,
		}
	}

	// 3. bash 命令兜底安全检查
	if toolName == "run_bash" {
		if cmd, ok := args["command"].(string); ok {
			if dangerous, why := isDangerousBash(cmd); dangerous {
				return Decision{
					Behavior: BehaviorDeny,
					Reason:   "bash safety: " + why,
				}
			}
		}
	}

	// 4. allow rules
	for _, rule := range m.AllowRules {
		if matchesRule(rule, toolName, args) {
			return Decision{
				Behavior: BehaviorAllow,
				Reason:   "matched allow rule: " + describeRule(rule),
			}
		}
	}

	// auto 模式默认放行只读工具（放在 allow rules 之后，让用户配置仍能覆盖）
	if m.mode == ModeAuto && IsReadOnly(toolName) {
		return Decision{
			Behavior: BehaviorAllow,
			Reason:   "auto mode allows read-only tool: " + toolName,
		}
	}

	// 5. fallback：交给用户
	return Decision{
		Behavior: BehaviorAsk,
		Reason:   "no rule matched in mode " + string(m.mode),
	}
}

// Decide 把 Check + 用户询问串成一句话语义：放行 or 拒绝。
//
//   - allow=true  → RunTool 继续执行工具
//   - allow=false → RunTool 直接返回 "Permission denied: <reason>"
func (m *Manager) Decide(toolName string, args map[string]any) (bool, string) {
	decision := m.Check(toolName, args)
	switch decision.Behavior {
	case BehaviorAllow:
		return true, decision.Reason
	case BehaviorDeny:
		return false, decision.Reason
	case BehaviorAsk:
		ok, why := m.Asker.Ask(toolName, args, decision.Reason)
		if ok {
			return true, "user approved: " + decision.Reason
		}
		if !ok && why == "" {
			why = "denied by user"
		}
		return false, why
	}
	return false, "unknown decision"
}

// matchesRule 判断一条 Rule 是否命中本次工具调用。
// 工具名 + path + content 三个维度都必须匹配（未填的维度视为通过）。
func matchesRule(rule Rule, toolName string, args map[string]any) bool {
	if rule.Tool != "" && rule.Tool != "*" && rule.Tool != toolName {
		return false
	}
	if rule.Path != "" && !matchPathArg(rule.Path, args) {
		return false
	}
	if rule.Content != "" && !matchContentArg(rule.Content, args) {
		return false
	}
	return true
}

// matchPathArg 在常见的"路径类参数"中按模式匹配。
func matchPathArg(pattern string, args map[string]any) bool {
	for _, key := range []string{"filename", "path", "file"} {
		if v, ok := args[key].(string); ok && substringOrRegex(pattern, v) {
			return true
		}
	}
	return false
}

// matchContentArg 在常见的"文本类参数"中按模式匹配。
func matchContentArg(pattern string, args map[string]any) bool {
	for _, key := range []string{"command", "content", "prompt"} {
		if v, ok := args[key].(string); ok && substringOrRegex(pattern, v) {
			return true
		}
	}
	return false
}

// substringOrRegex 支持两种 pattern：
//   - "re:xxx" → 按正则匹配
//   - 其他      → 按子串包含匹配
func substringOrRegex(pattern, target string) bool {
	if strings.HasPrefix(pattern, "re:") {
		rx, err := regexp.Compile(pattern[3:])
		if err != nil {
			return false
		}
		return rx.MatchString(target)
	}
	return strings.Contains(target, pattern)
}

// describeRule 把一条规则压成单行字符串，便于写入 reason 与日志。
func describeRule(rule Rule) string {
	parts := []string{string(rule.Behavior) + " " + rule.Tool}
	if rule.Path != "" {
		parts = append(parts, "path~"+rule.Path)
	}
	if rule.Content != "" {
		parts = append(parts, "content~"+rule.Content)
	}
	return strings.Join(parts, " ")
}
