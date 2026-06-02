# AI 桌面宠物系统设计规范

> 为 cc-learn (ZoomClient) 项目新增桌面宠物 GUI 前端的完整技术设计。

## 1. 架构概述

采用 **Go Native Backend + Tauri v2 Sidecar** 架构：

- **Go CLI (Sidecar)**：现有 Agent 运行时作为子进程，提供完整 AI 能力（agentLoop、工具调用、上下文压缩、Hook 系统等）。通过 `--mode api` 启动，输出结构化 NDJSON。
- **Tauri v2 主进程 (Rust)**：负责窗口管理（透明/无边框/置顶/拖拽）、系统托盘、Sidecar 生命周期管理、stdin/stdout 管道桥接。
- **Svelte 前端 (WebView)**：宠物渲染、动画、对话气泡 UI、右键菜单。

```
┌─────────────────────────────────────────────────────┐
│                 Tauri v2 主进程 (Rust)                │
│  - 窗口管理（透明/无边框/置顶/拖拽）                   │
│  - 系统托盘                                          │
│  - Sidecar 生命周期管理（拉起/重启/关闭 Go CLI）       │
│  - stdin/stdout 管道桥接                             │
└────────────────────────┬────────────────────────────┘
                         │ Tauri Command (invoke)
┌────────────────────────▼────────────────────────────┐
│              Svelte 前端 (WebView)                    │
│  ┌──────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ 宠物组件  │  │  对话气泡组件  │  │  右键菜单组件  │  │
│  │ (动画+   │  │  (Markdown   │  │  (设置面板)   │  │
│  │  情绪FSM)│  │   + 流式输出) │  │               │  │
│  └──────────┘  └──────────────┘  └───────────────┘  │
└─────────────────────────────────────────────────────┘
                         │ JSON over stdin/stdout
┌────────────────────────▼────────────────────────────┐
│              Go CLI Sidecar (子进程)                  │
│  --mode api 启动                                     │
│  - agentLoop（完整 Agent 能力）                       │
│  - 输出结构化 JSON 事件流                             │
│  - 接收 JSON 命令（chat/config/exit）                 │
└─────────────────────────────────────────────────────┘
```

### 1.1 核心设计原则

- Go CLI 零重构核心逻辑，仅新增 `--mode api` 输出适配层
- 前端与后端通过 NDJSON（每行一个 JSON）通信
- 宠物窗口与对话气泡共享同一个 Tauri 窗口（透明背景）
- Windows 优先，为 Linux 预留扩展空间

### 1.2 方案选型理由

| 对比维度 | Go+Tauri Sidecar | Electron+Go | Wails v3 | Go+嵌入WebView |
|---|---|---|---|---|
| Go 代码侵入性 | 最低（加 --mode api） | 中等（加 HTTP 层） | 高（函数绑定） | 中等 |
| 打包体积 | ~10MB（+Go 二进制） | ~120MB+ | ~15MB | ~8MB |
| 动画表现力 | 完整 Web 能力 | 完整 Web 能力 | WebView 受限 | 跨平台差异大 |
| 部署体验 | 单安装包 | 单安装包 | 单安装包 | 单安装包 |
| 透明窗口支持 | 原生支持 | 原生支持 | 有限 | 跨平台地狱 |

## 2. NDJSON 通信协议

### 2.1 设计原则

- 每行一条完整 JSON，`\n` 为分隔符
- 顶层 `ch` 字段做通道路由，区分业务输出、情绪控制和系统事件
- agent 通道有序流，emotion 通道状态快照，system 通道离散事件

## 2.2 上行协议（Tauri → Go stdin）

```jsonc
// 用户发送消息（带可选 id 用于 ACK 关联）
{"ch":"cmd", "id":"msg_01", "action":"chat", "payload":{"message":"帮我写个排序算法"}}

// 配置变更
{"ch":"cmd", "id":"cfg_01", "action":"config", "payload":{"model_type":"anthropic"}}

// 会话控制
{"ch":"cmd", "id":"ctl_01", "action":"clear"}
{"ch":"cmd", "id":"ctl_02", "action":"compact"}
{"ch":"cmd", "action":"exit"}
```

**请求-响应关联机制：**

- 上行消息可携带可选 `id` 字段（由前端生成，建议格式 `<action>_<序号>`）
- Go 侧收到带 `id` 的命令后，在开始处理前立即回复 ACK：
  ```jsonc
  {"ch":"system", "data":{"event":"ack", "id":"msg_01", "status":"accepted"}}
  ```
- 若 JSON 解析失败或参数非法，回复 NACK：
  ```jsonc
  {"ch":"system", "data":{"event":"ack", "id":"msg_01", "status":"rejected", "reason":"invalid payload"}}
  ```
