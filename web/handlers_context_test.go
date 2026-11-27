// web/handlers_context_test.go
//
// 上下文洞察端点单元测试：空折叠器降级、头部/详情响应形状、
// Session 缓存优先与方法校验。
package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"zoomClient/clients"
	"zoomClient/ctxinsight"
	"zoomClient/utils"
)

// newCtxServer 构造注入折叠器的测试 Server
func newCtxServer(t *testing.T, rec *ctxinsight.Recorder) *Server {
	t.Helper()
	sess := NewSession("ctx-session", "test-model")
	return NewServer(ServerDeps{
		Session:  sess,
		Config:   &utils.Config{},
		Recorder: rec,
	}, 0)
}

// TestContextTimeline_NilRecorder 折叠器未注入时端点返回空快照而非错误
func TestContextTimeline_NilRecorder(t *testing.T) {
	srv := newCtxServer(t, nil)
	rec := doJSON(t, srv, http.MethodGet, "/api/context/timeline", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var head ctxinsight.HeadSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &head); err != nil {
		t.Fatalf("unmarshal head: %v", err)
	}
	if head.Current.Total != 0 || head.Last != nil {
		t.Fatalf("nil recorder should serve empty head: %+v", head)
	}
	rec = doJSON(t, srv, http.MethodGet, "/api/context/timeline/detail", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", rec.Code)
	}
}

// TestContextTimeline_HeadAndDetail 验证头部与详情的响应形状
func TestContextTimeline_HeadAndDetail(t *testing.T) {
	r := ctxinsight.New(100000, "test-model")
	r.RecordUserMessage("hello")
	r.RecordStep(ctxinsight.StepInput{
		SystemPrompt:    "system prompt",
		ToolList:        nil,
		PayloadMessages: []clients.Message{{Role: "user", Content: "hello"}},
		Response: &clients.ChatResponse{
			Message: clients.Message{Role: "assistant", Content: "hi there"},
			Usage:   clients.TokenUsage{PromptTokens: 50, CompletionTokens: 5},
		},
	})
	srv := newCtxServer(t, r)

	rec := doJSON(t, srv, http.MethodGet, "/api/context/timeline", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var head ctxinsight.HeadSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &head); err != nil {
		t.Fatalf("unmarshal head: %v", err)
	}
	if head.Counts.Steps != 1 || head.Counts.HumanInputs != 1 || head.Last == nil || head.Last.PromptTokens != 50 {
		t.Fatalf("head = %+v", head)
	}
	if head.LimitBytes != 100000 || head.Model != "test-model" {
		t.Fatalf("head envelope = %+v", head)
	}

	rec = doJSON(t, srv, http.MethodGet, "/api/context/timeline/detail", "")
	var detail ctxinsight.DetailSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if len(detail.Requests) != 1 || len(detail.Nodes) != 2 || detail.Rev != head.DetailRev {
		t.Fatalf("detail = requests:%d nodes:%d rev:%d", len(detail.Requests), len(detail.Nodes), detail.Rev)
	}
}

// TestContextTimeline_LiveRead 验证 GET 直读折叠器：记录后立即反映，无缓存滞留
func TestContextTimeline_LiveRead(t *testing.T) {
	r := ctxinsight.New(100000, "m")
	srv := newCtxServer(t, r)
	r.RecordUserMessage("hi")
	rec := doJSON(t, srv, http.MethodGet, "/api/context/timeline", "")
	var head ctxinsight.HeadSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &head); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if head.Counts.HumanInputs != 1 {
		t.Fatalf("head should reflect live recorder state: %+v", head)
	}
}

// TestContextTimeline_MethodNotAllowed 非 GET 请求返回 405
func TestContextTimeline_MethodNotAllowed(t *testing.T) {
	srv := newCtxServer(t, nil)
	for _, path := range []string{"/api/context/timeline", "/api/context/timeline/detail"} {
		rec := doJSON(t, srv, http.MethodPost, path, "{}")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s POST status = %d, want 405", path, rec.Code)
		}
	}
}