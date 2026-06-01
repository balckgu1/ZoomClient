# 🤖 ZoomClient

**[中文文档](README.md) | English**

ZoomClient is a Go-based AI Agent framework that implements an autonomous task execution system with Tool-Use capabilities.
The project adopts a modular architecture, covering multi-backend model clients, tool control plane, concurrent execution runtime, subagents, on-demand skill loading, session plan management, context compaction, hook system, permission system, interactive CLI, and other core subsystems.

---

## 📚 Table of Contents

- [✨ Core Features](#-core-features)
- [🏗️ Architecture Overview](#️-architecture-overview)
- [🧩 Component Reference](#-component-reference)
- [⚙️ Installation & Configuration](#️-installation--configuration)
- [🚀 Usage](#-usage)
- [🛠️ Developer Guide](#️-developer-guide)
- [🚧 Planned Features](#-planned-features)

---

## ✨ Core Features

- **🔁 Agent Loop**: The main loop driving model interactions, supporting system prompt injection, message history maintenance, and multi-turn tool calls. Maximum turns configured via `agentloop.maxTurns` in `config.yaml` (default 25).
- **🔌 Multi-Backend ChatClient Abstraction**: Unified `clients.ChatClient` interface with built-in **OpenAI-compatible** (DeepSeek / Kimi / Qwen / OpenAI), **Ollama** (NDJSON streaming protocol), **Anthropic** (Claude), and **Gemini** implementations, switchable via the `-m` CLI flag.
- **🧠 Thinking Mode Support**: Full passthrough of the `reasoning_content` field, preserving it as-is across multi-turn conversations to avoid `invalid_request_error` rejections from the server.
- **🔧 Tool Control Plane**: Unified tool registry `tools.Registry` supporting tool discovery, registration, and dynamic dispatch by name, with a built-in permission gate (`PermissionDecider`) for pre-execution admission control.
- **⚡ Concurrent-Safe Scheduling**: Tool calls are automatically batched by read/write attributes — read-only tools (e.g., `read_file`) execute concurrently, while write tools run serially, balancing efficiency and safety.
- **🧬 Subagent**: Delegates subtasks to independently-contextualized subagents via the `sub_task` tool; supports blank-context mode and `fork=true` mode that inherits parent message context, with an independent tool whitelist to prevent side-effect leakage.
- **📖 On-Demand Skill Loading**: Scans `SKILL.md` files (with YAML frontmatter) under the `.skills/` directory, injecting only the skill catalog into the system prompt; full content is loaded on-demand via the `load_skill` tool, reducing context costs.
- **📝 Session Plan Management (Todo Write)**: Allows the model to maintain multi-step task plans during a session with status tracking; automatically injects reminder messages when the plan hasn't been updated for several consecutive turns.
- **🗜️ Context Compaction**: Three-tier compaction strategy — large tool results persisted to disk and replaced with previews, old tool results micro-compacted to placeholders, and full-history summarization via model invocation when overall history is too long, effectively controlling context bloat.
- **🪝 Hook System**: Event-driven hook framework supporting five event points: `SessionStart`, `PreToolUse`, `PostToolUse`, `ToolError`, `SessionEnd`, with built-in handlers for dangerous command blocking, sensitive file protection, rate limiting, audit logging, and error recovery.
- **🛡️ Permission System**: Fine-grained permission control based on a rule engine, supporting `default` (ask user on miss), `plan` (read-only mode), and `auto` (read-only auto-approve) modes, with configurable allow/deny rules and regex matching.
- **🛡️ Path Sandbox Protection**: File tools must pass path sandbox validation (based on `ToolContext.WorkPath`) before execution, preventing path traversal and working directory escape.
- **🚫 Dangerous Command Blocking**: The `run_bash` tool includes built-in command blacklist detection to intercept high-risk operations.
- **🖥️ Interactive CLI**: A `lipgloss`-based terminal renderer supporting REPL-style multi-turn interaction with `/exit`, `/clear`, `/compact`, `/help` slash commands.
- **🪵 Structured Logging**: Powered by Uber Zap, outputting colorful, timestamped logs with support for development and production configurations.
- **🗂️ YAML-Driven Configuration**: Manages API keys, max turns, skill directories, subagent default prompts, permission rules, compaction thresholds, and more via `config/config.yaml`, avoiding hardcoded secrets.

---

## 🏗️ Architecture Overview

The project adopts a layered architecture, from top to bottom: Agent Loop Layer, Capability Extension Layer, Tool Management Layer, State Machine Control Layer, and Runtime Support Layer:

```
┌─────────────────────────────────────────────────────────┐
│                  🔁 Agent Loop Layer                    │
│               (main.go → agentLoop)                     │
├─────────────────────────────────────────────────────────┤
│               🧬 Capability Extension Layer             │
│ (subagent/ · skills/ · tools/todo_manager               │
│  · compact/ · hook/ · permission/)                      │
├─────────────────────────────────────────────────────────┤
│               🔧 Tool Management Layer                  │
│    (tools/tools.go · tools/runtime.go · concrete tools) │
├─────────────────────────────────────────────────────────┤
│               📊 State Machine Control Layer            │
│                   (fsm/states.go)                       │
├─────────────────────────────────────────────────────────┤
│               🧱 Runtime Support Layer                  │
│ (clients/ · logger/logger.go · utils/loadconfig.go      │
│  · ui/renderer.go)                                      │
└─────────────────────────────────────────────────────────┘
```

A request lifecycle proceeds as follows:

1. **🚀 Initialization**: Loads `config/config.yaml`, creates the corresponding `ChatClient` (OpenAI-compatible / Ollama / Anthropic / Gemini) based on the `-m` flag; builds the tool registry, skill registry, plan manager, context compaction manager, hook dispatcher, permission manager, subagent, and session state.
2. **💬 Model Interaction**: Sends the system prompt (including skill catalog), user input, and message history to the LLM API, requesting a model response with `reasoning_content` preserved as-is.
3. **🔧 Tool Invocation**: If the model returns tool call requests, the Agent Loop first triggers the `EventPreToolUse` hook, then calls `PartitionToolCalls` to batch by concurrency safety. Concurrent batches execute in parallel via goroutines; serial batches execute one by one.
4. **📥 Result Writeback**: All tool results are written back to message history in the original call order, maintaining deterministic understanding of results by the model. Each `tool` message carries a `ToolCallID` for OpenAI protocol compatibility. Large results are automatically persisted to disk and replaced with previews.
5. **🪝 Hook Post-Processing**: Triggers `EventPostToolUse` / `EventToolError` hooks for audit logging, error recovery injection, and other post-processing.
6. **📝 Plan Maintenance**: Checks whether the `todo` tool was used in the current turn; if not updated for multiple consecutive turns, a reminder is injected into the message history to prompt the model to refresh the plan.
7. **🗜️ Context Compaction**: Checks whether three-tier compaction is triggered (manual request or automatic threshold); if so, invokes the model to generate a continuity summary and replaces the message history.
8. **🔁 Loop Termination**: The loop terminates when the model no longer requests tool calls or when the `agentloop.maxTurns` limit is reached.

---

## 🧩 Component Reference

### 1. 🚪 `main.go` — Agent Main Loop Entry

Defines the `agentLoop` function, orchestrating a complete Agent session. Key responsibilities:
- Manages message history (`fsm.State.Messages`), automatically appending system prompts, user input, assistant responses (including `reasoning_content`), and tool results.
- Calls `tools.PartitionToolCalls` and `tools.ExecuteBatches` for tool scheduling and prints batching info.
- Collaborates with `TodoManager` to maintain plan updates and reminder mechanisms.
- Parses `-m` CLI flag to construct the appropriate OpenAI-compatible / Ollama / Anthropic / Gemini client.
- Assembles the skill catalog into the system prompt and registers tools (including `load_skill`, `sub_task`, `todo`, `compact`, etc.).
- Integrates the context compaction manager, hook dispatcher, and permission manager for complete lifecycle control.

### 2. 🔌 `clients/` — Multi-Backend Model Client Adapters

| File | Description |
|------|-------------|
| `client.go` | Defines the unified `ChatClient` interface, abstracting `Chat(model, messages, tools, options)` to shield provider protocol differences. |
| `openai_chat.go` | OpenAI-compatible client supporting DeepSeek, Kimi, Qwen, OpenAI, and all OpenAI-compatible backends. Handles `tool_calls.arguments` JSON string conversion, `reasoning_content` passthrough, and `tool_call_id` backfill. |
| `ollama.go` | Defines `OllamaClient` struct, `OllamaTool` / `OllamaFunction` data models, and `BuildOllamaTools` format conversion. |
| `ollama_chat.go` | Ollama protocol implementation — constructs HTTP POST to `/api/chat` and parses NDJSON streaming responses, assembling complete content and tool calls. |
| `anthropic.go` | Anthropic Claude client using the official `anthropic-sdk-go`, handling system message extraction to top-level parameters and ToolUseBlock / ToolResultBlock conversion. |
| `gemini.go` | Google Gemini client using the official `google.golang.org/genai` SDK, handling FunctionCall / FunctionResponse conversion and system instruction injection. |

### 3. 🔧 `tools/` — Tool Control Plane & Execution Runtime

| File | Description |
|------|-------------|
| `tools.go` | Core abstractions: `Tool` interface, `ToolContext` (execution context with `Ctx` / `Logger` / `SessionID` / `AppState`), `ToolResult`, `Registry` (with permission gate injection), `ToolCall` / `ToolCallFunction`. |
| `runtime.go` | Execution runtime: `PartitionToolCalls` (batching), `ExecuteBatches` (scheduling), `ExecuteToolCalls` (top-level orchestration), `runConcurrently`, `runSerially`, and `QueuedContextModifiers`. |
| `readfile.go` | 📖 `ReadFileTool` — Reads file contents, marked as concurrency-safe. |
| `writefile.go` | 📄 `WriteFileTool` — Creates new files with content, includes path sandbox validation and auto parent directory creation. |
| `editfile.go` | ✏️ `EditFileTool` — Overwrites existing file contents. |
| `runbash.go` | 💻 `RunBashTool` — Executes bash commands with dangerous command blocking and cross-platform compatibility (Windows / Linux / macOS). |
| `todo_manager.go` | 📝 `TodoManager` — Session-level plan manager implementing the `Tool` interface, supporting plan updates, status validation (at most one `in_progress`), rendering, and reminders. |

### 4. 🧬 `subagent/` — Subagent

| File | Description |
|------|-------------|
| `subagent.go` | Defines `SubAgent` struct with two entry points: `Run` (blank context) and `RunWithFork` (inherits parent message context, auto-trims the triggering assistant message to avoid protocol violations). `BuildSubAgentRegistry` builds a tool whitelist (only `read_file` / `run_bash`, no recursive dispatch or writes). |
| `subtask_tool.go` | Defines `TaskTool` (tool name `sub_task`) implementing the `Tool` interface; supports `prompt` and `fork` parameters, using a `ParentMessagesProvider` closure for latest parent message snapshots when `fork=true`. |

### 5. 🗜️ `compact/` — Context Compaction

| File | Description |
|------|-------------|
| `compact.go` | `CompactManager` — Three-tier compaction: Tier 1 persists large tool outputs (`PersistLargeOutput`), Tier 2 micro-compacts old results (`MicroCompact`), Tier 3 summarizes full history (`CompactHistory` / `summarize`). Supports manual (`/compact` command or `compact` tool) and automatic threshold triggers. |
| `compact_tool.go` | `CompactTool` — A compaction tool for the model to invoke, marking pending and letting agentLoop execute full compaction at an appropriate time. |

### 6. 🪝 `hook/` — Hook System

| File | Description |
|------|-------------|
| `hook.go` | Defines `Runner` (event dispatcher), `Handler` (handler function signature), and `HookResult` (with `ExitCode`: `Continue` / `Block` / `Inject` / `Retry`). Multiple handlers per event, executed in registration order, short-circuiting on the first non-zero exit code. |
| `handlers.go` | Built-in handlers: `OnSessionStart`, `PreToolBlockDangerous`, `PreToolRateLimit`, `PreToolSensitiveFileGuard`, `PostToolAuditLog`, `OnToolErrorRecovery`, `OnSessionEnd`. |

### 7. 🛡️ `permission/` — Permission System

| File | Description |
|------|-------------|
| `permission.go` | `Manager` — Supports `default` / `plan` / `auto` modes with `DenyRules` / `AllowRules` for tool name, path, and content matching (regex supported), plus bash dangerous command fallback checks. |
| `asker.go` | `Asker` interface with `StdinAsker` (interactive prompt) and `DenyAsker` (default deny in non-interactive scenarios). |
| `bash.go` | `isDangerousBash` — Detects dangerous command patterns (e.g., `rm -rf /`, `mkfs`, `dd if=`). |

### 8. 📖 `skills/` — Skill Subsystem

| File | Description |
|------|-------------|
| `skill.go` | Defines `SkillManifest` (lightweight metadata) and `SkillDocument` (full content) two-level abstraction with a "catalog-first, content-on-demand" loading strategy. |
| `registry.go` | `SkillRegistry` scans all `SKILL.md` files in a directory, supporting `DescribeAvailable` (system prompt injection), `LoadFullText` (on-demand full content loading), `Names`, `Count`, etc. |
| `frontmatter.go` | Parses YAML-style frontmatter (`---` delimited) from SKILL.md files, extracting `name` and `description` metadata. |
| `load_skill_tool.go` | `LoadSkillTool` (tool name `load_skill`) implementing the `Tool` interface, loading skill content by name and wrapping it in `<skill name="...">...</skill>` tags. |

### 9. 📊 `fsm/states.go` — State Machine Control

Defines the core session state structure:
- `State`: Contains message list (`Messages`), turn count (`TurnCount`), and transition reason (`TransitionReason`).
- `Message`: Represents a single message with `role`, `content`, `tool_calls`, `tool_call_id`, and `reasoning_content` fields.

### 10. ⚙️ `utils/loadconfig.go` — Configuration Loading

Built on `spf13/viper` for YAML config file loading. Defines the `Config` struct covering `api_key`, `openai`, `subagent`, `skills`, `agentloop`, `compact`, `permission`, `tools` sections, initialized via `InitConfig()` with a global singleton via `GetConfig()`.

### 11. 🗂️ `config/` — Configuration Files

- `config.yaml.example`: Configuration template with API key placeholders for OpenAI-compatible, Anthropic, and Gemini backends, plus full examples for subagent, compaction, permission, and agent loop settings.
- `config.yaml`: Active configuration (added to `.gitignore` to prevent secret commits).

### 12. 🪵 `logger/logger.go` — Logging

Built on `go.uber.org/zap` for a global logger instance:
- `Init()`: Initializes development-mode config with colorful log levels and precise timestamps.
- `Sync()`: Flushes log buffers before program exit.

### 13. 🖥️ `ui/` — Terminal Renderer

| File | Description |
|------|-------------|
| `renderer.go` | `Renderer` — A `lipgloss`-based CLI frontend renderer providing full rendering methods for session banners, user prompts, assistant text, reasoning content, tool calls and results, subagents, plan panels, compaction info, hook blocks, errors, and info messages. |
| `styles.go` | Defines all `lipgloss.Style` constants (colors, borders, alignment, etc.). |

---

## ⚙️ Installation & Configuration

### 📋 Prerequisites

- Go 1.24.4 or higher
- For **Ollama** backend: Ollama running locally (default: `http://127.0.0.1:11434`) with a tool-calling-capable model (e.g., `modelscope.cn/Qwen/Qwen3-8B-GGUF:latest`).
- For **OpenAI-compatible** backend (DeepSeek / Kimi / Qwen / OpenAI): A valid API key.
- For **Anthropic** backend: A valid Anthropic API key.
- For **Gemini** backend: A valid Gemini API key.

### 📦 Installation

1. Clone the repository:

```bash
git clone <repository-url>
cd cc-learn
```

2. Install dependencies:

```bash
go mod tidy
```

3. Prepare the configuration file (first time):

```bash
cp config/config.yaml.example config/config.yaml
```

Edit `config/config.yaml` with your API keys and other parameters. You can also inject via environment variables (e.g., `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`).

4. (Optional) If using Ollama, ensure the service is running:

```bash
ollama serve
```

5. Run the project:

```bash
# Default: OpenAI-compatible backend (configured in config.yaml)
go run main.go

# Explicitly specify backend
go run main.go -m openai    # OpenAI-compatible
go run main.go -m ollama    # Local Ollama
go run main.go -m anthropic # Anthropic Claude
go run main.go -m gemini    # Google Gemini
```

---

## 🚀 Usage

### 🎛️ Switching Model Backends

Use the `-m` CLI flag to select a backend:

```bash
go run main.go -m openai     # OpenAI-compatible (default)
go run main.go -m ollama     # Local Ollama
go run main.go -m anthropic  # Anthropic Claude
go run main.go -m gemini     # Google Gemini
```

### ✍️ Customizing Tasks & System Prompt

Modify the following variables in the `main()` function:

```go
// Edit the systemPrompt variable in main()
systemPrompt := fmt.Sprintf(
    "You are a helpful assistant running on %s. "+
        "Use the todo tool to plan multi-step work. "+
        "Keep exactly one step in_progress when a task has multiple steps. "+
        "Refresh the plan as work advances. Prefer tools over prose.",
    runtime.GOOS,
)
```

### 📁 Adjusting the Working Directory

Set the file sandbox root via `ToolContext.WorkPath`:

```go
toolCtx := &tools.ToolContext{
    WorkPath: "./",
}
```

### 📖 Using Skills

Place your skill files in the directory specified by `skills.dir` in `config.yaml` (default `./.skills/`). Each skill gets its own subdirectory with a `SKILL.md` file:

```markdown
---
name: your-skill-name
description: One-line description of the skill's purpose
---

# Content

Detailed steps or playbook...
```

After startup, the model sees the skill catalog in the system prompt and loads full content on-demand via the `load_skill` tool.

### 🧬 Using Subagents

The model can dispatch subtasks via the `sub_task` tool:

- Default `fork=false`: Subagent runs with blank context; the prompt must be self-contained.
- Set `fork=true`: Subagent inherits parent message history, ideal for "further analysis based on current conversation" scenarios.

### ⚙️ Tuning Runtime Parameters

Edit `config/config.yaml`:

```yaml
agentloop:
  maxTurns: 25              # Max turns in main loop
  todoRoundsThreshold: 9    # Plan reminder threshold
  maxTools: 5               # Max tool calls per turn
  sensitiveFiles: [".env", "id_rsa"]  # Sensitive file list

subagent:
  defaultMaxTurns: 10       # Subagent max turns
  defaultSystemPrompt: "..."
  forkSubtaskPromptPrefix: "..."

skills:
  dir: "./.skills"          # Skills scan directory

compact:
  persistThreshold: 4000    # Large result persist threshold (bytes)
  previewBytes: 1000        # Preview bytes retained on disk
  keepRecentToolResults: 4  # Micro-compact recent results to keep
  contextLimit: 60000       # Full compaction trigger threshold (bytes)
  persistDir: ".task_outputs/tool-results"  # Persist directory

permission:
  mode: "auto"              # default | plan | auto
  interactive: true         # Ask user on rule miss
  denyRules: [...]          # Deny rules
  allowRules: [...]         # Allow rules
```

---

## 🛠️ Developer Guide

### ➕ Adding a New Tool

1. Create a new file in `tools/` (e.g., `mynewtool.go`).
2. Define a struct and implement the `Tool` interface's four methods:
   - `Name() string` — Unique tool identifier.
   - `Description() string` — Description for the model to decide when to call.
   - `Parameters() map[string]interface{}` — JSON Schema parameter definition.
   - `Call(args map[string]interface{}, ctx *ToolContext) ToolResult` — Execution logic.
3. If the tool is read-only with no side effects, register it in `tools/runtime.go`'s `concurrencySafeTools` map for concurrent execution.
4. Register the tool in `main.go` via `registry.Register(YourTool{})`.
5. To allow subagents to use the tool, also register it in `subagent.BuildSubAgentRegistry()`.
6. To let the permission system recognize the tool's read/write attributes, register it in `permission/permission.go`'s `readOnlyTools` or `writeTools` map.

### 🔌 Adding a New LLM Backend

1. Create a new file in `clients/` (e.g., `claude_chat.go`).
2. Define a client struct and implement the `ChatClient` interface's `Chat(model, messages, toolList, options)` method.
3. Inside the method: convert tool schemas → convert message protocols → make HTTP call → normalize response to `*ChatResponse`.
4. Add a corresponding case in `main.go`'s `switch modelType` branch, handling API key retrieval (with environment variable fallback) and client initialization.

### 📖 Adding a New Skill

No code required — just create a new subdirectory under `.skills/` with a `SKILL.md` file (see `.skills/skill-function-test/SKILL.md` for reference). It will be auto-discovered by `SkillRegistry` on restart.

### 🧪 Unit Testing

Each core module includes `*_test.go` test files. Run all tests:

```bash
go test ./...
```

Run tests for a specific package:

```bash
go test ./tools/...
go test ./subagent/...
go test ./skills/...
```

### 🐞 Debugging with Logs

Use Zap log levels (`Debug` / `Info` / `Warn` / `Error`) to observe message flow, tool call batching, and execution results at runtime. `.vscode/launch.json` provides multi-scenario debug configurations.

---

## 🚧 Planned Features

| Module | Description |
|--------|-------------|
| **🔗 MCP Integration** | Leverage the reserved `ToolContext.McpClients` field to integrate Model Context Protocol (MCP) external tool ecosystem for cross-process tool calls. |
| **💾 Persistent Storage** | Persist session state, message history, and plan data for session recovery, cross-process sharing, and audit trails. |
| **🧪 Read-Only Bash Sandbox** | Provide subagents with a restricted `run_bash` that only allows pure query commands, further reducing side-effect risks. |
| **📈 Execution Metrics & Observability** | Collect metrics on tool call counts, latency, and failure rates for performance tuning and troubleshooting. |
| **🧠 Memory System Enhancement** | Extend the `memory/` module for cross-session long-term memory storage and retrieval. |

---

## 📄 License

This project is open-source under the MIT License. See [LICENSE](LICENSE) for details.
