package tools

import (
	"fmt"
	"os"
)

type ReadFileTool struct{}

func (t ReadFileTool) Name() string {
	return "read_file"
}

func (t ReadFileTool) Description() string {
	return "Read file content, need to provide file name as parameter"
}

func (t ReadFileTool) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filename": map[string]any{
				"type":        "string",
				"description": "File name to read content from",
			},
		},
		"required": []string{"filename"},
	}
	return parameters
}

func (t ReadFileTool) Call(args map[string]any, ToolCtx *ToolContext) ToolResult {
	filename, ok := args["filename"].(string)
	if !ok || filename == "" {
		return ToolResult{Ok: false, Content: "Error: missing filename parameter or filename parameter is not a string", IsError: true}
	}
	targetPath, err := isSafePath(ToolCtx.WorkPath, filename)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	return ToolResult{Ok: true, Content: string(content), IsError: false, Attachments: nil}
}
