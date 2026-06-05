package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"go.uber.org/zap"
)

type GlobSearch struct{}

func (g GlobSearch) Name() string { return "glob_search" }

func (g GlobSearch) Description() string {
	return "Search for files by glob pattern. Supports ** for recursive directory matching, " +
		"* for single-segment wildcard, ? for single character, and [...] for character classes. " +
		"Examples: **/*.go, src/**/*.py, *.txt, test_*.py"
}

func (g GlobSearch) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match file paths. Use ** for recursive matching across directories.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Base directory to search in, relative to work directory. Defaults to work directory root.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return. Defaults to 200.",
			},
		},
		"required": []string{"pattern"},
	}
	return parameters
}

func (g GlobSearch) Call(args map[string]any, toolCtx *ToolContext) ToolResult {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return ToolResult{Ok: false, Content: "Error: missing pattern parameter or pattern parameter is not a string", IsError: true}
	}
	baseDir := toolCtx.WorkPath
	if pathArg, ok := args["path"].(string); ok && pathArg != "" {
		resolved, err := isSafePath(toolCtx.WorkPath, pathArg)
		if err != nil {
			return ToolResult{Ok: false, Content: fmt.Sprintf("Error: %v", err), IsError: true}
		}
		baseDir = resolved
	}

	maxResults := 200
	if maxArg, ok := args["max_results"].(float64); ok && maxArg > 0 {
		maxResults = int(maxArg)
	}

	toolCtx.Logger.Info("Glob search",
		zap.String("session", toolCtx.SessionID),
		zap.String("pattern", pattern),
		zap.String("base", baseDir),
		zap.Int("max_results", maxResults),
	)

	// 统一将 pattern 转为正斜杠，doublestar 默认使用 / 作为路径分隔符
	normalizedPattern := filepath.ToSlash(pattern)

	var matches []string
	truncated := false

	err := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// 跳过隐藏目录（以 . 开头的目录）
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(baseDir, path)
		if relErr != nil {
			return nil
		}
		// 统一转为正斜杠，确保跨平台一致性
		relSlash := filepath.ToSlash(relPath)

		// 使用 doublestar.Match 进行模式匹配，原生支持 ** 语法
		matched, matchErr := doublestar.Match(normalizedPattern, relSlash)
		if matchErr != nil {
			return nil // 跳过无效模式匹配
		}

		if matched {
			matches = append(matches, relSlash)
			if len(matches) >= maxResults {
				truncated = true
				return filepath.SkipAll
			}
		}

		return nil
	})

	if err != nil {
		return ToolResult{Ok: false, Content: fmt.Sprintf("Error: walk failed: %v", err), IsError: true}
	}

	if len(matches) == 0 {
		return ToolResult{
			Ok:      true,
			Content: fmt.Sprintf("No files matched pattern: %s", pattern),
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d file(s)", len(matches)))
	if truncated {
		sb.WriteString(fmt.Sprintf(" (truncated at %d)", maxResults))
	}
	sb.WriteString(":\n")
	for _, m := range matches {
		sb.WriteString(m)
		sb.WriteByte('\n')
	}

	return ToolResult{Ok: true, Content: sb.String()}
}
