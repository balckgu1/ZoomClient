package tools

import (
	"fmt"
	"os"

	"go.uber.org/zap"
)

type EditFileTool struct{}

func (t EditFileTool) Name() string {
	return "edit_file"
}

func (t EditFileTool) Description() string {
	return "Edit file content if file is exist, need to provide file name and new content as parameters"
}

func (t EditFileTool) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filename": map[string]any{
				"type":        "string",
				"description": "File name to edit content",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "New content to replace file content",
			},
		},
		"required": []string{"filename", "content"},
	}
	return parameters
}

func (t EditFileTool) Call(args map[string]any, ToolCtx *ToolContext) ToolResult {
	filename, ok := args["filename"].(string)
	if filename == "" || !ok {
		return ToolResult{Ok: false, Content: "Error: missing filename parameter or filename parameter is not a string", IsError: true}
	}
	content, ok := args["content"].(string)
	if content == "" || !ok {
		return ToolResult{Ok: false, Content: "Error: missing content parameter or content parameter is not a string", IsError: true}
	}
	targetPath, err := isSafePath(ToolCtx.WorkPath, filename)
	if err != nil {
		return ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	ToolCtx.Logger.Info("Editing file", zap.String("session", ToolCtx.SessionID), zap.String("filename", filename), zap.String("workdir", targetPath))
	file, err := os.OpenFile(targetPath, os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolResult{
				Ok:      false,
				Content: fmt.Sprintf("Error: file '%s' does not exist", filename),
				IsError: true,
			}
		}
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	defer file.Close()

	nbytes, err := file.WriteString(content)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	return ToolResult{
		Ok:          true,
		Content:     fmt.Sprintf("Success: wrote %d bytes to file %s", nbytes, targetPath),
		IsError:     false,
		Attachments: nil,
	}
}
