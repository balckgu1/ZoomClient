# 🤖 ZoomClient

ZoomClient 是一个基于 Go 语言实现的 AI Agent 框架，通过与 LLM 服务交互，实现具备 Tool-Use 能力的自主任务执行系统。
项目采用模块化架构，涵盖多后端模型客户端、工具控制平面、并发执行运行时、子智能体（Subagent）、技能（Skill）按需加载、会话计划管理等核心子系统。

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

- **🔁 Agent Loop**：驱动模型交互的主循环，支持系统提示注入、消息历史维护与多轮次工具调用，最大轮次由 `config.yaml` 中的 `app.maxTurns` 配置（默认 20）。
- **🔌 多后端 ChatClient 抽象**：统一 `clients.ChatClient` 接口，内置 **Ollama**（NDJSON 流式协议）与 **DeepSeek**（OpenAI 兼容协议）两种实现，支持通过命令行 `-m` 参数切换。
- **🧠 DeepSeek Thinking 模式支持**：完整透传 `reasoning_content` 字段，多轮对话中原样回传，避免服务端以 `invalid_request_error` 拒绝请求。
- **🔧 工具控制平面（Tool Control Plane）**：统一的工具注册表 `tools.Registry`，支持工具发现、注册与按名称动态调度执行。
- **⚡ 并发安全调度**：工具调用按读写属性自动分批，只读工具（如 `read_file`）可并发执行，写操作类工具串行执行，兼顾效率与安全性。
- **🧬 子智能体（Subagent）**：通过 `sub_task` 工具将子任务委托给独立上下文的子智能体；支持空白上下文模式与 `fork=true` 继承父消息上下文模式，子智能体拥有独立工具白名单避免副作用穿透。
- **📖 Skills 按需加载**：扫描 `.skills/` 目录下的 `SKILL.md`（含 YAML frontmatter），仅将目录清单注入 system prompt，完整正文通过 `load_skill` 工具按需加载，降低上下文成本。
- **📝 会话计划管理（Todo Write）**：允许模型在会话中维护多步骤任务计划，支持状态跟踪；连续多轮未更新计划时自动注入提醒消息。
- **🛡️ 路径沙箱防护**：文件类工具执行前需通过路径沙箱校验（基于 `ToolContext.WorkPath`），禁止路径穿越与工作目录逃逸。
- **🚫 危险命令拦截**：`run_bash` 工具内置命令黑名单检测，拦截高危操作指令。
- **🪵 结构化日志**：基于 Uber Zap 日志库，输出彩色、带时间戳的日志，支持开发与生产多环境配置。
- **🗂️ YAML 配置化启动**：通过 `config/config.yaml` 管理 API Key、最大轮次、Skills 目录、Subagent 默认 prompt 等参数，避免密钥硬编码。

---

## 🏗️ 架构设计说明

项目整体采用分层架构，自上而下分为 Agent Loop 层、能力扩展层、工具管理层、状态机控制层与运行时支撑层：

```
┌─────────────────────────────────────────────────────────┐
│                   🔁 Agent Loop 层                      │
│                (main.go → agentLoop)                    │
├─────────────────────────────────────────────────────────┤
│                  🧬 能力扩展层                           │
│     (subagent/  ·  skills/  ·  tools/todo_manager)      │
├─────────────────────────────────────────────────────────┤
│                  🔧 工具管理层                           │
│     (tools/tools.go · tools/runtime.go · 各具体工具)    │
├─────────────────────────────────────────────────────────┤
│                  📊 状态机控制层                         │
│                    (fsm/states.go)                      │
├─────────────────────────────────────────────────────────┤
│                  🧱 运行时支撑层                         │
│  (clients/ · logger/logger.go · utils/loadconfig.go)    │
└─────────────────────────────────────────────────────────┘
```

一次请求生命周期如下：

1. **🚀 初始化阶段**：加载 `config/config.yaml`，根据 `-m` 参数创建对应的 `ChatClient`（Ollama 或 DeepSeek）；构建工具注册表、技能注册表、计划管理器、子智能体与会话状态。
2. **💬 模型交互阶段**：将系统提示（含 Skills 目录清单）、用户输入与消息历史发送至 LLM API，请求模型生成响应，`reasoning_content` 字段原样保留。
3. **🔧 工具调用阶段**：若模型返回工具调用请求，Agent Loop 调用 `PartitionToolCalls` 按并发安全性分批，并发批次使用 goroutine 并行执行，串行批次逐个执行。
4. **📥 结果回写阶段**：所有工具执行结果按原始调用顺序写回消息历史，保持模型对执行结果理解的确定性；每条 `tool` 消息携带 `ToolCallID` 以兼容 OpenAI 协议。
5. **📝 计划维护阶段**：检查本轮是否使用了 `todo` 工具，若连续多轮未更新，将提醒注入到消息历史中，促使模型刷新计划。
6. **🔁 循环终止条件**：当模型不再请求工具调用，或达到 `app.maxTurns` 配置上限时，循环终止。

---

## 🧩 主要组件介绍

### 1. 🚪 `main.go` — Agent 主循环入口

