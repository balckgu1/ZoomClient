# ZoomClient

ZoomClient 是一个基于 Go 语言实现的 Agent 框架，通过与 LLM 服务交互，实现具备Tool-Use能力的自主任务执行系统。
项目采用模块化架构，涵盖状态机管理、工具控制平面、并发执行运行时、会话计划管理等核心子系统。

---

## 目录

- [核心功能特性](#核心功能特性)
- [架构设计说明](#架构设计说明)
- [主要组件介绍](#主要组件介绍)
- [安装与配置](#安装与配置)
- [使用方法](#使用方法)
- [开发指南](#开发指南)
- [待实现功能](#待实现功能)

---

## 核心功能特性

- **Agent Loop**：驱动模型交互的循环，支持系统提示注入、消息历史维护与多轮次工具调用，最多支持 10 轮次。
- **工具控制平面（Tool Control Plane）**：统一的工具注册表管理，支持工具发现、注册与按名称动态调度执行。
- **并发安全调度**：工具调用按读写属性自动分批，只读工具（如 `read_file`）可并发执行，写操作类工具串行执行，兼顾效率与安全性。
- **NDJSON 响应兼容**：Ollama 客户端支持解析多行 NDJSON 格式的流式响应，自动拼接片段内容与工具调用信息。
- **会话计划管理（Todo Write）**：允许模型在会话中维护多步骤任务计划，支持状态跟踪与计划外显提醒机制。
- **路径沙箱防护**：文件类工具执行前需通过路径沙箱校验，禁止路径穿越与工作目录逃逸。
- **危险命令拦截**：`run_bash` 工具内置命令白名单检测，拦截高危操作指令。
- **结构化日志**：基于 Uber Zap 日志库，输出彩色、带时间戳的日志，支持开发与生产多环境配置。

---

## 架构设计说明

项目整体采用分层架构，自上而下分为Agent Loop层、工具管理层、状态机控制层与运行时支撑层：

```
┌─────────────────────────────────────────────┐
│              Agent Loop层                  │
│         (main.go → agentLoop)               │
├─────────────────────────────────────────────┤
│              工具管理层                       │
│   (tools.go / runtime.go / todo_manager.go) │
├─────────────────────────────────────────────┤
│              状态机控制层                     │
│         (fsm/states.go)                      │
├─────────────────────────────────────────────┤
│              运行时支撑层                     │
│   (clients/ollama.go / logger/logger.go)    │
└─────────────────────────────────────────────┘
```

一次请求生命周期如下：

1. **初始化阶段**：创建 Ollama 客户端、工具注册表、计划管理器与会话状态。
2. **模型交互阶段**：将系统提示、用户输入与消息历史发送至 API，请求模型生成响应。
3. **工具调用阶段**：若模型返回工具调用请求，Agent Loop 将调用按并发安全性分批，并发批次使用 goroutine 并行执行，串行批次逐个执行。
4. **结果回写阶段**：所有工具执行结果按原始调用顺序写回消息历史，保证模型对执行结果的理解是确定性的。
5. **计划维护阶段**：检查本轮是否使用了 `todo` 工具，若连续多轮未更新plan，则向消息历史中注入提醒，促使模型更新计划。
6. **循环终止条件**：当模型不再请求工具调用，或达到最大轮次限制时，循环终止。

---

## 主要组件介绍

### 1. `main.go` — Agent 主循环入口

定义 `agentLoop` 函数，负责编排一次完整的 Agent 会话。核心职责包括：
- 管理消息历史（`fsm.State.Messages`），自动追加系统提示、用户输入、助手响应与工具结果。
- 调用 `tools.PartitionToolCalls` 与 `tools.ExecuteBatches` 完成工具调度。
- 与 `TodoManager` 协作，维护计划更新与提醒机制。

### 2. `clients/` — 客户端适配层

| 文件 | 说明 |
|------|------|
| `ollama.go` | 定义 `OllamaClient` 结构体、`OllamaTool` / `OllamaFunction` 数据模型，以及 `BuildOllamaTools` 工具格式转换函数。 |
| `chat.go` | 实现 `Chat` 方法，构造 HTTP POST 请求发送至 `/api/chat` 接口，并解析 NDJSON 流式响应，拼接完整内容与工具调用。 |

### 3. `tools/` — 工具控制平面与执行运行时

| 文件 | 说明 |
|------|------|
| `tools.go` | 定义核心抽象：`Tool` 接口、`ToolContext`（工具执行上下文）、`ToolResult`（执行结果）、`Registry`（工具注册表）。 |
| `runtime.go` | 实现工具执行运行时，包含 `PartitionToolCalls`（分批）、`ExecuteBatches`（调度执行）、`runConcurrently`（并发执行）、`runSerially`（串行执行）与上下文修改器队列。 |
| `readfile.go` | `ReadFileTool` — 读取指定文件内容，标记为并发安全。 |
| `writefile.go` | `WriteFileTool` — 创建新文件并写入内容，含路径沙箱校验。 |
| `editfile.go` | `EditFileTool` — 覆盖编辑已有文件内容。 |
| `runbash.go` | `RunBashTool` — 执行 Bash 命令，含危险命令拦截。 |
| `todo_manager.go` | `TodoManager` — 会话级计划管理器，实现 `Tool` 接口，支持计划更新、状态校验、渲染与提醒。 |

### 4. `fsm/states.go` — 状态机控制

定义会话的核心状态结构：
- `State`：包含消息列表（`Messages`）、轮次计数（`TurnCount`）与状态转移原因（`TransitionReason`）。
- `Message`：表示单条消息，支持角色（`role`）、内容（`content`）与工具调用（`tool_calls`）字段。

### 5. `logger/logger.go` — 日志记录

基于 `go.uber.org/zap` 实现全局日志实例：
- `Init()`：初始化开发模式配置，输出彩色日志级别与精确时间戳。
- `Sync()`：程序退出前刷新日志缓冲区。

---

## 安装与配置

### 环境要求

- Go 1.24.4 或更高版本
- 本地运行 Ollama 服务（默认地址：`http://127.0.0.1:11434`）
- 已下载支持工具调用的模型（如 `modelscope.cn/Qwen/Qwen3-8B-GGUF:latest`）

### 安装步骤

1. 克隆项目到本地：

```bash
git clone <repository-url>
cd cc-learn
```

2. 安装依赖：

```bash
go mod tidy
```

3. 确认 Ollama 服务已启动：

```bash
ollama serve
```

4. 运行项目：

```bash
go run main.go
```

---

## 使用方法

项目入口为 `main.go`，当前默认示例演示了让 Agent 执行以下任务：

1. 创建 `hello.py`，写入 `print("Hello, World!")`。
2. 读取 `hello.py` 的内容。
3. 修改 `hello.py`，覆盖为 `print("Hello, China!")`。

### 修改模型与提示词

在 `main()` 函数中调整以下变量即可自定义行为：

```go
modelname := "modelscope.cn/Qwen/Qwen3-8B-GGUF:latest"
userPrompt := "你的自定义任务描述"
systemPrompt := "You are a helpful AI assistant. Use the todo tool to plan multi-step work."
```

### 调整工作目录

通过 `ToolContext.WorkPath` 设置 Agent 允许操作的文件沙箱根目录：

```go
toolCtx := &tools.ToolContext{
    WorkPath: "./workspace",
}
```

---

## 开发指南

### 添加新工具

1. 在 `tools/` 目录下创建新文件（如 `mynewtool.go`）。
2. 定义结构体并实现 `Tool` 接口的四个方法：
   - `Name() string` — 工具唯一标识名。
   - `Description() string` — 模型判断何时调用的描述。
   - `Parameters() map[string]interface{}` — JSON Schema 格式的参数定义。
   - `Call(args map[string]interface{}, ctx *ToolContext) ToolResult` — 执行逻辑。
3. 若工具为只读且无副作用，在 `runtime.go` 的 `concurrencySafeTools` 映射中注册，使其可并发执行。
4. 在 `main.go` 中通过 `registry.Register()` 注册该工具。

### 单元测试

项目各核心模块均配有 `*_test.go` 测试文件。运行全部测试：

```bash
go test ./...
```

### 日志调试

通过 Zap 日志的级别控制（`Debug` / `Info` / `Warn` / `Error`）观察 Agent 运行时的消息流转、工具调用批次与执行结果。

---

## 待实现功能

| 模块 | 说明 |
|------|------|
| **s04: Subagents** | 子智能体（Subagent）支持，允许 Agent 调用其他专用 Agent 完成子任务，实现任务拆分与委托机制。 |
| **Chat 客户端扩展** | 除 Ollama 外，增加对 OpenAI API、Anthropic Claude 等云端模型的适配支持。 |
| **MCP 集成** | 利用 `ToolContext.McpClients` 预留字段，接入 Model Context Protocol（MCP）外部工具生态。 |
| **持久化存储** | 会话状态、消息历史与计划数据的持久化，支持会话恢复与审计追踪。 |
| **配置化启动** | 通过配置文件或环境变量动态指定模型地址、模型名称、轮次限制、工作目录等参数，替代当前硬编码方式。 |

---

## License

本项目基于 MIT License 开源，详见 [LICENSE](LICENSE) 文件。
