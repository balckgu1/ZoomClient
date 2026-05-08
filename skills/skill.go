package skills

// SkillManifest skill 的轻量元信息(目录层只展示它), 用于让模型知道: 有这份 skill 存在，大概是干什么用的。
type SkillManifest struct {
	Name        string // skill 名称
	Description string // skill 描述
}

// SkillDocument 是 skill 的完整内容（按需加载时才会进入上下文）。
type SkillDocument struct {
	Manifest SkillManifest // 元信息
	Body     string        // SKILL.md 去掉 frontmatter 后的正文
	Path     string        // 源文件路径，便于调试与日志
}
