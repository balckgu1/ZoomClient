// src/lib/slash.ts
//
// 输入框 "/" 扩展框的数据模型与匹配逻辑。
//
// 把"菜单里有什么"和"怎么过滤"抽成纯函数放在这里，而不是写进组件：
// 匹配规则涉及分组与打分，放进组件后既难读也无法单测。
import type { SkillMeta } from "../types";

// SlashItem 是扩展框里一行条目的统一形状。
// 内置命令与 skill 共用同一结构，SlashMenu 才能用一套渲染与键盘导航逻辑处理两类内容。
export interface SlashItem {
  // kind 决定选中后的行为：command 立即执行，skill 回填到输入框等待用户补充任务
  kind: "command" | "skill";
  // name 不含斜杠，用于过滤匹配与提交时拼装
  name: string;
  // hint 一句话说清"选中后会发生什么"，而不是复述名字
  hint: string;
  // meta 右侧弱化信息（skill 的版本号等），可为空
  meta?: string;
}

// WEB_COMMANDS —— Web 模式下真正可用的内置命令。
//
// 刻意不列 /models、/workspace、/session 等 CLI 专有命令：它们在 Web 模式没有对应处理分支，
// 列出来只会得到"未知命令"的提示。菜单里的每一项都必须点了就有用。
export const WEB_COMMANDS: SlashItem[] = [
  { kind: "command", name: "clear", hint: "清空当前会话的历史消息" },
  { kind: "command", name: "compact", hint: "把早期对话摘要压缩，腾出上下文空间" },
  { kind: "command", name: "exit", hint: "结束会话并关闭后端服务" },
];

// skillsToSlashItems 把后端技能目录转换为扩展框条目。
// meta 放版本号，没有版本信息时留空，避免界面上出现 "undefined"。
export function skillsToSlashItems(skills: SkillMeta[]): SlashItem[] {
  return skills.map((s) => ({
    kind: "skill",
    name: s.name,
    hint: s.description || "暂无说明",
    meta: s.version || "",
  }));
}

// buildSlashItems 组装完整菜单：命令在前、技能在后。
// 命令数量固定且少，放前面符合"常用在上"的阅读顺序。
export function buildSlashItems(skills: SkillMeta[]): SlashItem[] {
  return [...WEB_COMMANDS, ...skillsToSlashItems(skills)];
}

// scoreItem 给单个条目打分，越小越靠前；返回 -1 表示不匹配、应被过滤掉。
//
// 打分顺序体现了用户的真实意图强弱：
//   0 —— 名称前缀命中（键入 "/go" 时 go-hids 应当排第一）
//   1 —— 名称包含命中
//   2 —— 说明文本命中（只记得技能是干什么的、忘了叫什么）
function scoreItem(item: SlashItem, query: string): number {
  const name = item.name.toLowerCase();
  if (name.startsWith(query)) return 0;
  if (name.includes(query)) return 1;
  if (item.hint.toLowerCase().includes(query)) return 2;
  return -1;
}

// filterSlashItems 按查询串过滤并排序菜单条目。
//
// 查询串为空时原样返回（命令在前、技能在后）；否则按打分升序，
// 同分保持原有顺序——Array.prototype.sort 在现代引擎上是稳定排序。
export function filterSlashItems(items: SlashItem[], query: string): SlashItem[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  return items
    .map((item, index) => ({ item, index, score: scoreItem(item, q) }))
    .filter((e) => e.score >= 0)
    .sort((a, b) => a.score - b.score || a.index - b.index)
    .map((e) => e.item);
}

// SlashInput 是用户提交的一整行斜杠输入的解析结果。
export interface SlashInput {
  // head 为首个 token，含斜杠且已转小写，用于与内置命令比对
  head: string;
  // name 为 head 去掉斜杠后的原样文本，保留大小写以便回填与展示
  name: string;
  // rest 为首个 token 之后的剩余文本（已 trim），技能场景下就是用户的任务描述
  rest: string;
}

// parseSlashInput 把一行斜杠输入拆成"意图"与"参数"两部分。
//
// 只按第一个空白切分：技能名里不会出现空格，而任务描述里必然会有，
// 因此首个 token 就是全部意图，其余内容原样交给模型。
export function parseSlashInput(raw: string): SlashInput {
  const trimmed = raw.trim();
  const idx = trimmed.search(/\s/);
  const headRaw = idx >= 0 ? trimmed.slice(0, idx) : trimmed;
  const rest = idx >= 0 ? trimmed.slice(idx + 1).trim() : "";
  const name = headRaw.startsWith("/") ? headRaw.slice(1) : headRaw;
  return { head: headRaw.toLowerCase(), name, rest };
}

// findSkill 在技能目录中按名称查找，大小写不敏感；未命中返回 undefined。
// 技能名来自各 SKILL.md 的 frontmatter，书写习惯不统一，因此不能要求大小写完全一致。
export function findSkill(skills: SkillMeta[], name: string): SkillMeta | undefined {
  const target = name.toLowerCase();
  return skills.find((s) => s.name.toLowerCase() === target);
}

// skillPrompt 把 "/技能名 任务描述" 翻译成发给模型的指令。
//
// 明确点名 load_skill 工具，是因为技能正文并不在上下文里——系统提示只登记了名称与简介。
// 不点名工具的话，模型可能仅凭一句简介就开始作答，等于没载入技能。
export function skillPrompt(skill: SkillMeta, task: string): string {
  const body = task.trim();
  if (!body) {
    return `请先调用 load_skill 工具载入技能「${skill.name}」，然后简要说明它能帮我做什么、需要我提供哪些信息。`;
  }
  return `请先调用 load_skill 工具载入技能「${skill.name}」，再严格按该技能的指引完成下面的任务：\n\n${body}`;
}
