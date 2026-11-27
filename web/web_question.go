package web

// 本文件实现 Web 模式的结构化提问器：把 ask_user_question 的提问
// 通过 SSE 推送到前端弹窗，并阻塞等待 /api/question 返回的回答。

import (
	"errors"
	"fmt"
	"time"

	"zoomClient/tools"
)

// 默认等待时长：用户在浏览器前可能暂时离开，给足时间再判定超时
const defaultWebQuestionTimeout = 5 * time.Minute

// WebQuestionAsker 实现 tools.UserAsker 接口。
type WebQuestionAsker struct {
	session *Session
	timeout time.Duration
}

// NewWebQuestionAsker 创建 Web 提问器。
func NewWebQuestionAsker(s *Session) *WebQuestionAsker {
	return &WebQuestionAsker{
		session: s,
		timeout: defaultWebQuestionTimeout,
	}
}

// AskQuestion 推送提问事件并阻塞等待回答或超时。
func (a *WebQuestionAsker) AskQuestion(req tools.QuestionRequest) (string, error) {
	id, ch := a.session.RequestQuestion(req.Question, req.Header, req.Options)
	select {
	case resp := <-ch:
		a.session.DropQuestion(id)
		if resp.cancelled {
			return "", errors.New("user cancelled the question")
		}
		return resp.answer, nil
	case <-time.After(a.timeout):
		a.session.DropQuestion(id)
		return "", fmt.Errorf("question timed out after %s without an answer", a.timeout)
	}
}
