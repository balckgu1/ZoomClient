package permission

import (
	"regexp"
	"strings"
	"sync"
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

// Decision 一次权限检查的结果。reason 用于日志和给用户的解释。
type Decision struct {
	Behavior Behavior
	Reason   string
}

// readOnlyTools 被视作"只读、安全"的工具白名单。
var readOnlyTools = map[string]bool{
	"read_file":      true,
	"list_directory": true,
	"load_skill":     true,
	"todo":           true,
	"compact":        true,
	"glob_search":    true,
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
	// mu 保护 mode / DenyRules / AllowRules 的并发读写：
	// agentLoop 通过 Check 读取规则，Web 处理器可在运行时更新规则，二者可能并发。
	mu         sync.RWMutex
	mode       Mode
	DenyRules  []Rule // 命中即拒绝
	AllowRules []Rule // 命中即放行
	Asker      Asker  // 命中 ask 时如何与用户交互
}

// NewManager 构造一个权限管理器，asker 为 nil 时使用 DenyAsker
func NewManager(mode Mode, denyRules []Rule, allowRules []Rule, asker Asker) *Manager {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	switch mode {
	case ModeDefault, ModePlan, ModeAuto:
		m.mode = mode
	default:
		m.mode = ModeDefault
	}
}

// GetMode 返回当前模式
func (m *Manager) GetMode() Mode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}

// Check 执行权限检查
func (m *Manager) Check(toolName string, args map[string]any) Decision {
	// 读取 mode 与规则期间持有读锁；Check 只做快速匹配、不阻塞，
	// 真正会阻塞的 Asker.Ask 在 Decide 中于 Check 返回后调用，不会持锁等待用户。
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 检查是否命中 deny rules
	for _, rule := range m.DenyRules {
		if matchesRule(rule, toolName, args) {
			return Decision{
				Behavior: BehaviorDeny,
				Reason:   "matched deny rule: " + describeRule(rule),
			}
		}
	}

	// 若当前模式为 ModePlan，则只允许读类型 tool
	if m.mode == ModePlan && IsWrite(toolName) {
		return Decision{
			Behavior: BehaviorDeny,
			Reason:   "plan mode blocks write tool: " + toolName,
		}
	}

	// run_bash tool 安全检查
	if toolName == "run_bash" {
		cmd, ok := args["command"].(string)
		if ok {
			dangerous, reason := isDangerousBash(cmd)
			if dangerous {
				return Decision{
					Behavior: BehaviorDeny,
					Reason:   "bash safety: " + reason,
				}
			}
		}
	}

	// 检查是否命中 allow rules
	for _, rule := range m.AllowRules {
		if matchesRule(rule, toolName, args) {
			return Decision{
				Behavior: BehaviorAllow,
				Reason:   "matched allow rule: " + describeRule(rule),
			}
		}
	}

	// auto 模式自动放行只读工具（放在 allow rules 之后，让用户配置仍能覆盖）
	if m.mode == ModeAuto && IsReadOnly(toolName) {
		return Decision{
			Behavior: BehaviorAllow,
			Reason:   "auto mode allows read-only tool: " + toolName,
		}
	}

	// 都没命中，则交给用户决定
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
	// 判断是否放行该命令
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

// PermissionSnapshot 权限配置的只读快照，用于 Web 前端展示与编辑。
// 通过值拷贝隔离内部状态，避免前端直接持有 Manager 的规则切片。
type PermissionSnapshot struct {
	Mode       Mode   `json:"mode"`
	DenyRules  []Rule `json:"deny_rules"`
	AllowRules []Rule `json:"allow_rules"`
}

// Snapshot 返回当前权限配置（模式 + deny/allow 规则副本）。
// 读取期间持有读锁，返回的规则切片是内部切片的拷贝，调用方可安全修改。
func (m *Manager) Snapshot() PermissionSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	deny := make([]Rule, len(m.DenyRules))
	copy(deny, m.DenyRules)
	allow := make([]Rule, len(m.AllowRules))
	copy(allow, m.AllowRules)

	return PermissionSnapshot{
		Mode:       m.mode,
		DenyRules:  deny,
		AllowRules: allow,
	}
}

// UpdateRules 在运行时整体替换 deny / allow 规则列表。
// 写入期间持有写锁，保证与 Check 的读取互斥；传入切片会被拷贝，避免外部后续修改影响内部状态。
func (m *Manager) UpdateRules(deny, allow []Rule) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DenyRules = make([]Rule, len(deny))
	copy(m.DenyRules, deny)
	m.AllowRules = make([]Rule, len(allow))
	copy(m.AllowRules, allow)
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
