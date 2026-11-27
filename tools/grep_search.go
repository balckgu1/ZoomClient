package tools

// 本文件实现 grep_search 工具：在工作区内按内容搜索文本文件。
// 之所以用纯 Go 实现而不是让模型跑 grep/findstr：
//   - Windows 没有 grep，findstr 语法不同，跨平台行为不一致；
//   - 复用 isSafePath 做路径边界校验，避免绕过工作区限制；
//   - 输出带行号并支持 head_limit 截断，避免原始输出撑爆上下文。

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// grepIgnoreDirs 搜索时跳过的目录名：版本库、依赖、构建产物与虚拟环境。
// 这些目录体积大且几乎不含源码，遍历它们只会拖慢搜索并产生噪声。
var grepIgnoreDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"out":          true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
	".idea":        true,
	".vscode":      true,
	"target":       true,
	".next":        true,
	".nuxt":        true,
	".gradle":      true,
	".cache":       true,
}

const (
	grepDefaultHeadLimit = 100     // 默认返回条数上限
	grepMaxHeadLimit     = 500     // 允许模型调高的上限
	grepMaxFileBytes     = 2 << 20 // 单文件最大扫描体积（2MB），超出视为生成物跳过
	grepMaxLineRunes     = 300     // 单行内容截断长度，避免超长行（压缩代码）刷屏
)

// GrepSearch 按内容搜索工作区内的文本文件。
type GrepSearch struct{}

func (g GrepSearch) Name() string { return "grep_search" }

func (g GrepSearch) Description() string {
	return "Search file CONTENTS inside the workspace with a regular expression (ripgrep-style). " +
		"Returns matches with file path and line number. Use glob to restrict file types (e.g. **/*.go), " +
		"output_mode=files_with_matches to only list matching files, or output_mode=count for per-file match counts. " +
		"Skips .git, node_modules, vendor, dist and other dependency/build directories automatically."
}

func (g GrepSearch) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regular expression to search for (Go RE2 syntax). Use fixed_strings for literal text.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Base directory or single file to search in, relative to the work directory. Defaults to the work directory root.",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Optional file filter, e.g. *.go or src/**/*.ts. Matches against the relative path and the base name.",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"content", "files_with_matches", "count"},
				"description": "content (default): matching lines with line numbers; files_with_matches: only file paths; count: match count per file.",
			},
			"case_sensitive": map[string]any{
				"type":        "boolean",
				"description": "Match case-sensitively. Defaults to false.",
			},
			"fixed_strings": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as a literal string instead of a regular expression. Defaults to false.",
			},
			"head_limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of result lines/files to return. Defaults to 100, capped at 500.",
			},
		},
		"required": []string{"pattern"},
	}
}

// grepContentMatch 内容模式下的一行命中。
type grepContentMatch struct {
	Path string
	Line int
	Text string
}

// grepFileCount 计数模式下单个文件的命中次数。
type grepFileCount struct {
	Path  string
	Count int
}

