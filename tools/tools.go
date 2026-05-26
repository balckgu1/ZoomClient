package tools

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Tool 定义工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Call(args map[string]interface{}, ctx *ToolContext) ToolResult
}

// ToolContext 工具执行上下文
type ToolContext struct {
	WorkPath           string          // 当前工作目录
	Ctx                context.Context // 超时控制与优雅取消
	DefaultBashTimeout time.Duration   // 工具执行超时时间
	Logger             *zap.Logger     // 结构化日志
	SessionID          string          // 当前会话标识
	Handlers           map[string]any  // 额外处理器（预留给 MCP / agent 等能力来源）
	McpClients         map[string]any  // MCP 外部客户端
	Messages           []any           // 当前消息列表
	AppState           map[string]any  // 应用状态
	Notifications      []any           // 通知队列
}

// ToolResult 工具执行结果
type ToolResult struct {
	Ok          bool   // 是否成功
	Content     string // 返回内容
	IsError     bool   // 是否为错误结果
	Attachments []any
}

// String 返回结果内容，便于直接使用
func (r ToolResult) String() string {
	return r.Content
}

// ToolCall 表示模型返回的工具调用
// ID 字段为 OpenAI 兼容协议（如 DeepSeek）所必需，用于关联 tool 角色消息；
// 在 Ollama 协议中该字段可能为空，序列化时会被自动忽略。
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具调用的函数信息
type ToolCallFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// PermissionDecider 在工具真正执行前的权限闸门函数。
//
// 返回 allow=false 时，RunTool 直接以错误结果返回，不会调用工具本体。
type PermissionDecider func(toolName string, args map[string]interface{}) (allow bool, reason string)

// Registry 工具注册表，管理所有可用工具
type Registry struct {
	tools  map[string]Tool
	permit PermissionDecider // 可选：执行前的权限闸门，为 nil 时不做权限检查
}

// NewRegistry 创建新的工具注册表
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册一个工具
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// SetPermissionDecider 注入权限闸门，为 nil 时 RunTool 不做权限检查
func (r *Registry) SetPermissionDecider(fn PermissionDecider) {
	r.permit = fn
}

// GetAll 返回所有已注册的工具
func (r *Registry) GetAll() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// GetAllNames 返回所有已注册的工具名
func (r *Registry) GetAllNames() []string {
	toolList := make([]string, 0, len(r.tools))
	for _, t := range r.tools {
		toolList = append(toolList, t.Name())
	}
	return toolList
}

// RunTool 按名称执行工具。 若已通过 SetPermissionDecider 注入权限闸门，会先做一次权限判定；拒绝时直接返回，不调用工具
func (r *Registry) RunTool(toolName string, args map[string]interface{}, toolCtx *ToolContext) ToolResult {
	t, ok := r.tools[toolName]
	if !ok {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Unknown tool: %s", toolName), IsError: true}
	}

	// permission check
	if r.permit != nil {
		allow, reason := r.permit(toolName, args)
		if !allow {
			return ToolResult{
				Ok:      false,
				Content: "Permission denied: " + reason,
				IsError: true,
			}
		}
	}

	return t.Call(args, toolCtx)
}
