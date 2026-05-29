# Message & Prompt Pipeline Design Spec

## Overview

将 `main.go` agentLoop 中散落的消息注入逻辑重构为 `prompt/` 包中的 `MessagePipeline`，实现 s10a 教学文档描述的完整四管线模型输入架构：Prompt Blocks + Normalized Messages + Reminders + Attachments。

## Background

s10 已完成 SystemPromptBuilder（6 段流水线），解决了 system prompt 的组装问题。

但完整的模型输入不只是 system prompt，还包括：消息流、临时提醒（reminder）、附件（attachment）。当前这些逻辑散落在 agentLoop 各处：

- hook `ExitInject` → 伪装为 `user` 角色直接 append 到 state.Messages
- todo `Reminder()` → 伪装为 `user` 角色直接 append 到 state.Messages  
- post-hook `ExitInject` → 伪装为 `user` 角色直接 append 到 state.Messages

问题：
1. 语义模糊 — 这些不是用户说的话，但用了 user 角色
2. 无边界 — reminder 和真实对话混在一起，压缩/回放时无法区分
3. 持久化污染 — 临时提醒被写入 state.Messages 永久保留

## Goals

- 实现 s10a 描述的四管线架构：prompt blocks / normalized messages / reminders / attachments
- Reminder 使用独立的 `system` 角色消息，语义清晰
- Reminder 支持 OneShot（用完即删），不污染对话历史
- agentLoop 中的注入行为统一通过 pipeline.AddReminder() 完成
- Attachment 管线预留接口，本次不实际填充
- 为 Pipeline 编写单元测试

## Non-Goals

- 不改变 ChatClient 接口签名
- 不引入消息格式的 content block 化（保持现有 string/interface{} content）
- 不实现 attachment 的实际内容填充
- 不改变 compact 系统的工作方式

## Architecture

### Data Flow

```
┌─────────────────────────────────────────────────────┐
│               MessagePipeline                       │
│                                                     │
│  1. Prompt Blocks ─→ SystemPromptBuilder.Build()    │
│  2. Raw Messages  ─→ normalizeMessages()            │
│  3. Reminders     ─→ injectReminders() [system role]│
│  4. Attachments   ─→ injectAttachments() [预留]     │
│                                                     │
│         ↓ AssemblePayload()                         │
│                                                     │
│  APIPayload {                                       │
│      SystemPrompt string       ← 来自 Builder       │
│      Messages     []fsm.Message ← normalize+inject  │
│  }                                                  │
└─────────────────────────────────────────────────────┘
```

### File Structure

| 文件 | 职责 |
|------|------|
| `prompt/pipeline.go` | `MessagePipeline` struct + `NewPipeline()` + `AssemblePayload()` |
| `prompt/normalize.go` | `normalizeMessages()` — 统一消息格式 |
| `prompt/reminder.go` | `Reminder` 类型 + `AddReminder()` / `ClearOneShotReminders()` |
| `prompt/attachment.go` | `Attachment` 类型 — 最小占位 |
| `prompt/pipeline_test.go` | Pipeline 单元测试 |

### Data Structures

```go
// Reminder 系统提醒，只活一轮或几轮。
// 与长期 prompt block 的区别：reminder 是临时注入，可一轮后自动消失。
type Reminder struct {
    Content string // 提醒文本
    Source  string // 来源标识：hook / todo / post_hook
    OneShot bool   // true = 用完即删
}

// Attachment 大块可选补充信息（预留）。
type Attachment struct {
    ID      string
    Content string
    Source  string
}

// APIPayload 最终组装的模型输入载荷。
type APIPayload struct {
    SystemPrompt string        // 来自 SystemPromptBuilder.Build()
    Messages     []fsm.Message // normalize + reminders + attachments
}

// MessagePipeline 模型输入的完整组装管道。
type MessagePipeline struct {
    builder     *SystemPromptBuilder
    reminders   []Reminder
    attachments []Attachment
}
```

### Key Methods

```go
// NewPipeline 构造管道实例。
func NewPipeline(builder *SystemPromptBuilder) *MessagePipeline

// AddReminder 添加一条临时提醒。
func (p *MessagePipeline) AddReminder(r Reminder)

// ClearOneShotReminders 清除所有 OneShot=true 的 reminder。每轮调 LLM 后调用。
func (p *MessagePipeline) ClearOneShotReminders()

// AssemblePayload 执行完整组装流水线，返回可直接传给 client.Chat 的载荷。
func (p *MessagePipeline) AssemblePayload(rawMessages []fsm.Message) APIPayload
```

