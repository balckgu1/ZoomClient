// web/handlers_test.go
//
// 针对 Web 后端新增能力（工作目录切换、权限配置运行时管理）的单元测试。
// 使用 httptest 直接驱动内部 mux，验证请求路由、状态码与 JSON 响应体。
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zoomClient/compact"
	"zoomClient/permission"
	"zoomClient/skills"
	"zoomClient/utils"
)

// newTestServer 构造一个仅包含被测处理器所需依赖的 Server。
// 这些测试不触及模型/会话持久化，故对应依赖可留空。
func newTestServer(t *testing.T, mode permission.Mode, deny, allow []permission.Rule) *Server {
	t.Helper()
	sess := NewSession("test-session", "test-model")
	permMgr := permission.NewManager(mode, deny, allow, permission.DenyAsker{})
	deps := ServerDeps{
		Session:       sess,
		PermissionMgr: permMgr,
		Config:        &utils.Config{},
	}
	return NewServer(deps, 0)
}

// doJSON 发起一个带 JSON body 的请求并返回响应记录器。
func doJSON(t *testing.T, srv *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	return rec
}

// ─── 权限配置 ───

func TestHandlePermissionConfig_GET(t *testing.T) {
	deny := []permission.Rule{{Tool: "run_bash", Behavior: permission.BehaviorDeny, Content: "sudo "}}
	allow := []permission.Rule{{Tool: "run_bash", Behavior: permission.BehaviorAllow, Content: "git status"}}
	srv := newTestServer(t, permission.ModeAuto, deny, allow)

	rec := doJSON(t, srv, http.MethodGet, "/api/permission/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp permissionConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.Mode != permission.ModeAuto {
		t.Errorf("expected mode auto, got %s", resp.Mode)
	}
	if len(resp.DenyRules) != 1 || resp.DenyRules[0].Content != "sudo " {
		t.Errorf("unexpected deny rules: %+v", resp.DenyRules)
	}
	if len(resp.AllowRules) != 1 || resp.AllowRules[0].Content != "git status" {
		t.Errorf("unexpected allow rules: %+v", resp.AllowRules)
	}
}

func TestHandlePermissionConfig_PUT_UpdatesModeAndRules(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)

	body := `{
		"mode": "plan",
		"deny_rules": [{"tool":"run_bash","content":"rm -rf /"}],
		"allow_rules": [{"tool":"read_file","path":"src/"}]
	}`
	rec := doJSON(t, srv, http.MethodPut, "/api/permission/config", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	// 校验管理器内部状态已被更新
	if got := srv.permissionMgr.GetMode(); got != permission.ModePlan {
		t.Errorf("expected mode plan, got %s", got)
	}
	snap := srv.permissionMgr.Snapshot()
	if len(snap.DenyRules) != 1 || snap.DenyRules[0].Content != "rm -rf /" {
		t.Errorf("deny rules not updated: %+v", snap.DenyRules)
	}
	if len(snap.AllowRules) != 1 || snap.AllowRules[0].Path != "src/" {
		t.Errorf("allow rules not updated: %+v", snap.AllowRules)
	}

	// 校验响应体回显最新快照
	var resp permissionConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.Mode != permission.ModePlan || len(resp.DenyRules) != 1 || len(resp.AllowRules) != 1 {
		t.Errorf("response did not echo updated snapshot: %+v", resp)
	}
}

