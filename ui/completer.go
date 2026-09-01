// Package ui 集中负责 CLI 前端的"用户可见"渲染。
package ui

// 本文件集中维护斜杠命令清单，用于 Tab 补全与横幅命令提示。
import "github.com/chzyer/readline"

// commandList 记录可用的斜杠命令（唯一真源，供补全与提示共用）。
var commandList = []string{
	"/clear", "/compact", "/exit", "/quit",
	"/help", "/models", "/setmode", "/selectmode",
	"/workspace", "/session",
}

// sessionSub 是 /session 的子命令，用于二级补全。
var sessionSub = []string{"list", "load", "new", "delete", "rename", "current"}

// buildCompleter 构造 readline 的 Tab 补全器：
//   - 顶层：所有斜杠命令
//   - /session：其子命令
func buildCompleter() readline.AutoCompleter {
	sessionChildren := make([]readline.PrefixCompleterInterface, 0, len(sessionSub))
	for _, s := range sessionSub {
		sessionChildren = append(sessionChildren, &readline.PrefixCompleter{Name: []rune(s)})
	}

	children := make([]readline.PrefixCompleterInterface, 0, len(commandList))
	for _, c := range commandList {
		item := &readline.PrefixCompleter{Name: []rune(c)}
		if c == "/session" {
			item.Children = sessionChildren
		}
		children = append(children, item)
	}
	return readline.NewPrefixCompleter(children...)
}

// commandHintRows 返回横幅中展示的命令提示（已对齐）。
func commandHintRows() []string {
	return []string{
		"  /clear       清空会话历史        /session     管理会话",
		"  /compact     压缩历史            /setmode     新增/更新模型预设",
		"  /exit        退出程序            /selectmode  切换模型",
		"  /help        显示帮助            /workspace   查看/切换工作目录",
		"  /models      列出模型预设        /quit        退出程序",
	}
}