- 前端发送后启动 **5s ACK 超时**：
  - 收到 `accepted` → 正常等待 agent 通道输出
  - 收到 `rejected` → 立即显示错误提示
  - 超时未收到 → 显示"发送失败，请重试"（无需等心跳 10s 超时）
- `exit` 命令无需 ACK（进程即将退出）

### 2.3 下行协议（Go stdout → Tauri）

**agent 通道** — 纯 LLM/Tool 输出流，喂给对话气泡组件：

```jsonc
{"ch":"agent", "data":{"type":"reasoning", "content":"用户需要一个冒泡排序..."}}
{"ch":"agent", "data":{"type":"token", "content":"def "}}
{"ch":"agent", "data":{"type":"token", "content":"bubble_sort"}}
{"ch":"agent", "data":{"type":"tool_call", "name":"write_file", "args":"..."}}
{"ch":"agent", "data":{"type":"tool_result", "name":"write_file", "ok":true, "summary":"wrote 25 lines"}}
{"ch":"agent", "data":{"type":"done"}}
```

**emotion 通道** — 驱动宠物 FSM，不显示在对话框：

```jsonc
{"ch":"emotion", "data":{"state":"thinking"}}
{"ch":"emotion", "data":{"state":"executing", "tool":"write_file"}}
{"ch":"emotion", "data":{"state":"idle"}}
```

**system 通道** — 生命周期/元信息：

```jsonc
{"ch":"system", "data":{"event":"ready", "version":"0.1.0", "model":"claude-opus-4-8"}}
{"ch":"system", "data":{"event":"heartbeat", "ts":1717344000}}
{"ch":"system", "data":{"event":"error", "message":"API key invalid"}}
{"ch":"system", "data":{"event":"config_updated", "key":"model_type", "value":"gemini"}}
```

### 2.4 通道语义总结

| 通道 | 消费方式 | 说明 |
|---|---|---|
| `agent` | 有序流，token 按序拼接 | tool_call/tool_result 成对出现 |
| `emotion` | 状态快照，收到即覆盖 | 无需关心历史 |
| `system` | 离散事件，一次性消费 | 不累积 |

## 3. 前端组件架构

### 3.1 Svelte 组件树

```
App.svelte
├── PetView.svelte          ← 宠物主体（SVG + CSS 动画）
│   └── EmotionFSM.ts       ← 状态机逻辑（纯 TS，驱动动画切换）
├── ChatBubble.svelte       ← 对话气泡（点击宠物时展开）
│   ├── MessageList.svelte  ← 消息列表（Markdown 渲染 + 流式 token 拼接）
│   └── InputBar.svelte     ← 输入框 + 发送按钮
├── ContextMenu.svelte      ← 右键菜单
└── SidecarBridge.ts        ← Tauri Command 桥接层
```

### 3.2 前端技术栈

- **Svelte** — 编译型框架，产物极小，响应式语法适合动画密集场景
- **CSS 动画** — MVP 阶段所有动画效果用纯 CSS 实现
- **TypeScript** — 类型安全
- **Vite** — 构建工具

## 4. 宠物情绪状态机

### 4.1 状态集合（6 种）

| 状态 | 触发方 | 视觉含义 |
|---|---|---|
| `idle` | 前端（默认/超时回落） | 无任务，待机呼吸 |
| `thinking` | Go 推送 | LLM 正在推理 |
| `executing` | Go 推送 | 工具正在执行 |
| `talking` | 前端推断（首个 token） | 正在输出回复 |
| `happy` | 前端推断（done 事件） | 任务完成 |
| `error` | Go 推送 | 出错了 |

### 4.2 状态转换规则

| 当前状态 | 触发条件 | 目标状态 |
|---|---|---|
| any | `ch:emotion` → `thinking` | thinking |
| any | `ch:emotion` → `executing` | executing |
| any | `ch:emotion` → `idle` | idle |
| any | `ch:system` → `error` | error |
| thinking/executing | 前端收到首个 `ch:agent` token | talking |
| talking | `ch:agent` → `done` | happy |
| happy | 3 秒后自动 | idle |
| error | 5 秒后自动 | idle |

### 4.3 MVP 动画映射（CSS + SVG）

| 状态 | CSS 动画效果 |
|---|---|
| idle | `scale(0.95↔1.0)` 缓慢呼吸，2s 周期 |
| thinking | `rotate(-5deg↔5deg)` 左右摇摆，0.8s 周期 |
| executing | 齿轮图标旋转 + `scale(0.98↔1.02)` 微脉冲，0.5s 周期 |
| talking | 嘴部 `scaleY(0.8↔1.2)` 模拟说话，0.3s 周期 |
| happy | `translateY(0↔-8px)` 弹跳 + 眼睛变弯，0.4s 周期 |
| error | `translateX(-2px↔2px)` 抖动 + 色调偏红，0.1s 周期 |

