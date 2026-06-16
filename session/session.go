package session

import (
	"time"
	"zoomClient/fsm"
)

// SessionRecord 表示一个完整的session记录，包含消息历史，持久化为 {id}.json 文件
type SessionRecord struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Model     string        `json:"model"`
	TurnCount int           `json:"turn_count"`
	Messages  []fsm.Message `json:"messages"`
}

// SessionMeta 仅包含session元信息，用于 index.json 中的列表显示
type SessionMeta struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TurnCount int       `json:"turn_count"`
}

// ToMeta 从 session 中提取元信息
func (r *SessionRecord) ToMeta() SessionMeta {
	return SessionMeta{
		ID:        r.ID,
		Title:     r.Title,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		TurnCount: r.TurnCount,
	}
}

// IsEmpty 判断session是否为空
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
