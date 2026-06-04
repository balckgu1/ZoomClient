<!-- desktop/src/lib/PetView.svelte -->
<!-- 宠物主体：SVG 猫 + 6 种 CSS 动画，class 绑定情绪状态 -->
<script lang="ts">
  import { emotionState, sidecarAlive } from "./SidecarBridge";

  $: stateClass = `pet pet--${$emotionState}`;
  $: if (!$sidecarAlive) stateClass = "pet pet--error";
</script>

<div class={stateClass} data-tauri-drag-region>
  <svg width="100" height="100" viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">
    <!-- 耳朵 -->
    <polygon points="25,35 35,10 45,35" fill="#6b7280"/>
    <polygon points="55,35 65,10 75,35" fill="#6b7280"/>
    <polygon points="30,33 37,15 43,33" fill="#f9a8d4"/>
    <polygon points="57,33 63,15 70,33" fill="#f9a8d4"/>
    <!-- 头 -->
    <ellipse cx="50" cy="52" rx="30" ry="28" fill="#9ca3af"/>
    <!-- 眼睛 -->
    <ellipse class="eye eye-l" cx="38" cy="47" rx="5" ry="6" fill="white"/>
    <ellipse class="eye eye-r" cx="62" cy="47" rx="5" ry="6" fill="white"/>
    <circle class="pupil pupil-l" cx="39" cy="48" r="3" fill="#1f2937"/>
    <circle class="pupil pupil-r" cx="63" cy="48" r="3" fill="#1f2937"/>
    <!-- 鼻子 -->
    <polygon points="48,55 52,55 50,58" fill="#f472b6"/>
    <!-- 嘴 -->
    <g class="mouth">
      <path d="M44,60 Q50,66 56,60" fill="none" stroke="#4b5563" stroke-width="1.5"/>
    </g>
    <!-- 胡须 -->
    <line x1="20" y1="54" x2="35" y2="56" stroke="#d1d5db" stroke-width="1"/>
    <line x1="20" y1="58" x2="35" y2="58" stroke="#d1d5db" stroke-width="1"/>
    <line x1="65" y1="56" x2="80" y2="54" stroke="#d1d5db" stroke-width="1"/>
    <line x1="65" y1="58" x2="80" y2="58" stroke="#d1d5db" stroke-width="1"/>
    <!-- 腮红 -->
    <ellipse cx="30" cy="56" rx="5" ry="3" fill="#fda4af" opacity="0.4"/>
    <ellipse cx="70" cy="56" rx="5" ry="3" fill="#fda4af" opacity="0.4"/>
    <!-- 身体（简化） -->
    <ellipse cx="50" cy="82" rx="20" ry="12" fill="#9ca3af"/>
    <!-- 尾巴 -->
    <path class="tail" d="M70,80 Q85,70 80,55" fill="none" stroke="#9ca3af" stroke-width="5" stroke-linecap="round"/>
  </svg>
</div>

<style>
  .pet {
    width: 120px;
    height: 120px;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: grab;
    transition: transform 0.3s ease;
  }
  .pet:active { cursor: grabbing; }

  /* idle: 缓慢呼吸 */
  .pet--idle { animation: breathe 2.5s ease-in-out infinite; }
  /* thinking: 左右摇摆 */
  .pet--thinking { animation: wobble 0.8s ease-in-out infinite; }
  /* executing: 微脉冲 */
  .pet--executing { animation: pulse 0.5s ease-in-out infinite; }
  /* talking: 微跳 */
  .pet--talking { animation: talk-bounce 0.4s ease-in-out infinite; }
  /* happy: 弹跳 */
  .pet--happy { animation: happy-bounce 0.4s ease-in-out infinite; }
  /* error: 抖动+变红 */
  .pet--error { animation: shake 0.15s ease-in-out infinite; filter: hue-rotate(-30deg) saturate(2); }

  @keyframes breathe {
    0%, 100% { transform: scale(0.96); }
    50% { transform: scale(1.0); }
  }
  @keyframes wobble {
    0%, 100% { transform: rotate(-3deg); }
    50% { transform: rotate(3deg); }
  }
  @keyframes pulse {
    0%, 100% { transform: scale(0.98); }
    50% { transform: scale(1.02); }
  }
  @keyframes talk-bounce {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-2px); }
  }
  @keyframes happy-bounce {
    0%, 100% { transform: translateY(0) scale(1); }
    50% { transform: translateY(-6px) scale(1.05); }
  }
  @keyframes shake {
    0%, 100% { transform: translateX(0); }
    25% { transform: translateX(-3px); }
    75% { transform: translateX(3px); }
  }

  /* 尾巴摆动 */
  .tail { animation: tail-wag 1.5s ease-in-out infinite; transform-origin: 70px 80px; }
  @keyframes tail-wag {
    0%, 100% { transform: rotate(-5deg); }
    50% { transform: rotate(5deg); }
  }

  /* happy 状态眼睛变弯 */
  .pet--happy :global(.eye) {
    ry: 2;
  }
</style>
