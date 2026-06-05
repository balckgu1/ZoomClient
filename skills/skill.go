package skills

// SkillManifest skill 的 frontmatter
type SkillManifest struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	Version       string `yaml:"version"`
	Author        string `yaml:"author"`
	Compatibility string `yaml:"compatibility"`
}

// SkillDocument skill 包含frontmatter的所有完整内容
type SkillDocument struct {
	Manifest SkillManifest // 元信息
	Body     string        // SKILL.md 去掉 frontmatter 后的正文
	Path     string        // 源文件路径
}
