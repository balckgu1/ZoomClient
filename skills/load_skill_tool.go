package skills

import (
	"fmt"
	"strings"
	"zoomClient/tools"
)

type LoadSkillTool struct {
	registry *SkillRegistry
}

func NewLoadSkillTool(registry *SkillRegistry) *LoadSkillTool {
	return &LoadSkillTool{
		registry: registry,
	}
}

func (t *LoadSkillTool) Name() string {
	return "load_skill"
}

func (t *LoadSkillTool) Description() string {
	return "Load the full body of an optional skill (a reusable playbook for a category of tasks) into the current context. " +
		"Call this only when the current task actually needs the playbook; the skill directory is already listed in the system prompt. " +
		"Returns the full skill text wrapped in <skill name=\"...\">...</skill>."
}

func (t *LoadSkillTool) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The name of the skill to load.  Must be one of the skills listed in the system prompt's skill directory.",
			},
		},
		"required": []string{"name"},
	}
	return parameters
}

func (t *LoadSkillTool) Call(args map[string]any, _ *tools.ToolContext) tools.ToolResult {
	nameRaw, exists := args["name"]
	if !exists {
		return tools.ToolResult{Ok: false, Content: "Error: missing name parameter", IsError: true}
	}
	name, ok := nameRaw.(string)
	if !ok || strings.TrimSpace(name) == "" {
		return tools.ToolResult{Ok: false, Content: "Error: name parameter must be a non-empty string", IsError: true}
	}
	skillBody, err := t.registry.LoadFullText(strings.TrimSpace(name))
	if err != nil {
		// 进行模糊匹配
		suggestion := fuzzyMatch(strings.TrimSpace(name), t.registry.Names())
		errMsg := fmt.Sprintf("Error: %v.", err)
		if suggestion != "" {
			errMsg += fmt.Sprintf(" Did you mean: %q?", suggestion)
		} else {
			// 完全没有相似项时，给模型显示所有 skill 列表
			available := strings.Join(t.registry.Names(), ", ")
			errMsg += fmt.Sprintf(" Available skills: [%s]", available)
		}
		return tools.ToolResult{
			Ok:      false,
			Content: errMsg,
			IsError: true,
		}
	}
	return tools.ToolResult{Ok: true, Content: skillBody, IsError: false}
}

// fuzzyMatch 在候选列表中查找与 target 最相似的名称
func fuzzyMatch(target string, candidates []string) string {
	targetLower := strings.ToLower(target)

	var bestMatch string
	bestDist := 4 // 最大允许编辑距离为 3

	for _, candidate := range candidates {
		candidateLower := strings.ToLower(candidate)

		// 精确匹配或前缀匹配具有最高优先级
		if candidateLower == targetLower || strings.HasPrefix(candidateLower, targetLower) {
			return candidate
		}

		// 计算LevenShtein距离
		dist := levenshteinDistance(targetLower, candidateLower)
		if dist < bestDist {
			bestDist = dist
			bestMatch = candidate
		}
	}

	return bestMatch
}

// levenshteinDistance 计算两个字符串的编辑距离
func levenshteinDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	if la > lb {
		a, b = b, a
		la, lb = lb, la
	}

	prevRow := make([]int, la+1)
	currRow := make([]int, la+1)
	for i := 0; i <= la; i++ {
		prevRow[i] = i
	}

	for j := 1; j <= lb; j++ {
		currRow[0] = j
		for i := 1; i <= la; i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prevRow[i] + 1
			ins := currRow[i-1] + 1
			sub := prevRow[i-1] + cost

			// 取三者最小值
			minVal := del
			if ins < minVal {
				minVal = ins
			}
			if sub < minVal {
				minVal = sub
			}
			currRow[i] = minVal
		}
		prevRow, currRow = currRow, prevRow
	}
	return prevRow[la]
}
