package web

// 本文件覆盖 Web 结构化提问的挂起/回复流程与 HTTP 处理器。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"zoomClient/tools"

	"net/http"
	"net/http/httptest"
)

// waitQuestionEvent 从事件通道中取出 user_question 事件并解析字段。
func waitQuestionEvent(t *testing.T, sess *Session) map[string]string {
	t.Helper()
	select {
	case ev := <-sess.EventCh:
		if ev.CH != "system" {
			t.Fatalf("事件通道应为 system，实际 %s", ev.CH)
		}
		payload, ok := ev.Data.(map[string]string)
		if !ok {
			t.Fatalf("事件负载类型错误：%T", ev.Data)
		}
		if payload["event"] != "user_question" {
			t.Fatalf("事件名应为 user_question，实际 %s", payload["event"])
		}
		return payload
	case <-time.After(time.Second):
		t.Fatal("等待 user_question 事件超时")
		return nil
	}
}

func TestWebQuestionAsker_正常回答(t *testing.T) {
	sess := NewSession("s1", "")
	asker := NewWebQuestionAsker(sess)

	type result struct {
		answer string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		answer, err := asker.AskQuestion(tools.QuestionRequest{
			Header:   "选择",
			Question: "用哪个方案？",
			Options:  []string{"A", "B"},
		})
		done <- result{answer: answer, err: err}
	}()

	payload := waitQuestionEvent(t, sess)
	if payload["question"] != "用哪个方案？" || payload["header"] != "选择" {
		t.Errorf("事件字段错误：%+v", payload)
	}
	var options []string
	if err := json.Unmarshal([]byte(payload["options"]), &options); err != nil {
		t.Fatalf("options 应为 JSON 数组：%v", err)
	}
	if len(options) != 2 || options[1] != "B" {
		t.Errorf("options 解析错误：%+v", options)
	}

	sess.ResolveQuestion(payload["id"], "B", false)
	select {
	case r := <-done:
		if r.err != nil || r.answer != "B" {
			t.Errorf("应返回用户回答 B，实际 %q err=%v", r.answer, r.err)
		}
	case <-time.After(time.Second):
		t.Fatal("等待回答结果超时")
	}
}

func TestWebQuestionAsker_取消与超时(t *testing.T) {
	// 取消：返回错误
	cancelSess := NewSession("s2", "")
	cancelAsker := NewWebQuestionAsker(cancelSess)
	cancelDone := make(chan error, 1)
	go func() {
		_, err := cancelAsker.AskQuestion(tools.QuestionRequest{Question: "q"})
		cancelDone <- err
	}()
	payload := waitQuestionEvent(t, cancelSess)
	cancelSess.ResolveQuestion(payload["id"], "", true)
	select {
	case err := <-cancelDone:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Errorf("取消应返回 cancelled 错误，实际 %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("等待取消结果超时")
	}

	// 超时：缩短等待时长后应返回超时错误
	timeoutSess := NewSession("s3", "")
	timeoutAsker := NewWebQuestionAsker(timeoutSess)
	timeoutAsker.timeout = 10 * time.Millisecond
	if _, err := timeoutAsker.AskQuestion(tools.QuestionRequest{Question: "q"}); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("超时应返回 timed out 错误，实际 %v", err)
	}
	// 事件仍会推送，消费掉避免影响后续断言
	<-timeoutSess.EventCh
}

func TestHandleQuestion_写入回答(t *testing.T) {
	sess := NewSession("s4", "")
	srv := &Server{session: sess}
	id, ch := sess.RequestQuestion("q", "", nil)

	body := `{"id":"` + id + `","answer":"  已确认  "}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/question", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleQuestion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("应返回 200，实际 %d", rec.Code)
	}
	select {
	case resp := <-ch:
		if resp.cancelled || resp.answer != "  已确认  " {
			t.Errorf("回答透传错误：%+v", resp)
		}
	case <-time.After(time.Second):
		t.Fatal("等待写入回答超时")
	}
}

func TestHandleQuestion_参数校验(t *testing.T) {
	srv := &Server{session: NewSession("s5", "")}

	getReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/question", nil)
	getRec := httptest.NewRecorder()
	srv.handleQuestion(getRec, getReq)
	if getRec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET 应返回 405，实际 %d", getRec.Code)
	}

	missingID := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/question", strings.NewReader(`{"answer":"x"}`))
	rec := httptest.NewRecorder()
	srv.handleQuestion(rec, missingID)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺少 id 应返回 400，实际 %d", rec.Code)
	}

	badJSON := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/question", strings.NewReader(`{`))
	rec = httptest.NewRecorder()
	srv.handleQuestion(rec, badJSON)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("非法 JSON 应返回 400，实际 %d", rec.Code)
	}
}
