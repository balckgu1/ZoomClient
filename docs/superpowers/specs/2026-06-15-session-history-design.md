# 历史会话管理 Design Spec

## Overview

为 cc-learn agent 项目新增历史会话管理功能。用户可以创建多个会话，系统自动根据第一轮对话内容通过 LLM 生成会话标题。Web 模式下左侧新增侧边栏展示历史会话列表（类似通义千问），支持加载、删除、重命名历史会话。会话数据持久化为本地 JSON 文件。

## Decisions

| 决策项 | 结论 |
|--------|------|
| 自动命名 | LLM 摘要：第一轮对话结束后调用模型生成简短标题 |
| 持久化 | 本地 JSON 文件，`.sessions/` 目录，双层存储（index.json + 单会话文件） |
| 恢复范围 | 完整消息历史展示（不延续对话上下文） |
| 功能范围 | 创建、自动命名、列表、加载、删除、重命名 |
| 架构方案 | 独立 `session/` 包，解耦设计，未来 CLI 可复用 |
| UI 布局 | Web 模式左右两栏：左侧侧边栏 + 右侧聊天区域 |

## 1. 数据模型

### 1.1 SessionRecord（完整会话数据）

```go
type SessionRecord struct {
    ID        string        `json:"id"`
    Title     string        `json:"title"`
    CreatedAt time.Time     `json:"created_at"`
    UpdatedAt time.Time     `json:"updated_at"`
    Model     string        `json:"model"`
    TurnCount int           `json:"turn_count"`
    Messages  []fsm.Message `json:"messages"`
}
```

### 1.2 SessionMeta（索引条目）

```go
type SessionMeta struct {
    ID        string    `json:"id"`
    Title     string    `json:"title"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    TurnCount int       `json:"turn_count"`
}
```

## 2. 存储设计

### 2.1 目录结构

```
.sessions/
├── index.json          # 会话索引（仅 SessionMeta 数组）
├── {uuid1}.json        # 单个会话完整 SessionRecord
├── {uuid2}.json
└── ...
```

### 2.2 存储策略

- **index.json** 存储 `[]SessionMeta`，用于快速列表展示，避免扫描所有文件
- **单个 JSON 文件**以 `{uuid}.json` 命名，存完整 `SessionRecord`（含 messages）
- 存储目录可通过 config 的 `session.dir` 配置，默认 `./.sessions/`

### 2.3 保存时机

- 每轮对话结束后（agentLoop done 事件之后）自动保存当前会话到磁盘
- LLM 生成标题后更新 title 字段并再次保存
- 删除/重命名操作立即写盘

### 2.4 一致性保障

- 启动时 reconcile：扫描目录文件，与 index.json 对比，修复不一致
- JSON 文件损坏时跳过并 log.Warn，不影响其他会话

## 3. 包结构

```
session/
├── session.go        # 数据结构定义（SessionRecord, SessionMeta）
├── manager.go        # SessionManager 核心逻辑
├── store.go          # JSON 文件持久化（读写 index.json + 单会话文件）
└── naming.go         # LLM 自动命名（调用 ChatClient 生成标题）
```

### 3.1 Manager 核心接口

```go
type Manager struct {
    store   *Store
    client  clients.ChatClient
    model   string
    current string        // 当前活跃会话 ID
    mu      sync.Mutex
}

