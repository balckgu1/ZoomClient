package tools

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

type ToolContext struct {
	WorkPath           string          // 当前工作目录
	Ctx                context.Context // 超时控制与优雅退出
	DefaultBashTimeout time.Duration   // 工具执行超时时间
	Logger             *zap.Logger     // 结构化日志
	SessionID          string          // 当前会话标识
	AppState           map[string]any  //应用状态
	Messages           []any           // 当前消息列表
	Notifications      []any           // 通知队列
}

type ToolResult struct {
	Content     string // 工具执行结果
	Attachments []any  // 附件
	IsError     bool   // 是否出现了错误
	Ok          bool   // 是否成功
}

type ToolRegister struct {
	tools             map[string]Tool
	permissionDecider func(string, map[string]any) (bool, string)
}

// ToolCall 表示模型返回的工具调用
type ToolCall struct {
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// NewToolRegister 创建一个新的工具注册表实例
func NewToolRegister() *ToolRegister {
	return &ToolRegister{
		tools: make(map[string]Tool),
	}
}

// Register 注册一个工具
func (r *ToolRegister) Register(t Tool) {
	r.tools[t.Name()] = t
}

// GetAllNames 返回所有已注册的工具名
func (r *ToolRegister) GetAllNames() []string {
	result := make([]string, 0, len(r.tools))
	for _, v := range r.tools {
		result = append(result, v.Name())
	}
	return result
}

// GetAll 返回所有已注册的工具
func (r *ToolRegister) GetAll() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, v := range r.tools {
		result = append(result, v)
	}
	return result
}

// SetPermissionDecider 设置权限判定器
func (r *ToolRegister) SetPermissionDecider(decider func(string, map[string]any) (bool, string)) {
	r.permissionDecider = decider
}

// RunTool 按名称执行单个工具。
// 若已通过 SetPermissionDecider 注入权限闸门，会先做一次权限判定；拒绝时直接返回，不调用工具
func (r *ToolRegister) RunTool(toolName string, args map[string]interface{}, toolCtx *ToolContext) ToolResult {
	tool, ok := r.tools[toolName]
	if !ok {
		return ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Unknown tool: %s", toolName),
			IsError: true,
		}
	}
	if r.permissionDecider != nil {
		isAllow, reason := r.permissionDecider(toolName, args)
		if !isAllow {
			return ToolResult{
				Ok:      false,
				Content: fmt.Sprintf("Permission denied: %s", reason),
				IsError: true,
			}
		}
	}

	return tool.Call(args, toolCtx)
}