func (g GrepSearch) Call(args map[string]any, toolCtx *ToolContext) ToolResult {
	rawPattern, ok := args["pattern"].(string)
	if !ok || strings.TrimSpace(rawPattern) == "" {
		return ToolResult{Ok: false, Content: "Error: missing pattern parameter or pattern parameter is not a string", IsError: true}
	}

	// 编译正则：字面量模式先转义；大小写不敏感用 (?i) 前缀实现
	expr := rawPattern
	if fixed, _ := args["fixed_strings"].(bool); fixed {
		expr = regexp.QuoteMeta(expr)
	}
	caseSensitive, _ := args["case_sensitive"].(bool)
	if !caseSensitive {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: invalid regular expression: %v", err), IsError: true}
	}

	// 解析搜索根路径并做工作区边界校验
	baseDir := toolCtx.WorkPath
	if p, ok := args["path"].(string); ok && p != "" {
		resolved, serr := isSafePath(toolCtx.WorkPath, p)
		if serr != nil {
			return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", serr), IsError: true}
		}
		baseDir = resolved
	}

	outputMode := "content"
	if m, ok := args["output_mode"].(string); ok && m != "" {
		switch m {
		case "content", "files_with_matches", "count":
			outputMode = m
		default:
			return ToolResult{Ok: false, Content: "Error: output_mode must be content, files_with_matches or count", IsError: true}
		}
	}

	headLimit := grepDefaultHeadLimit
	if v, ok := args["head_limit"].(float64); ok && v > 0 {
		headLimit = int(v)
		if headLimit > grepMaxHeadLimit {
			headLimit = grepMaxHeadLimit
		}
	}

	globPattern, _ := args["glob"].(string)

	// 结果收集：三种模式各自维护结构，最后统一格式化
	var contentMatches []grepContentMatch
	var fileMatches []string
	var fileCounts []grepFileCount
	totalFilesScanned := 0

	// searchFile 处理单个文件，返回是否命中。
	// 读取失败、二进制、超体积文件都静默跳过：单个坏文件不应中断整次搜索。
	searchFile := func(fullPath, relPath string) {
		if toolCtx.Ctx != nil {
			select {
			case <-toolCtx.Ctx.Done():
				return
			default:
			}
		}
		info, statErr := os.Stat(fullPath)
		if statErr != nil || info.IsDir() || info.Size() == 0 {
			return
		}
		if info.Size() > grepMaxFileBytes {
			return
		}
		if globPattern != "" && !matchGrepGlob(globPattern, relPath, filepath.Base(fullPath)) {
			return
		}
		data, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			return
		}
		// 含 NUL 字节视为二进制文件，跳过
		if bytes.IndexByte(data, 0) >= 0 {
			return
		}
		totalFilesScanned++

		switch outputMode {
		case "files_with_matches":
			if re.Match(data) {
				fileMatches = append(fileMatches, relPath)
			}
		case "count":
			n := len(re.FindAllIndex(data, -1))
			if n > 0 {
				fileCounts = append(fileCounts, grepFileCount{Path: relPath, Count: n})
			}
		default:
			// 逐行扫描，保留行号；TrimSuffix 去掉 Windows CRLF 残留的 \r
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if re.MatchString(line) {
					text := strings.TrimSuffix(line, "\r")
					text = strings.TrimSpace(text)
					if r := []rune(text); len(r) > grepMaxLineRunes {
						text = string(r[:grepMaxLineRunes]) + "…"
					}
					contentMatches = append(contentMatches, grepContentMatch{Path: relPath, Line: i + 1, Text: text})
				}
			}
		}
	}

	// 路径本身就是文件时只搜索该文件；否则递归遍历目录
	info, statErr := os.Stat(baseDir)
	if statErr != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: cannot access search path: %v", statErr), IsError: true}
	}
	if info.IsDir() {
		walkErr := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				//nolint:nilerr // 单个路径不可读（权限等）时跳过，继续搜索其余文件
				return nil
			}
			if toolCtx.Ctx != nil && toolCtx.Ctx.Err() != nil {
				return filepath.SkipAll
			}
			if d.IsDir() {
				if path != baseDir && grepIgnoreDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			relPath, relErr := filepath.Rel(baseDir, path)
			if relErr != nil {
				relPath = path
			}
			searchFile(path, filepath.ToSlash(relPath))
			return nil
		})
		if walkErr != nil {
			return ToolResult{Ok: false, Content: fmt.Sprintf("Error: search failed: %v", walkErr), IsError: true}
		}
	} else {
		relPath, relErr := filepath.Rel(toolCtx.WorkPath, baseDir)
		if relErr != nil {
			relPath = filepath.Base(baseDir)
		}
		searchFile(baseDir, filepath.ToSlash(relPath))
	}

	return formatGrepResult(outputMode, headLimit, contentMatches, fileMatches, fileCounts, totalFilesScanned)
}

// matchGrepGlob 用 doublestar 同时匹配相对路径与文件名，
// 让 "*.go"（按文件名）和 "src/**/*.ts"（按相对路径）两种写法都符合直觉。
func matchGrepGlob(pattern, relPath, baseName string) bool {
	if ok, err := doublestar.Match(pattern, relPath); err == nil && ok {
		return true
	}
	if ok, err := doublestar.Match(pattern, baseName); err == nil && ok {
		return true
	}
	return false
}

// formatGrepResult 把三种模式的收集结果格式化为给模型看的文本，并按 head_limit 截断。
func formatGrepResult(mode string, headLimit int,
	contentMatches []grepContentMatch, fileMatches []string, fileCounts []grepFileCount,
	scanned int) ToolResult {

	var b strings.Builder
	truncated := false

	switch mode {
	case "files_with_matches":
		sort.Strings(fileMatches)
		count := len(fileMatches)
		if count == 0 {
			return ToolResult{Ok: true, Content: fmt.Sprintf("No matches found (%d files scanned).", scanned)}
		}
		if count > headLimit {
			fileMatches = fileMatches[:headLimit]
			truncated = true
		}
		for _, p := range fileMatches {
			b.WriteString(p)
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "\n[%d matching files, %d scanned]", count, scanned)
	case "count":
		sort.Slice(fileCounts, func(i, j int) bool { return fileCounts[i].Path < fileCounts[j].Path })
		if len(fileCounts) == 0 {
			return ToolResult{Ok: true, Content: fmt.Sprintf("No matches found (%d files scanned).", scanned)}
		}
		total, shown := 0, 0
		for _, fc := range fileCounts {
			total += fc.Count
			if shown >= headLimit {
				truncated = true
				continue
			}
			fmt.Fprintf(&b, "%s: %d\n", fc.Path, fc.Count)
			shown++
		}
		fmt.Fprintf(&b, "\n[%d matches in %d files, %d scanned]", total, len(fileCounts), scanned)
	default:
		if len(contentMatches) == 0 {
			return ToolResult{Ok: true, Content: fmt.Sprintf("No matches found (%d files scanned).", scanned)}
		}
		for i, m := range contentMatches {
			if i >= headLimit {
				truncated = true
				break
			}
			fmt.Fprintf(&b, "%s:%d: %s\n", m.Path, m.Line, m.Text)
		}
		fmt.Fprintf(&b, "\n[%d matching lines, %d files scanned]", len(contentMatches), scanned)
	}

	if truncated {
		fmt.Fprintf(&b, "\n[truncated: showing first %d results; narrow the pattern/glob or raise head_limit]", headLimit)
	}
	return ToolResult{Ok: true, Content: strings.TrimRight(b.String(), "\n")}
}
