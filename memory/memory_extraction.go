package memory

// 本文件实现记忆抽取的同步核心：
//   - extractMemoryFromWindow：窗口抽取核心，供 Extractor.Run 调用；
//   - shouldRunMemoryExtraction：skip 判定（无散文 / 用户要求忽略 / 已手写记忆）。
// 抽取 prompt / 受限工具集 / 清单构建在 memory.go；
// per-turn 同步调度见 memory_extractor.go。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"zoomClient/clients"
	"zoomClient/emitter"
	"zoomClient/logger"
	"zoomClient/subagent"
	"zoomClient/tools"

	"go.uber.org/zap"
)

const (
	memoryExtractionMaxTurns    = 5                // 抽取记忆subagent的运行最大轮数
	ExtractionTimeout           = 60 * time.Second // 是用户等待的硬上限；超时即放弃并退出
	memoryExtractionTail        = 20               // fork 给抽取记忆subagent的最近消息数
	minUserRunesForExtraction   = 10               // 触发抽取记忆所需的最小用户散文 rune 数
	memoryExtractionTemperature = 0.3              // 抽取记忆的subagent温度
)

// memoryIgnorePhrases 是跳过抽取记忆的user prompt
var memoryIgnorePhrases = []string{
	"不要记", "别记", "忽略记忆", "不用记", "不要保存记忆",
	"don't remember", "do not remember", "ignore memory", "don't save memory",
}

// extractMemoryFromWindow 对给定消息窗口执行一次同步记忆抽取。
// 返回 nil 表示抽取记忆完成；error 表示失败或超时
func extractMemoryFromWindow(client clients.ChatClient, model string, toolCtx *tools.ToolContext,
	memoryDir string, window []clients.Message, em emitter.Emitter) error {

	// 获取消息序列的最后 memoryExtractionTail条
	tail := tailMessages(window, memoryExtractionTail)

	// 注入超时 ctx 作 subagent Loop 的轮次边界
	extractionCtx, cancel := context.WithTimeout(context.Background(), ExtractionTimeout)
	defer cancel()
	subToolCtx := *toolCtx
	subToolCtx.Ctx = extractionCtx

	// 创建一个用于抽取记忆的 subagent
	extractor := subagent.NewSubAgent(
		client, model,
		ExtractionSystemPrompt,
		ExtractionForkPrefix,
		BuildExtractionRegistry(memoryDir),
		&subToolCtx,
		memoryExtractionMaxTurns,
		map[string]interface{}{"temperature": memoryExtractionTemperature},
	)

	// 构造抽取记忆的 prompt
	prompt := BuildExtractionForkPrompt(BuildManifest(memoryDir))

	emitInfo(em, "正在保存记忆…")

	type extractionResult struct {
		summary string
		err     error
	}

	done := make(chan extractionResult, 1)
	go func() {
		s, err := extractor.RunWithFork(prompt, tail)
		done <- extractionResult{summary: s, err: err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			if logger.Log != nil {
				logger.Log.Warn("memory extraction failed", zap.Error(res.err))
			}
			emitInfo(em, "记忆保存未完成")
			return res.err
		}
		if strings.Contains(res.summary, "Nothing to save") {
			emitInfo(em, "无新增记忆")
		} else {
			emitInfo(em, "记忆已更新")
		}
		return nil
	case <-time.After(ExtractionTimeout):
		if logger.Log != nil {
			logger.Log.Warn("memory extraction timed out", zap.Duration("timeout", ExtractionTimeout))
		}
		emitInfo(em, "记忆保存超时，已跳过")
		return context.DeadlineExceeded
	}
}

// shouldRunMemoryExtraction 判断给定消息窗口是否值得抽取。
// 满足任一条件则返回 (false, 原因)：
//  1. 窗口内用户消息总数 < 阈值；
//  2. 用户要求忽略记忆；
//  3. 模型已调用过 save_memory/update_memory
func shouldRunMemoryExtraction(messages []clients.Message) (bool, string) {
	var userRunes int
	sawHandWrite := false

	// 遍历每条消息
	for _, m := range messages {
		switch m.Role {
		case "user":
			content := messageContentToString(m.Content)
			userRunes += len([]rune(content))
			lower := strings.ToLower(content)
			for _, p := range memoryIgnorePhrases {
				if strings.Contains(lower, strings.ToLower(p)) {
					return false, "user requested to ignore memory"
				}
			}
		case "assistant":
			for _, tc := range m.ToolCalls {
				if tc.Name == "save_memory" || tc.Name == "update_memory" {
					sawHandWrite = true
				}
			}
		}
	}

	if userRunes < minUserRunesForExtraction {
		return false, "no substantial user prose"
	}
	if sawHandWrite {
		return false, "memory already written manually in window"
	}
	return true, ""
}

// tailMessages 返回消息序列的最后 n 条（不足 n 则原样返回）。
func tailMessages(messages []clients.Message, n int) []clients.Message {
	if n <= 0 || len(messages) <= n {
		return messages
	}
	return messages[len(messages)-n:]
}

// emitInfo 在 emitter 非 nil 时推送一条普通提示。
func emitInfo(em emitter.Emitter, msg string) {
	if em != nil {
		em.EmitInfo(msg)
	}
}

// messageContentToString 把消息内容（string / 多模态结构）安全转换为字符串，
// 语义与 main 包渲染函数一致，供 skip 判定与增量指纹计算使用。
func messageContentToString(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
