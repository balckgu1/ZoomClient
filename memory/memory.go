package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"zoomClient/tools"
)

const (
	MaxBodyChars             = 15 * 1024                   // search_memory 返回的单条正文上限，超出时截断并提示模型改用 read_file 读取完整 memory 文件。
	MaxMemoryEntries         = 50                          // system prompt 中最多显示的 memory 条目数
	manifestMaxBytes         = 25 * 1024                   // 记忆抽取 prompt 的 MEMORY.md 清单上限（25KB），超出截断
	emptyManifestPlaceholder = "(empty — no memories yet)" // 在无现有记忆时注入 prompt，避免空清单误导模型
)

const (
	MTypeUser      = "user"
	MTypeFeedback  = "feedback"
	MTypeProject   = "project"
	MTypeReference = "reference"
)

// MemoryPriority 定义type优先级，数值越小优先级越高
var MemoryPriority = map[string]int{
	MTypeFeedback:  0,
	MTypeUser:      1,
	MTypeProject:   2,
	MTypeReference: 3,
}

type MemoryDocument struct {
	FrontMatter MemoryFrontMatter
	Body        string
}

type MemoryFrontMatter struct {
	Name        string
	Description string
	Type        string
}

// ExtractionSystemPrompt 记忆抽取 subagent 的 system prompt
const ExtractionSystemPrompt = `You are a memory extraction agent. Analyze the conversation above and update the persistent memory directory so that future sessions retain durable, non-obvious facts.

Each memory is one Markdown file with YAML frontmatter:
---
name: <short-kebab-case-slug>
description: <one-line summary>
type: user | feedback | project | reference
---
<body: the fact itself>

Types:
- user: who the user is (role, expertise, preferences).
- feedback: guidance the user gave on how to work — corrections AND confirmed approaches; include the why.
- project: ongoing work, goals, or constraints not derivable from code or git history; convert relative dates to absolute.
- reference: pointers to external resources (URLs, dashboards, tickets).

Do NOT save: code-derivable info (signatures, file structure), temporary state (current branch, open PRs, TODOs), secrets (API keys, passwords), or anything that only matters to this conversation.

Rules:
- Prefer updating an existing memory over creating a duplicate; inspect existing memories first.
- Delete memories that turn out to be wrong or outdated.
- If the user asked to ignore memory, or nothing is worth saving, output only: Nothing to save.

Be efficient: read/list the memory files you might update first, then write. Return one concise summary of what you changed as your final answer.`

// ExtractionForkPrefix 在 fork 模式下注入到父消息末尾、抽取指令之前的引导前缀。
const ExtractionForkPrefix = "\n\n[The above is the conversation to analyze. Now perform memory extraction as instructed below.]\n\n"

// BuildExtractionRegistry 构建抽取记忆subagent的工具集
// read_file/glob_search/list_directory + save/update/delete_memory。
func BuildExtractionRegistry(memoryDir string) *tools.ToolRegister {
	reg := tools.NewToolRegister()
	reg.Register(tools.ReadFileTool{})
	reg.Register(&tools.GlobSearch{})
	reg.Register(tools.ListDirectory{})
	reg.Register(NewSaveMemoryTool(memoryDir))
	reg.Register(NewUpdateMemoryTool(memoryDir))
	reg.Register(NewDeleteMemoryTool(memoryDir))
	return reg
}

// BuildManifest 读取 MEMORY.md 索引作为抽取去重参照，超过 manifestMaxBytes 截断。索引缺失或为空时返回占位文本。
func BuildManifest(memoryDir string) string {
	if memoryDir == "" {
		return emptyManifestPlaceholder
	}
	data, err := os.ReadFile(filepath.Join(memoryDir, "MEMORY.md"))
	if err != nil {
		return emptyManifestPlaceholder
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return emptyManifestPlaceholder
	}
	if len(content) > manifestMaxBytes {
		content = content[:manifestMaxBytes] + "\n...[manifest truncated]"
	}
	return content
}

// BuildExtractionForkPrompt 构造 fork 模式下追加的抽取指令（含现有记忆清单）。
func BuildExtractionForkPrompt(manifest string) string {
	return fmt.Sprintf(`Analyze the conversation above and update the persistent memory accordingly.

Existing memory index (MEMORY.md):
%s

Steps:
1. If nothing is durable and non-obvious worth saving (or the user asked to ignore memory), output only: Nothing to save.
2. Otherwise, use glob_search/list_directory/read_file to inspect existing memory files you might update.
3. Use save_memory for new facts, update_memory to refine existing ones, delete_memory for wrong ones.

Keep each memory atomic (one fact per file). Prefer updating over duplicating.`, manifest)
}
