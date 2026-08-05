# 🤖 ZoomClient

**[中文文档](README_ZH.md) | English**

ZoomClient is a Go-based AI Agent framework that implements an autonomous task execution system with Tool-Use capabilities.
The project adopts a modular architecture, covering multi-backend model clients, tool control plane, concurrent execution runtime, subagents, on-demand skill loading, session management, context compaction, prompt pipeline, hook system, permission system, cross-session memory, event emitter, TUI/Web frontends, and other core subsystems.

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
- **🧠 Thinking Mode Support**: Full passthrough of the `reasoning_content` field, preserving it as-is across multi-turn conversations to avoid `invalid_request_error` rejections from the server.
- **🧠 Model Registry & Presets**: Centralized `model.Registry` for managing model presets (client type, base URL, model name). Supports runtime hot-switching between backends without losing conversation history via the `SwitchModel` API.
- **🗄️ Session Management**: Persistent session CRUD via `session.Manager` with JSON-file storage, automatic session naming, metadata indexing, and lifecycle tracking. Enables session recovery, cross-process sharing, and historical session browsing.
- **🧩 FSM (Finite State Machine)**: Encapsulates agent session state including message history, turn count, and transition reasons. Provides a clean state abstraction decoupled from business logic.
- **📦 Prompt Pipeline**: Multi-stage prompt construction system (`prompt.MessagePipeline`) that sequentially injects skill catalogs, memory summaries, dynamic instructions, plan reminders, and file attachments into the system prompt, then normalizes the result for the target model protocol.
- **💭 Cross-Session Memory (`memory/`)**: Long-term memory system supporting four types (user, feedback, project, reference) with `save_memory`, `search_memory`, `delete_memory`, and `update_memory` tools. Memories are stored as Markdown files with YAML frontmatter and automatically injected into the system prompt.
- **🔄 Event Emitter (`emitter/`)**: Unified output abstraction for all agent-visible events (session lifecycle, assistant output, tool calls, tool results, subagent activity, hooks, errors, plan updates, compaction). Supports three implementations: CLI TUI renderer, API NDJSON emitter, and Web SSE emitter.
- **🌐 Web UI (`web/`)**: Built-in web server with SSE real-time streaming, RESTful session APIs, HTML frontend (embedded via `go:embed`), and web-based permission asker. Usable as an alternative to the CLI TUI.
- **🖥️ Interactive CLI (TUI)**: A `bubbletea`/`lipgloss`-based terminal renderer supporting REPL-style multi-turn interaction with `/exit`, `/clear`, `/compact`, `/help` slash commands, plus markdown rendering and status bar.
- **📂 Work Directory Management**: Supports specifying and switching the `WorkPath` at runtime, controlling the file sandbox root for file-based tools.
- **🪵 Structured Logging**: Powered by Uber Zap, outputting colorful, timestamped logs with support for development and production configurations.
- **🗂️ YAML-Driven Configuration**: Manages API keys, max turns, skill directories, subagent default prompts, permission rules, compaction thresholds, and more via `config/config.yaml`, avoiding hardcoded secrets.

---

## 🏗️ Architecture Overview

The project adopts a layered architecture, from top to bottom: Agent Loop Layer, Capability Extension Layer, Tool Management Layer, State Machine Control Layer, and Runtime Support Layer:

```
┌──────────────────────────────────────────────────────────────────┐
│                      🔁 Agent Loop Layer                        │
│                    (main.go → agentLoop)                        │
├──────────────────────────────────────────────────────────────────┤
│                   🧬 Capability Extension Layer                  │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │subagent/ │  │ skills/  │  │  hooks/  │  │ session/ │       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │ memory/  │  │ compact/ │  │ prompt/  │  │ model/   │       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
├──────────────────────────────────────────────────────────────────┤
│                    🔧 Tool Management Layer                      │
│   (tools/registry · tools/runtime · concrete tool impls)        │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │ readfile │  │ writefile│  │ run_bash │  │glob_search│       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐                      │
│   │edit_file │  │list_dir  │  │todo_mgr  │                      │
│   └──────────┘  └──────────┘  └──────────┘                      │
├──────────────────────────────────────────────────────────────────┤
│                  ⚙️ State Machine Control Layer                  │
│            (fsm/ · permission/ · emitter/)                      │
│         ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│         │  FSM     │  │Permission│  │ Emitter  │               │
│         └──────────┘  └──────────┘  └──────────┘               │
├──────────────────────────────────────────────────────────────────┤
│                    🖥️ Presentation Layer                         │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│   │  CLI (TUI)   │  │  API/NDJSON  │  │  Web UI      │         │
│   └──────────────┘  └──────────────┘  └──────────────┘         │
├──────────────────────────────────────────────────────────────────┤
│                 🛠️ Runtime Support Layer                         │
│    (utils/config · logger/ · web/ · workdir/)                   │
└──────────────────────────────────────────────────────────────────┘
```

