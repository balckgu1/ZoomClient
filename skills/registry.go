package skills

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"zoomClient/logger"

	"go.uber.org/zap"
)

type SkillRegistry struct {
	skillsDir string
	skills    map[string]*SkillDocument
}

// NewRegistry 扫描 skillsDir 下所有 SKILL.md 并构建 SkillRegistry
func NewSkillRegistry(skillsDir string) (*SkillRegistry, error) {
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

// loadAll 遍历 skillsDir 目录，递归加载所有 SKILL.md 文件到 SkillRegistry
func (r *SkillRegistry) loadAll() error {
	return filepath.WalkDir(r.skillsDir, r.walkFunc)
}

// walkFunc 处理单个路径节点
func (r *SkillRegistry) walkFunc(path string, d os.DirEntry, err error) error {
	// 处理系统错误或权限问题
	if err != nil {
		return err
	}

	// 跳过目录
	if d.IsDir() {
		return nil
	}

	// 处理 SKILL.md
	if !strings.EqualFold(d.Name(), "SKILL.md") {
		return nil
	}

	// 加载并解析 skill.md
	skillDoc, parseErr := r.parseSkillFile(path)
	// 跳过无法解析的 skill.md
	if parseErr != nil {
		log := logger.Log
		log.Warn("The skill.md format is incorrect or corrupted", zap.String("skill_file", d.Name()), zap.Error(parseErr))
		return nil
	}

	// 注册到 Map 中
	existing, _ := r.skills[skillDoc.Manifest.Name]
	// 有冲突的skill，覆写并记录日志
	if existing != nil {
		log := logger.Log
		log.Warn("Skill name conflict, overwriting", zap.String("skill_name", skillDoc.Manifest.Name),
			zap.String("existing_path", existing.Path),
			zap.String("new_path", path))
	}
	r.skills[skillDoc.Manifest.Name] = skillDoc
	return nil
}

// parseSkillFile 负责读取文件, 解析 Frontmatter 并构建 SkillDocument 对象
func (r *SkillRegistry) parseSkillFile(path string) (*SkillDocument, error) {
	// 读取文件内容
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill %q failed: %w", path, err)
	}

	// 解析 frontmatter 和正文
	skillMeta, body, err := parseFrontmatter(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter in %q: %w", path, err)
	}

	// 确定技能名称：优先使用 frontmatter 中的 name，否则回退到目录名
	name := skillMeta.Name
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}

	// 构建并返回文档对象
	return &SkillDocument{
		Manifest: SkillManifest{
			Name:          name,
			Description:   skillMeta.Description,
			Version:       skillMeta.Version,
			Author:        skillMeta.Author,
			Compatibility: skillMeta.Compatibility,
		},
		Body: body,
		Path: path,
	}, nil
}

// Names 返回所有已加载 skill 的名称
func (r *SkillRegistry) Names() []string {
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DescribeAvailable 生成适合塞入 system prompt 的目录文本
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

// LoadFullText 返回某个 skill 的完整正文
//
// 输出格式：<skill name="xxx" basedir="/absolute/path/to/skill/dir">
//
//	... body ...
//	</skill>
func (r *SkillRegistry) LoadFullText(name string) (string, error) {
	doc, ok := r.skills[name]
	if !ok {
		return "", fmt.Errorf("skill %q not found", name)
	}

	// 计算 SKILL.md 所在目录
	basedir := filepath.Dir(doc.Path)

	// 用 <skill> 包裹
	type skillWrapper struct {
		XMLName xml.Name `xml:"skill"`
		Name    string   `xml:"name,attr"`
		BaseDir string   `xml:"basedir,attr"`
		Body    string   `xml:",chardata"`
	}

	wrapper := skillWrapper{
		Name:    doc.Manifest.Name,
		BaseDir: basedir,
		Body:    doc.Body,
	}

	output, err := xml.Marshal(wrapper)
	if err != nil {
		return "", fmt.Errorf("failed to marshal skill %q: %w", name, err)
	}

	return string(output), nil
}

// Count 当前加载 skill 数量
func (r *SkillRegistry) Count() int {
	return len(r.skills)
}
