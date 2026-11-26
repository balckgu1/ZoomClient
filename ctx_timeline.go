package main

// 本文件实现上下文洞察快照的持久化辅助：
// saveCtxTimeline 在每轮会话保存时同步落盘 {id}.context.json，
// restoreCtxTimeline 在会话加载时恢复折叠器状态（无快照的旧会话降级为空）。

import (
	"encoding/json"

	"zoomClient/ctxinsight"
	"zoomClient/logger"
	"zoomClient/session"

	"go.uber.org/zap"
)

// saveCtxTimeline 将当前洞察状态随会话一并保存。失败仅告警，不影响会话主流程。
func saveCtxTimeline(s *AgentSession, mgr *session.Manager, id string) {
	if s.CtxRecorder == nil || mgr == nil || id == "" {
		return
	}
	data, err := json.Marshal(s.CtxRecorder.Export())
	if err != nil {
		logger.Log.Warn("marshal context timeline failed", zap.Error(err))
		return
	}
	if err := mgr.SaveContext(id, data); err != nil {
		logger.Log.Warn("save context timeline failed", zap.String("id", id), zap.Error(err))
	}
}

// restoreCtxTimeline 会话加载时恢复洞察状态；快照缺失或损坏时重置为空 timeline。
// 阈值以当前配置为准（快照中的旧阈值可能已过期）；模型名保持当前值——
// 后端加载会话并不切换模型客户端，洞察应反映实际在用的模型。
func restoreCtxTimeline(s *AgentSession, mgr *session.Manager, id string, limitBytes int) {
	if s.CtxRecorder == nil {
		return
	}
	if mgr != nil {
		if data, err := mgr.LoadContext(id); err == nil {
			var st ctxinsight.State
			if json.Unmarshal(data, &st) == nil {
				s.CtxRecorder.Restore(st)
				s.CtxRecorder.SetLimit(limitBytes)
				return
			}
			logger.Log.Warn("corrupt context timeline, reset to empty", zap.String("id", id))
		}
	}
	s.CtxRecorder.Reset()
	s.CtxRecorder.SetLimit(limitBytes)
}
