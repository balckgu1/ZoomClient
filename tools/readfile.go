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

func (t ReadFileTool) Call(args map[string]any, workpath string) string {
	filename, ok := args["filename"].(string)
	if !ok || filename == "" {
		return "Error: missing filename parameter or filename parameter is not a string"
	}
	targetPath, err := isSafePath(workpath, filename)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(content)
}
