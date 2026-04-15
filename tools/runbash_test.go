package tools

import "testing"

func TestIsDangerousCommand_Normal(t *testing.T) {
	command := "ls"
	if isDangerousCommand(command) {
		t.Errorf("command %s should not be dangerous", command)
	}
}

func TestIsDangerousCommand_Dangerous(t *testing.T) {
	command := "rm -rf /"
	if !isDangerousCommand(command) {
		t.Errorf("command %s should be dangerous", command)
	}
}

func TestIsDangerousCommand_Mixed(t *testing.T) {
	command := "ls && rm -rf /"
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
	if param == nil {
		t.Fatalf("tool parameters should not be nil")
	}
	if param["type"] != "object" {
		t.Errorf("tool parameters type should be object")
	}

}