func NewManager(dir string, client clients.ChatClient, model string) *Manager
func (m *Manager) CreateSession() *SessionRecord          // 新建空会话并设为当前
func (m *Manager) Current() *SessionRecord                // 获取当前会话
func (m *Manager) Save(record *SessionRecord) error       // 保存到磁盘 + 更新索引
func (m *Manager) List() []SessionMeta                    // 列出所有会话（按时间倒序）
func (m *Manager) Load(id string) (*SessionRecord, error) // 加载指定会话并设为当前
func (m *Manager) Delete(id string) error                 // 删除会话文件 + 更新索引
func (m *Manager) Rename(id, newTitle string) error       // 重命名
func (m *Manager) GenerateTitle(record *SessionRecord) (string, error)  // LLM 生成标题
```

### 3.2 线程安全

- Manager 内部使用 `sync.Mutex` 保护 current 切换和文件写入
- GenerateTitle 在 goroutine 中异步执行，完成后通过 SSE 推送标题更新事件

### 3.3 LLM 命名

- 第一轮对话结束后，提取用户首条消息和助手首条回复，构造 prompt 调用 ChatClient
- 要求模型生成 ≤20 字的简短标题
- 失败时降级：截断用户首条消息前 30 字符作为标题

## 4. API 端点

### 4.1 新增路由

| 方法 | 路径 | 功能 | 请求体 | 响应 |
|------|------|------|--------|------|
| GET | `/api/sessions` | 会话列表 | — | `[{id, title, created_at, updated_at, turn_count}]` |
| POST | `/api/sessions` | 新建会话 | — | `{id, title, created_at}` |
| GET | `/api/sessions/{id}` | 加载指定会话完整消息 | — | `{id, title, messages: [...]}` |
| DELETE | `/api/sessions/{id}` | 删除会话 | — | `{status: "deleted"}` |
| PATCH | `/api/sessions/{id}` | 重命名会话 | `{title: "新标题"}` | `{id, title}` |

### 4.2 Server 结构变更

```go
type Server struct {
    session    *Session
    sessionMgr *session.Manager   // 新增
    mux        *http.ServeMux
    port       int
}
```

### 4.3 SSE 新事件

当 LLM 异步生成标题完成后，通过 SSE 推送：

```json
{"ch": "system", "data": {"event": "session_renamed", "id": "xxx", "title": "Python排序算法"}}
```

## 5. 与现有代码集成

### 5.1 main.go 变更

- 新增 `initSessionManager()` 创建 Manager，传入 ChatClient 和 model
- `runWebREPL` 启动前创建首个会话
- 每轮 chat 结束后调用 `manager.Save(record)`
- 首次对话后异步调用 `GenerateTitle`

### 5.2 web.Session 变更

- 新增 `RecordID string` 字段关联到 `SessionRecord`
- 其余结构保持不变

### 5.3 切换会话流程

1. 前端点击侧边栏历史会话 → `GET /api/sessions/{id}`
2. 后端 `manager.Load(id)` 将消息加载到当前 `fsm.State.Messages`
3. 前端清空消息列表并填入历史消息
4. 用户发新消息 → 正常走 `/api/chat`，agentLoop 在历史上下文上继续运行

## 6. 前端 UI

### 6.1 布局

App 从单栏改为左右两栏：

```
┌─────────────────────────────────────────────┐
│ ┌────────────┐ ┌───────────────────────────┐ │
│ │ Sidebar    │ │ StatusBar                 │ │
│ │            │ ├───────────────────────────┤ │
│ │ [+新会话]  │ │                           │ │
│ │            │ │ MessageList               │ │
│ │ ○ 今天     │ │ (聊天消息区域)              │ │
│ │   会话A    │ │                           │ │
│ │   会话B    │ │                           │ │
│ │            │ ├───────────────────────────┤ │
│ │ ○ 昨天     │ │ InputBar                  │ │
│ │   会话C    │ │                           │ │
│ └────────────┘ └───────────────────────────┘ │
└─────────────────────────────────────────────┘
```

### 6.2 新增组件

- **Sidebar.tsx**：会话列表组件，按时间分组（今天/昨天/更早），当前会话高亮
- **新增 API 函数**：`fetchSessions`, `createSession`, `loadSession`, `deleteSession`, `renameSession`

### 6.3 新增类型

```typescript
export interface SessionMeta {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  turn_count: number;
}

export interface SessionRecord extends SessionMeta {
  messages: ChatMessage[];
  model: string;
}
```

### 6.4 交互流程

- 页面加载 → `GET /api/sessions` 填充侧边栏，自动选中最新会话
- 点击「+ 新会话」→ `POST /api/sessions` → 清空消息列表
- 点击历史会话 → `GET /api/sessions/{id}` → 替换消息列表
- hover 会话条目 → 显示重命名/删除操作按钮
- 收到 SSE `session_renamed` → 更新侧边栏标题

## 7. 错误处理与边界情况

| 场景 | 处理 |
|------|------|
| 存储目录不存在 | Manager 初始化时 `os.MkdirAll` |
| JSON 文件损坏 | 读取时跳过并 log.Warn |
| index.json 与文件不一致 | 启动时 reconcile |
| 并发写入 | Manager 的 Mutex 保证串行 |
| LLM 命名失败 | 降级截断首条消息前 30 字符 |
| 空会话不保存 | 新建但未发消息就切换，不写磁盘 |
| 删除当前活跃会话 | 自动切换到最新会话；无剩余则新建 |

## 8. 配置变更

`config.yaml` 新增：

```yaml
session:
  dir: "./.sessions"
```

`utils/loadconfig.go` 的 Config 新增 `Session.SessionDir` 字段。

## 9. 测试计划

- `session/store_test.go`：Store 的 CRUD（创建/读取/删除/索引更新）
- `session/manager_test.go`：Manager 会话生命周期（Create → Save → List → Load → Delete → Rename）
- `session/naming_test.go`：GenerateTitle 的 LLM 调用和降级逻辑
