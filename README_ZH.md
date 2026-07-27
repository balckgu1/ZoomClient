# 🤖 ZoomClient

**中文 | [English](README.md)**

ZoomClient 是一个基于 Go 语言的 AI Agent 框架，实现具备 Tool-Use 能力的自主任务执行系统。
项目采用模块化架构，涵盖多后端模型客户端、工具控制平面、并发执行运行时、Subagent、Skill 按需加载、会话管理、上下文压缩、提示词管道、Hook 系统、权限系统、跨会话记忆、事件发射器、TUI/Web 前端等核心子系统。

---

## 📚 目录

- [✨ 核心功能特性](#-核心功能特性)
- [🏗️ 架构设计说明](#️-架构设计说明)
- [🧩 主要组件介绍](#-主要组件介绍)
- [⚙️ 安装与配置](#️-安装与配置)
- [🚀 使用方法](#-使用方法)
- [🛠️ 开发指南](#️-开发指南)
- [🚧 待实现功能](#-待实现功能)

---

## ✨ 核心功能特性

- **🔁 Agent Loop**：驱动模型交互的主循环，支持系统提示注入、消息历史维护与多轮次工具调用，最大轮次由 `config.yaml` 中的 `agentloop.maxTurns` 配置（默认 25）。
- **🔌 多后端 ChatClient 抽象**：统一 `clients.ChatClient` 接口，内置 **OpenAI 兼容**（DeepSeek / Kimi / Qwen / OpenAI）、**Ollama**（NDJSON 流式协议）、**Anthropic**（Claude）与 **Gemini** 四种实现，支持通过命令行 `-m` 参数切换。
- **🔧 工具控制平面（Tool Control Plane）**：统一的工具注册表 `tools.Registry`，支持工具发现、注册与按名称动态调度执行，内置权限门控（`PermissionDecider`）实现执行前准入控制。
- **⚡ 并发安全调度**：工具调用按读写属性自动分批，只读工具（如 `read_file`）可并发执行，写操作类工具串行执行，兼顾效率与安全性。
- **🧬 子智能体（Subagent）**：通过 `sub_task` 工具将子任务委托给独立上下文的子智能体；支持空白上下文模式与 `fork=true` 继承父消息上下文模式，子智能体拥有独立工具白名单避免副作用穿透。
- **📖 Skills 按需加载**：扫描 `.skills/` 目录下的 `SKILL.md`（含 YAML frontmatter），仅将目录清单注入 system prompt，完整正文通过 `load_skill` 工具按需加载，降低上下文成本。
- **📝 会话计划管理（Todo Write）**：允许模型在会话中维护多步骤任务计划，支持状态跟踪；连续多轮未更新计划时自动注入提醒消息。
- **🗜️ 上下文压缩（Context Compact）**：三层压缩策略——大工具结果落盘替换为预览、旧工具结果微压缩为占位符、整体历史过长时调模型生成连续性摘要，有效控制上下文膨胀。
- **🪝 Hook 系统**：事件驱动钩子框架，支持 `SessionStart`、`PreToolUse`、`PostToolUse`、`ToolError`、`SessionEnd` 五个事件点，内置危险命令拦截、敏感文件保护、速率限制、审计日志、错误恢复等处理器。
- **🛡️ 权限系统**：基于规则引擎的细粒度权限控制，支持 `default`（未命中问用户）、`plan`（只读模式）、`auto`（只读自动放行）三种模式，可配置 allow/deny 规则并支持正则匹配。
- **🛡️ 路径沙箱保护**：文件类工具执行前需通过路径沙箱校验（基于 `ToolContext.WorkPath`），禁止路径穿越与工作目录逃逸。
- **🚫 危险命令拦截**：`run_bash` 工具内置命令黑名单检测，拦截高危操作指令。
- **🧠 Thinking 模式支持**：完整透传 `reasoning_content` 字段，多轮对话中原样回传，避免服务端以 `invalid_request_error` 拒绝请求。
- **🧠 模型注册表与预设（Model Registry）**：集中管理模型预设（客户端类型、地址、模型名）的 `model.Registry`，支持运行时热切换后端而不丢失对话历史（`SwitchModel` API）。
- **🗄️ 会话管理（Session Management）**：基于 JSON 文件持久化的会话 CRUD（`session.Manager`），支持自动命名、元数据索引与生命周期追踪，实现会话恢复、跨进程共享与历史浏览。
- **🧩 FSM（有限状态机）**：封装 Agent 会话状态，包含消息历史、轮次计数与转移原因，提供与业务逻辑解耦的干净状态抽象。
- **📦 提示词管道（Prompt Pipeline）**：多阶段提示词构建系统（`prompt.MessagePipeline`），依次向 system prompt 注入技能目录、记忆摘要、动态指令、计划提醒与文件附件，最后按目标模型协议归一化输出。
- **💭 跨会话记忆系统（`memory/`）**：支持四种记忆类型（user / feedback / project / reference），提供 `save_memory`、`search_memory`、`delete_memory`、`update_memory` 工具。记忆以 Markdown + YAML frontmatter 格式存储，自动注入 system prompt。
- **🔄 事件发射器（`emitter/`）**：统一的 Agent 可见事件输出抽象（会话生命周期、助手输出、工具调用/结果、子智能体、Hook、错误、计划更新、压缩等），提供三种实现：CLI TUI 渲染器、API NDJSON 发射器、Web SSE 发射器。
- **🌐 Web UI（`web/`）**：内置 Web 服务器，支持 SSE 实时流式传输、RESTful 会话 API、HTML 前端（通过 `go:embed` 嵌入）、基于 Web 的权限询问器，可作为 CLI TUI 的替代方案。
- **🖥️ 交互式 CLI（TUI）**：基于 `bubbletea`/`lipgloss` 的终端渲染器，支持 REPL 风格多轮交互与 `/exit`、`/clear`、`/compact`、`/help` 斜杠命令，包含 Markdown 渲染与状态栏。
- **📂 工作目录管理**：支持在运行时指定与切换 `WorkPath`，控制文件工具的沙箱根目录。
- **🪵 结构化日志**：基于 Uber Zap，输出带颜色与时间戳的日志，支持开发与生产配置。
- **🗂️ YAML 驱动配置**：通过 `config/config.yaml` 管理 API Key、最大轮次、技能目录、子智能体默认提示词、权限规则、压缩阈值等，避免硬编码密钥。

---

## 🏗️ 架构设计说明

项目采用分层架构设计，从上到下依次为：Agent 循环层、能力扩展层、工具管理层、状态机控制层、表现层、运行时支持层：

```
┌──────────────────────────────────────────────────────────────────┐
│                      🔁 Agent 循环层                             │
│                    (main.go → agentLoop)                        │
├──────────────────────────────────────────────────────────────────┤
│                   🧬 能力扩展层                                  │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │subagent/ │  │ skills/  │  │  hooks/  │  │ session/ │       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │ memory/  │  │ compact/ │  │ prompt/  │  │ model/   │       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
├──────────────────────────────────────────────────────────────────┤
│                    🔧 工具管理层                                 │
│   (tools/registry · tools/runtime · concrete tool impls)        │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │ readfile │  │ writefile│  │ run_bash │  │glob_search│       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐                      │
│   │edit_file │  │list_dir  │  │todo_mgr  │                      │
│   └──────────┘  └──────────┘  └──────────┘                      │
├──────────────────────────────────────────────────────────────────┤
│                  ⚙️ 状态机控制层                                 │
│            (fsm/ · permission/ · emitter/)                      │
│         ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│         │  FSM     │  │Permission│  │ Emitter  │               │
│         └──────────┘  └──────────┘  └──────────┘               │
├──────────────────────────────────────────────────────────────────┤
│                    🖥️ 表现层                                    │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│   │  CLI (TUI)   │  │  API/NDJSON  │  │  Web UI      │         │
│   └──────────────┘  └──────────────┘  └──────────────┘         │
├──────────────────────────────────────────────────────────────────┤
│                 🛠️ 运行时支持层                                  │
│    (utils/config · logger/ · web/ · workdir/)                   │
└──────────────────────────────────────────────────────────────────┘
```

### 分层说明

| 层级 | 模块 | 职责 |
|------|------|------|
| **Agent 循环层** | `main.go` | 编排模型交互循环，管理轮次生命周期 |
| **能力扩展层** | `subagent/`, `skills/`, `hooks/`, `session/`, `memory/`, `compact/`, `prompt/`, `model/` | 可插拔扩展，增加推理、持久化与智能能力 |
| **工具管理层** | `tools/` | 工具注册表、读写调度、具体工具实现 |
| **状态机控制层** | `fsm/`, `permission/`, `emitter/` | 状态持久化、准入控制、事件输出抽象 |
| **表现层** | `ui/` (TUI), `emitter/api_emitter` (API), `web/` (Web) | 面向不同部署场景的多前端 |
| **运行时支持层** | `utils/`, `logger/`, `web/`, `workdir/` | 配置加载、结构化日志、HTTP 服务器、文件沙箱 |

---

## 🧩 主要组件介绍

### 核心模块

| 模块 | 包路径 | 说明 |
|------|--------|------|
| **Agent Loop** | `main.go:agentLoop` | 主交互循环；调用 LLM、派发工具调用、管理轮次 |
| **ChatClient** | `clients/` | 统一的 ChatClient 接口，支持 OpenAI / Ollama / Anthropic / Gemini |
| **模型注册表** | `model/` | 模型预设定义、选择与运行时热切换 |
| **工具注册表** | `tools/` | 工具注册、发现、派发、并发调度 |
| **子智能体** | `subagent/` | 独立上下文的子智能体执行，隔离上下文与工具 |
| **技能系统** | `skills/` | 从 SKILL.md 文件按需加载技能 |
| **会话管理** | `session/` | 会话 CRUD、JSON 文件持久化、自动命名、历史浏览 |
| **FSM** | `fsm/` | 会话状态封装（消息、轮次、转移） |
| **上下文压缩** | `compact/` | 三层压缩策略（落盘、微压缩、摘要） |
| **Hook 系统** | `hook/` | 事件驱动插件框架，用于工具前后执行 |
| **权限系统** | `permission/` | 基于规则的访问控制、路径沙箱、命令黑名单 |
| **提示词管道** | `prompt/` | 多阶段提示词构建（技能 + 记忆 + 提醒 + 附件） |
| **记忆系统** | `memory/` | 跨会话长期记忆（保存/搜索/删除/更新） |
| **事件发射器** | `emitter/` | 统一输出接口，适配 CLI / API / Web 前端 |
| **交互式 CLI** | `ui/` | Bubble Tea TUI，支持 Markdown 渲染、斜杠命令、状态栏 |
| **Web 服务器** | `web/` | HTTP 服务器，支持 SSE 流式传输、RESTful API、嵌入前端 |
| **结构化日志** | `logger/` | 基于 Zap 的带颜色时间戳日志 |
| **配置加载** | `utils/` | YAML 配置加载，支持默认值与校验 |

### 具体工具

| 工具 | 包路径 | 说明 |
|------|--------|------|
| **read_file** | `tools/` | 读取文件内容 |
| **write_file** | `tools/` | 写入文件 |
| **edit_file** | `tools/` | 搜索替换方式编辑文件 |
| **run_bash** | `tools/` | 执行 shell 命令（含黑名单保护） |
| **glob_search** | `tools/` | 基于模式的文件搜索 |
| **list_directory** | `tools/` | 列出目录内容 |
| **todo_manager** | `tools/` | 多步骤任务计划管理（创建/更新/标记） |
| **sub_task** | `subagent/` | 将工作委托给子智能体 |
| **load_skill** | `skills/` | 按需加载完整技能内容 |
| **save_memory** | `memory/` | 持久化新记忆条目 |
| **search_memory** | `memory/` | 搜索已存储的记忆 |
| **delete_memory** | `memory/` | 删除记忆条目 |
| **update_memory** | `memory/` | 修改已有记忆条目 |
| **compact_history** | `compact/` | 手动触发上下文压缩 |

---

## ⚙️ 安装与配置

### 前置条件

- Go 1.21+
- （可选，用于 Ollama）本地 Ollama 服务
- API 密钥（通过环境变量或 `config/config.yaml` 设置）

### 安装

```bash
git clone https://github.com/balckgu1/ZoomClient.git
cd ZoomClient
go build -o zoomclient .
```

### 配置

编辑 `config/config.yaml` 设置 API 密钥与运行参数：

```yaml
apikeys:
  openai: "sk-xxx"           # OpenAI / 兼容服务
  anthropic: "sk-ant-xxx"    # Anthropic Claude
  gemini: "..."
  # 为空时自动回退到环境变量

agentloop:
  maxTurns: 25
  todoRoundsThreshold: 9
  maxTools: 5
  sensitiveFiles: [".env", "id_rsa"]

compact:
  persistThreshold: 4000
  previewBytes: 1000
  keepRecentToolResults: 4
  contextLimit: 60000
  persistDir: ".task_outputs/tool-results"

subagent:
  defaultMaxTurns: 10
  defaultSystemPrompt: "..."

skills:
  dir: "./.skills"

permission:
  mode: "auto"              # default | plan | auto
  interactive: true
  denyRules: [...]
  allowRules: [...]
```

---

## 🚀 使用方法

### 🎛️ 切换模型后端

使用 `-m` 命令行参数选择后端：

```bash
go run main.go -m openai     # OpenAI 兼容（默认）
go run main.go -m ollama     # 本地 Ollama
go run main.go -m anthropic  # Anthropic Claude
go run main.go -m gemini     # Google Gemini
```

### 🌐 启动 Web 服务器

启动 Web UI 模式替代 CLI TUI：

```bash
go run main.go -m openai -web
```

这将启动 HTTP 服务器（默认端口在 `web/server.go` 中定义），提供完整的 HTML 前端，支持 SSE 实时流式传输、会话浏览和模型切换。

### 🗄️ 会话管理

会话会自动持久化为 JSON 文件。通过 API 或会话管理器可以列出、加载或创建会话。自动命名功能根据对话内容为每个会话生成描述性标题。

### 🧠 运行时模型切换

使用 Web UI 时，可通过 `SwitchModel` API 在运行时热切换模型，不会丢失对话历史。

### ✍️ 修改用户任务与系统提示

在 `main()` 函数中调整以下变量即可自定义行为：

```go
systemPrompt := fmt.Sprintf(
    "You are a helpful assistant running on %s. "+
        "Use the todo tool to plan multi-step work. "+
        "Keep exactly one step in_progress when a task has multiple steps. "+
        "Refresh the plan as work advances. Prefer tools over prose.",
    runtime.GOOS,
)
```

### 📁 调整工作目录

通过 `ToolContext.WorkPath` 设置 Agent 允许操作的文件沙箱根目录：

```go
toolCtx := &tools.ToolContext{
    WorkPath: "./",
}
```

### 📖 使用 Skills

将你的技能文件放到 `config.yaml` 中 `skills.dir` 指定的目录（默认 `./.skills/`），每个技能一个子目录，内含一个 `SKILL.md` 文件，格式如下：

```markdown
---
name: your-skill-name
description: 一句话描述该技能用途
---

# 正文

详细的操作步骤或 playbook...
```

启动后模型会在 system prompt 中看到技能目录清单，并通过 `load_skill` 工具按需加载完整正文。

### 🧬 使用子智能体

模型可通过 `sub_task` 工具派发子任务：

- 默认 `fork=false`：子智能体以空白上下文执行，prompt 需自包含。
- 设置 `fork=true`：子智能体继承父消息历史，适合"基于当前对话做进一步分析"的场景。

---

## 🛠️ 开发指南

### ➕ 添加新工具

1. 在 `tools/` 目录下创建新文件（如 `mynewtool.go`）。
2. 定义结构体并实现 `Tool` 接口的四个方法：
   - `Name() string` — 工具唯一标识名。
   - `Description() string` — 模型判断何时调用的描述。
   - `Parameters() map[string]interface{}` — JSON Schema 格式的参数定义。
   - `Call(args map[string]interface{}, ctx *ToolContext) ToolResult` — 执行逻辑。
3. 若工具为只读且无副作用，在 `tools/runtime.go` 的 `concurrencySafeTools` 映射中注册，使其可并发执行。
4. 在 `main.go` 中通过 `registry.Register(YourTool{})` 显式注册该工具。
5. 如需让子智能体也能使用该工具，请同步在 `subagent.BuildSubAgentRegistry()` 中注册。
6. 如需让权限系统识别该工具的读写属性，请同步在 `permission/permission.go` 的 `readOnlyTools` 或 `writeTools` 映射中注册。

### 🔌 接入新的 LLM 后端

1. 在 `clients/` 下创建新文件（如 `claude_chat.go`）。
2. 定义客户端结构体，并实现 `ChatClient` 接口的 `Chat(model, messages, toolList, options)` 方法。
3. 在方法内完成：工具 schema 转换 → 消息协议转换 → HTTP 调用 → 响应归一化为 `*ChatResponse`。
4. 在 `main.go` 的 `switch modelType` 分支中新增对应 case，处理 API Key 读取（支持环境变量回退）与客户端初始化。

### 📖 添加新 Skill

无需写代码，直接在 `.skills/` 下新建子目录并放入 `SKILL.md`（参考 `.skills/skill-function-test/SKILL.md`），重启后即可被 `SkillRegistry` 自动发现。

### 🧠 添加模型预设

在 `model/registry.go` 的 presets 映射中添加新条目（客户端类型、地址、模型名），即可通过 `-m` 参数选择并支持运行时热切换。

### 🔄 添加新 Emitter 实现

实现 `emitter.Emitter` 接口，在 `main.go` 中注册。当前实现包括：`ui.Renderer`（CLI TUI）、`emitter.ApiEmitter`（NDJSON stdout）、`web.SSEEmitter`（Web SSE 事件）。

### 🧪 单元测试

项目各核心模块均配有 `*_test.go` 测试文件。运行全部测试：

```bash
go test ./...
```

单独运行某个包的测试：

```bash
go test ./tools/...
go test ./subagent/...
go test ./skills/...
go test ./session/...
go test ./memory/...
go test ./prompt/...
go test ./model/...
go test ./compact/...
go test ./hook/...
go test ./permission/...
```

### 🐞 日志调试

通过 Zap 日志的级别控制（`Debug` / `Info` / `Warn` / `Error`）观察 Agent 运行时的消息流转、工具调用批次与执行结果。`.vscode/launch.json` 已提供多场景调试配置。

---

## 🚧 待实现功能

| 模块 | 说明 |
|------|------|
| **🔗 MCP 集成** | 利用 `ToolContext.McpClients` 预留字段，接入 Model Context Protocol（MCP）外部工具生态，实现跨进程工具调用。 |
| **🧪 只读 Bash 沙箱** | 为子智能体提供受限版的 `run_bash`，仅允许纯查询类命令，进一步降低副作用风险。 |
| **📈 执行指标与可观测性** | 工具调用次数、耗时、失败率等指标采集，辅助性能调优与问题定位。 |
| **🧠 记忆排序增强** | 引入相关性评分、时间衰减与跨会话去重，优化记忆检索质量。 |
| **🔌 插件 / MCP 客户端 SDK** | 正式发布 MCP 客户端集成 SDK，供第三方工具提供商接入。 |
| **🔄 多用户与协作** | 支持多用户会话、共享工作区与协同 Agent 工作流。 |

---

## 📄 License

本项目基于 MIT License 开源，详见 [LICENSE](LICENSE) 文件。
