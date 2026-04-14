package tools

import (
	"os/exec"
	"strings"
)

type RunBashTool struct{}

func isDangerousCommand(command string) bool {
	dangerous := []string{"rm -rf /", "sudo", "shutdown", "reboot", "> /dev/"}
	for _, d := range dangerous {
		if strings.Contains(command, d) {
			return true
		}
	}
	return false
}

func (t RunBashTool) Name() string {
	return "run_bash"
}

func (t RunBashTool) Description() string {
	return "Open a bash and execute the given command. "
}

func (t RunBashTool) Parameters() map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Command to execute",
			},
		},
		"required": []string{"command"},
	}
	return parameters
}

func (t RunBashTool) Call(args map[string]any, workpath string) string {
	command, ok := args["command"].(string)
	if command == "" || !ok {
		return "Error: command parameter is missing or not a string"
	}
	if isDangerousCommand(command) {
		return "Error: dangerous command detected"
	}
	// Execute the command
	output, err := exec.Command("bash", "-c", command).CombinedOutput()
	if err != nil {
		return "Error: " + err.Error()
	}
	return string(output)
}
