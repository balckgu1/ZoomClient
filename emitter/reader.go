// emitter/reader.go
//
// Reader 负责从 stdin 逐行读取 NDJSON 命令，解析为 Command 结构体。
// API 模式下替代 CLI 的 PromptUser() 输入方式。
package emitter

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// Command 表示一条来自 Tauri 前端的上行 JSON 命令。
type Command struct {
	CH      string          `json:"ch"`                // 固定 "cmd"
	ID      string          `json:"id,omitempty"`      // 可选的请求 ID，用于 ACK 关联
	Action  string          `json:"action"`            // chat | config | clear | compact | exit
	Payload json.RawMessage `json:"payload,omitempty"` // 各 action 的附带数据
}

// ChatPayload 解析 chat 命令的 payload。
type ChatPayload struct {
	Message string `json:"message"`
}

// ConfigPayload 解析 config 命令的 payload。
type ConfigPayload struct {
	ModelType string `json:"model_type,omitempty"`
}

// Reader 从 io.Reader（通常是 os.Stdin）逐行读取 JSON 命令。
type Reader struct {
	scanner *bufio.Scanner
}

// NewReader 创建一个 stdin JSON 命令读取器。
func NewReader(r io.Reader) *Reader {
	return &Reader{scanner: bufio.NewScanner(r)}
}

// ReadCommand 读取并解析下一条命令。
// 返回 nil 表示 EOF（stdin 已关闭）。
func (r *Reader) ReadCommand() (*Command, error) {
	for r.scanner.Scan() {
		line := strings.TrimSpace(r.scanner.Text())
		if line == "" {
			continue // 跳过空行
		}
		var cmd Command
		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			return nil, err
		}
		return &cmd, nil
	}
	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// ParseChatPayload 从 Command 中提取 chat payload。
func ParseChatPayload(cmd *Command) (ChatPayload, error) {
	var p ChatPayload
	err := json.Unmarshal(cmd.Payload, &p)
	return p, err
}

// ParseConfigPayload 从 Command 中提取 config payload。
func ParseConfigPayload(cmd *Command) (ConfigPayload, error) {
	var p ConfigPayload
	err := json.Unmarshal(cmd.Payload, &p)
	return p, err
}

// PermissionReplyPayload 解析 permission_reply 命令的 payload。
type PermissionReplyPayload struct {
	ID     string `json:"id"`
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
}

// ParsePermissionReplyPayload 从 Command 中提取权限回复 payload。
func ParsePermissionReplyPayload(cmd *Command) (PermissionReplyPayload, error) {
	var p PermissionReplyPayload
	err := json.Unmarshal(cmd.Payload, &p)
	return p, err
}

// ─── 并发读取器 ───

// PermissionResolver 是 ConcurrentReader 路由权限回复的回调接口。
type PermissionResolver interface {
	Resolve(id string, ok bool, reason string)
}

// ConcurrentReader 在后台 goroutine 中持续读取 stdin NDJSON 命令。
//
// • 普通命令（chat / config / clear / compact / exit）放入缓冲队列，由主循环通过 Next() 消费；
// • permission_reply 命令直接路由给 PermissionResolver，不占用主循环。
//
// 这解决了 agentLoop 运行期间主循环无法读取 stdin 的问题。
type ConcurrentReader struct {
	reader   *Reader
	resolver PermissionResolver
	cmdCh    chan *Command
	stopCh   chan struct{}
}

// NewConcurrentReader 创建并发读取器并启动后台 goroutine。
func NewConcurrentReader(r io.Reader, resolver PermissionResolver) *ConcurrentReader {
	cr := &ConcurrentReader{
		reader:   NewReader(r),
		resolver: resolver,
		cmdCh:    make(chan *Command, 16),
		stopCh:   make(chan struct{}),
	}
	go cr.loop()
	return cr
}

// loop 后台 goroutine：持续读取 stdin，按命令类型分流。
func (cr *ConcurrentReader) loop() {
	for {
		select {
		case <-cr.stopCh:
			return
		default:
		}
		cmd, err := cr.reader.ReadCommand()
		if err != nil {
			return // EOF 或读取错误，结束
		}
		// 权限回复直接路由，不进队列
		if cmd.Action == "permission_reply" && cr.resolver != nil {
			reply, perr := ParsePermissionReplyPayload(cmd)
			if perr == nil && reply.ID != "" {
				cr.resolver.Resolve(reply.ID, reply.Allow, reply.Reason)
			}
			continue
		}
		// 其他命令进队列，阻塞等待主循环消费
		select {
		case cr.cmdCh <- cmd:
		case <-cr.stopCh:
			return
		}
	}
}

// Next 返回下一条普通命令（阻塞等待）。
// 返回 nil 表示 stdin 已关闭。
func (cr *ConcurrentReader) Next() *Command {
	return <-cr.cmdCh
}

// Stop 停止后台读取 goroutine。
func (cr *ConcurrentReader) Stop() {
	select {
	case <-cr.stopCh:
		// 已经关闭
	default:
		close(cr.stopCh)
	}
}