### 4.4 动画演进路线

- **MVP**：CSS + SVG 占位动画（纯代码实现）
- **最终形态**：像素风 Spritesheet 帧动画（每个状态一组帧序列）

## 5. Go 侧改造

### 5.1 Emitter 接口抽象

```go
// emitter/emitter.go
type Emitter interface {
    EmitToken(content string)
    EmitReasoning(content string)
    EmitToolCall(name string, args string)
    EmitToolResult(name string, ok bool, summary string)
    EmitEmotion(state string, meta map[string]string)
    EmitSystem(event string, data map[string]string)
    EmitDone()
}
```

两个实现：
- `ui.Renderer` — 现有 CLI 模式（lipgloss 终端渲染），实现 Emitter 接口
- `emitter.ApiEmitter` — API 模式（NDJSON 写 stdout）

### 5.2 main.go 改造

```go
// 新增 --mode 参数
var mode string
flag.StringVar(&mode, "mode", "cli", "Output mode: cli | api")

// 根据 mode 选择 Emitter
var em Emitter
switch mode {
case "api":
    em = emitter.NewApiEmitter(os.Stdout)
case "cli":
    em = ui.New() // 现有 Renderer
}
```

`agentLoop` 的渲染调用从 `view.PrintXxx()` 改为 `em.EmitXxx()`，核心逻辑完全不变。

### 5.3 API 模式的 stdin 读取

API 模式下，REPL 循环从 `view.PromptUser()` 改为逐行读取 stdin JSON：

```go
scanner := bufio.NewScanner(os.Stdin)
for scanner.Scan() {
    var cmd Command
    json.Unmarshal(scanner.Bytes(), &cmd)
    switch cmd.Action {
    case "chat":
        state.Messages = append(state.Messages, fsm.Message{Role: "user", Content: cmd.Payload.Message})
        agentLoop(...)
    case "config":
        // 热更新配置
    case "exit":
        return
    }
}
```

## 6. Tauri 窗口配置

### 6.1 tauri.conf.json 关键配置

```jsonc
{
  "app": {
    "windows": [
      {
        "label": "pet",
        "width": 200,
        "height": 280,
        "decorations": false,
        "transparent": true,
        "alwaysOnTop": true,
        "resizable": false,
        "skipTaskbar": true,
        "url": "index.html"
      }
    ]
  },
  "bundle": {
    "externalBin": ["binaries/zoomcli"]
  }
}
```

### 6.2 交互行为

| 用户操作 | 前端响应 | 窗口行为 |
|---|---|---|
| 左键点击宠物 | 展开/收起 ChatBubble | 窗口高度 200→480px |
| 拖拽宠物 | `data-tauri-drag-region` | 整个窗口跟随鼠标 |
| 右键点击宠物 | 显示 ContextMenu | 自定义浮层 |
| 双击宠物 | 隐藏对话框 | 窗口缩回 200×200px |
| 发送消息 | stdin 写入 JSON | 宠物切换 thinking |
| 收到 done | 消息列表更新完毕 | 宠物 happy → 3s → idle |

### 6.3 鼠标点击穿透管理

透明窗口的可点击区域不能等于窗口矩形，否则会拦截下方桌面应用的点击事件。

**核心原则：** 不使用 CSS `pointer-events: none`（Tauri 透明窗口下不可靠），必须在 Rust 侧通过 `WebviewWindow::set_ignore_cursor_events()` 动态切换。

**实现机制：**

```
前端 Svelte                          Tauri Rust 侧
─────────────────                    ────────────────
on:mouseenter (宠物SVG/气泡容器)  →   set_ignore_cursor_events(false)
  鼠标进入可见元素边界                    窗口接收点击

on:mouseleave (宠物SVG/气泡容器)  →   set_ignore_cursor_events(true)
  鼠标离开可见元素边界                    点击穿透到桌面
```

**关键细节：**
- 前端监听 `on:mouseenter` / `on:mouseleave` 绑定在宠物 SVG 和气泡容器的**实际可见边界**上，而非整个窗口
- 窗口默认状态为 `ignore_cursor_events(true)`（穿透），仅当鼠标悬停在可见内容上时切换为可交互
- 气泡展开/收起时需同步更新监听区域的边界范围
- 右键菜单弹出期间强制 `ignore_cursor_events(false)` 直到菜单关闭

### 6.4 窗口大小策略

- 气泡收起：200×200px，仅宠物可见
- 气泡展开：200×480px（气泡在宠物上方弹出）
- 大小切换（小/中/大）：整体缩放 0.75x / 1.0x / 1.5x

### 6.5 右键菜单选项（MVP）

1. 模型切换（OpenAI / Ollama / Anthropic / Gemini）
2. 宠物大小（小 / 中 / 大）
3. 透明度调节
4. 置顶 / 取消置顶
5. 退出

