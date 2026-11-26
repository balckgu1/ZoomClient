# 🤖 ZoomClient

**[中文文档](README_ZH.md) | English**

ZoomClient is a Go-based AI Agent runtime built around Tool-Use, implementing an autonomous task-execution loop.
The project follows a modular architecture covering multi-backend model clients, a tool control plane, concurrent execution runtime, subagents, on-demand skill loading with security vetting, session management, context compaction, prompt pipeline, hook system, permission system, persistent task graph, cross-session memory, event emitter, metrics collection, and readline CLI / embedded Web UI frontends.

---

## 📚 Table of Contents

- [✨ Core Features](#-core-features)
- [🏗️ Architecture Overview](#️-architecture-overview)
- [🧩 Component Reference](#-component-reference)
- [⚙️ Installation & Configuration](#️-installation--configuration)
- [🚀 Usage](#-usage)
- [🛠️ Developer Guide](#️-developer-guide)
- [🚧 Roadmap](#-roadmap)

---

## ✨ Core Features

- **🔁 Agent Loop**: The main loop driving model interactions, supporting system prompt injection, message history maintenance, and multi-turn tool calls. Maximum turns are configured via `agentloop.maxTurns` (default 25), and failed/empty LLM responses are retried up to `agentloop.maxLLMRetries` times through hooks.
- **🔌 Multi-Backend ChatClient Abstraction**: Unified `clients.ChatClient` interface with built-in **OpenAI-compatible** (DeepSeek / Kimi / Qwen / OpenAI), **Ollama** (NDJSON streaming), **Anthropic** (Claude), and **Gemini** implementations, selected with the `-m` CLI flag.
- **⚙️ Model Presets & Hot Switching**: Presets (client type, base URL, API key, model name) are managed by `model.Registry` and persisted to `config/models.yaml`. Add/list/switch presets from the CLI (`/setmode`, `/models`, `/selectmode`) or the Web UI (CRUD + connectivity test), and hot-switch backends at runtime without losing conversation history.
- **🔧 Tool Control Plane**: Unified tool registry `tools.ToolRegister` supporting discovery, registration, and dynamic dispatch by name, with a built-in permission gate (`PermissionDecider`) for pre-execution admission control.
- **⚡ Concurrent-Safe Scheduling**: Tool calls are automatically batched by read/write attributes — read-only tools (e.g., `read_file`, `glob_search`) execute concurrently, while write tools run serially, balancing efficiency and safety.
- **🧬 Subagent**: Delegates subtasks to independently-contextualized subagents via the `sub_task` tool; supports blank-context mode and `fork=true` mode that inherits parent messages, with an independent, restricted tool whitelist to prevent side-effect leakage.
- **📖 On-Demand Skill Loading**: Recursively scans `SKILL.md` files (with YAML frontmatter) under `.skills/`, injecting only the skill catalog into the system prompt; full content is loaded on demand via `load_skill` (with fuzzy name matching).
- **🛡️ Skill Security Vetting**: Skill metadata and bodies pass static red-flag scanning (Unicode normalization, word-family co-occurrence, credential/upload/exfiltration combos), description sanitization, symlink rejection, and an "untrusted" banner before any degraded skill body is exposed. `skilllab/` ships a local experiment server for demonstrating these defenses.
- **📝 Session Plan Management (Todo Write)**: Allows the model to maintain multi-step task plans during a session with status tracking; automatically injects reminder messages when the plan hasn't been updated for `agentloop.todoRoundsThreshold` consecutive turns.
- **🗂️ Persistent Task Graph**: The `task/` module stores each task as a JSON file (default `.tasks/`), supporting `create_task`, `list_tasks`, `get_task`, `claim_task`, and `complete_task` with `blockedBy` dependencies, `pending` / `in_progress` / `completed` states, and cross-session persistence.
- **🗜️ Context Compaction**: Three-tier compaction strategy — large tool results persisted to disk and replaced with previews, old tool results micro-compacted to placeholders, and full-history summarization via model invocation when overall history is too long. A context-usage snapshot (system prompt / skills / tool schemas / messages) is pushed to the Web UI.
- **🪝 Hook System**: Event-driven hook framework covering eight event points: `SessionStart`, `PreChat`, `PostChat`, `LLMError`, `PreToolUse`, `PostToolUse`, `ToolError`, and `SessionEnd`, with built-in handlers for dangerous command blocking, sensitive file protection, rate limiting, audit logging, LLM retry, and error recovery.
- **🛡️ Permission System**: Rule-engine-based permission control supporting `default` (ask on miss), `plan` (read-only), `auto` (read-only auto-approve), and `root` (full access) modes, with configurable allow/deny rules and `re:`-prefixed regex matching. Rules can also be updated at runtime from the Web UI.
- **🛡️ Path Sandbox Protection**: File tools must pass path sandbox validation (based on `ToolContext.WorkPath`) before execution, preventing path traversal and working-directory escape.
- **🚫 Dangerous Command Blocking**: `run_bash` includes blacklist detection (substring and regex rules from `permission.denyRules`) to intercept high-risk operations on Linux/macOS/Windows.
- **🧠 Thinking Mode Support**: Full passthrough of the `reasoning_content` field, preserving it as-is across multi-turn conversations to avoid `invalid_request_error` rejections from the server.
- **🗄️ Session Management**: Persistent session CRUD via `session.Manager` with JSON-file storage, a metadata index, automatic title generation, and work-directory tracking. Supports recovery, cross-process browsing, and list/load/new/delete/rename from both CLI and Web UI.
- **🧩 FSM (Finite State Machine)**: Encapsulates agent session state including message history, turn count, and transition reasons, providing a clean abstraction decoupled from business logic.
- **📦 Prompt Pipeline**: Multi-stage prompt construction (`prompt.MessagePipeline`) that assembles core instructions, skill catalog, memory summary, `CLAUDE.md`, dynamic context, per-turn reminders, and attachments, then normalizes the result for the target model protocol.
- **💭 Cross-Session Memory (`memory/`)**: Long-term memory with four types (user, feedback, project, reference) and `save_memory`, `search_memory` (metadata match by default, `full_text=true` for body search), `update_memory`, and `delete_memory` tools. `MEMORY.md` acts as the index (up to 50 entries injected into the prompt), bodies are truncation-limited to 15 KB per search hit, and per-turn synchronous incremental extraction (cursor + fingerprint based, deduplicated) runs at the end of each turn when `memory.autoExtractOnTurn` is enabled.
- **🔄 Event Emitter (`emitter/`)**: Unified output abstraction for all agent-visible events (session lifecycle, assistant output, reasoning, tool calls/results, subagent activity, hook blocks, errors, plan updates, compaction, phases). Implementations: the CLI renderer and the Web SSE emitter.
- **🌐 Web UI (`web/`)**: Built-in HTTP server with SSE streaming, RESTful APIs (chat, sessions, model presets + connectivity test, work directory, skills, permission config, context usage, stop), permission ask dialog, and a Preact/Vite frontend embedded via `go:embed`. The browser opens automatically in web mode.
- **🖥️ Interactive CLI**: A `readline`-based line renderer with Tab completion, ↑↓ history, colored output, and slash commands (`/help`, `/clear`, `/compact`, `/exit`, `/quit`, `/models`, `/setmode`, `/selectmode`, `/workspace`, `/session`).
- **📈 Observability**: Optional hook-driven metrics collector writing JSONL (LLM latency, token usage, tool traces, errors/retries, session lifecycle) to `logs/metrics.jsonl`, ready for evaluation harnesses and LangSmith ingestion.
- **📂 Work Directory Management**: Set the sandbox root at startup with `-workdir`, or switch it at runtime with `/workspace` (CLI) or `POST /api/workdir` (Web).
- **🪵 Structured Logging & YAML Configuration**: Uber Zap logging with colorful, timestamped output, and a YAML config covering API keys, turns, skills, memory, sessions, tasks, permissions, compaction, and observability.

---

## 🏗️ Architecture Overview

The project adopts a layered architecture, from top to bottom: Agent Loop Layer, Capability Extension Layer, Tool Management Layer, State & Control Layer, Presentation Layer, and Runtime Support Layer:

```
┌──────────────────────────────────────────────────────────────────┐
│                      🔁 Agent Loop Layer                        │
│              (main.go · agent.go · agent_loop.go · repl.go)      │
├──────────────────────────────────────────────────────────────────┤
│                   🧩 Capability Extension Layer                  │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │subagent/ │  │ skills/  │  │  hook/   │  │ session/ │       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │ memory/  │  │ compact/ │  │ prompt/  │  │ model/   │       │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐                                                  │
│   │  task/   │  persistent task graph                           │
│   └──────────┘                                                  │
├──────────────────────────────────────────────────────────────────┤
│                    🔧 Tool Management Layer                      │
│          (tools/ · ToolRegister · concurrency runtime)          │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │
│   │ read_file│  │write_file│  │ run_bash │  │glob_search│      │
│   └──────────┘  └──────────┘  └──────────┘  └──────────┘       │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐                     │
│   │ edit_file│  │list_dir  │  │ todo     │                     │
│   └──────────┘  └──────────┘  └──────────┘                     │
├──────────────────────────────────────────────────────────────────┤
│                  ⚙️ State & Control Layer                        │
│            (fsm/ · permission/ · emitter/)                      │
│         ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│         │  FSM     │  │Permission│  │ Emitter  │               │
│         └──────────┘  └──────────┘  └──────────┘               │
├──────────────────────────────────────────────────────────────────┤
│                    🖥️ Presentation Layer                         │
│   ┌──────────────────────┐  ┌──────────────────────┐           │
│   │  CLI (readline)      │  │  Web UI (SSE)        │           │
│   └──────────────────────┘  └──────────────────────┘           │
├──────────────────────────────────────────────────────────────────┤
│                 🛠️ Runtime Support Layer                         │
│    (utils/ · logger/ · web/ · metrics/ · skilllab/)             │
└──────────────────────────────────────────────────────────────────┘
```

### Layer Breakdown

| Layer | Modules | Responsibility |
|-------|---------|----------------|
| **Agent Loop Layer** | `main.go`, `agent.go`, `agent_loop.go`, `repl.go` | Orchestrates the model-interaction loop and REPL/Web command handling |
| **Capability Extension Layer** | `subagent/`, `skills/`, `hook/`, `session/`, `memory/`, `compact/`, `prompt/`, `model/`, `task/` | Pluggable extensions for reasoning, persistence, planning, and intelligence |
| **Tool Management Layer** | `tools/` | Tool registry, read/write scheduling, concrete tool implementations |
| **State & Control Layer** | `fsm/`, `permission/`, `emitter/` | State persistence, admission control, event output abstraction |
| **Presentation Layer** | `ui/` (CLI), `web/` (Web UI + SSE) | Frontends for terminal and browser scenarios |
| **Runtime Support Layer** | `utils/`, `logger/`, `web/`, `metrics/`, `skilllab/` | Config loading, structured logging, HTTP serving, metrics, skill security lab |

---

## 🧩 Component Reference

### Core Modules

| Module | Package | Description |
|--------|---------|-------------|
| **Agent Loop** | `agent_loop.go` | Main interaction loop; calls the LLM, dispatches tool calls, manages turns and retries |
| **ChatClient** | `clients/` | Unified `ChatClient` interface for OpenAI-compatible, Ollama, Anthropic, Gemini |
| **Model Registry** | `model/` | Model preset definition, YAML persistence, runtime hot-switching |
| **Tool Registry** | `tools/` | `ToolRegister`: registration, discovery, dispatch, concurrency scheduling |
| **Subagent** | `subagent/` | Independent sub-agent execution with isolated context and restricted tools |
| **Skills** | `skills/` | Recursive SKILL.md loading, on-demand content, fuzzy matching, security vetting |
| **Task Graph** | `task/` | Persistent tasks with dependencies and status transitions |
| **Session Management** | `session/` | Session CRUD, JSON persistence, index, auto-naming, workdir metadata |
| **FSM** | `fsm/` | Session state encapsulation (messages, turn count, transitions) |
| **Context Compaction** | `compact/` | Three-tier compression (persist, micro-compact, summarize) + usage snapshot |
| **Hook System** | `hook/` | Event-driven plugin framework for session/LLM/tool lifecycle |
| **Permission System** | `permission/` | Rule-based access control, path sandbox, bash blacklist |
| **Prompt Pipeline** | `prompt/` | Multi-stage prompt construction (core + skills + memory + CLAUDE.md + reminders + attachments) |
| **Memory System** | `memory/` | Cross-session memory, index loading, full-text search, per-turn extraction |
| **Event Emitter** | `emitter/` | Unified output interface for CLI and Web frontends |
| **Interactive CLI** | `ui/` | readline-based REPL with completion, history, colors, slash commands |
| **Web Server** | `web/` | HTTP server with SSE, REST APIs, embedded Preact frontend, permission asker |
| **Metrics** | `metrics/` | Hook-based JSONL metrics collector |
| **Skill Lab** | `skilllab/` | Local web lab for demonstrating skill-poisoning defenses |
| **Structured Logger** | `logger/` | Zap-based colorful, timestamped logging |
| **Config Loader** | `utils/` | YAML config loading with defaults and validation |

### Concrete Tools

| Tool | Package | Description |
|------|---------|-------------|
| **read_file** | `tools/` | Read file content from disk |
| **write_file** | `tools/` | Write content to a file |
| **edit_file** | `tools/` | Apply search/replace edits to files |
| **run_bash** | `tools/` | Execute shell commands with blacklist protection and timeout |
| **glob_search** | `tools/` | Pattern-based file search |
| **list_directory** | `tools/` | List directory contents |
| **todo** | `tools/` | Multi-step session plan management (create/update/mark) |
| **create_task** | `task/` | Create a persistent task with optional `blockedBy` dependencies |
| **list_tasks** | `task/` | List tasks with status, owner, and dependencies |
| **get_task** | `task/` | Fetch full task details by ID |
| **claim_task** | `task/` | Claim a pending task (respects unmet dependencies) |
| **complete_task** | `task/` | Complete an in-progress task and report unblocked downstream tasks |
| **sub_task** | `subagent/` | Delegate work to a subagent (blank or forked context) |
| **load_skill** | `skills/` | Load full skill content on demand |
| **save_memory** | `memory/` | Persist a new memory entry and update `MEMORY.md` |
| **search_memory** | `memory/` | Search memories by metadata, or by full text with `full_text=true` |
| **delete_memory** | `memory/` | Remove a memory entry from disk and the index |
| **update_memory** | `memory/` | Modify an existing memory entry |
| **compact** | `compact/` | Trigger context compaction manually |

---

## ⚙️ Installation & Configuration

### Prerequisites

- **Go 1.25+**
- **Node.js 22+ with npm** (only needed to build the Web UI)
- (Optional for Ollama) a local Ollama server
- API keys (set in `config/config.yaml` or environment variables)

### Quick Start (Prebuilt Release)

Windows binaries are published on the GitHub Releases page by the release workflow:

1. Download `zoomclient_<version>_windows_amd64.zip`.
2. Unzip it and copy `config/config.yaml.example` to `config/config.yaml`.
3. Fill in your API keys and run `zoomclient.exe` (CLI) or `zoomclient.exe -mode web` (browser UI).

### Build From Source

```bash
git clone https://github.com/balckgu1/ZoomClient.git
cd ZoomClient

# Optional, but required for a functional Web UI
cd web/frontend
npm ci
npm run build
cd ../..

go build -o zoomclient .
```

> The Web frontend is embedded with `//go:embed all:frontend/dist`. The repository tracks only `web/frontend/dist/.gitkeep` (and `public/.gitkeep`), so a clean clone compiles and the CLI works immediately, but the Web UI serves placeholder content until you run `npm run build`.

### Configuration

Copy `config/config.yaml.example` to `config/config.yaml` (or point to another directory with `-config-dir`):

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
  defaultBashTimeout: 120      # seconds

skills:
  dir: "./.skills"

memory:
  dir: "./.memory"
  autoExtractOnTurn: true      # synchronous incremental extraction at the end of each turn

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
    enabled: false             # write JSONL metrics when true
    path: "logs/metrics.jsonl"
```

> `config/models.yaml` stores the model presets created via `/setmode` or the Web UI. It is managed automatically by the registry.

---

### 🎛️ CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-m` | `openai` | Backend: `openai` \| `ollama` \| `anthropic` \| `gemini` |
| `-mode` | `cli` | Output mode: `cli` (terminal) \| `web` (browser UI) |
| `-web` | `false` | Deprecated alias for `-mode web` |
| `-port` | `8080` | Web server port (web mode only) |
| `-config-dir` | `./config` | Config directory containing `config.yaml` |
| `-workdir` | current directory | Tool sandbox root |

```bash
go run .                                  # CLI with the default OpenAI-compatible backend
go run . -m ollama                        # local Ollama
go run . -m anthropic                     # Anthropic Claude
go run . -m gemini -workdir D:/workspace  # Gemini, custom sandbox root
go run . -m openai -mode web -port 9090   # Web UI on http://localhost:9090
```

### 🌐 Running as a Web Server

```bash
go run . -m openai -mode web     # legacy shorthand: go run . -m openai -web
```

The server serves the embedded frontend and opens the browser automatically. Main endpoints:

| Group | Endpoints |
|-------|-----------|
| Chat | `POST /api/chat`, `GET /api/events` (SSE), `POST /api/stop`, `POST /api/clear`, `POST /api/compact` |
| Permission | `POST /api/permission` (answer ask), `GET/PUT /api/permission/config` |
| Sessions | `GET/POST /api/sessions`, `GET/PATCH/DELETE /api/sessions/{id}` |
| Models | `GET/POST /api/models`, `POST /api/model/select`, `POST /api/models/test`, `PUT/DELETE /api/models/{name}` |
| Runtime | `GET/POST /api/workdir`, `GET /api/skills`, `GET /api/status`, `GET /api/context-usage` |

### ⌨️ CLI Slash Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/clear` | Clear conversation history (keeps the system prompt) |
| `/compact` | Manually compact conversation history |
| `/exit`, `/quit` | Exit |
| `/models` | List configured model presets |
| `/setmode -m <name> -t <type> -u <url> -k <key> [--model <model>]` | Add or update a model preset |
| `/selectmode -m <name>` | Switch to a preset (history preserved) |
| `/workspace [path]` | Show or switch the working directory |
| `/session list\|load <id>\|new\|delete <id>\|rename <id> <title>\|current` | Manage sessions |

### 🧠 Runtime Model Switching

In the Web UI, choose a preset from the model panel (or use `POST /api/model/select`); in the CLI, use `/selectmode -m <name>`. The agent re-binds the client, prompt builder, and compaction manager while keeping the message history.

### 📖 Using Skills

Place skills under the directory specified by `skills.dir` (default `./.skills/`). Skills are discovered recursively; each skill is a subdirectory containing a `SKILL.md` with YAML frontmatter:

```markdown
---
name: your-skill-name
description: One-line description of the skill's purpose
---

# Content

Detailed steps or playbook...
```

On startup, the model sees only the skill catalog. Full content is loaded on demand via `load_skill`. Skills that trigger security red flags are either rejected or marked untrusted before their bodies are injected.

### 🧬 Using Subagents

The model can dispatch subtasks via the `sub_task` tool:

- Default `fork=false`: the subagent runs with a blank context; the prompt must be self-contained.
- Set `fork=true`: the subagent inherits the parent message history, ideal for "continue analyzing the current conversation" scenarios.

Subagents use a separate registry (read_file / list_directory / run_bash / glob_search) and share the same permission policy.

### 🗂️ Task Graph

Use `create_task` to add persistent tasks with optional `blockedBy` dependencies, `list_tasks` / `get_task` to inspect them, `claim_task` to move a ready task to `in_progress`, and `complete_task` to finish it and report which downstream tasks became unblocked. Tasks live in `tasks.dir` and survive across sessions and processes.

### 💭 Memory Extraction

With `memory.autoExtractOnTurn: true`, an incremental extraction runs synchronously at the end of each turn (the turn does not finish until extraction completes). A cursor plus message fingerprint prevents duplicate extraction; failures are retried on the next turn. Manual `/clear`, `/compact`, and `/session` switches also flush the pending memory window first.

### 📈 Metrics

Set `observability.metrics.enabled: true` to append one JSON object per event to `logs/metrics.jsonl`. Events cover session start/end, LLM calls (latency, token estimates), errors/retries, and tool calls. The collector is a pure observer and never changes agent behavior.

---

## 🛠️ Developer Guide

### ➕ Adding a New Tool

1. Create a new file in `tools/` (e.g., `mynewtool.go`).
2. Define a struct and implement the `Tool` interface's four methods:
   - `Name() string` — Unique tool identifier.
   - `Description() string` — Description for the model to decide when to call.
   - `Parameters() map[string]any` — JSON Schema parameter definition.
   - `Call(args map[string]any, ctx *ToolContext) ToolResult` — Execution logic.
3. Register the tool in `init.go:initTools` via `registry.Register(YourTool{})`.
4. If the tool is read-only with no side effects, add it to `tools/runtime.go`'s `concurrencySafeTools` map for concurrent execution.
5. Add it to `permission/permission.go`'s `readOnlyTools` or `writeTools` map so the permission system classifies it correctly.
6. To allow subagents to use the tool, also register it in `subagent.BuildSubAgentRegistry()`.

### 🔌 Adding a New LLM Backend

1. Create a new file in `clients/` (e.g., `claude_chat.go`).
2. Define a client struct and implement the `ChatClient` interface's `Chat(model, messages, toolList, options)` method.
3. Inside the method: convert tool schemas → convert message protocols → make the HTTP call → normalize the response to `*ChatResponse`.
4. Add a case in `init.go:initClient` for API-key retrieval (with env-var fallback), base URL/model defaults, and client construction.

### 📖 Adding a New Skill

No code required — create a subdirectory under `.skills/` with a `SKILL.md` (see `.skills/skill-function-test/SKILL.md`). It is discovered recursively on restart. `skilllab` can be used to validate how the security vetting reacts:

```bash
go run ./skilllab -addr 127.0.0.1:19490 -skills ./.skills
```

### 🧠 Adding a Model Preset

Use `/setmode` in the CLI, add it in the Web UI, or edit `config/models.yaml`. Presets are persisted by `model.Registry` and become available to `-m`, `/selectmode`, and runtime switching.

### 🔄 Adding a New Emitter Implementation

Implement the `emitter.Emitter` interface (including `EmitPhase` and `EmitSystem`) and wire it in `init.go:initEmitter`. Current implementations: `ui.Renderer` (CLI) and `web.SseEmitter` (Web SSE).

### 🎨 Frontend Development

```bash
cd web/frontend
npm ci
npm run dev      # Vite dev server with API proxying
npm run build    # emit dist/ for go:embed
```

Because `dist/` is embedded into the binary, always run `npm run build` before `go build` when you change frontend code. Do not delete `web/frontend/dist/.gitkeep` or `web/frontend/public/.gitkeep`.

### 🧪 Unit Testing & Lint

Each core module includes `*_test.go` files. Run all tests:

```bash
go test ./...
```

Run a specific package:

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

CI (`.github/workflows/test.yml`) runs Go tests, `golangci-lint` (only new issues), and a TypeScript type check for the frontend.

### 🚀 Release

Push a `v*` tag. The release workflow builds the frontend, then runs GoReleaser to compile a Windows amd64 `zoomclient` binary, package it as a zip with `LICENSE`, both READMEs, and `config/config.yaml.example`, and publish the GitHub Release with checksums.

### 🐞 Debugging with Logs

Use Zap log levels (`Debug` / `Info` / `Warn` / `Error`) to observe message flow, tool call batching, and execution results. `.vscode/launch.json` provides multi-scenario debug configurations.

---

## 🚧 Roadmap

| Module | Description |
|--------|-------------|
| **🔗 MCP Integration** | Integrate the Model Context Protocol (MCP) external tool ecosystem for cross-process tool calls. |
| **🧪 Read-Only Bash Sandbox** | Provide subagents with a restricted `run_bash` that only allows pure query commands, further reducing side-effect risks. |
| **🧠 Enhanced Memory Ranking** | Improve memory retrieval with relevance scoring, temporal decay, and cross-session deduplication. |
| **🔌 Plugin / MCP Client SDK** | Officially release an MCP client integration SDK for third-party tool providers. |
| **🔄 Multi-User & Collaboration** | Support multi-user sessions, shared workspaces, and collaborative agent workflows. |
| **🖥️ Multi-Platform Releases** | Extend release builds beyond Windows amd64 (Linux/macOS archives). |

---

## 📄 License

This project is open-source under the **Apache License 2.0**. See [LICENSE](LICENSE) for details.
