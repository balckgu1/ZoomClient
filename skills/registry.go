package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SkillRegistry struct {
	skillsDir string
	skills    map[string]*SkillDocument
}

// NewRegistry 扫描 skillsDir 下所有 SKILL.md 并构建SkillRegistry注册表。
// 目录不存在或为空时不会报错，而是返回一个空注册表。
func NewRegistry(skillsDir string) (*SkillRegistry, error) {
	reg := &SkillRegistry{
		skillsDir: skillsDir,
		skills:    make(map[string]*SkillDocument),
	}
	if skillsDir == "" {
		return reg, nil
	}
	info, err := os.Stat(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// 目录不存在视为无可用 skill，保持空注册表
			return reg, nil
		}
		return nil, fmt.Errorf("stat skills dir %q failed: %w", skillsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills path %q is not a directory", skillsDir)
	}

	err = reg.loadAll()
	if err != nil {
		return nil, err
	}
	return reg, nil
}

// loadAll 遍历 skillsDir 目录，递归加载所有 SKILL.md 文件到注册表中。
func (r *SkillRegistry) loadAll() error {
	return filepath.WalkDir(r.skillsDir, r.walkFunc)
}

// walkFunc 是 filepath.WalkDir 的回调函数，处理单个路径节点。
func (r *SkillRegistry) walkFunc(path string, d os.DirEntry, err error) error {
	// 1. 处理系统错误或权限问题
	if err != nil {
		return err
	}

	// 2. 过滤：跳过目录
	if d.IsDir() {
		return nil
	}

	// 3. 过滤：只处理名为 "SKILL.md" 的文件（忽略大小写）
	if !strings.EqualFold(d.Name(), "SKILL.md") {
		return nil
	}

	// 4. 加载并解析技能文件
	skillDoc, parseErr := r.parseSkillFile(path)
	if parseErr != nil {
		return parseErr
	}

	// 5. 注册到内存 Map 中
	r.skills[skillDoc.Manifest.Name] = skillDoc
	return nil
}

// parseSkillFile 负责读取文件、解析 Frontmatter 并构建 SkillDocument 对象。
func (r *SkillRegistry) parseSkillFile(path string) (*SkillDocument, error) {
	// 读取文件内容
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill %q failed: %w", path, err)
	}

	// 解析元数据和正文
	meta, body := parseFrontmatter(string(raw))

	// 确定技能名称：优先使用 frontmatter 中的 name，否则回退到目录名
	name := meta["name"]
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}

	// 构建并返回文档对象
	return &SkillDocument{
		Manifest: SkillManifest{
			Name:        name,
			Description: meta["description"],
		},
		Body: body,
		Path: path,
	}, nil
}

// Names 返回所有已加载 skill 的名称（按字典序，便于展示稳定）
func (r *SkillRegistry) Names() []string {
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DescribeAvailable 生成适合塞入 system prompt 的目录文本。
// 只包含名称与一句描述，保持轻量；若没有任何 skill，返回空字符串。
func (r *SkillRegistry) DescribeAvailable() string {
	if len(r.skills) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range r.Names() {
		skillDoc := r.skills[name]
		skillDescription := skillDoc.Manifest.Description
		if skillDescription == "" {
			skillDescription = "No description provided."
		}
		fmt.Fprintf(&b, "- %s: %s\n", name, skillDescription)
	}
	return strings.TrimRight(b.String(), "\n")
}

// LoadFullText 返回某个 skill 的完整正文，用工具结果的典型包裹形式。
// 调用方（如 LoadSkillTool）可以把它直接作为 tool_result 返回。
func (r *SkillRegistry) LoadFullText(name string) (string, error) {
	doc, ok := r.skills[name]
	if !ok {
		return "", fmt.Errorf("skill %q not found", name)
	}
	// 按文档示例用 <skill> 标签包裹，便于模型感知正文边界
	return fmt.Sprintf("<skill name=%q>\n%s\n</skill>", doc.Manifest.Name, doc.Body), nil
}

// Count 当前加载了多少 skill，主要给日志/诊断用
func (r *SkillRegistry) Count() int {
	return len(r.skills)
}
