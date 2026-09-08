import { useState } from "preact/hooks";
import { IconChevronDown, IconChevronRight, IconSpark } from "../lib/icons";

interface Props {
  content: string;
}

// ReasoningBlock —— 推理过程：mauve 色轨 + 可折叠细节。
// 默认折叠只留一行触发点，因为推理是"过程"，不该和"结论"抢注意力。
export function ReasoningBlock({ content }: Props) {
  const [open, setOpen] = useState(false);
  const toggle = () => setOpen((v) => !v);

  return (
    <div class="rail rail--reason">
      <button class="reason__head" onClick={toggle} aria-expanded={open}>
        <IconSpark size={13} />
        <span class="reason__label">思考过程</span>
        <span class="reason__chev">{open ? <IconChevronDown size={13} /> : <IconChevronRight size={13} />}</span>
      </button>
      {open && <div class="reason__body">{content}</div>}
    </div>
  );
}
