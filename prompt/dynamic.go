package prompt

import (
	"fmt"
	"runtime"
	"time"
)

// buildDynamic 生成动态环境信息段。
// 包含运行时变化的 4 项信息：当前日期、工作目录、模型名称、操作系统。
func (b *SystemPromptBuilder) buildDynamic() string {
	return fmt.Sprintf(
		"## Current Environment\n"+
			"- Date: %s\n"+
			"- Working directory: %s\n"+
			"- Model: %s\n"+
			"- OS: %s",
		time.Now().Format("2006-01-02"),
		b.workDir,
		b.model,
		runtime.GOOS,
	)
}
