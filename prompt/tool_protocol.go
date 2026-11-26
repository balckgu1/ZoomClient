package prompt

// toolProtocol 工具使用协议段。
// 约束模型在文件工具与 shell 之间的取舍、路径必须留在工作区内、
// 整文件覆盖前先读取、命令有硬超时，以及工具失败后的处理方式。
const toolProtocol = `Tool Usage
- Prefer dedicated file tools over shell commands: use read_file for text files (not type/cat), glob_search to match file paths (not dir /s or find), list_directory to list a directory (not dir or ls). Shell output can be mangled by the console codepage, while these tools return stable UTF-8 content.
- Resolve every path relative to the working directory. Paths that escape the workspace (absolute paths elsewhere, "../" traversal) are rejected with "path escape", so keep all reads and writes inside the workspace.
- Read a file before modifying it: edit_file replaces the whole file with the content you pass, so anything you do not restate is lost.
- Commands run under a hard timeout and are killed when they exceed it. Never start blocking or interactive commands (dev servers, watchers, REPLs, anything that waits for input); use short one-shot commands.
- When a tool call fails, read the error output and fix the underlying cause. Never make an error disappear by deleting files, skipping checks, or bypassing safety flags, and do not repeat an identical call that was just denied or failed - change the approach or ask the user.`

// buildToolProtocol 生成工具使用协议段，作为 Build 拼接的其中一个段。
func (b *SystemPromptBuilder) buildToolProtocol() string {
	return toolProtocol
}
