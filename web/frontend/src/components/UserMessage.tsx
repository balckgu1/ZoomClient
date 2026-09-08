interface Props {
  content: string;
}

// UserMessage —— 用户消息：右侧对齐的墨色气泡，无外框无图标。
// 对齐方向本身就是"这是谁说的"的提示，因此不需要角色名牌。
export function UserMessage({ content }: Props) {
  return <div class="msg-user">{content}</div>;
}
