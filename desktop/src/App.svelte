<!-- desktop/src/App.svelte -->
<!-- 主应用：集成宠物、对话气泡、右键菜单 -->
<script lang="ts">
  import PetView from "./lib/PetView.svelte";
  import ChatBubble from "./lib/ChatBubble.svelte";
  import ContextMenu from "./lib/ContextMenu.svelte";
  import { emotionState, sidecarAlive, agentMessages } from "./lib/SidecarBridge";
  import { transitionFromBackend, transitionToTalking, transitionToHappy } from "./lib/EmotionFSM";
  import { getCurrentWindow } from "@tauri-apps/api/window";
  import { LogicalSize } from "@tauri-apps/api/dpi";

  let bubbleVisible = false;
  let ctxMenuVisible = false;
  let ctxX = 0;
  let ctxY = 0;

  // 监听情绪状态变化 → 驱动 FSM 前端推断
  let prevEmotion = "idle";
  $: {
    if ($emotionState !== prevEmotion) {
      transitionFromBackend($emotionState);
      prevEmotion = $emotionState;
    }
  }

  // 监听 agent 事件做前端推断
  let prevAgentCount = 0;
  $: {
    const count = $agentMessages.length;
    if (count > prevAgentCount) {
      const latest = $agentMessages[count - 1];
      if (latest.type === "token" && ($emotionState === "thinking" || $emotionState === "executing")) {
        transitionToTalking();
      }
      if (latest.type === "done") {
        transitionToHappy();
      }
      prevAgentCount = count;
    }
  }

  // 左键点击宠物 → 切换对话气泡
  function toggleBubble() {
    bubbleVisible = !bubbleVisible;
    const win = getCurrentWindow();
    if (bubbleVisible) {
      win.setSize(new LogicalSize(280, 500));
    } else {
      win.setSize(new LogicalSize(200, 200));
    }
  }

  // 右键菜单
  function handleContextMenu(e: MouseEvent) {
    e.preventDefault();
    ctxX = e.clientX;
    ctxY = e.clientY;
    ctxMenuVisible = true;
  }

  // 双击关闭气泡
  function handleDblClick() {
    bubbleVisible = false;
    const win = getCurrentWindow();
    win.setSize(new LogicalSize(200, 200));
  }

  // 键盘无障碍：Enter/Space 切换气泡
  function handlePetKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      toggleBubble();
    }
  }

  // 点击穿透管理
  function handleMouseEnter() {
    getCurrentWindow().setCursorGrab(false);
  }
  function handleMouseLeave() {
    // 如果菜单或气泡打开，不恢复穿透
    if (!ctxMenuVisible && !bubbleVisible) {
      // window.setIgnoreCursorEvents(true) would be called here
      // but for MVP we keep it simple
    }
  }
</script>

<main
  data-tauri-drag-region
  on:contextmenu={handleContextMenu}
  on:mouseenter={handleMouseEnter}
  on:mouseleave={handleMouseLeave}
>
  <!-- 对话气泡（在宠物上方弹出） -->
  <div class="bubble-area">
    <ChatBubble bind:visible={bubbleVisible} />
  </div>

  <!-- 宠物区域 -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="pet-area" role="button" tabindex="0" on:click={toggleBubble} on:dblclick={handleDblClick} on:keydown={handlePetKeydown}>
    <PetView />
    {#if !$sidecarAlive}
      <div class="offline-badge">离线</div>
    {/if}
  </div>

  <!-- 右键菜单 -->
  <ContextMenu x={ctxX} y={ctxY} bind:visible={ctxMenuVisible} />
</main>

<style>
  main {
    width: 100vw;
    height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: flex-end;
    background: transparent;
  }

  .bubble-area {
    flex: 1;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    width: 100%;
    padding-bottom: 4px;
  }

  .pet-area {
    position: relative;
    cursor: pointer;
    transform: scale(var(--pet-scale));
    opacity: var(--pet-opacity);
    transition: transform 0.3s ease, opacity 0.3s ease;
  }

  .offline-badge {
    position: absolute;
    bottom: 2px;
    left: 50%;
    transform: translateX(-50%);
    background: rgba(239, 68, 68, 0.8);
    color: white;
    font-size: 10px;
    padding: 2px 8px;
    border-radius: 8px;
    font-family: sans-serif;
  }
</style>
