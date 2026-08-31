---
name: go-hids
description: Step-by-step guide for building a Linux Host-based Intrusion Detection System (HIDS) in Go. Use when the user asks how to build a HIDS from scratch with Go, wants a phased implementation plan, or needs guidance on eBPF/fanotify collection, detection rules, or agent architecture.
author: balckgu1
version: v1.0
compatibility: Go 1.21+, Linux (kernel 4.x+ for eBPF)
---

# Go HIDS 开发指南

本 skill 提供一套**从零开始用 Go 编写 Linux HIDS（主机入侵检测系统）**的分步实施指南。按阶段推进，每阶段都有明确目标、产出物和验收标准。

## 总体架构

```
┌─────────────────────────────────────┐
│            Server 端 (可选)          │
│  规则下发 / 告警汇聚 / 数据存储      │
└───────────────┬─────────────────────┘
                │ gRPC / HTTPS / MQTT
┌───────────────▼─────────────────────┐
│            Agent 端 (Go)            │
│  ┌──────────┐ ┌──────────┐ ┌─────┐  │
│  │  采集模块  │ │  检测引擎  │ │响应  │  │
│  │ eBPF/    │ │ 规则/行为 │ │模块 │  │
│  │ fanotify │ │ 分析      │ │     │  │
│  └────┬─────┘ └────┬─────┘ └──┬──┘  │
│       └──────┬─────┴─────┬────┘     │
│          ┌───▼───┐  ┌────▼───┐      │
│          │ 上报/  │  │ 自身安全│      │
│          │ 缓存   │  │ 资源控制│      │
│          └───────┘  └────────┘      │
└─────────────────────────────────────┘
```

## 前置准备

1. **环境**：Linux 主机（建议 Ubuntu 22.04 / CentOS 8+），内核 4.x+（eBPF 需要）
2. **Go 版本**：1.21+，初始化模块
   ```bash
   go mod init github.com/you/hids
   ```
3. **目录规划**（推荐结构）：
   ```
   hids/
   ├── cmd/agent/          # Agent 入口
   ├── cmd/server/         # Server 入口（可选）
   ├── internal/
   │   ├── collector/      # 采集模块
   │   ├── detector/       # 检测引擎
   │   ├── responder/      # 响应模块
   │   ├── report/         # 上报/缓存
   │   └── config/         # 配置
   └── pkg/                # 公共库
   ```

---

## 阶段一：项目骨架与配置

**目标**：搭起可运行的最小 Agent，能加载配置、启动、优雅退出。

**步骤**：
1. 建立上述目录结构
2. 编写配置文件（YAML/TOML）：
   ```yaml
   agent:
     id: "host-001"
     interval: 60s        # 心跳周期
     collect:
       process: true
       file: true
       network: true
       user: true
     report:
       endpoint: "https://server.example.com:8443"
       cache_dir: "/var/lib/hids/cache"
     resource:
       max_cpu_percent: 20
       max_mem_mb: 256
   ```
3. 用 `viper` 或 `gopkg.in/yaml.v3` 解析配置
4. 实现主循环 + 信号处理（SIGTERM/SIGINT 优雅退出）
5. 加入结构化日志（`slog` 标准库或 `zerolog`）

**验收**：`go run ./cmd/agent` 能启动、读取配置、打印日志、Ctrl+C 优雅退出。

---

## 阶段二：进程采集模块

**目标**：实时监控进程创建/退出事件。

**技术选型（按优先级）**：
1. **eBPF**（`cilium/ebpf` 库）—— 性能最好，推荐
2. **fanotify**（`github.com/fsnotify/fsnotify` 不适用进程，用 `fanotify` 或 netlink）
3. **auditd** —— 读取 `/var/log/audit/audit.log`
4. **轮询 /proc** —— 最简单但延迟高、开销大

**推荐实现（eBPF tracepoint）**：
```go
// 使用 cilium/ebpf 挂载 sched_process_exec / sched_process_exit
// 采集: pid, ppid, comm, 可执行文件路径, 时间戳, uid
```

**数据结构**：
```go
type ProcessEvent struct {
    PID        int32     `json:"pid"`
    PPID       int32     `json:"ppid"`
    Comm       string    `json:"comm"`
    ExePath    string    `json:"exe_path"`
    UID        uint32    `json:"uid"`
    Timestamp  time.Time `json:"timestamp"`
}
```

**验收**：运行任意命令（如 `ls`, `sleep`），能采集到对应进程事件。

---

## 阶段三：文件系统监控

**目标**：监控关键文件/目录的增删改。

**技术选型**：
- **inotify/fanotify**（`fsnotify` 库）—— 实时、轻量
- 对 `/etc`, `/usr/bin`, `/usr/local/bin` 等敏感目录做监控
- **文件完整性校验**（FIM）：对关键文件计算哈希（SHA-256），定时比对

**实现要点**：
```go
watcher, _ := fsnotify.NewWatcher()
watcher.Add("/etc")
watcher.Add("/usr/bin")
// 处理 Create / Write / Remove / Rename 事件
```