### Layer Breakdown

| Layer | Modules | Responsibility |
|-------|---------|----------------|
| **Agent Loop Layer** | `main.go` | Orchestrates the model-interaction loop, manages turn lifecycle |
| **Capability Extension Layer** | `subagent/`, `skills/`, `hooks/`, `session/`, `memory/`, `compact/`, `prompt/`, `model/` | Pluggable extensions that add reasoning, persistence, and intelligence capabilities |
| **Tool Management Layer** | `tools/` | Tool registry, read/write scheduling, concrete tool implementations |
| **State Machine Control Layer** | `fsm/`, `permission/`, `emitter/` | State persistence, admission control, event output abstraction |
| **Presentation Layer** | `ui/` (TUI), `emitter/api_emitter` (API), `web/` (Web) | Multiple frontends for different deployment scenarios |
| **Runtime Support Layer** | `utils/`, `logger/`, `web/`, `workdir/` | Config loading, structured logging, HTTP server, file sandbox |

---

## 🧩 Component Reference

### Core Modules

| Module | Package | Description |
|--------|---------|-------------|
| **Agent Loop** | `main.go:agentLoop` | Main interaction loop; calls LLM, dispatches tool calls, manages turns |
| **ChatClient** | `clients/` | Unified `ChatClient` interface for OpenAI, Ollama, Anthropic, Gemini |
| **Model Registry** | `model/` | Model preset definition, selection, and runtime hot-switching |
| **Tool Registry** | `tools/` | Tool registration, discovery, dispatch, concurrency scheduling |
| **Subagent** | `subagent/` | Independent sub-agent execution with isolated context and tools |
| **Skills** | `skills/` | On-demand skill loading from SKILL.md files with YAML frontmatter |
| **Session Management** | `session/` | Session CRUD, JSON-file persistence, auto-naming, history browsing |
| **FSM** | `fsm/` | Session state encapsulation (messages, turn count, transitions) |
| **Context Compaction** | `compact/` | Three-tier context compression (persist, micro-compact, summarize) |
| **Hook System** | `hook/` | Event-driven plugin framework for pre/post tool execution |
| **Permission System** | `permission/` | Rule-based access control, path sandbox, bash blacklist |
| **Prompt Pipeline** | `prompt/` | Multi-stage prompt construction (skills + memory + reminders + attachments) |
| **Memory System** | `memory/` | Cross-session long-term memory (save/search/delete/update) |
| **Event Emitter** | `emitter/` | Unified output interface for CLI, API, and Web frontends |
| **Interactive CLI** | `ui/` | Bubble Tea TUI with markdown rendering, slash commands, status bar |
| **Web Server** | `web/` | HTTP server with SSE streaming, RESTful APIs, embed frontend |
| **Structured Logger** | `logger/` | Zap-based colorful, timestamped logging |
| **Config Loader** | `utils/` | YAML config loading with defaults and validation |

### Concrete Tools

| Tool | Package | Description |
|------|---------|-------------|
| **read_file** | `tools/` | Read file content from disk |
| **write_file** | `tools/` | Write content to a file |
| **edit_file** | `tools/` | Apply search/replace edits to files |
| **run_bash** | `tools/` | Execute shell commands with blacklist protection |
| **glob_search** | `tools/` | Pattern-based file search |
| **list_directory** | `tools/` | List directory contents |
| **todo_manager** | `tools/` | Multi-step task plan management (create/update/mark) |
| **sub_task** | `subagent/` | Delegate work to a subagent |
| **load_skill** | `skills/` | Load full skill content on-demand |
| **save_memory** | `memory/` | Persist a new memory entry |
| **search_memory** | `memory/` | Search across stored memories |
| **delete_memory** | `memory/` | Remove a memory entry |
| **update_memory** | `memory/` | Modify an existing memory entry |
| **compact_history** | `compact/` | Trigger context compaction manually |

---

## ⚙️ Installation & Configuration

### Prerequisites

- Go 1.21+
- (Optional for Ollama) Local Ollama server
- API keys (set via environment variables or `config/config.yaml`)

### Installation

```bash
git clone https://github.com/balckgu1/ZoomClient.git
cd ZoomClient
go build -o zoomclient .
```

### Configuration

Edit `config/config.yaml` to set API keys and runtime parameters:

