package permission

import "testing"

// recordingAsker 测试用 Asker：记录是否被调用，并按预设结果返回
type recordingAsker struct {
	approve bool
	called  bool
}

func (a *recordingAsker) Ask(toolName string, args map[string]any, reason string) (bool, string) {
	a.called = true
	if a.approve {
		return true, ""
	}
	return false, "denied by test"
}

func newManagerWithMode(mode Mode, asker Asker) *Manager {
	return NewManager(mode, nil, nil, asker)
}

// ===================== Step 1: deny rules 优先级最高 =====================

// 即使 mode=auto + 工具是只读，命中 deny rule 也必须拒绝
func TestCheck_DenyRule_BeatsAutoMode(t *testing.T) {
	deny := []Rule{{Tool: "read_file", Behavior: BehaviorDeny, Path: "secret"}}
	m := NewManager(ModeAuto, deny, nil, AllowAsker{})

	got := m.Check("read_file", map[string]any{"filename": "secret.txt"})
	if got.Behavior != BehaviorDeny {
		t.Fatalf("deny rule should beat auto mode, got %v (%s)", got.Behavior, got.Reason)
	}
}

// 通配符工具的 deny rule 应能命中任意工具
func TestMatchesRule_WildcardTool(t *testing.T) {
	r := Rule{Tool: "*", Behavior: BehaviorDeny, Content: "danger"}
	if !matchesRule(r, "run_bash", map[string]any{"command": "danger me"}) {
		t.Fatal("wildcard tool should match any tool")
	}
	if !matchesRule(r, "write_file", map[string]any{"content": "danger zone"}) {
		t.Fatal("wildcard tool should also match write_file")
	}
}

// content 用 "re:" 前缀时按正则匹配
func TestMatchesRule_RegexContent(t *testing.T) {
	r := Rule{Tool: "run_bash", Behavior: BehaviorDeny, Content: "re:^git\\s+push"}
	if !matchesRule(r, "run_bash", map[string]any{"command": "git push origin main"}) {
		t.Fatal("regex content should match git push")
	}
	if matchesRule(r, "run_bash", map[string]any{"command": "git status"}) {
		t.Fatal("regex content should not match git status")
	}
}

// ===================== Step 2: 模式硬约束 =====================

func TestCheck_PlanMode_BlocksAllWriteTools(t *testing.T) {
	m := newManagerWithMode(ModePlan, AllowAsker{})
	for _, tool := range []string{"write_file", "edit_file", "run_bash", "sub_task"} {
		got := m.Check(tool, map[string]any{})
		if got.Behavior != BehaviorDeny {
			t.Errorf("plan mode should deny %s, got %v", tool, got.Behavior)
		}
	}
}

// plan 模式下读类工具应走 ask（不属于硬拒绝，但也不自动放行）
func TestCheck_PlanMode_AsksForRead(t *testing.T) {
	m := newManagerWithMode(ModePlan, AllowAsker{})
	got := m.Check("read_file", map[string]any{"filename": "a.txt"})
	if got.Behavior != BehaviorAsk {
		t.Fatalf("plan mode should ask for read_file, got %v", got)
	}
}

func TestCheck_AutoMode_AllowsReadOnly(t *testing.T) {
	m := newManagerWithMode(ModeAuto, DenyAsker{})
	for _, tool := range []string{"read_file", "load_skill", "todo", "compact"} {
		got := m.Check(tool, map[string]any{})
		if got.Behavior != BehaviorAllow {
			t.Errorf("auto mode should allow read-only tool %s, got %v", tool, got.Behavior)
		}
	}
}

// auto 模式下写类工具走 ask
func TestCheck_AutoMode_AsksForWrite(t *testing.T) {
	m := newManagerWithMode(ModeAuto, DenyAsker{})
	got := m.Check("write_file", map[string]any{"filename": "a.txt"})
	if got.Behavior != BehaviorAsk {
		t.Fatalf("auto mode should ask for write_file, got %v", got)
	}
}

// ===================== Step 3: bash 兜底安全检查 =====================

func TestCheck_RunBash_DangerousIsDeniedEvenWithoutRule(t *testing.T) {
	m := newManagerWithMode(ModeAuto, AllowAsker{})
	got := m.Check("run_bash", map[string]any{"command": "sudo apt update"})
	if got.Behavior != BehaviorDeny {
		t.Fatalf("dangerous bash must be denied, got %v (%s)", got.Behavior, got.Reason)
	}
}

func TestIsDangerousBash_RmRfRoot(t *testing.T) {
	if dangerous, _ := isDangerousBash("rm -rf /"); !dangerous {
		t.Errorf("`rm -rf /` should be dangerous")
	}
}

