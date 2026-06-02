package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

type ListDirectory struct{}

func (l ListDirectory) Name() string {
	return "list_directory"
}

func (l ListDirectory) Description() string {
	return "List files and directories in the given directory. " +
		"Returns names with type indicators (/ for directories) and file sizes."
}

func (l ListDirectory) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "The directory to list, relative to work directory. Defaults to work directory root if empty.",
			},
		},
	}
	return parameters
}

func (l ListDirectory) Call(args map[string]any, toolCtx *ToolContext) ToolResult {
	// directory 参数可选，为空时列出工作目录根
	dirArg, _ := args["directory"].(string)

	baseDir := toolCtx.WorkPath
	if dirArg != "" {
		resolved, err := isSafePath(toolCtx.WorkPath, dirArg)
		if err != nil {
			return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
		}
		baseDir = resolved
	}

	toolCtx.Logger.Info("List directory",
		zap.String("session", toolCtx.SessionID),
		zap.String("directory", baseDir),
	)

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	if len(entries) == 0 {
		return ToolResult{Ok: true, Content: "(empty directory)"}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Directory: %s\n", filepath.ToSlash(baseDir)))
	sb.WriteString(fmt.Sprintf("Entries: %d\n", len(entries)))
	sb.WriteString("---\n")

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("  %s/\n", name))
		} else {
			info, infoErr := entry.Info()
			if infoErr != nil {
				sb.WriteString(fmt.Sprintf("  %s  (stat error)\n", name))
				continue
			}
			sb.WriteString(fmt.Sprintf("  %s  (%s)\n", name, formatSize(info.Size())))
		}
	}

	return ToolResult{Ok: true, Content: sb.String()}
}

// formatSize 将字节数格式化为可读大小
func formatSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	}
}
