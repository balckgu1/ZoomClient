package main

import "testing"

// TestParseFlagsOutputMode 覆盖 -mode 与 -web 别名的优先级规则。
func TestParseFlagsOutputMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"默认 CLI 模式", nil, "cli"},
		{"显式 -mode web", []string{"-mode", "web"}, "web"},
		{"-web 别名等价 -mode web", []string{"-web"}, "web"},
		{"-web 与 -m 同时使用", []string{"-web", "-m", "ollama"}, "web"},
		{"显式 -mode 优先于 -web", []string{"-web", "-mode", "cli"}, "cli"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseFlagsFrom(tt.args).OutputMode; got != tt.want {
				t.Fatalf("OutputMode = %q, 期望 %q", got, tt.want)
			}
		})
	}
}

// TestParseFlagsValues 覆盖其余标志的解析，避免别名改动影响既有参数。
func TestParseFlagsValues(t *testing.T) {
	f := parseFlagsFrom([]string{"-m", "anthropic", "-port", "9001", "-workdir", "/tmp/work", "-config-dir", "/tmp/cfg"})
	if f.ModelType != "anthropic" {
		t.Fatalf("ModelType = %q, 期望 anthropic", f.ModelType)
	}
	if f.WebPort != 9001 {
		t.Fatalf("WebPort = %d, 期望 9001", f.WebPort)
	}
	if f.WorkDir != "/tmp/work" {
		t.Fatalf("WorkDir = %q, 期望 /tmp/work", f.WorkDir)
	}
	if f.ConfigDir != "/tmp/cfg" {
		t.Fatalf("ConfigDir = %q, 期望 /tmp/cfg", f.ConfigDir)
	}
}