定义 `agentLoop` 函数，负责编排一次完整的 Agent 会话。核心职责包括：
- 管理消息历史（`fsm.State.Messages`），自动追加系统提示、用户输入、助手响应（含 `reasoning_content`）与工具结果。
- 调用 `tools.PartitionToolCalls` 与 `tools.ExecuteBatches` 完成工具调度并打印分批信息。
- 与 `TodoManager` 协作，维护计划更新与提醒机制。
- 解析 `-m` 命令行参数，按需构造 Ollama 或 DeepSeek 客户端。
- 装配 Skills 目录清单到 system prompt，注册工具（含 `load_skill`、`sub_task`、`todo`）。

### 2. 🔌 `clients/` — 多后端模型客户端适配层

| 文件 | 说明 |
|------|------|
| `client.go` | 定义 `ChatClient` 统一接口，抽象出 `Chat(model, messages, tools, options)` 能力，屏蔽不同服务商协议差异。 |
| `ollama.go` | 定义 `OllamaClient` 结构体、`OllamaTool` / `OllamaFunction` 数据模型，以及 `BuildOllamaTools` 工具格式转换函数。 |
| `ollama_chat.go` | Ollama 协议实现，负责构造 HTTP POST 请求发送至 `/api/chat`，并解析 NDJSON 流式响应，拼接完整内容与工具调用。 |
| `deepseek_chat.go` | DeepSeek 客户端（OpenAI 兼容协议），处理 `tool_calls.arguments` JSON 字符串互转、`reasoning_content` 透传、`tool_call_id` 回填等关键协议细节。 |

### 3. 🔧 `tools/` — 工具控制平面与执行运行时

| 文件 | 说明 |
|------|------|
| `tools.go` | 定义核心抽象：`Tool` 接口、`ToolContext`（工具执行上下文）、`ToolResult`（执行结果）、`Registry`（工具注册表）、`ToolCall` / `ToolCallFunction`（工具调用结构）。 |
| `runtime.go` | 实现工具执行运行时，包含 `PartitionToolCalls`（分批）、`ExecuteBatches`（调度执行）、`ExecuteToolCalls`（顶层编排）、`runConcurrently`（并发执行）、`runSerially`（串行执行）与 `QueuedContextModifiers`（上下文修改器队列）。 |
| `readfile.go` | 📖 `ReadFileTool` — 读取指定文件内容，标记为并发安全。 |
| `writefile.go` | 📄 `WriteFileTool` — 创建新文件并写入内容，含路径沙箱校验。 |
| `editfile.go` | ✏️ `EditFileTool` — 覆盖编辑已有文件内容。 |
| `runbash.go` | 💻 `RunBashTool` — 执行 Bash 命令，含危险命令拦截。 |
| `todo_manager.go` | 📝 `TodoManager` — 会话级计划管理器，实现 `Tool` 接口，支持计划更新、状态校验、渲染与提醒。 |

### 4. 🧬 `subagent/` — 子智能体

| 文件 | 说明 |
|------|------|
| `subagent.go` | 定义 `SubAgent` 结构体，提供两种执行入口：`Run`（空白上下文）与 `RunWithFork`（继承父消息上下文，自动裁剪触发本次调用的 assistant 消息以避免 DeepSeek 协议违规）。`BuildSubAgentRegistry` 构建子智能体的工具白名单（仅允许 `read_file` / `run_bash`，禁止递归派生与写操作）。 |
| `subtask_tool.go` | 定义 `TaskTool`（工具名 `sub_task`），实现 `Tool` 接口；支持 `prompt` 与 `fork` 两个参数，`fork=true` 时通过 `ParentMessagesProvider` 闭包获取最新父消息快照。 |

### 5. 📖 `skills/` — 技能（Skill）子系统

| 文件 | 说明 |
|------|------|
| `skill.go` | 定义 `SkillManifest`（轻量元信息）与 `SkillDocument`（含完整正文）两级抽象，配合"目录先行、正文按需"的分层加载策略。 |
| `registry.go` | `SkillRegistry` 扫描指定目录下所有 `SKILL.md`，支持 `DescribeAvailable`（生成 system prompt 注入内容）、`LoadFullText`（按名称加载完整正文）、`Names`、`Count` 等能力。 |
| `frontmatter.go` | 解析 SKILL.md 文件顶部的 YAML 风格 frontmatter（`---` 分隔），提取 `name` 与 `description` 元数据。 |
| `load_skill_tool.go` | `LoadSkillTool`（工具名 `load_skill`），实现 `Tool` 接口，按名称加载技能正文并以 `<skill name="...">...</skill>` 包裹返回。 |

### 6. 📊 `fsm/states.go` — 状态机控制

定义会话的核心状态结构：
- `State`：包含消息列表（`Messages`）、轮次计数（`TurnCount`）与状态转移原因（`TransitionReason`）。
- `Message`：表示单条消息，支持角色（`role`）、内容（`content`）、工具调用（`tool_calls`）、工具调用 ID（`tool_call_id`）与推理内容（`reasoning_content`）字段。

### 7. ⚙️ `utils/loadconfig.go` — 配置加载