**FIM 校验逻辑**：
1. 首次运行建立基线（文件路径 + 哈希）
2. 定期（如每小时）重新计算并比对
3. 哈希变化 → 告警

**验收**：在监控目录创建/修改/删除文件，能捕获事件；修改 /etc/passwd 触发 FIM 告警。

---

## 阶段四：网络与用户行为监控

**目标**：监控网络连接和登录/用户操作。

**网络监控**：
- 监听 TCP/UDP 连接建立/关闭
- 采集：本地地址、远端地址、进程 PID、协议
- 方案：eBPF（`tcp_connect`, `tcp_close`）或 netlink socket 监视

**用户行为监控**：
- 解析 `/var/run/utmp`、`/var/log/wtmp` 获取登录/登出
- 监控 `/etc/passwd`、`/etc/shadow`、`/etc/sudoers` 变化
- 监控 sudo 执行（结合进程采集 + 审计）

**验收**：发起 SSH 登录、执行 sudo 命令能被记录。

---

## 阶段五：检测引擎

**目标**：把采集到的事件转化为告警。

**规则匹配（静态）**：
- 支持的规则格式：YARA、Sigma 或自定义 JSON 规则
- 示例规则：
  ```json
  {
    "id": "RULE-001",
    "name": "suspicious-bash-reverse-shell",
    "match": {
      "event": "process",
      "comm": "bash",
      "args_contains": ["/dev/tcp/", "nc -e", "sh -i"]
    },
    "severity": "high",
    "action": ["alert", "kill_process"]
  }
  ```

**行为分析（动态，可选）**：
- 基线学习：正常登录时间、命令频率
- 异常检测：深夜登录、异常 IP、高频失败登录（暴力破解）

**实现**：
```go
type RuleEngine struct {
    rules []Rule
}
func (e *RuleEngine) Match(evt Event) []Alert
```

**验收**：构造一个匹配规则的事件（如执行反向 shell 命令），能触发对应告警。

---

## 阶段六：响应与处置

**目标**：对告警采取动作。

**响应动作**：
- `alert`：仅记录/上报
- `kill_process`：终止恶意进程
- `block_ip`：iptables/nftables 封禁 IP
- `quarantine`：隔离文件（移动/加权限）
- `notify`：webhook / syslog / 邮件

**实现**：
```go
type Responder interface {
    Alert(a Alert) error
    KillProcess(pid int32) error
    BlockIP(ip string) error
}
```

**验收**：触发高危险告警时，恶意进程被终止、IP 被封禁。

---

## 阶段七：上报、缓存与自身安全

**目标**：数据可靠上报，Agent 自身健壮。

**上报**：
- 加密通道（gRPC+TLS / HTTPS）
- 断网缓存：本地队列（如 SQLite 或文件），恢复后补传
- 批量上报减少开销

**自身安全**：
- 资源限制：CPU/内存/磁盘上限（`golang.org/x/sys` 设置 rlimit）
- 防篡改：Agent 二进制哈希自校验
- 防卸载：root 权限运行、文件权限收紧
- 日志轮转，防止磁盘写满

**验收**：断网时事件缓存不丢；Agent 内存占用稳定在限制内。

---

## 阶段八：Server 端与测试（可选）

**目标**：汇聚多 Agent 数据，形成完整方案。

**Server 功能**：
- Agent 注册与心跳管理
- 规则下发（动态更新检测规则）
- 告警汇聚、去重、展示
- 数据存储（PostgreSQL / ClickHouse / Elasticsearch）
- Web 控制台（可选）

**测试**：
- 单元测试：规则引擎、配置解析
- 集成测试：模拟事件流
- 压测：高事件量下 CPU/内存表现
- `go test ./...` 保证通过

---

## 常用库速查

| 用途 | 推荐库 |
|------|--------|
| eBPF | `github.com/cilium/ebpf` |
| 文件监控 | `github.com/fsnotify/fsnotify` |
| 配置解析 | `github.com/spf13/viper` / `gopkg.in/yaml.v3` |
| 结构化日志 | `log/slog`（标准库）/ `github.com/rs/zerolog` |
| gRPC | `google.golang.org/grpc` |
| 队列/存储 | `modernc.org/sqlite` / 文件缓存 |
| 进程信息 | `github.com/shirou/gopsutil/v3` |

## 开发顺序建议

按**阶段一 → 二 → 三 → 四 → 五 → 六 → 七 → 八**顺序推进。每个阶段独立可交付，先跑通采集（二三四），再做检测（五），最后响应和上报（六七）。

## 注意事项

1. **权限**：采集类操作通常需要 root 权限，Agent 建议以 root 运行
2. **性能**：eBPF 优先，避免高频轮询 /proc
3. **安全**：Agent 自身是攻击目标，务必做防篡改和资源限制
4. **合规**：涉及用户行为监控需符合当地隐私法规
5. **内核兼容**：eBPF 特性依赖内核版本，做好降级方案（如回退到 fanotify/auditd）
