<!-- desktop/src/lib/ChatBubble.svelte -->
<!-- 对话气泡：点击宠物展开/收起，包含消息列表和输入框 -->
<script lang="ts">
  import { chatHistory, isProcessing, sendChat, permissionRequests, sendPermissionReply } from "./SidecarBridge";
  import { afterUpdate } from "svelte";

  let inputText = "";
  let messagesEl: HTMLDivElement;

  export let visible = false;

  // 自动滚动到底部
  afterUpdate(() => {
    if (messagesEl) messagesEl.scrollTop = messagesEl.scrollHeight;
  });

  async function handleSend() {
    const msg = inputText.trim();
    if (!msg || $isProcessing) return;
    inputText = "";
    await sendChat(msg);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  async function handleApprove(id: string) {
    await sendPermissionReply(id, true);
  }

  async function handleDeny(id: string) {
    await sendPermissionReply(id, false, "denied by user");
  }
</script>

{#if visible}
<div class="bubble">
  <div class="bubble-header">
    <span class="title">ZoomClient</span>
    <button class="close-btn" on:click={() => visible = false}>✕</button>
  </div>

  <div class="messages" bind:this={messagesEl}>
    {#each $chatHistory as msg, i}
      <div class="msg msg--{msg.role}" class:error={msg.isError}>
        {msg.content}
      </div>
    {/each}
    {#if $isProcessing}
      <div class="msg msg--assistant typing">
        <span class="dot"></span><span class="dot"></span><span class="dot"></span>
      </div>
    {/if}
  </div>

  <!-- 权限请求面板 -->
  {#each $permissionRequests as req}
    <div class="perm-card">
      <div class="perm-header">⚠ 权限请求</div>
      <div class="perm-body">
        <div class="perm-field"><span class="perm-label">工具</span><span class="perm-val">{req.tool}</span></div>
        <div class="perm-field"><span class="perm-label">原因</span><span class="perm-val">{req.reason}</span></div>
        {#if req.args && req.args !== '{}' && req.args !== 'null'}
          <div class="perm-field perm-args"><span class="perm-label">参数</span><span class="perm-val perm-args-val">{req.args}</span></div>
        {/if}
      </div>
      <div class="perm-actions">
        <button class="perm-btn perm-approve" on:click={() => handleApprove(req.id)}>允许</button>
        <button class="perm-btn perm-deny" on:click={() => handleDeny(req.id)}>拒绝</button>
      </div>
    </div>
  {/each}

  <div class="input-row">
    <input
      type="text"
      bind:value={inputText}
      on:keydown={handleKeydown}
      placeholder="输入消息..."
      disabled={$isProcessing}
    />
    <button on:click={handleSend} disabled={$isProcessing || !inputText.trim()}>发送</button>
  </div>
</div>
{/if}

<style>
  .bubble {
    width: 280px;
    max-height: 350px;
    background: rgba(30, 30, 40, 0.92);
    border: 1px solid rgba(255,255,255,0.1);
    border-radius: 12px;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px rgba(0,0,0,0.4);
    backdrop-filter: blur(10px);
    overflow: hidden;
  }

  .bubble-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    border-bottom: 1px solid rgba(255,255,255,0.08);
  }
  .title { color: #e5e7eb; font-size: 12px; font-weight: 600; font-family: sans-serif; }
  .close-btn {
    background: none; border: none; color: #9ca3af; cursor: pointer;
    font-size: 14px; padding: 0 4px;
  }
  .close-btn:hover { color: #fff; }

  .messages {
    flex: 1;
    overflow-y: auto;
    padding: 8px;
    max-height: 240px;
    scrollbar-width: thin;
    scrollbar-color: rgba(255,255,255,0.15) transparent;
  }

  .msg {
    font-size: 12px;
    line-height: 1.5;
    padding: 4px 8px;
    margin: 3px 0;
    border-radius: 8px;
    font-family: sans-serif;
    word-break: break-word;
    white-space: pre-wrap;
  }
  .msg--user { background: rgba(59,130,246,0.25); color: #bfdbfe; }
  .msg--assistant { background: rgba(255,255,255,0.06); color: #e5e7eb; }
  .msg--tool { background: rgba(16,185,129,0.12); color: #6ee7b7; font-size: 11px; }
  .msg--system { background: none; color: #6b7280; font-size: 11px; font-style: italic; }
  .msg.error { color: #fca5a5; }

  .typing { display: flex; gap: 4px; align-items: center; padding: 8px 12px; }
  .dot {
    width: 6px; height: 6px; background: #6b7280; border-radius: 50%;
    animation: dot-pulse 1s ease-in-out infinite;
  }
  .dot:nth-child(2) { animation-delay: 0.2s; }
  .dot:nth-child(3) { animation-delay: 0.4s; }
  @keyframes dot-pulse {
    0%, 100% { opacity: 0.3; transform: scale(0.8); }
    50% { opacity: 1; transform: scale(1); }
  }

  .input-row {
    display: flex;
    padding: 6px;
    gap: 4px;
    border-top: 1px solid rgba(255,255,255,0.08);
  }
  .input-row input {
    flex: 1;
    background: rgba(255,255,255,0.06);
    border: 1px solid rgba(255,255,255,0.1);
    border-radius: 8px;
    color: #e5e7eb;
    font-size: 12px;
    padding: 6px 10px;
    outline: none;
    font-family: sans-serif;
  }
  .input-row input:focus { border-color: rgba(59,130,246,0.5); }
  .input-row input::placeholder { color: #6b7280; }
  .input-row button {
    background: #3b82f6;
    border: none;
    border-radius: 8px;
    color: white;
    font-size: 11px;
    padding: 6px 12px;
    cursor: pointer;
    font-family: sans-serif;
  }
  .input-row button:disabled { opacity: 0.4; cursor: default; }
  .input-row button:hover:not(:disabled) { background: #2563eb; }

  /* 权限请求卡片 */
  .perm-card {
    margin: 6px 8px;
    border: 1px solid rgba(251, 191, 36, 0.5);
    border-radius: 10px;
    background: rgba(251, 191, 36, 0.08);
    overflow: hidden;
  }
  .perm-header {
    background: rgba(251, 191, 36, 0.18);
    color: #fbbf24;
    font-size: 11px;
    font-weight: 600;
    padding: 5px 10px;
    font-family: sans-serif;
  }
  .perm-body {
    padding: 6px 10px;
  }
  .perm-field {
    display: flex;
    gap: 6px;
    margin: 2px 0;
    font-size: 11px;
    font-family: sans-serif;
  }
  .perm-label {
    color: #9ca3af;
    flex-shrink: 0;
    min-width: 36px;
  }
  .perm-val {
    color: #e5e7eb;
    word-break: break-all;
  }
  .perm-args-val {
    font-size: 10px;
    font-family: monospace;
    color: #a5b4fc;
    white-space: pre-wrap;
    max-height: 60px;
    overflow-y: auto;
  }
  .perm-actions {
    display: flex;
    gap: 6px;
    padding: 6px 10px;
    border-top: 1px solid rgba(251, 191, 36, 0.2);
  }
  .perm-btn {
    flex: 1;
    border: none;
    border-radius: 6px;
    font-size: 11px;
    font-weight: 600;
    padding: 5px 0;
    cursor: pointer;
    font-family: sans-serif;
    transition: opacity 0.15s;
  }
  .perm-btn:hover { opacity: 0.85; }
  .perm-approve { background: #22c55e; color: #fff; }
  .perm-deny { background: #ef4444; color: #fff; }
</style>
