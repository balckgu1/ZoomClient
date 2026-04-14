package tools

import (
	"fmt"
	"os"
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

func (t EditFileTool) Call(args map[string]any, workpath string) string {
	filename, ok := args["filename"].(string)
	if filename == "" || !ok {
		return "Error: missing filename parameter or filename parameter is not a string"
	}
	content, ok := args["content"].(string)
	if content == "" || !ok {
		return "Error: missing content parameter or content parameter is not a string"
	}
	targetPath, err := isSafePath(workpath, filename)
	if err != nil {
		return "Error: " + err.Error()
	}
	file, err := os.OpenFile(targetPath, os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file '%s' does not exist", filename)
		}
		return fmt.Sprintf("Error: %v", err)
	}
	defer file.Close()

	nbytes, err := file.WriteString(content)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return fmt.Sprintf("Success: wrote %d bytes to file %s", nbytes, targetPath)
}
