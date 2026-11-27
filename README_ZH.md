# 🤖 ZoomClient

**中文 | [English](README.md)**

ZoomClient 是一个基于 Go 语言、以 Tool-Use 为核心的 AI Agent 运行时，实现自主任务执行循环。
项目采用模块化架构，涵盖多后端模型客户端、工具控制平面、并发执行运行时、Subagent、带安全校验的 Skill 按需加载、会话管理、上下文压缩、提示词管道、Hook 系统、权限系统、持久化任务图、跨会话记忆、事件发射器、指标采集，以及 readline CLI 与内嵌 Web UI 前端。

---

## 📚 目录

- [✨ 核心功能特性](#-核心功能特性)
- [🏗️ 架构设计说明](#️-架构设计说明)
- [🧩 主要组件介绍](#-主要组件介绍)
- [⚙️ 安装与配置](#️-安装与配置)
- [🚀 使用方法](#-使用方法)
- [🛠️ 开发指南](#️-开发指南)
- [🚧 后续规划](#-后续规划)

---

## ✨ 核心功能特性

- **🔁 Agent Loop**：驱动模型交互的主循环，支持系统提示注入、消息历史维护与多轮工具调用。最大轮次由 `agentloop.maxTurns` 配置（默认 25）；LLM 调用失败或返回空响应时，通过 Hook 按 `agentloop.maxLLMRetries` 自动重试。
- **🔌 多后端 ChatClient 抽象**：统一 `clients.ChatClient` 接口，内置 **OpenAI 兼容**（DeepSeek / Kimi / Qwen / OpenAI）、**Ollama**（NDJSON 流式协议）、**Anthropic**（Claude）与 **Gemini** 四种实现，通过 `-m` 参数切换。
- **⚙️ 模型预设与热切换**：`model.Registry` 管理模型预设（客户端类型、地址、API Key、模型名），并持久化到 `config/models.yaml`。可在 CLI 中通过 `/setmode`、`/models`、`/selectmode` 管理，也可在 Web UI 中增删改查与连通性测试；运行时切换后端不丢失对话历史。
- **🔧 工具控制平面（Tool Control Plane）**：统一的工具注册表 `tools.ToolRegister`，支持工具发现、注册与按名称动态调度执行，内置权限门控（`PermissionDecider`）实现执行前准入控制。
- **⚡ 并发安全调度**：工具调用按读写属性自动分批，只读工具（如 `read_file`、`glob_search`、`grep_search`、`web_fetch`）并发执行，写操作类工具串行执行，兼顾效率与安全性。
- **📡 联网与人机协作**：`web_fetch` 抓取 URL（HTML 转文本），配置后可用 `web_search` 通过 Tavily / Brave / Serper / Bocha 联网检索；`ask_user_question` 让模型发起结构化提问并阻塞等待，CLI 走标准输入、Web 走浏览器弹窗（`/api/question`）。
- **🧬 子智能体（Subagent）**：通过 `sub_task` 工具将子任务委托给独立上下文的子智能体；支持空白上下文模式与 `fork=true` 继承父消息上下文模式，子智能体使用独立的受限工具白名单，避免副作用穿透。
- **📖 Skills 按需加载**：递归扫描 `.skills/` 下的 `SKILL.md`（含 YAML frontmatter），仅将目录清单注入 system prompt，完整正文通过 `load_skill` 工具按需加载（支持名称模糊匹配）。
- **🛡️ Skill 安全校验**：对技能名称、描述与正文进行静态红旗扫描（Unicode 归一化、词族共现、凭据/外传组合判定），并包含 description 净化、符号链接拒绝与「不可信」声明包裹；`skilllab/` 提供本地实验台，可直观演示这些防线。
- **📝 会话计划管理（Todo Write）**：允许模型在会话中维护多步骤任务计划并跟踪状态；连续 `agentloop.todoRoundsThreshold` 轮未更新计划时自动注入提醒。
- **🗂️ 持久化任务图（Task Graph）**：`task/` 模块将每个任务以 JSON 文件存储（默认 `.tasks/`），提供 `create_task`、`list_tasks`、`get_task`、`claim_task`、`complete_task`，支持 `blockedBy` 依赖、`pending` / `in_progress` / `completed` 状态与跨会话持久化。
- **🗜️ 上下文压缩（Context Compact）**：三层压缩策略——大工具结果落盘替换为预览、旧工具结果微压缩为占位符、整体历史过长时调模型生成连续性摘要；同时向 Web UI 推送上下文占用快照（system prompt / skills / 工具 schema / 消息历史）。
- **🪝 Hook 系统**：事件驱动钩子框架，覆盖 `SessionStart`、`PreChat`、`PostChat`、`LLMError`、`PreToolUse`、`PostToolUse`、`ToolError`、`SessionEnd` 八个事件点，内置危险命令拦截、敏感文件保护、速率限制、审计日志、LLM 重试与错误恢复等处理器。
- **🛡️ 权限系统**：基于规则引擎的细粒度权限控制，支持 `default`（未命中问用户）、`plan`（只读）、`auto`（只读自动放行）、`root`（完全放行）四种模式；allow/deny 规则支持子串与 `re:` 前缀正则匹配，并可在 Web UI 中运行时修改。
- **🛡️ 路径沙箱保护**：文件类工具执行前需通过路径沙箱校验（基于 `ToolContext.WorkPath`），禁止路径穿越与工作目录逃逸。
- **🚫 危险命令拦截**：`run_bash` 内置命令黑名单检测（支持 `permission.denyRules` 中的子串与正则规则），覆盖 Linux / macOS / Windows 高危操作。
- **🧠 Thinking 模式支持**：完整透传 `reasoning_content` 字段，多轮对话中原样回传，避免服务端以 `invalid_request_error` 拒绝请求。
- **🗄️ 会话管理（Session Management）**：基于 JSON 文件持久化的会话 CRUD（`session.Manager`），包含元数据索引、自动标题生成与工作目录记录，支持会话恢复、跨进程浏览，以及 CLI / Web 端的列表、加载、新建、删除、重命名。
- **🧩 FSM（有限状态机）**：封装 Agent 会话状态，包含消息历史、轮次计数与转移原因，提供与业务逻辑解耦的干净状态抽象。
- **📦 提示词管道（Prompt Pipeline）**：多阶段提示词构建系统（`prompt.MessagePipeline`），依次组装核心指令、技能目录、记忆摘要、`CLAUDE.md`、动态上下文、逐轮提醒与附件，最后按目标模型协议归一化输出。
- **💭 跨会话记忆系统（`memory/`）**：支持四种记忆类型（user / feedback / project / reference），提供 `save_memory`、`search_memory`（默认匹配元数据，`full_text=true` 时匹配正文）、`update_memory`、`delete_memory` 工具。`MEMORY.md` 作为索引（最多注入 50 条），单条正文搜索结果截断至 15 KB；开启 `memory.autoExtractOnTurn` 后，每轮结束时同步执行增量抽取（基于游标 + 指纹去重）。
- **🔄 事件发射器（`emitter/`）**：统一的 Agent 可见事件输出抽象（会话生命周期、助手输出、推理内容、工具调用/结果、子智能体、Hook 拦截、错误、计划更新、压缩、相位事件）。当前实现为 CLI 渲染器与 Web SSE 发射器。
- **🌐 Web UI（`web/`）**：内置 HTTP 服务器，支持 SSE 实时流式传输、RESTful API（对话、会话、模型预设与连通性测试、工作目录、技能目录、权限配置、上下文占用、停止执行）、权限询问弹窗，以及通过 `go:embed` 内嵌的 Preact/Vite 前端；Web 模式启动后自动打开浏览器。
- **🖥️ 交互式 CLI**：基于 `readline` 的行渲染器，支持 Tab 补全、↑↓ 历史、彩色输出与斜杠命令（`/help`、`/clear`、`/compact`、`/exit`、`/quit`、`/models`、`/setmode`、`/selectmode`、`/workspace`、`/session`）。
- **📈 可观测性**：可选的 Hook 指标采集器，将 LLM 延迟、Token 用量、工具调用轨迹、错误/重试、会话生命周期以 JSONL 追加写入 `logs/metrics.jsonl`，可供评测 harness 与 LangSmith 上报消费。
- **📂 工作目录管理**：启动时通过 `-workdir` 指定沙箱根目录，运行时可通过 `/workspace`（CLI）或 `POST /api/workdir`（Web）切换。
- **🪵 结构化日志与 YAML 配置**：基于 Uber Zap 输出带颜色与时间戳的日志；`config/config.yaml` 覆盖 API Key、轮次、技能、记忆、会话、任务、权限、压缩与可观测性等配置。

---

## 🏗️ 架构设计说明

项目采用分层架构设计，从上到下依次为：Agent 循环层、能力扩展层、工具管理层、状态与控制层、表现层、运行时支持层：

```
┌──────────────────────────────────────────────────────────────────┐
│                      🔁 Agent 循环层                             │
│              (main.go · agent.go · agent_loop.go · repl.go)      │
├──────────────────────────────────────────────────────────────────┤
│                   🧩 能力扩展层                                  │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │subagent/ │  │ skills/  │  │  hook/   │  │ session/ │       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │ memory/  │  │ compact/ │  │ prompt/  │  │ model/   │       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐                                                  │
│   │  task/   │  持久化任务图                                    │
│   └──────────┘                                                  │
├──────────────────────────────────────────────────────────────────┤
│                    🔧 工具管理层                                 │
│          (tools/ · ToolRegister · 并发执行运行时)                │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │ read_file│  │write_file│  │ run_bash │  │glob_search│      │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐                     │
│   │ edit_file│  │list_dir  │  │  todo    │                     │
│   └──────────┘  └──────────┘  └──────────┘                     │
├──────────────────────────────────────────────────────────────────┤
│                  ⚙️ 状态与控制层                                 │
│            (fsm/ · permission/ · emitter/)                      │
│         ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│         │  FSM     │  │Permission│  │ Emitter  │               │
│         └──────────┘  └──────────┘  └──────────┘               │
├──────────────────────────────────────────────────────────────────┤
│                    🖥️ 表现层                                    │
│   ┌──────────────────────┐  ┌──────────────────────┐           │
│   │  CLI (readline)      │  │  Web UI (SSE)        │           │
│   └──────────────────────┘  └──────────────────────┘           │
├──────────────────────────────────────────────────────────────────┤
│                 🛠️ 运行时支持层                                  │
│    (utils/ · logger/ · web/ · metrics/ · skilllab/)             │
└──────────────────────────────────────────────────────────────────┘
```

### 分层说明

| 层级 | 模块 | 职责 |
|------|------|------|
| **Agent 循环层** | `main.go`, `agent.go`, `agent_loop.go`, `repl.go` | 编排模型交互循环，处理 CLI / Web 命令 |
| **能力扩展层** | `subagent/`, `skills/`, `hook/`, `session/`, `memory/`, `compact/`, `prompt/`, `model/`, `task/` | 可插拔扩展，提供推理、持久化、计划与智能能力 |
| **工具管理层** | `tools/` | 工具注册表、读写调度、具体工具实现 |
| **状态与控制层** | `fsm/`, `permission/`, `emitter/` | 状态持久化、准入控制、事件输出抽象 |
| **表现层** | `ui/`（CLI）, `web/`（Web UI + SSE） | 面向终端与浏览器的多前端 |
| **运行时支持层** | `utils/`, `logger/`, `web/`, `metrics/`, `skilllab/` | 配置加载、结构化日志、HTTP 服务、指标采集、Skill 安全实验台 |

---

## 🧩 主要组件介绍

### 核心模块

| 模块 | 包路径 | 说明 |
|------|--------|------|
| **Agent Loop** | `agent_loop.go` | 主交互循环；调用 LLM、派发工具调用、管理轮次与重试 |
| **ChatClient** | `clients/` | 统一的 ChatClient 接口，支持 OpenAI 兼容 / Ollama / Anthropic / Gemini |
| **模型注册表** | `model/` | 模型预设定义、YAML 持久化与运行时热切换 |
| **工具注册表** | `tools/` | `ToolRegister`：工具注册、发现、派发、并发调度 |
| **子智能体** | `subagent/` | 独立上下文的子智能体执行，隔离上下文与工具 |
| **技能系统** | `skills/` | SKILL.md 递归加载、按需读取、模糊匹配与安全校验 |
| **任务图** | `task/` | 带依赖与状态流转的持久化任务 |
| **会话管理** | `session/` | 会话 CRUD、JSON 持久化、索引、自动命名、工作目录元数据 |
| **FSM** | `fsm/` | 会话状态封装（消息、轮次、转移） |
| **上下文压缩** | `compact/` | 三层压缩策略（落盘、微压缩、摘要）与占用快照 |
| **Hook 系统** | `hook/` | 事件驱动插件框架，覆盖会话 / LLM / 工具生命周期 |
| **权限系统** | `permission/` | 基于规则的访问控制、路径沙箱、命令黑名单 |
| **提示词管道** | `prompt/` | 多阶段提示词构建（核心 + 技能 + 记忆 + CLAUDE.md + 提醒 + 附件） |
| **记忆系统** | `memory/` | 跨会话记忆、索引加载、全文检索与逐轮抽取 |
| **事件发射器** | `emitter/` | 统一输出接口，适配 CLI 与 Web 前端 |
| **交互式 CLI** | `ui/` | 基于 readline 的 REPL，支持补全、历史、颜色与斜杠命令 |
| **Web 服务器** | `web/` | HTTP 服务，支持 SSE、REST API、内嵌 Preact 前端与权限询问 |
| **指标采集** | `metrics/` | 基于 Hook 的 JSONL 指标采集器 |
| **Skill 实验台** | `skilllab/` | 演示 Skill 投毒防护的本地 Web 实验台 |
| **结构化日志** | `logger/` | 基于 Zap 的带颜色时间戳日志 |
| **配置加载** | `utils/` | YAML 配置加载，支持默认值与校验 |

### 具体工具

| 工具 | 包路径 | 说明 |
|------|--------|------|
| **read_file** | `tools/` | 读取文件内容 |
| **write_file** | `tools/` | 写入文件 |
| **edit_file** | `tools/` | 搜索替换方式编辑文件 |
| **run_bash** | `tools/` | 执行 shell 命令（含黑名单保护与超时控制） |
| **glob_search** | `tools/` | 基于模式的文件搜索 |
| **grep_search** | `tools/` | 基于正则的内容搜索，带行号、glob 过滤与 content / files / count 三种输出模式 |
| **web_fetch** | `tools/` | 抓取 HTTP(S) 页面并转换为可读文本 |
| **web_search** | `tools/` | 通过 Tavily / Brave / Serper / Bocha 联网搜索（需配置 `websearch.api_key`） |
| **ask_user_question** | `tools/` | 向用户发起结构化提问（CLI 标准输入 / Web 弹窗）并等待回答 |
| **list_directory** | `tools/` | 列出目录内容 |
| **todo** | `tools/` | 多步骤会话计划管理（创建/更新/标记） |
| **create_task** | `task/` | 创建带 `blockedBy` 依赖的持久化任务 |
| **list_tasks** | `task/` | 列出任务的状态、持有者与依赖 |
| **get_task** | `task/` | 按 ID 获取任务完整详情 |
| **claim_task** | `task/` | 认领待办任务（校验依赖是否满足） |
| **complete_task** | `task/` | 完成任务并报告被解锁的下游任务 |
| **sub_task** | `subagent/` | 将工作委托给子智能体（空白或继承上下文） |
| **load_skill** | `skills/` | 按需加载完整技能内容 |
| **save_memory** | `memory/` | 持久化新记忆条目并更新 `MEMORY.md` |
| **search_memory** | `memory/` | 按元数据搜索记忆，`full_text=true` 时全文搜索 |
| **delete_memory** | `memory/` | 从磁盘与索引中删除记忆条目 |
| **update_memory** | `memory/` | 修改已有记忆条目 |
| **compact** | `compact/` | 手动触发上下文压缩 |

---

## ⚙️ 安装与配置

### 前置条件

- **Go 1.25+**
- **Node.js 22+ 与 npm**（仅构建 Web UI 时需要）
- （可选，用于 Ollama）本地 Ollama 服务
- API 密钥（写入 `config/config.yaml` 或通过环境变量设置）

### 快速开始（预编译版本）

发布工作流会在 GitHub Releases 页面提供 Windows 二进制：

1. 下载 `zoomclient_<version>_windows_amd64.zip`。
2. 解压后复制 `config/config.yaml.example` 为 `config/config.yaml`。
3. 填入 API 密钥，运行 `zoomclient.exe`（CLI）或 `zoomclient.exe -mode web`（浏览器 UI）。

### 从源码构建

```bash
git clone https://github.com/balckgu1/ZoomClient.git
cd ZoomClient

# 可选，但 Web UI 完整功能需要
cd web/frontend
npm ci
npm run build
cd ../..

go build -o zoomclient .
```

> Web 前端通过 `//go:embed all:frontend/dist` 内嵌。仓库仅跟踪 `web/frontend/dist/.gitkeep`（以及 `public/.gitkeep`），因此全新克隆可正常编译、CLI 可直接使用，但在执行 `npm run build` 之前 Web UI 只会提供占位内容。

### 配置

复制 `config/config.yaml.example` 为 `config/config.yaml`（也可通过 `-config-dir` 指定其他目录）：

```yaml
openai:
  api_key: "sk-xxx"
  base_url: "https://api.deepseek.com"
  model_name: "deepseek-v4-flash"

ollama:
  base_url: "http://127.0.0.1:11434"
  model_name: "qwen3:8b"

anthropic:
  api_key: "sk-ant-xxx"
  model_name: "claude-opus-4-8"

gemini:
  api_key: "AIzaSyXXX"
  model_name: "gemini-3.5-flash"

agentloop:
  maxTurns: 25
  todoRoundsThreshold: 9
  maxTools: 5
  sensitiveFiles: [".env", "id_rsa"]
  maxLLMRetries: 3

compact:
  persistThreshold: 4000
  previewBytes: 1000
  keepRecentToolResults: 4
  contextLimit: 60000
  persistDir: ".task_outputs/tool-results"

subagent:
  defaultMaxTurns: 20
  defaultSystemPrompt: "..."
  forkSubtaskPromptPrefix: "..."
  temperature: 0.3

tools:
  defaultBashTimeout: 120      # 秒

websearch:                     # 可选：配置 api_key 后启用 web_search 工具
  provider: "tavily"           # tavily | brave | serper | bocha
  api_key: ""
  max_results: 5
  timeoutSeconds: 20

skills:
  dir: "./.skills"

memory:
  dir: "./.memory"
  autoExtractOnTurn: true      # 每轮结束时同步增量抽取记忆

session:
  dir: "./.sessions"

tasks:
  dir: "./.tasks"

permission:
  mode: "auto"                 # default | plan | auto | root
  interactive: true
  denyRules:
    - tool: "run_bash"
      content: "rm -rf /"
    - tool: "run_bash"
      content: "re:^git\\s+push"
  allowRules:
    - tool: "run_bash"
      content: "git status"

observability:
  metrics:
    enabled: false             # 开启后写入 JSONL 指标
    path: "logs/metrics.jsonl"
```

> `config/models.yaml` 保存通过 `/setmode` 或 Web UI 创建的模型预设，由模型注册表自动管理。

---

### 🎛️ 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-m` | `openai` | 后端类型：`openai` \| `ollama` \| `anthropic` \| `gemini` |
| `-mode` | `cli` | 输出模式：`cli`（终端）\| `web`（浏览器 UI） |
| `-web` | `false` | `-mode web` 的废弃别名 |
| `-port` | `8080` | Web 服务端口（仅 web 模式） |
| `-config-dir` | `./config` | 配置目录（包含 `config.yaml`） |
| `-workdir` | 当前目录 | 工具沙箱根目录 |

```bash
go run .                                  # CLI，默认 OpenAI 兼容后端
go run . -m ollama                        # 本地 Ollama
go run . -m anthropic                     # Anthropic Claude
go run . -m gemini -workdir D:/workspace  # Gemini，自定义沙箱根目录
go run . -m openai -mode web -port 9090   # Web UI，监听 9090
```

### 🌐 启动 Web 服务器

```bash
go run . -m openai -mode web     # 旧写法等价：go run . -m openai -web
```

服务器会提供内嵌前端并自动打开浏览器。主要接口：

| 分组 | 接口 |
|------|------|
| 对话 | `POST /api/chat`、`GET /api/events`（SSE）、`POST /api/stop`、`POST /api/clear`、`POST /api/compact` |
| 权限 | `POST /api/permission`（回答询问）、`GET/PUT /api/permission/config` |
| 会话 | `GET/POST /api/sessions`、`GET/PATCH/DELETE /api/sessions/{id}` |
| 模型 | `GET/POST /api/models`、`POST /api/model/select`、`POST /api/models/test`、`PUT/DELETE /api/models/{name}` |
| 运行时 | `GET/POST /api/workdir`、`GET /api/skills`、`GET /api/status`、`GET /api/context-usage` |

### ⌨️ CLI 斜杠命令

| 命令 | 说明 |
|------|------|
| `/help` | 显示可用命令 |
| `/clear` | 清空对话历史（保留 system prompt） |
| `/compact` | 手动压缩对话历史 |
| `/exit`、`/quit` | 退出 |
| `/models` | 列出已配置的模型预设 |
| `/setmode -m <name> -t <type> -u <url> -k <key> [--model <model>]` | 新增或更新模型预设 |
| `/selectmode -m <name>` | 切换模型预设（保留历史） |
| `/workspace [path]` | 查看或切换工作目录 |
| `/session list\|load <id>\|new\|delete <id>\|rename <id> <title>\|current` | 管理会话 |

### 🧠 运行时模型切换

在 Web UI 的模型面板中选择预设（或调用 `POST /api/model/select`）；在 CLI 中使用 `/selectmode -m <name>`。Agent 会重新绑定客户端、提示词构建器与压缩管理器，同时保留消息历史。

### 📖 使用 Skills

将技能放到 `config.yaml` 中 `skills.dir` 指定的目录（默认 `./.skills/`）。技能会被递归发现，每个技能一个子目录，内含带 YAML frontmatter 的 `SKILL.md`：

```markdown
---
name: your-skill-name
description: 一句话描述该技能用途
---

# 正文

详细的操作步骤或 playbook...
```

启动后模型只看到技能目录清单，完整正文通过 `load_skill` 按需加载。命中安全红旗的技能会被拒绝加载，或在正文前附带「不可信」声明。

### 🧬 使用子智能体

模型可通过 `sub_task` 工具派发子任务：

- 默认 `fork=false`：子智能体以空白上下文执行，prompt 需自包含。
- 设置 `fork=true`：子智能体继承父消息历史，适合「基于当前对话继续分析」的场景。

子智能体使用独立注册表（read_file / list_directory / run_bash / glob_search），并共享同一套权限策略。

### 🗂️ 任务图

使用 `create_task` 创建带 `blockedBy` 依赖的持久化任务，`list_tasks` / `get_task` 查看任务，`claim_task` 将就绪任务置为 `in_progress`，`complete_task` 完成任务并报告被解锁的下游任务。任务保存在 `tasks.dir`，可跨会话、跨进程持久化。

### 💭 记忆抽取

开启 `memory.autoExtractOnTurn: true` 后，每轮结束时会同步执行一次增量抽取（抽取完成前该轮不会结束）。游标与消息指纹用于避免重复抽取；失败会在下一轮自动重试。手动 `/clear`、`/compact` 与 `/session` 切换前也会先冲刷待抽取的记忆窗口。

### 📈 指标采集

设置 `observability.metrics.enabled: true` 后，每个事件会向 `logs/metrics.jsonl` 追加一行 JSON。事件覆盖会话开始/结束、LLM 调用（延迟、Token 估算）、错误/重试与工具调用。采集器是纯观察者，不会改变 Agent 行为。

---

## 🛠️ 开发指南

### ➕ 添加新工具

1. 在 `tools/` 目录下创建新文件（如 `mynewtool.go`）。
2. 定义结构体并实现 `Tool` 接口的四个方法：
   - `Name() string` — 工具唯一标识名。
   - `Description() string` — 模型判断何时调用的描述。
   - `Parameters() map[string]any` — JSON Schema 格式的参数定义。
   - `Call(args map[string]any, ctx *ToolContext) ToolResult` — 执行逻辑。
3. 在 `init.go:initTools` 中通过 `registry.Register(YourTool{})` 注册该工具。
4. 若工具为只读且无副作用，在 `tools/runtime.go` 的 `concurrencySafeTools` 映射中注册，使其可并发执行。
5. 在 `permission/permission.go` 的 `readOnlyTools` 或 `writeTools` 映射中登记，确保权限系统正确分类。
6. 如需让子智能体也能使用该工具，请同步在 `subagent.BuildSubAgentRegistry()` 中注册。

### 🔌 接入新的 LLM 后端

1. 在 `clients/` 下创建新文件（如 `claude_chat.go`）。
2. 定义客户端结构体，并实现 `ChatClient` 接口的 `Chat(model, messages, toolList, options)` 方法。
3. 在方法内完成：工具 schema 转换 → 消息协议转换 → HTTP 调用 → 响应归一化为 `*ChatResponse`。
4. 在 `init.go:initClient` 中新增 case，处理 API Key 读取（支持环境变量回退）、默认地址/模型与客户端初始化。

### 📖 添加新 Skill

无需写代码，直接在 `.skills/` 下新建子目录并放入 `SKILL.md`（参考 `.skills/skill-function-test/SKILL.md`），重启后会被递归发现。可用 `skilllab` 验证安全校验行为：

```bash
go run ./skilllab -addr 127.0.0.1:19490 -skills ./.skills
```

### 🧠 添加模型预设

在 CLI 中使用 `/setmode`，或在 Web UI 中新增，或直接编辑 `config/models.yaml`。预设由 `model.Registry` 持久化，可被 `-m`、`/selectmode` 与运行时切换使用。

### 🔄 添加新 Emitter 实现

实现 `emitter.Emitter` 接口（包括 `EmitPhase` 与 `EmitSystem`），并在 `init.go:initEmitter` 中接入。当前实现：`ui.Renderer`（CLI）与 `web.SseEmitter`（Web SSE）。

### 🎨 前端开发

```bash
cd web/frontend
npm ci
npm run dev      # Vite 开发服务器，带 API 代理
npm run build    # 生成 dist/ 供 go:embed 使用
```

由于 `dist/` 会被嵌入二进制，修改前端代码后务必先执行 `npm run build` 再执行 `go build`。请勿删除 `web/frontend/dist/.gitkeep` 与 `web/frontend/public/.gitkeep`。

### 🧪 单元测试与静态检查

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
go test ./task/...
go test ./metrics/...
go test ./web/...
```

CI（`.github/workflows/test.yml`）会运行 Go 测试、`golangci-lint`（仅检查新增问题）以及前端 TypeScript 类型检查。

### 🚀 发布

推送 `v*` 标签后，发布工作流会先构建前端，再调用 GoReleaser 交叉编译 Windows amd64 的 `zoomclient`，与 `LICENSE`、中英文 README、`config/config.yaml.example` 一起打包为 zip，并附校验和发布到 GitHub Releases。

### 🐞 日志调试

通过 Zap 日志的级别控制（`Debug` / `Info` / `Warn` / `Error`）观察 Agent 运行时的消息流转、工具调用批次与执行结果。`.vscode/launch.json` 已提供多场景调试配置。

---

## 🚧 后续规划

| 模块 | 说明 |
|------|------|
| **🔗 MCP 集成** | 接入 Model Context Protocol（MCP）外部工具生态，实现跨进程工具调用。 |
| **🧪 只读 Bash 沙箱** | 为子智能体提供受限版的 `run_bash`，仅允许纯查询类命令，进一步降低副作用风险。 |
| **🧠 记忆排序增强** | 引入相关性评分、时间衰减与跨会话去重，优化记忆检索质量。 |
| **🔌 插件 / MCP 客户端 SDK** | 正式发布 MCP 客户端集成 SDK，供第三方工具提供商接入。 |
| **🔄 多用户与协作** | 支持多用户会话、共享工作区与协同 Agent 工作流。 |
| **🖥️ 多平台发布** | 在 Windows amd64 之外增加 Linux / macOS 构建产物。 |

---

## 📄 License

本项目基于 **Apache License 2.0** 开源，详见 [LICENSE](LICENSE) 文件。
