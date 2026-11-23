package memory

// 本文件提供 memory 目录写入的进程内互斥：
// per-turn 后台抽取 worker 与主 agent（含并行工具批次）可能同时读写
// MEMORY.md 索引与记忆文件，索引更新是"读-改-写"序列，无锁会丢失更新。

import "sync"

// writeMu 保护 save/update/delete 三类工具的完整写入序列
// （存在性检查 → 写/删文件 → 更新索引）。
// 仅覆盖单进程；多实例并发写同一 memory 目录不在保护范围。
var writeMu sync.Mutex
