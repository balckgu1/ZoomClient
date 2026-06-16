// session/session.go
//
// 会话管理数据结构定义。
// SessionRecord 存储完整会话数据（含消息历史），SessionMeta 仅存元信息用于索引列表。
package session

import (
	"time"
	"zoomClient/fsm"
)

// SessionRecord 表示一个完整的会话记录，包含消息历史。
// 持久化为 .sessions/{id}.json 文件。
type SessionRecord struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Model     string        `json:"model"`
	TurnCount int           `json:"turn_count"`
	Messages  []fsm.Message `json:"messages"`
}

// SessionMeta 仅包含会话元信息，用于 index.json 中的快速列表展示。
type SessionMeta struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TurnCount int       `json:"turn_count"`
}

// ToMeta 从 SessionRecord 提取元信息。
func (r *SessionRecord) ToMeta() SessionMeta {
	return SessionMeta{
		ID:        r.ID,
		Title:     r.Title,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		TurnCount: r.TurnCount,
	}
}

// IsEmpty 判断会话是否为空（无消息或仅含 system 消息且无对话轮次）。
func (r *SessionRecord) IsEmpty() bool {
	if r.TurnCount > 0 {
		return false
	}
	userMsgCount := 0
	for _, msg := range r.Messages {
		if msg.Role == "user" {
			userMsgCount++
		}
	}
	return userMsgCount == 0
}