func TestHandlePermissionConfig_PUT_NormalizesBehavior(t *testing.T) {
	srv := newTestServer(t, permission.ModeDefault, nil, nil)

	// 前端误传 behavior=allow 到 deny 列表，应被强制规范化为 deny
	body := `{"mode":"default","deny_rules":[{"tool":"run_bash","behavior":"allow","content":"mkfs"}]}`
	rec := doJSON(t, srv, http.MethodPut, "/api/permission/config", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	snap := srv.permissionMgr.Snapshot()
	if len(snap.DenyRules) != 1 || snap.DenyRules[0].Behavior != permission.BehaviorDeny {
		t.Errorf("deny rule behavior not normalized: %+v", snap.DenyRules)
	}
}

func TestHandlePermissionConfig_PUT_EmptyModeKeepsCurrent(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)

	// mode 为空字符串时应保持原模式不变
	rec := doJSON(t, srv, http.MethodPut, "/api/permission/config", `{"mode":"","deny_rules":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := srv.permissionMgr.GetMode(); got != permission.ModeAuto {
		t.Errorf("expected mode to remain auto, got %s", got)
	}
}

// 前端切换到 root（完全访问）模式时，PUT 必须真实生效而不是被 SetMode 回退
func TestHandlePermissionConfig_PUT_RootMode(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)

	rec := doJSON(t, srv, http.MethodPut, "/api/permission/config", `{"mode":"root","deny_rules":[],"allow_rules":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if got := srv.permissionMgr.GetMode(); got != permission.ModeRoot {
		t.Errorf("expected mode root, got %s", got)
	}
}

func TestHandlePermissionConfig_PUT_InvalidJSON(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)
	rec := doJSON(t, srv, http.MethodPut, "/api/permission/config", `{invalid`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", rec.Code)
	}
}

func TestHandlePermissionConfig_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)
	rec := doJSON(t, srv, http.MethodDelete, "/api/permission/config", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// ─── 工作目录 ───

func TestHandleWorkDir_GET(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)
	srv.session.SetWorkDir("/tmp/project")

	rec := doJSON(t, srv, http.MethodGet, "/api/workdir", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Path != "/tmp/project" {
		t.Errorf("expected /tmp/project, got %s", resp.Path)
	}
}

func TestHandleWorkDir_POST_EnqueuesCommand(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)

	rec := doJSON(t, srv, http.MethodPost, "/api/workdir", `{"path":"  /tmp/newdir  "}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	// 校验命令已投递到 CmdCh，且路径已去除首尾空白
	select {
	case cmd := <-srv.session.CmdCh:
		if cmd.Action != "set_workdir" {
			t.Errorf("expected action set_workdir, got %s", cmd.Action)
		}
		if cmd.WorkDir != "/tmp/newdir" {
			t.Errorf("expected trimmed path /tmp/newdir, got %q", cmd.WorkDir)
		}
	default:
		t.Fatal("expected a command in CmdCh, but channel was empty")
	}
}

func TestHandleWorkDir_POST_EmptyPath(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)
	rec := doJSON(t, srv, http.MethodPost, "/api/workdir", `{"path":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty path, got %d", rec.Code)
	}
}

func TestHandleWorkDir_POST_InvalidJSON(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)
	rec := doJSON(t, srv, http.MethodPost, "/api/workdir", `{invalid`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ─── 状态查询 ───

func TestHandleStatus_IncludesWorkDirAndPermissionMode(t *testing.T) {
	srv := newTestServer(t, permission.ModePlan, nil, nil)
	srv.session.SetWorkDir("/tmp/wd")

	rec := doJSON(t, srv, http.MethodGet, "/api/status", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["workdir"] != "/tmp/wd" {
		t.Errorf("expected workdir /tmp/wd, got %v", resp["workdir"])
	}
	if resp["permission_mode"] != string(permission.ModePlan) {
		t.Errorf("expected permission_mode plan, got %v", resp["permission_mode"])
	}
}

// ─── 上下文占用 ───

// TestHandleContextUsage_ZeroCacheByDefault 未推送过快照时返回零值快照（前端据此显示占位）。
func TestHandleContextUsage_ZeroCacheByDefault(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)

	rec := doJSON(t, srv, http.MethodGet, "/api/context-usage", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp compact.UsageSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.LimitBytes != 0 || resp.TotalBytes != 0 {
		t.Errorf("expected zero snapshot, got %+v", resp)
	}
}

// TestHandleContextUsage_ReturnsCachedSnapshot 推送缓存后 GET 应回读一致快照。
func TestHandleContextUsage_ReturnsCachedSnapshot(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)
	want := compact.UsageSnapshot{
		LimitBytes: 60000, TotalBytes: 1234,
		SystemPromptBytes: 100, SkillsBytes: 20, SkillsCount: 2,
		ToolsBytes: 300, MessagesBytes: 814,
	}
	srv.session.SetContextUsage(want)

	rec := doJSON(t, srv, http.MethodGet, "/api/context-usage", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp compact.UsageSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp != want {
		t.Errorf("expected cached snapshot %+v, got %+v", want, resp)
	}
}

// TestHandleContextUsage_MethodNotAllowed 非 GET 请求应返回 405。
func TestHandleContextUsage_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t, permission.ModeAuto, nil, nil)
	rec := doJSON(t, srv, http.MethodPost, "/api/context-usage", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// TestNormalizeRules 校验规则规范化纯函数。
func TestNormalizeRules(t *testing.T) {
	in := []permission.Rule{
		{Tool: "  run_bash ", Behavior: permission.BehaviorAllow, Path: " a ", Content: " b "},
	}
	out := normalizeRules(in, permission.BehaviorDeny)
	if len(out) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(out))
	}
	r := out[0]
	if r.Behavior != permission.BehaviorDeny {
		t.Errorf("expected behavior forced to deny, got %s", r.Behavior)
	}
	if r.Tool != "run_bash" || r.Path != "a" || r.Content != "b" {
		t.Errorf("expected trimmed fields, got %+v", r)
	}
	// 确认未修改原始输入
	if in[0].Behavior != permission.BehaviorAllow {
		t.Errorf("normalizeRules mutated its input")
	}
}

// ─── 技能目录 ───

// writeSkill 在指定目录下生成一个带 frontmatter 的 SKILL.md，供测试构造真实注册表。
func writeSkill(t *testing.T, dir, folder, name, description string) {
	t.Helper()
	skillDir := filepath.Join(dir, folder)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("create skill dir failed: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\nbody\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md failed: %v", err)
	}
}

