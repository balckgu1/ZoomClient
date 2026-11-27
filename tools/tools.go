// Package tools 定义 agent 可调用的内置工具：文件读写、路径/内容搜索、
// 命令执行、联网抓取与搜索、记忆、任务管理以及向用户提问等，
// 并提供统一的注册表（ToolRegister）与执行框架。
package tools

// Tool 定义工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Call(args map[string]interface{}, ctx *ToolContext) ToolResult
}
