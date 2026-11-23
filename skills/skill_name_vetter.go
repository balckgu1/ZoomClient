package skills

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxSkillNameLen = 64

// validateSkillName 校验 skill 的name 是否可以安全地进入 system prompt 目录段。
func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("名称为空")
	}
	if !utf8.ValidString(name) {
		return fmt.Errorf("名称不是合法 UTF-8")
	}
	if n := utf8.RuneCountInString(name); n > maxSkillNameLen {
		return fmt.Errorf("名称超长（%d > %d）", n, maxSkillNameLen)
	}

	for i, r := range name {
		// 首字符额外限制：不允许列表符号/隐藏前缀
		if i == 0 && (r == '.' || r == '-' || r == '_') {
			return fmt.Errorf("名称不能以 %q 开头", r)
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '.', '_', '-':
			continue
		}
		return fmt.Errorf("名称含非法字符 %q（仅允许字母/数字/. _ -）", r)
	}
	return nil
}

// sanitizeNameForPrompt 输出期兜底净化：
// 把任何不属于「字母/数字/. _ -」的字符替换为 '_'，并按 maxSkillNameLen 截断。
// 这是第二道防线——即便某个 skill 绕过了 validateSkillName 进入注册表，
// 拼进 system prompt 的名称也不可能带换行或冒号去伪造目录条目。
func sanitizeNameForPrompt(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	out := b.String()
	if r := []rune(out); len(r) > maxSkillNameLen {
		out = string(r[:maxSkillNameLen])
	}
	if out == "" {
		out = "unnamed"
	}
	return out
}