// TestHandleSkills_ListsLoadedSkills 已加载的 skill 应按名称排序完整返回。
func TestHandleSkills_ListsLoadedSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "zeta", "zeta-tool", "Zeta 工具说明")
	writeSkill(t, dir, "alpha", "alpha-tool", "Alpha 工具说明")

	reg, err := skills.NewSkillRegistry(dir)
	if err != nil {
		t.Fatalf("build registry failed: %v", err)
	}
	srv := newTestServer(t, permission.ModeDefault, nil, nil)
	srv.skillRegistry = reg

	rec := doJSON(t, srv, http.MethodGet, "/api/skills", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp skillsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.Count != 2 || len(resp.Skills) != 2 {
		t.Fatalf("expected 2 skills, got count=%d len=%d", resp.Count, len(resp.Skills))
	}
	if resp.Skills[0].Name != "alpha-tool" || resp.Skills[1].Name != "zeta-tool" {
		t.Errorf("expected skills sorted by name, got %s / %s", resp.Skills[0].Name, resp.Skills[1].Name)
	}
	if resp.Skills[0].Description != "Alpha 工具说明" {
		t.Errorf("expected description returned, got %q", resp.Skills[0].Description)
	}
}

// TestHandleSkills_EmptyWhenRegistryMissing 注册表未注入时应返回空数组而非 null，
// 前端可直接遍历而无需额外判空。
func TestHandleSkills_EmptyWhenRegistryMissing(t *testing.T) {
	srv := newTestServer(t, permission.ModeDefault, nil, nil)

	rec := doJSON(t, srv, http.MethodGet, "/api/skills", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"skills":[]`) {
		t.Errorf("expected empty skills array, got body %s", rec.Body.String())
	}
	var resp skillsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp.Count != 0 || len(resp.Skills) != 0 {
		t.Errorf("expected zero skills, got %+v", resp)
	}
}

// TestHandleSkills_MethodNotAllowed 非 GET 请求应返回 405。
func TestHandleSkills_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t, permission.ModeDefault, nil, nil)
	rec := doJSON(t, srv, http.MethodPost, "/api/skills", "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