func TestIsDangerousBash_CommandSubstitution(t *testing.T) {
	if dangerous, _ := isDangerousBash("echo $(whoami)"); !dangerous {
		t.Errorf("$( ) should be flagged")
	}
}

func TestIsDangerousBash_Backtick(t *testing.T) {
	if dangerous, _ := isDangerousBash("echo `whoami`"); !dangerous {
		t.Errorf("backtick should be flagged")
	}
}

func TestIsDangerousBash_Normal(t *testing.T) {
	for _, cmd := range []string{"ls -la", "git status", "echo hello", "go test ./..."} {
		if dangerous, why := isDangerousBash(cmd); dangerous {
			t.Errorf("%q should be safe, got dangerous: %s", cmd, why)
		}
	}
}

// ===================== Step 4: allow rules =====================

// allow rule 应能让 default 模式下原本要 ask 的调用直接放行
func TestCheck_AllowRule_HitInDefaultMode(t *testing.T) {
	allow := []Rule{{Tool: "run_bash", Behavior: BehaviorAllow, Content: "git status"}}
	m := NewManager(ModeDefault, nil, allow, DenyAsker{})

	got := m.Check("run_bash", map[string]any{"command": "git status"})
	if got.Behavior != BehaviorAllow {
		t.Fatalf("allow rule should hit, got %v (%s)", got.Behavior, got.Reason)
	}
}

// allow rule 不能覆盖 plan 模式硬约束（mode 在 allow 之前）
func TestCheck_AllowRule_CannotOverridePlanMode(t *testing.T) {
	allow := []Rule{{Tool: "write_file", Behavior: BehaviorAllow}}
	m := NewManager(ModePlan, nil, allow, AllowAsker{})

	got := m.Check("write_file", map[string]any{"filename": "a.txt"})
	if got.Behavior != BehaviorDeny {
		t.Fatalf("plan mode hard-block should beat allow rule, got %v", got.Behavior)
	}
}

// ===================== Step 5: ask fallback =====================

func TestCheck_DefaultMode_AsksWhenUnknown(t *testing.T) {
	m := newManagerWithMode(ModeDefault, DenyAsker{})
	got := m.Check("read_file", map[string]any{"filename": "a.txt"})
	if got.Behavior != BehaviorAsk {
		t.Fatalf("default mode should ask, got %v", got.Behavior)
	}
}

// ===================== Decide 整合：把 Check + Asker 串成 (allow, reason) =====================

func TestDecide_AllowDirectly(t *testing.T) {
	m := newManagerWithMode(ModeAuto, DenyAsker{})

	allow, _ := m.Decide("read_file", map[string]any{"filename": "a.txt"})
	if !allow {
		t.Fatal("auto mode should directly allow read_file without asking")
	}
}

func TestDecide_DenyDirectly(t *testing.T) {
	asker := &recordingAsker{approve: true} // 即使 asker 想放行，也不该被调用
	m := newManagerWithMode(ModePlan, asker)

	allow, _ := m.Decide("write_file", map[string]any{"filename": "a.txt"})
	if allow {
		t.Fatal("plan mode should directly deny write_file")
	}
	if asker.called {
		t.Fatal("asker should NOT be called when decision is deny")
	}
}

func TestDecide_AskBranch_Approve(t *testing.T) {
	asker := &recordingAsker{approve: true}
	m := NewManager(ModeDefault, nil, nil, asker)

	allow, _ := m.Decide("read_file", map[string]any{"filename": "a.txt"})
	if !allow {
		t.Fatal("expected allow when asker approves")
	}
	if !asker.called {
		t.Fatal("asker should have been called")
	}
}

func TestDecide_AskBranch_Reject(t *testing.T) {
	asker := &recordingAsker{approve: false}
	m := NewManager(ModeDefault, nil, nil, asker)

	allow, reason := m.Decide("read_file", map[string]any{"filename": "a.txt"})
	if allow {
		t.Fatal("expected deny when asker rejects")
	}
	if reason == "" {
		t.Fatal("reason should not be empty when denied")
	}
	if !asker.called {
		t.Fatal("asker should have been called")
	}
}

// ===================== Manager 构造与 mode 兜底 =====================

func TestNewManager_NilAskerFallsBackToDeny(t *testing.T) {
	m := NewManager(ModeDefault, nil, nil, nil)
	if _, ok := m.Asker.(DenyAsker); !ok {
		t.Fatalf("nil asker should fall back to DenyAsker, got %T", m.Asker)
	}
}

func TestSetMode_InvalidFallsBackToDefault(t *testing.T) {
	m := NewManager(Mode("garbage"), nil, nil, DenyAsker{})
	if m.GetMode() != ModeDefault {
		t.Fatalf("invalid mode should fall back to default, got %s", m.GetMode())
	}
}
