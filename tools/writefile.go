package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// isSafePath 验证 filename 解析后的绝对路径是否在 workpath 工作区目录内。
// 返回规范化后的目标绝对路径，防止路径穿越（如 ../../ 或绝对路径逃逸）。
func isSafePath(workpath string, filename string) (string, error) {
	// 规范化工作目录为绝对路径
	cleanWorkpath, err := filepath.Abs(filepath.Clean(workpath))
	if err != nil {
		return "", fmt.Errorf("failed to resolve work directory: %v", err)
	}

	// 将 filename 解析为绝对路径
	var targetPath string
	if filepath.IsAbs(filename) {
		targetPath = filepath.Clean(filename)
	} else {
		targetPath = filepath.Clean(filepath.Join(cleanWorkpath, filename))
	}

	// 检查目标路径是否在工作区内
	if targetPath != cleanWorkpath && !strings.HasPrefix(targetPath, cleanWorkpath+string(filepath.Separator)) {
		return "", fmt.Errorf("path escape: %q is not within the work directory %q", targetPath, cleanWorkpath)
	}

	return targetPath, nil
}

type WriteFileTool struct{}

func (t WriteFileTool) Name() string {
	return "write_file"
}

func (t WriteFileTool) Description() string {
	return "Create a new file and write content to it. Parent directories are created automatically if they do not exist."
}

func (t WriteFileTool) Parameters() map[string]any {
	parameter := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filename": map[string]any{
				"type":        "string",
				"description": "The name of the file to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to write to the file.",
			},
		},
		"required": []string{"filename", "content"},
	}
	return parameter
}

func (t WriteFileTool) Call(args map[string]any, toolCtx *ToolContext) ToolResult {
	filename, ok := args["filename"].(string)
	if !ok || filename == "" {
		return ToolResult{
			Ok:          false,
			Content:     "Error: missing filename parameter or filename parameter is not a string",
			IsError:     true,
			Attachments: nil,
		}
	}
	content, ok := args["content"].(string)
	if !ok {
		return ToolResult{Ok: false, Content: "Error: missing content parameter", IsError: true}
	}

	targetPath, err := isSafePath(toolCtx.WorkPath, filename)
	toolCtx.Logger.Info("Writing file", zap.String("session", toolCtx.SessionID), zap.String("filename", filename), zap.String("workdir", targetPath))
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	err = os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: failed to create directory: %v", err), IsError: true}
	}

	err = os.WriteFile(targetPath, []byte(content), 0644)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	return ToolResult{Ok: true, Content: fmt.Sprintf("Success to write file: %s, content length: %d bytes", targetPath, len(content))}
}
