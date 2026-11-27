import { useState } from "preact/hooks";
import type { UserQuestion } from "../types";
import { IconCheck, IconClose, IconSpark } from "../lib/icons";

interface Props {
  question: UserQuestion;
  onAnswer: (answer: string, cancelled: boolean) => void;
}

// QuestionDialog —— ask_user_question 的前端弹窗。
// 智能体在缺少关键信息时会主动提问：有选项时一键选择，
// 没有选项或选项不合适时仍可自由输入；跳过会以取消错误回到智能体。
export function QuestionDialog({ question, onAnswer }: Props) {
  const [value, setValue] = useState("");

  const submit = () => {
    const answer = value.trim();
    if (!answer) return;
    onAnswer(answer, false);
  };

  return (
    <div class="modal-overlay">
      <div class="permission-dialog question-dialog">
        <div class="permission-dialog__head">
          <span class="permission-dialog__mark"><IconSpark size={16} /></span>
          <div>
            <h3>{question.header || "智能体需要你的确认"}</h3>
            <p class="permission-dialog__sub">回答后智能体会继续执行</p>
          </div>
        </div>

        <div class="permission-dialog__body">
          <div class="question-text">{question.question}</div>
          {question.options.length > 0 && (
            <div class="question-options">
              {question.options.map((opt) => (
                <button
                  key={opt}
                  class="question-option"
                  onClick={() => onAnswer(opt, false)}
                >
                  {opt}
                </button>
              ))}
            </div>
          )}
          <input
            class="field__input"
            placeholder="输入你的回答…"
            value={value}
            onInput={(e) => setValue((e.target as HTMLInputElement).value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") submit();
            }}
          />
        </div>

        <div class="perm-actions">
          <button class="btn btn--ghost" onClick={() => onAnswer("", true)}>
            <IconClose size={14} /> 跳过
          </button>
          <button class="btn btn-allow" onClick={submit} disabled={!value.trim()}>
            <IconCheck size={14} /> 提交
          </button>
        </div>
      </div>
    </div>
  );
}