基于 `spf13/viper` 实现 YAML 配置文件加载。定义 `Config` 结构体，涵盖 `api_key`、`subagent`、`skills`、`app` 等分组，通过 `InitConfig()` 完成初始化，`GetConfig()` 获取全局单例。

### 8. 🗂️ `config/` — 配置文件

- `config.yaml.example`：配置模板，包含 API Key 占位、子智能体默认 prompt 等。
- `config.yaml`：实际生效的配置（已加入 `.gitignore`，避免密钥提交）。

### 9. 🪵 `logger/logger.go` — 日志记录

基于 `go.uber.org/zap` 实现全局日志实例：
- `Init()`：初始化开发模式配置，输出彩色日志级别与精确时间戳。
- `Sync()`：程序退出前刷新日志缓冲区。

---

## ⚙️ 安装与配置

### 📋 环境要求

- Go 1.24.4 或更高版本
- 若使用 **Ollama** 后端：本地运行 Ollama 服务（默认地址：`http://127.0.0.1:11434`），并下载支持工具调用的模型（如 `modelscope.cn/Qwen/Qwen3-8B-GGUF:latest`）。
- 若使用 **DeepSeek** 后端：准备有效的 DeepSeek API Key。

### 📦 安装步骤

1. 克隆项目到本地：

```bash
git clone <repository-url>
cd cc-learn
```

2. 安装依赖：

```bash
go mod tidy
```

3. 准备配置文件（首次使用时）：

```bash
cp config/config.yaml.example config/config.yaml
```

编辑 `config/config.yaml`，填入你的 DeepSeek API Key 与其他参数；也可通过环境变量 `DEEPSEEK_API_KEY` 注入。

4. （可选）若使用 Ollama，确认服务已启动：

```bash
ollama serve
```

5. 运行项目：

```bash
# 默认使用 DeepSeek
go run main.go

# 或显式指定 Ollama
go run main.go -m ollama
```

---

## 🚀 使用方法

### 🎛️ 切换模型后端

通过 `-m` 命令行参数选择后端：

```bash
go run main.go -m deepseek   # 使用 DeepSeek（默认）
go run main.go -m ollama     # 使用本地 Ollama
```

### ✍️ 修改用户任务与系统提示

在 `main()` 函数中调整以下变量即可自定义行为：

```go
userPrompt := "你的自定义任务描述"
systemPrompt := "You are a helpful assistant. Use the todo tool to plan multi-step work. ..."
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

### ⚙️ 调整运行参数

编辑 `config/config.yaml`：

```yaml
app:
  maxTurns: 20              # 主循环最大轮次

subagent:
  defaultMaxTurns: 5        # 子智能体最大轮次
  defaultSystemPrompt: "..."
  forkSubtaskPromptPrefix: "..."

skills:
  dir: "./.skills"          # Skills 扫描目录
```

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

### 🔌 接入新的 LLM 后端

1. 在 `clients/` 下创建新文件（如 `claude_chat.go`）。
2. 定义客户端结构体，并实现 `ChatClient` 接口的 `Chat(model, messages, toolList, options)` 方法。
3. 在方法内完成：工具 schema 转换 → 消息协议转换 → HTTP 调用 → 响应归一化为 `*ChatResponse`。
4. 在 `main.go` 的 `switch modelType` 分支中新增对应 case。

### 📖 添加新 Skill

无需写代码，直接在 `.skills/` 下新建子目录并放入 `SKILL.md`（参考 `.skills/skill-function-test/SKILL.md`），重启后即可被 `SkillRegistry` 自动发现。

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
```

### 🐞 日志调试

通过 Zap 日志的级别控制（`Debug` / `Info` / `Warn` / `Error`）观察 Agent 运行时的消息流转、工具调用批次与执行结果。`.vscode/launch.json` 已提供多场景调试配置。

---

## 🚧 待实现功能

| 模块 | 说明 |
|------|------|
| **🔗 MCP 集成** | 利用 `ToolContext.McpClients` 预留字段，接入 Model Context Protocol（MCP）外部工具生态，实现跨进程工具调用。 |
| **💾 持久化存储** | 会话状态、消息历史与计划数据的持久化，支持会话恢复、跨进程共享与审计追踪。 |
| **🌐 更多 LLM 后端** | 在 `ChatClient` 抽象之上，新增 OpenAI 原生、Anthropic Claude、Qwen 等云端模型适配（`config.yaml` 中的 `api_key.openai` / `api_key.qwen` 字段已预留）。 |
| **🛡️ 细粒度权限沙箱** | 扩展 `ToolContext.PermissionContext`，实现基于工具名、参数模式、路径白名单的动态准入控制。 |
| **🧪 只读 Bash 沙箱** | 为子智能体提供受限版的 `run_bash`，仅允许纯查询类命令，进一步降低副作用风险。 |
| **🖥️ 交互式 CLI** | 当前用户输入为硬编码 prompt，后续支持 REPL 风格的多轮人机交互界面。 |
| **📈 执行指标与可观测性** | 工具调用次数、耗时、失败率等指标采集，辅助性能调优与问题定位。 |

---

## 📄 License

本项目基于 MIT License 开源，详见 [LICENSE](LICENSE) 文件。
