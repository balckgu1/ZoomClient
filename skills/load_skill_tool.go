package skills

import (
	"fmt"
	"strings"
	"zoomClient/tools"
)

// LoadSkillTool
type LoadSkillTool struct {
	registry *SkillRegistry
}

// NewLoadSkillTool LoadSkillTool的构造方法
func NewLoadSkillTool(registry *SkillRegistry) *LoadSkillTool {
	return &LoadSkillTool{
		registry: registry,
	}
}

// Name 返回工具的名称
func (t *LoadSkillTool) Name() string {
	return "load_skill"
}

// Description 返回工具的描述
func (t *LoadSkillTool) Description() string {
	return "Load the full body of an optional skill (a reusable playbook for a category of tasks) into the current context. " +
		"Call this only when the current task actually needs the playbook; the skill directory is already listed in the system prompt. " +
		"Returns the full skill text wrapped in <skill name=\"...\">...</skill>."
}

// Parameters 返回工具的参数
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

// Call 调用工具
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
		// 附上可用列表，帮模型自我纠正
		available := strings.Join(t.registry.Names(), ", ")
		return tools.ToolResult{
			Ok:      false,
			Content: fmt.Sprintf("Error: %v. Available skills: [%s]", err, available),
			IsError: true,
		}
	}
	return tools.ToolResult{Ok: true, Content: skillBody, IsError: false}
}
