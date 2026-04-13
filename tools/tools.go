package tools

import "fmt"

// Tool 定义工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Call(args map[string]interface{}) string
}

// ToolCall 表示模型返回的工具调用
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具调用的函数信息
type ToolCallFunction struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// Registry 工具注册表，管理所有可用工具
type Registry struct {
	tools map[string]Tool
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

// GetAll 返回所有已注册的工具
func (r *Registry) GetAll() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// RunTool 按名称执行工具
func (r *Registry) RunTool(toolName string, args map[string]interface{}) string {
	t, ok := r.tools[toolName]
	if !ok {
		return fmt.Sprintf("Unknown tool: %s", toolName)
	}
	return t.Call(args)
}
