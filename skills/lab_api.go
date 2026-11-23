package skills

// 本文件是「Skill 投毒防护实验台」(skilllab) 专用的只读适配层。
//
// 设计原则：
//  1. 只做「导出包装」，不修改任何既有逻辑 —— 实验台调用到的仍是生产代码里的
//     vetSkill / sanitizeDescription / highestSeverity / parseFrontmatter，
//     因此实验观察到的是真实防线行为，而不是另写一份的模拟实现；
//  2. 生产运行路径（main.go / repl.go / web/）不引用本文件中的任何函数，
//     新增它不会改变线上行为；
//  3. 所有函数均为只读：不写文件、不修改注册表内部状态。

// MaxDescriptionLen 是 description 进入 system prompt 前的截断阈值（按 rune 计）。
// 暴露给实验台，便于界面上直接展示「超过多少字会被截断」。
const MaxDescriptionLen = maxDescriptionLen

// ScanSkill 对给定元信息与正文执行静态红旗扫描。
// 内部复用生产逻辑 vetSkill，因此返回结果与真实加载流程完全一致
// （包括「description 命中的 info 级规则自动升级为 warn」这条升级规则）。
func ScanSkill(manifest SkillManifest, body string) []RedFlag {
	return vetSkill(manifest, body)
}

// MaxSeverity 返回红旗列表中的最高严重级别。
// 列表为空时返回 FlagInfo（即「无红旗」）。
func MaxSeverity(flags []RedFlag) int {
	return highestSeverity(flags)
}

// SanitizeDescription 复用生产逻辑净化 description：
// 去换行 / 折叠连续空白 / 截断到 MaxDescriptionLen，
// 防止投毒者用换行在 system prompt 的目录段里伪造新条目或注入独立指令行。
func SanitizeDescription(desc string) string {
	return sanitizeDescription(desc)
}

// ParseSkillContent 解析一段 SKILL.md 文本，返回元信息与正文。
// 供实验台在「不落盘」的情况下预演 frontmatter 的解析结果。
func ParseSkillContent(raw string) (SkillManifest, string, error) {
	return parseFrontmatter(raw)
}

// VetRuleStats 返回静态扫描规则表的规模（规则组数、匹配模式总数），供实验台展示。
// 新模式表里每条规则可能带字面量、正则、词族交集三种匹配方式，这里一并计数。
func VetRuleStats() (groups int, patterns int) {
	for _, g := range skillVetRules {
		groups++
		patterns += len(g.Literals) + len(g.Regexps) + len(g.AllOf)
	}
	return groups, patterns
}

// NormalizeForScan 返回「归一化 + 压缩」后的文本（仅保留字母数字）。
// 供实验台直观展示：插入空格、零宽字符、全角字符的变体在归一化后会被折叠成同一形态，
// 因此靠这些手法绕不过基于压缩串的匹配。
func NormalizeForScan(s string) string {
	return compactForScan(s)
}

// ValidateSkillName 暴露 skill 名称校验，供实验台演示「目录注入型名称」被拒。
func ValidateSkillName(name string) error {
	return validateSkillName(name)
}

// Get 返回某个已注册 skill 的完整文档副本（含 VetFlags 静态扫描结果）。
// 生产代码不需要它，但实验台要展示「命中了哪些红旗」就必须拿到 VetFlags。
// 返回值是拷贝，调用方修改不会影响注册表内部状态。
func (r *SkillRegistry) Get(name string) (SkillDocument, bool) {
	doc, ok := r.skills[name]
	if !ok {
		return SkillDocument{}, false
	}
	return *doc, true
}
