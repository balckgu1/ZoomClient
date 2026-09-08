package skills

// SkillManifest skill 的 frontmatter
//
// 同时带 yaml 与 json 标签：yaml 用于解析 SKILL.md 头部，
// json 用于 Web 端 /api/skills 把元信息作为技能目录直接返回给前端。
type SkillManifest struct {
	Name          string `yaml:"name" json:"name"`
	Description   string `yaml:"description" json:"description"`
	Version       string `yaml:"version" json:"version"`
	Author        string `yaml:"author" json:"author"`
	Compatibility string `yaml:"compatibility" json:"compatibility"`
}

// SkillDocument skill 完整内容
type SkillDocument struct {
	Manifest SkillManifest // 元信息
	Body     string        // SKILL.md 去掉 frontmatter 后的正文
	Path     string        // 源文件路径
}
