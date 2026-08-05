package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// isSafePath 验证 filename 解析后的绝对路径是否在 workpath 工作区目录内。
// 返回规范化后的绝对路径
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
