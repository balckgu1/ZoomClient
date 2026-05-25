package tools

import "testing"

func TestIsDangerousCommand_Normal(t *testing.T) {
	command := "ls"
	if isDangerousCommand(command) {
		t.Errorf("command %s should not be dangerous", command)
	}
}

func TestIsDangerousCommand_Dangerous(t *testing.T) {
	command := "shutdown"
	if !isDangerousCommand(command) {
		t.Errorf("command %s should be dangerous", command)
	}
}

func TestIsDangerousCommand_Mixed(t *testing.T) {
	command := "echo hello && shutdown /s"
	if !isDangerousCommand(command) {
		t.Errorf("command %s should be dangerous", command)
	}
}

func TestRunBashTool_Name(t *testing.T) {
	tool := RunBashTool{}
	if tool.Name() != "run_bash" {
		t.Errorf("tool name should be run_bash")
	}
}

func TestRunBashTool_Description(t *testing.T) {
	tool := RunBashTool{}
	if tool.Description() == "" {
		t.Errorf("tool description should not be empty")
	}
}

func TestRunBashTool_Parameters(t *testing.T) {
	tool := RunBashTool{}
	param := tool.Parameters()
	// parameters := map[string]any{
	// 	"type": "object",
	// 	"properties": map[string]any{
	// 		"command": map[string]any{
	// 			"type":        "string",
	// 			"description": "Command to execute",
	// 		},
	// 	},
	// 	"required": []string{"command"},
	// }
	if param == nil {
		t.Fatalf("tool parameters should not be nil")
	}
	if param["type"] != "object" {
		t.Errorf("tool parameters type should be object")
	}
	if param["properties"] == nil {
		t.Errorf("tool parameters properties should not be nil")
	}
	if param["required"] == nil {
		t.Errorf("tool parameters required should not be nil")
	}
	if param["properties"].(map[string]any)["command"] == nil {
		t.Errorf("tool parameters properties command should not be nil")
	}
	command_require := []string{"type", "description"}
	command_param := param["properties"].(map[string]any)["command"]
	for _, value := range command_require {
		if _, ok := command_param.(map[string]any)[value]; !ok {
			t.Errorf("tool parameters properties command %s should exist", value)
		}
	}
	param_require := []string{"command"}
	for _, value := range param_require {
		exists := false
		for _, param := range param["required"].([]string) {
			if param == value {
				exists = true
				break
			}
		}
		if !exists {
			t.Errorf("tool parameters required %s should exist", value)
		}
	}
}

func TestRunBashTool_Call_Normal(t *testing.T) {
	workDir := t.TempDir()
	tool := RunBashTool{}
	args := map[string]any{
		"command": "echo hello",
	}
	if isDangerousCommand(args["command"].(string)) {
		t.Errorf("tool should not be dangerous")
	}
	toolCtx := newTestContext(workDir)
	result := tool.Call(args, toolCtx)
	if !result.Ok {
		t.Errorf("tool result should be ok")
	}
	if result.IsError {
		t.Errorf("tool result should not have error")
	}
}