```yaml
apikeys:
  openai: "sk-xxx"           # OpenAI / compatible
  anthropic: "sk-ant-xxx"    # Anthropic Claude
  gemini: "..."
  # Keys fall back to environment variables when empty

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

## 🚀 Usage

### 🎛️ Switching Model Backends

Use the `-m` CLI flag to select a backend:

```bash
go run main.go -m openai     # OpenAI-compatible (default)
go run main.go -m ollama     # Local Ollama
go run main.go -m anthropic  # Anthropic Claude
go run main.go -m gemini     # Google Gemini
```

### 🌐 Running as Web Server

Start the web UI server instead of the CLI TUI:

```bash
go run main.go -m openai -web
```

This launches an HTTP server (default port defined in `web/server.go`) serving a full HTML frontend with SSE real-time streaming, session browsing, and model switching.

### 🗄️ Session Management

Sessions are automatically persisted to disk as JSON files. Use the API or the session manager to list, load, or create sessions. The auto-naming feature generates descriptive titles for each session based on conversation content.

### 🧠 Runtime Model Switching

When using the web UI, models can be hot-switched at runtime without losing conversation history via the `SwitchModel` API.

### ✍️ Customizing User Task & System Prompt

Adjust the following variables in `main()` to customize behavior:

```go
systemPrompt := fmt.Sprintf(
    "You are a helpful assistant running on %s. "+
        "Use the todo tool to plan multi-step work. "+
        "Keep exactly one step in_progress when a task has multiple steps. "+
        "Refresh the plan as work advances. Prefer tools over prose.",
    runtime.GOOS,
)
```

### 📁 Adjusting Work Directory

Set `ToolContext.WorkPath` to control the file sandbox root:

```go
toolCtx := &tools.ToolContext{
    WorkPath: "./",
}
```

### 📖 Using Skills

Place skill files under the directory specified by `skills.dir` in `config.yaml` (default `./.skills/`). Each skill is a subdirectory containing a `SKILL.md` file:

```markdown
---
name: your-skill-name
description: One-line description of the skill's purpose
---

# Content

Detailed steps or playbook...
```

On startup, the model sees the skill catalog in the system prompt and loads full content on-demand via the `load_skill` tool.

### 🧬 Using Subagents

The model can dispatch subtasks via the `sub_task` tool:

- Default `fork=false`: Subagent runs with blank context; the prompt must be self-contained.
- Set `fork=true`: Subagent inherits parent message history, ideal for "further analysis based on current conversation" scenarios.

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
4. Add a corresponding case in `main.go`'s `switch modelType` branch, handling API key retrieval (with env var fallback) and client initialization.

### 📖 Adding a New Skill

No code required — just create a new subdirectory under `.skills/` with a `SKILL.md` file (see `.skills/skill-function-test/SKILL.md` for reference). It will be auto-discovered by `SkillRegistry` on restart.

### 🧠 Adding a Model Preset

Add a new entry in `model/registry.go`'s presets map with client type, base URL, and model name. The preset will be selectable via the `-m` flag and available for runtime hot-switching.

### 🔄 Adding a New Emitter Implementation

Implement the `emitter.Emitter` interface and register it in `main.go`. Current implementations: `ui.Renderer` (CLI TUI), `emitter.ApiEmitter` (NDJSON stdout), `web.SSEEmitter` (Web SSE events).

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
go test ./session/...
go test ./memory/...
go test ./prompt/...
go test ./model/...
go test ./compact/...
go test ./hook/...
go test ./permission/...
```

### 🐞 Debugging with Logs

Use Zap log levels (`Debug` / `Info` / `Warn` / `Error`) to observe message flow, tool call batching, and execution results at runtime. `.vscode/launch.json` provides multi-scenario debug configurations.

---

## 🚧 Planned Features

| Module | Description |
|--------|-------------|
| **🔗 MCP Integration** | Leverage the reserved `ToolContext.McpClients` field to integrate Model Context Protocol (MCP) external tool ecosystem for cross-process tool calls. |
| **🧪 Read-Only Bash Sandbox** | Provide subagents with a restricted `run_bash` that only allows pure query commands, further reducing side-effect risks. |
| **📈 Execution Metrics & Observability** | Collect metrics on tool call counts, latency, and failure rates for performance tuning and troubleshooting. |
| **🧠 Enhanced Memory Ranking** | Improve memory retrieval with relevance scoring, temporal decay, and cross-session deduplication. |
| **🔌 Plugin / MCP Client SDK** | Officially release the MCP client integration SDK for third-party tool providers. |
| **🔄 Multi-User & Collaboration** | Support multi-user sessions, shared workspaces, and collaborative agent workflows. |

---

## 📄 License

This project is open-source under the MIT License. See [LICENSE](LICENSE) for details.
