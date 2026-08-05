package tools

// Tool 定义工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Call(args map[string]interface{}, ctx *ToolContext) ToolResult
}
