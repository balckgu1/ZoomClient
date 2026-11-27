package tools

// 本文件实现 ask_user_question 工具：模型在缺少关键信息时向用户发起结构化提问。
// 提问通道通过 UserAsker 抽象注入：CLI 模式从标准输入读取，
// Web 模式通过 SSE 事件推送到前端并由 HTTP 回复；两者共用同一个工具实现。

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// maxQuestionOptions 单个问题允许携带的选项上限，避免前端弹窗过长。
const maxQuestionOptions = 6

// QuestionRequest 一次结构化提问。
type QuestionRequest struct {
	Header   string   // 简短标题，用于前端弹窗标识问题主题
	Question string   // 问题正文
	Options  []string // 可选项；为空时用户自由输入
}

// UserAsker 向用户提问并等待回答的抽象。
// 实现必须阻塞直到用户回答、取消或超时，并在失败时返回 error。
type UserAsker interface {
	AskQuestion(req QuestionRequest) (string, error)
}

// AskUserQuestionTool 让模型向用户发起提问。
// 注意：这是交互式工具，不能与其他工具并发执行（见 concurrencySafeTools）。
type AskUserQuestionTool struct{}

func (t AskUserQuestionTool) Name() string { return "ask_user_question" }

func (t AskUserQuestionTool) Description() string {
	return "Ask the user a question and wait for their answer. Use it ONLY when you are blocked on information " +
		"you cannot determine yourself (missing requirements, ambiguous choices, destructive-action confirmation). " +
		"Provide 2-6 options when the answer is a choice; leave options empty for free-form input."
}

func (t AskUserQuestionTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The question to ask. Be specific and self-contained.",
			},
			"header": map[string]any{
				"type":        "string",
				"description": "Optional short label (a few words) describing the question topic.",
			},
			"options": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional answer choices (max 6). The user may still answer freely.",
			},
		},
		"required": []string{"question"},
	}
}

func (t AskUserQuestionTool) Call(args map[string]any, toolCtx *ToolContext) ToolResult {
	question, ok := args["question"].(string)
	if !ok || strings.TrimSpace(question) == "" {
		return ToolResult{Ok: false, Content: "Error: missing question parameter or question parameter is not a string", IsError: true}
	}
	if toolCtx == nil || toolCtx.UserAsker == nil {
		return ToolResult{Ok: false, Content: "Error: ask_user_question is not available in this session", IsError: true}
	}

	header, _ := args["header"].(string)

	// 选项既可能是 []string（直接调用），也可能是 []any（JSON 反序列化）
	options := make([]string, 0, maxQuestionOptions)
	seen := make(map[string]bool, maxQuestionOptions)
	appendOption := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return false
		}
		seen[s] = true
		options = append(options, s)
		return len(options) >= maxQuestionOptions
	}
	if raw, ok := args["options"].([]any); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok && appendOption(s) {
				break
			}
		}
	} else if raw, ok := args["options"].([]string); ok {
		for _, s := range raw {
			if appendOption(s) {
				break
			}
		}
	}

	answer, err := toolCtx.UserAsker.AskQuestion(QuestionRequest{
		Header:   strings.TrimSpace(header),
		Question: strings.TrimSpace(question),
		Options:  options,
	})
	if err != nil {
		return ToolResult{Ok: false, Content: "Error: " + err.Error(), IsError: true}
	}
	if strings.TrimSpace(answer) == "" {
		return ToolResult{Ok: false, Content: "Error: user did not answer the question", IsError: true}
	}
	return ToolResult{Ok: true, Content: strings.TrimSpace(answer)}
}

// writeTo 输出提问内容。终端写入失败在当前场景下无法补救（输入读取失败仍会返回 error），
// 因此显式忽略错误，避免为不可恢复的 stdout 故障增加返回分支。
func writeTo(w io.Writer, format string, args ...any) {
	//nolint:errcheck // 终端写入失败无法补救
	fmt.Fprintf(w, format, args...)
}

// StdinUserAsker 通过标准输入完成提问（CLI 模式）。
// 与 permission.StdinAsker 保持一致：直接读取 os.Stdin 的一行，
// 数字输入映射到对应选项，其余内容按自由回答处理。
type StdinUserAsker struct {
	Out io.Writer // 默认 os.Stdout
	In  io.Reader // 默认 os.Stdin
}

func (a *StdinUserAsker) AskQuestion(req QuestionRequest) (string, error) {
	out := a.Out
	if out == nil {
		out = os.Stdout
	}
	in := a.In
	if in == nil {
		in = os.Stdin
	}

	writeTo(out, "\n")
	if req.Header != "" {
		writeTo(out, "[question] %s\n", req.Header)
	}
	writeTo(out, "%s\n", req.Question)
	for i, opt := range req.Options {
		writeTo(out, "  %d) %s\n", i+1, opt)
	}
	writeTo(out, "answer: ")

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return "", errors.New("no input available, question cancelled")
	}
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return "", errors.New("user did not answer the question")
	}
	// 数字选择更常用：命中选项直接返回选项文本，否则按自由文本回答
	if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(req.Options) {
		return req.Options[n-1], nil
	}
	return answer, nil
}
