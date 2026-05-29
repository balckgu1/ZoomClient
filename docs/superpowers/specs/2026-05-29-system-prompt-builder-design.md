# System Prompt Builder Design Spec

## Overview

将 `main.go` 中内联的 system prompt 拼接逻辑重构为 `prompt/` 包中的 `SystemPromptBuilder`，按 s10 教学文档的 6 段流水线模式组装系统提示词。

## Background

当前 system prompt 构建逻辑散落在 `main.go` L322-374，通过硬编码字符串拼接 4 块内容（core、skills、memory、memory rules）。随着系统功能增长，这种方式不易维护、测试和扩展。

s10 教学文档要求将 system prompt 拆成 6 段独立来源，由 Builder 按序组装。

## Goals

- 将 prompt 构建逻辑从 `main.go` 抽离到 `prompt/` 包
- 实现 s10 定义的 6 段流水线：core → tools → skills → memory → CLAUDE.md → dynamic
- 构造函数注入外部依赖（SkillRegistry、memoryDir、model、workDir）
- 为 `Build()` 方法编写单元测试

## Non-Goals

- 不实现 CLAUDE.md 文件扫描逻辑（仅预留方法占位）
- 不引入 Section 接口等抽象层（Flat Builder 即可）
- 不实现 system reminder 机制（那是后续章节内容）

## Architecture

### File Structure

| 文件 | 职责 |
|------|------|
| `prompt/builder.go` | `SystemPromptBuilder` struct + `NewSystemPromptBuilder()` + `Build()` |
| `prompt/core.go` | `buildCore()` — 核心身份和行为说明 |
| `prompt/tools.go` | `buildTools()` — 占位，返回空字符串 |
| `prompt/skills.go` | `buildSkills()` — 从 SkillRegistry 生成 skill 目录 |
| `prompt/memory.go` | `buildMemory()` — 加载历史 memory + memory 保存规则 |
| `prompt/claude_md.go` | `buildClaudeMD()` — 最小占位，返回空字符串 |
| `prompt/dynamic.go` | `buildDynamic()` — date / cwd / model / GOOS |
| `prompt/builder_test.go` | `Build()` 方法单元测试 |

### Builder Struct

```go
type SystemPromptBuilder struct {
    skillRegistry *skills.SkillRegistry
    memoryDir     string
    model         string
    workDir       string
}
```

### Constructor

```go
func NewSystemPromptBuilder(
    skillRegistry *skills.SkillRegistry,
    memoryDir string,
    model string,
    workDir string,
) *SystemPromptBuilder
```

### Build() Method

按序调用 6 个 `build*()` 方法，收集非空段，用 `"\n\n"` 拼接返回：

```go
func (b *SystemPromptBuilder) Build() string {
    parts := []string{
        b.buildCore(),
        b.buildTools(),
        b.buildSkills(),
        b.buildMemory(),
        b.buildClaudeMD(),
        b.buildDynamic(),
    }
    // 过滤空段，用 "\n\n" 拼接
    var nonEmpty []string
    for _, p := range parts {
        if p != "" {
            nonEmpty = append(nonEmpty, p)
        }
    }
    return strings.Join(nonEmpty, "\n\n")
}
```

## Section Details

### 1. buildCore() — 核心身份说明

从现有 `main.go` L322-328 搬入，包含 `runtime.GOOS` 和行为指引：

```
You are a helpful assistant running on {GOOS}. Use the todo tool to plan multi-step work. Keep exactly one step in_progress when a task has multiple steps. Refresh the plan as work advances. Prefer tools over prose.
```

### 2. buildTools() — 工具列表

返回空字符串。工具 schema 已通过 API 协议的 `tools` 参数传递给模型，无需在 prompt 中重复。预留方法签名供后续扩展。

### 3. buildSkills() — Skills 目录

调用 `skillRegistry.DescribeAvailable()` 获取 skill 列表。非空时添加前缀标题：

```
Skills available (call the load_skill tool to load the full body on demand):
- skill-name: description
- another-skill: another description
```

若 `skillRegistry` 为 nil 或无可用 skill，返回空字符串。

### 4. buildMemory() — Memory 段

两块拼合：

1. **历史 memory**：调用 `memory.LoadMemorySection(memoryDir)` 获取上次会话保存的 memory
2. **memory 保存规则**：硬编码的规则文本（从 `main.go` L363-374 搬入）

两块均有内容时用 `\n\n` 拼接；均为空时返回空字符串。

### 5. buildClaudeMD() — CLAUDE.md 指令链（占位）

返回空字符串。预留方法签名和注释，说明后续将实现分层指令链：
- 用户全局级
- 项目根目录级
- 当前子目录级

### 6. buildDynamic() — 动态环境信息

运行时生成，包含 4 项：

```
## Current Environment
- Date: 2026-05-29
- Working directory: ./workdir
- Model: deepseek-v4-flash
- OS: windows
```

- Date：`time.Now().Format("2006-01-02")`
- Working directory：构造函数传入的 `workDir`
- Model：构造函数传入的 `model`
- OS：`runtime.GOOS`

## Integration

### main.go 改造

替换 L322-374 的内联拼接为：

```go
builder := prompt.NewSystemPromptBuilder(skillregistry, cfg.Memory.Dir, modelname, toolCtx.WorkPath)
systemPrompt := builder.Build()
```

需要在 `main.go` import 中添加 `"zoomClient/prompt"`。

### 对 agentLoop 的影响

无。`systemPrompt` 仍为 `string` 类型，传入方式不变。

## Testing

为 `Build()` 方法编写单元测试 `prompt/builder_test.go`：

- 测试空 registry + 空 memoryDir 场景（仅 core + dynamic 非空）
- 测试有 skills + memory 场景（验证各段拼接）
- 测试 `Build()` 返回值包含预期子串

## Dependencies

- `zoomClient/skills` — SkillRegistry.DescribeAvailable()
- `zoomClient/memory` — LoadMemorySection()
- `runtime` — runtime.GOOS
- `time` — time.Now()
