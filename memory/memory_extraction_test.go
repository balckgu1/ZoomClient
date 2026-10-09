package memory

import (
	"fmt"
	"testing"

	"zoomClient/clients"
	"zoomClient/tools"
)

func TestShouldRunMemoryExtraction(t *testing.T) {
	t.Run("no substantial user prose", func(t *testing.T) {
		msgs := []clients.Message{{Role: "user", Content: "hi"}}
		run, reason := shouldRunMemoryExtraction(msgs)
		if run {
			t.Error("expected skip for short prose")
		}
		if reason != "no substantial user prose" {
			t.Errorf("unexpected reason %q", reason)
		}
	})

	t.Run("user requests ignore memory", func(t *testing.T) {
		msgs := []clients.Message{{Role: "user", Content: "please ignore memory for this session and store nothing"}}
		run, reason := shouldRunMemoryExtraction(msgs)
		if run {
			t.Error("expected skip on ignore-memory")
		}
		if reason != "user requested to ignore memory" {
			t.Errorf("unexpected reason %q", reason)
		}
	})

	t.Run("already hand-wrote memory", func(t *testing.T) {
		msgs := []clients.Message{
			{Role: "user", Content: "Remember that I prefer tabs over spaces in all Go code I write."},
			{Role: "assistant", ToolCalls: []tools.ToolCall{{Name: "save_memory"}}},
		}
		run, reason := shouldRunMemoryExtraction(msgs)
		if run {
			t.Error("expected skip when save_memory already called")
		}
		if reason != "memory already written manually in window" {
			t.Errorf("unexpected reason %q", reason)
		}
	})

	t.Run("substantial prose runs", func(t *testing.T) {
		msgs := []clients.Message{
			{Role: "user", Content: "I prefer tabs over spaces when writing Go code."},
			{Role: "assistant", Content: "Noted."},
		}
		if run, _ := shouldRunMemoryExtraction(msgs); !run {
			t.Error("expected run for substantial prose without handwrite/ignore")
		}
	})
}

func TestTailMessages(t *testing.T) {
	msgs := make([]clients.Message, 50)
	for i := range msgs {
		msgs[i] = clients.Message{Role: "user", Content: fmt.Sprintf("msg-%d", i)}
	}

	if got := tailMessages(msgs, 40); len(got) != 40 {
		t.Errorf("expected 40, got %d", len(got))
	}
	if got := tailMessages(msgs[:30], 40); len(got) != 30 {
		t.Errorf("expected 30 (all when fewer than n), got %d", len(got))
	}
	if got := tailMessages(msgs[:40], 40); len(got) != 40 {
		t.Errorf("expected 40 (all when equal), got %d", len(got))
	}
	tail := tailMessages(msgs, 40)
	if tail[0].Content != "msg-10" {
		t.Errorf("expected tail to start at msg-10, got %v", tail[0].Content)
	}
}
