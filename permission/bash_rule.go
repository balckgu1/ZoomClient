package permission

import "strings"

// dangerousBashSubstrings Dangerous bash keywords to be directly banned
// Any match will immediately reject the run_bash call
var dangerousBashSubstrings = []string{
	"sudo ",
	"rm -rf /",
	"rm -rf /*",
	"mkfs",
	"shutdown",
	"reboot",
	"> /dev/sda",
	":(){:|:&};:", // Classic fork bomb
}

// suspiciousBashSubstrings Suspicious shell meta-characters
var suspiciousBashSubstrings = []string{
	"$(",
	"`",
	"> /dev/",
}

// isDangerousBash Determine whether a bash command should be rejected
func isDangerousBash(command string) (bool, string) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return false, ""
	}
	lowered := strings.ToLower(cmd)

	for _, key := range dangerousBashSubstrings {
		if strings.Contains(lowered, key) {
			return true, "dangerous bash keyword: " + key
		}
	}
	for _, key := range suspiciousBashSubstrings {
		if strings.Contains(cmd, key) {
			return true, "suspicious bash metachar: " + key
		}
	}
	return false, ""
}
