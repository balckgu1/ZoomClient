import { useState, useRef } from "preact/hooks";
import type { JSX } from "preact";

interface Props {
  disabled: boolean;
  onSend: (message: string) => void;
  onSlashCommand: (cmd: string) => void;
}

export function InputBar({ disabled, onSend, onSlashCommand }: Props) {
  const [text, setText] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleKeyDown = (e: JSX.TargetedKeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  const handleSubmit = () => {
    const trimmed = text.trim();
    if (!trimmed || disabled) return;
    if (trimmed.startsWith("/")) {
      onSlashCommand(trimmed);
    } else {
      onSend(trimmed);
    }
    setText("");
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
  };

  const handleInput = () => {
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
      textareaRef.current.style.height = textareaRef.current.scrollHeight + "px";
    }
  };

  return (
    <footer class="input-bar">
      <textarea
        ref={textareaRef}
        class="input-textarea"
        placeholder={disabled ? "Agent is thinking..." : "Type a message... (/ for commands)"}
        value={text}
        onInput={(e) => { setText((e.target as HTMLTextAreaElement).value); handleInput(); }}
        onKeyDown={handleKeyDown}
        disabled={disabled}
        rows={1}
      />
      <button
        class="send-btn"
        onClick={handleSubmit}
        disabled={disabled || !text.trim()}
      >
        Send
      </button>
    </footer>
  );
}
