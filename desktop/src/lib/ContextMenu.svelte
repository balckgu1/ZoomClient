<!-- desktop/src/lib/ContextMenu.svelte -->
<!-- 右键菜单：模型切换 / 宠物大小 / 透明度 / 置顶 / 退出 -->
<script lang="ts">
  import { getCurrentWindow } from "@tauri-apps/api/window";

  export let visible = false;
  export let x = 0;
  export let y = 0;

  let petScale = 1.0;
  let petOpacity = 1.0;
  let alwaysOnTop = true;

  function close() { visible = false; }

  async function toggleTop() {
    alwaysOnTop = !alwaysOnTop;
    await getCurrentWindow().setAlwaysOnTop(alwaysOnTop);
    close();
  }

  function setScale(s: number) {
    petScale = s;
    document.documentElement.style.setProperty("--pet-scale", String(s));
    close();
  }

  function setOpacity(o: number) {
    petOpacity = o;
    document.documentElement.style.setProperty("--pet-opacity", String(o));
    close();
  }

  async function quit() {
    await getCurrentWindow().close();
  }

  // 点击外部关闭
  function handleClickOutside(e: MouseEvent) {
    if (visible) close();
  }

  // 键盘无障碍：Esc 关闭菜单
  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") close();
  }
</script>

<svelte:window on:click={handleClickOutside} on:contextmenu={handleClickOutside} />

{#if visible}
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="ctx-menu" style="left:{x}px; top:{y}px" role="menu" tabindex="-1" on:click|stopPropagation on:keydown={handleKeydown}>
  <div class="section">
    <div class="label">置顶</div>
    <button class:active={alwaysOnTop} on:click={toggleTop}>
      {alwaysOnTop ? "✓ 已置顶" : "取消置顶"}
    </button>
  </div>

  <div class="section">
    <div class="label">大小</div>
    <div class="btn-group">
      <button class:active={petScale === 0.75} on:click={() => setScale(0.75)}>小</button>
      <button class:active={petScale === 1.0} on:click={() => setScale(1.0)}>中</button>
      <button class:active={petScale === 1.5} on:click={() => setScale(1.5)}>大</button>
    </div>
  </div>

  <div class="section">
    <div class="label">透明度</div>
    <div class="btn-group">
      <button class:active={petOpacity === 1.0} on:click={() => setOpacity(1.0)}>100%</button>
      <button class:active={petOpacity === 0.7} on:click={() => setOpacity(0.7)}>70%</button>
      <button class:active={petOpacity === 0.4} on:click={() => setOpacity(0.4)}>40%</button>
    </div>
  </div>

  <div class="divider"></div>
  <button class="danger" on:click={quit}>退出</button>
</div>
{/if}

<style>
  .ctx-menu {
    position: fixed;
    z-index: 1000;
    min-width: 160px;
    background: rgba(25, 25, 35, 0.95);
    border: 1px solid rgba(255,255,255,0.1);
    border-radius: 10px;
    padding: 6px;
    box-shadow: 0 8px 32px rgba(0,0,0,0.5);
    backdrop-filter: blur(12px);
    font-family: sans-serif;
  }

  .section { padding: 4px 0; }
  .label {
    font-size: 10px;
    color: #6b7280;
    padding: 2px 8px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .btn-group { display: flex; gap: 2px; padding: 2px 6px; }

  button {
    background: rgba(255,255,255,0.06);
    border: 1px solid rgba(255,255,255,0.08);
    border-radius: 6px;
    color: #d1d5db;
    font-size: 11px;
    padding: 4px 10px;
    cursor: pointer;
    transition: background 0.15s;
  }
  button:hover { background: rgba(255,255,255,0.12); }
  button.active { background: rgba(59,130,246,0.3); border-color: rgba(59,130,246,0.5); color: #93c5fd; }

  .divider {
    height: 1px;
    background: rgba(255,255,255,0.08);
    margin: 4px 0;
  }

  .danger {
    width: 100%;
    text-align: center;
    color: #fca5a5;
    border-color: rgba(239,68,68,0.2);
  }
  .danger:hover { background: rgba(239,68,68,0.15); }
</style>