## 7. 心跳与优雅降级

### 7.1 心跳协议

Go 侧在 `--mode api` 启动后，启动独立 goroutine 周期性输出心跳：

```jsonc
{"ch":"system", "data":{"event":"heartbeat", "ts":1717344000}}
```

- 心跳间隔：3 秒
- 前端超时阈值：10 秒未收到心跳 → 判定假死

心跳 goroutine 独立于 agentLoop，即使 agentLoop 卡在 LLM 调用（可能 30s+），心跳也不会中断。只有真正的 panic/OOM/死锁才会导致心跳停止。

### 7.2 三级降级策略

| 级别 | 触发条件 | 前端行为 | 用户感知 |
|---|---|---|---|
| L0 正常 | 心跳正常 | 全功能 | 无 |
| L1 重启 | 超时 10s | kill + 重启 sidecar | 宠物 error 动画 + "正在重连..." |
| L2 降级 | 重启 3 次均失败 | 禁用对话，保留宠物待机 | "后端离线，右键可重试" |
| L3 通知 | 手动重试仍失败 | 系统通知弹窗 | "请检查配置或重启应用" |

### 7.3 Sidecar 生命周期管理（Rust 侧）

```rust
// 伪代码
fn main() {
    let sidecar = app.shell()
        .sidecar("zoomcli")
        .args(["--mode", "api", "-m", &model_type])
        .spawn()
        .expect("failed to spawn sidecar");

    // stdout 逐行读取 → emit 到 WebView
    tauri::async_runtime::spawn(async move {
        let reader = BufReader::new(sidecar.stdout);
        for line in reader.lines() {
            app.emit_all("sidecar-event", line);
        }
    });

    // 监听 WebView 消息 → 写入 stdin
    app.listen_global("send-to-sidecar", move |event| {
        sidecar.stdin.write_all(event.payload().as_bytes());
        sidecar.stdin.write_all(b"\n");
    });
}
```

## 8. 项目目录结构

```
cc-learn/
├── main.go                    ← 新增 --mode 参数分支
├── emitter/
│   ├── emitter.go             ← Emitter 接口定义
│   └── api_emitter.go         ← NDJSON 输出实现
├── ui/                        ← 现有 CLI 渲染（实现 Emitter 接口）
│   ├── renderer.go
│   └── styles.go
├── ... (现有 Go 代码不变)
│
└── desktop/                   ← Tauri 桌面宠物
    ├── src-tauri/
    │   ├── Cargo.toml
    │   ├── tauri.conf.json
    │   ├── src/
    │   │   └── main.rs        ← Sidecar 管理 + Tauri Commands
    │   └── binaries/
    │       └── zoomcli-x86_64-pc-windows-msvc.exe
    ├── src/
    │   ├── App.svelte
    │   ├── lib/
    │   │   ├── PetView.svelte
    │   │   ├── ChatBubble.svelte
    │   │   ├── ContextMenu.svelte
    │   │   ├── MessageList.svelte
    │   │   ├── InputBar.svelte
    │   │   ├── EmotionFSM.ts
    │   │   └── SidecarBridge.ts
    │   └── assets/
    │       └── pet.svg
    ├── package.json
    ├── svelte.config.js
    ├── vite.config.ts
    └── tsconfig.json
```

## 9. 构建流程

```bash
# 1. 编译 Go CLI
go build -o desktop/src-tauri/binaries/zoomcli-x86_64-pc-windows-msvc.exe .

# 2. 构建 Tauri 应用
cd desktop && npm run tauri build

# 3. 产物位置
# desktop/src-tauri/target/release/bundle/ (MSI/EXE 安装包)
```

## 10. 平台支持

- **Windows**：首要目标平台，依赖 WebView2（Win10+ 自带）
- **Linux**：预留扩展空间，目录结构和协议设计均平台无关，后续适配仅涉及 Tauri 窗口层差异（WebKitGTK）

## 11. 行为模式演进

- **MVP**：固定位置 + 呼吸动画，架构预留状态机扩展点
- **后续**：自由漫游（屏幕边缘吸附、随机小动作），通过扩展 EmotionFSM 状态集合实现

## 12. MVP 范围定义

**包含：**
- Go 侧 `--mode api` + Emitter 接口
- Tauri 项目脚手架 + 透明窗口
- SVG 宠物 + 6 种 CSS 动画状态
- 气泡式对话框（流式输出 + Markdown）
- 右键菜单（模型/大小/透明度/置顶/退出）
- 心跳检测 + L1 自动重启
- 拖拽移动

**不包含（后续迭代）：**
- 像素风 Spritesheet 替换
- 自由漫游行为
- 开机自启
- 多宠物/换装系统
- 对话历史持久化/导出
- Linux 平台适配