## Section Details

### 1. normalizeMessages()

职责：
1. 去除 rawMessages 中的首条 system 消息（system prompt 由 Builder 独立生成）
2. 验证消息角色合法性（只允许 user / assistant / tool）
3. 滤掉空内容消息（Content == nil 或 ""）

```go
func (p *MessagePipeline) normalizeMessages(raw []fsm.Message) []fsm.Message {
    var result []fsm.Message
    for i, msg := range raw {
        // 跳过首条 system 消息
        if i == 0 && msg.Role == "system" {
            continue
        }
        // 跳过空内容
        if msg.Content == nil || msg.Content == "" {
            // 但 assistant 的 tool_calls 消息 content 可能为空，需保留
            if len(msg.ToolCalls) == 0 {
                continue
            }
        }
        result = append(result, msg)
    }
    return result
}
```

### 2. injectReminders()

将所有 reminder 以 `role: "system"` 追加到消息列表末尾（模型最后看到，优先级高）：

```go
func (p *MessagePipeline) injectReminders(msgs []fsm.Message) []fsm.Message {
    for _, r := range p.reminders {
        msgs = append(msgs, fsm.Message{
            Role:    "system",
            Content: r.Content,
        })
    }
    return msgs
}
```

### 3. injectAttachments()

当前为占位实现，直接返回原消息：

```go
func (p *MessagePipeline) injectAttachments(msgs []fsm.Message) []fsm.Message {
    // 预留：后续可将 attachment 作为 user 消息追加
    return msgs
}
```

### 4. AssemblePayload()

完整流水线：

```go
func (p *MessagePipeline) AssemblePayload(rawMessages []fsm.Message) APIPayload {
    systemPrompt := p.builder.Build()
    messages := p.normalizeMessages(rawMessages)
    messages = p.injectReminders(messages)
    messages = p.injectAttachments(messages)
    return APIPayload{SystemPrompt: systemPrompt, Messages: messages}
}
```

## Integration: main.go 改造

### agentLoop 签名变更

```go
// 改前
func agentLoop(..., systemPrompt string, ...) 

// 改后
func agentLoop(..., pipeline *prompt.MessagePipeline, ...)
```

### 循环内改造

```go
// 改前：hook inject
state.Messages = append(state.Messages, fsm.Message{Role: "user", Content: preDecisions[i].Message})

// 改后
pipeline.AddReminder(prompt.Reminder{Content: preDecisions[i].Message, Source: "hook", OneShot: true})
```

```go
// 改前：todo reminder
state.Messages = append(state.Messages, fsm.Message{Role: "user", Content: reminder})

// 改后
pipeline.AddReminder(prompt.Reminder{Content: reminder, Source: "todo", OneShot: true})
```

```go
// 改前：调 LLM
response, err := client.Chat(model, state.Messages, toolList, opts)

// 改后
payload := pipeline.AssemblePayload(state.Messages)
finalMessages := append([]fsm.Message{{Role: "system", Content: payload.SystemPrompt}}, payload.Messages...)
response, err := client.Chat(model, finalMessages, toolList, opts)
pipeline.ClearOneShotReminders()
```

### main() 中的构造

```go
// 改前
builder := prompt.NewSystemPromptBuilder(skillregistry, cfg.Memory.Dir, modelname, toolCtx.WorkPath)
systemPrompt := builder.Build()

// 改后
builder := prompt.NewSystemPromptBuilder(skillregistry, cfg.Memory.Dir, modelname, toolCtx.WorkPath)
pipeline := prompt.NewPipeline(builder)
```

### state.Messages 不再存首条 system

改造后 `state.Messages` 只存对话历史（user / assistant / tool），system prompt 由 pipeline 每轮动态生成。agentLoop 开头的 system 消息插入逻辑可以删除。

## Testing

`prompt/pipeline_test.go` 覆盖：

- TestAssemblePayload_BasicFlow — 基本组装流程
- TestAssemblePayload_RemovesLeadingSystem — 验证 normalize 去除首条 system
- TestAddReminder_OneShotCleanup — 验证 ClearOneShotReminders 只删 OneShot
- TestAssemblePayload_WithReminders — 验证 reminder 以 system role 追加
- TestAssemblePayload_EmptyMessages — 空消息列表不 panic

## Dependencies

- `zoomClient/prompt` (existing) — SystemPromptBuilder
- `zoomClient/fsm` — Message struct
- `zoomClient/skills` — SkillRegistry (间接，通过 Builder)
