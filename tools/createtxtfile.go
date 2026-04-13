package tools

import (
	"fmt"
	"os"
)

// CreateTxtFileTool 示例工具 - 创建txt文件
type CreateTxtFileTool struct{}

func (t CreateTxtFileTool) Name() string {
	return "create_txt_file"
}

func (t CreateTxtFileTool) Description() string {
	return "创建一个txt文件，需要提供文件名和文件内容作为参数"
}

func (t CreateTxtFileTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "要创建的txt文件名，例如 example.txt",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "写入文件的文本内容",
			},
		},
		"required": []string{"filename", "content"},
	}
}

func (t CreateTxtFileTool) Call(args map[string]interface{}) string {
	filename, ok := args["filename"].(string)
	if !ok || filename == "" {
		return "错误：缺少有效的 filename 参数"
	}

	content, ok := args["content"].(string)
	if !ok {
		content = ""
	}

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		return fmt.Sprintf("错误：创建文件失败 - %v", err)
	}

	return fmt.Sprintf("成功创建文件: %s，内容长度: %d 字节", filename, len(content))
}
