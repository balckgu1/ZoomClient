package tools

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"go.uber.org/zap"
)

// // defaultBashTimeout 命令执行的默认超时时间
// const defaultBashTimeout = 30 * time.Second

type RunBashTool struct{}

func isDangerousCommand(command string) bool {
	// 通用危险命令
	dangerous := []string{"shutdown", "reboot"}

	// 平台特定危险命令
	if runtime.GOOS == "windows" {
		dangerous = append(dangerous,
			"format ",    // 格式化磁盘
			"del /f",     // 强制删除
			"rmdir /s",   // 递归删除目录
			"diskpart",   // 磁盘分区工具
			"reg delete", // 删除注册表
		)
	} else {
		dangerous = append(dangerous,
			"rm -rf /", // 递归强制删除根目录
			"sudo",     // 提权
			"> /dev/",  // 写入块设备
			"mkfs.",    // 格式化文件系统
		)
	}

	lowered := strings.ToLower(command)
	for _, d := range dangerous {
		if strings.Contains(lowered, d) {
			return true
		}
	}
	return false
}

func (t RunBashTool) Name() string {
	return "run_bash"
}

func (t RunBashTool) Description() string {
	if runtime.GOOS == "windows" {
		return "Execute a command via cmd.exe. Use Windows-compatible commands (e.g., dir, type). "
	}
	return "Execute a command via bash. Use Unix-compatible commands (e.g., ls, cat). "
}

func (t RunBashTool) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Command to execute",
			},
		},
		"required": []string{"command"},
	}
	return parameters
}

// resolveShell 根据当前操作系统构建合适的 shell 执行命令。
// Windows：chcp 65001 将控制台代码页切换为 UTF-8，避免中文 GBK 乱码。
// Linux/macOS：直接使用 bash -c。
func resolveShellWithContext(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", "chcp 65001 >nul 2>&1 && "+command)
	}
	return exec.CommandContext(ctx, "bash", "-c", command)
}

func (t RunBashTool) Call(args map[string]any, ToolCtx *ToolContext) ToolResult {
	command, ok := args["command"].(string)
	if command == "" || !ok {
		return ToolResult{Ok: false, Content: "Error: command parameter is missing or not a string", IsError: true}
	}
	if isDangerousCommand(command) {
		return ToolResult{Ok: false, Content: "Error: dangerous command detected", IsError: true}
	}
	// Execute the command
	_, err := isSafePath(ToolCtx.WorkPath, command)
	if err != nil {
		return ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}

	// 使用 context 实现超时控制与取消传播
	execCtx := context.Background()
	if ToolCtx.Ctx != nil {
		execCtx = ToolCtx.Ctx
	}
	execCtx, cancel := context.WithTimeout(execCtx, ToolCtx.DefaultBashTimeout)
	defer cancel()

	ToolCtx.Logger.Info("Executing command", zap.String("session", ToolCtx.SessionID), zap.String("workdir", ToolCtx.WorkPath), zap.String("command", command))
	cmd := resolveShellWithContext(execCtx, command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return ToolResult{
				Ok:      false,
				Content: fmt.Sprintf("Error: command timed out after %s", ToolCtx.DefaultBashTimeout),
				IsError: true,
			}
		}
		if execCtx.Err() == context.Canceled {
			return ToolResult{
				Ok:      false,
				Content: "Error: command was cancelled",
				IsError: true,
			}
		}
		return ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	return ToolResult{Ok: true, Content: string(output), IsError: false, Attachments: nil}
}
