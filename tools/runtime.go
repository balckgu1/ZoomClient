package tools

import (
	"fmt"
	"sync"
)

// ===================== 常量与类型定义 =====================

// ToolStatus 表示工具执行的当前状态
type ToolStatus string

const (
	ToolStatusQueued    ToolStatus = "queued"    // 排队等待执行
	ToolStatusExecuting ToolStatus = "executing" // 正在执行中
	ToolStatusCompleted ToolStatus = "completed" // 已完成
	ToolStatusYielded   ToolStatus = "yielded"   // 已产出中间进度
)

// ContextModifier 上下文修改器函数类型。
// 接收当前 ToolContext 指针，对其进行修改。
type ContextModifier func(ctx *ToolContext)

// ===================== 核心数据结构 =====================

// TrackedTool 跟踪单个工具调用的完整生命周期。
// 包含工具的基本信息、执行状态、进度消息、最终结果和上下文修改器。
type TrackedTool struct {
	ID                string            // 工具调用唯一标识
	Name              string            // 工具名称
	Args              map[string]any    // 工具调用参数
	Status            ToolStatus        // 当前执行状态
	IsConcurrencySafe bool              // 是否允许并发执行
	PendingProgress   []string          // 尚未消费的中间进度消息
	Result            *ToolResult       // 最终执行结果（未完成时为 nil）
	ContextModifiers  []ContextModifier // 工具执行产生的上下文修改器
}

// ToolExecutionBatch 将多个工具调用按并发安全性分为一批。
// 同一批次内的工具要么全部可并发，要么全部需串行。
type ToolExecutionBatch struct {
	IsConcurrencySafe bool           // 该批次内的工具是否可并发执行
	Tools             []*TrackedTool // 该批次包含的工具列表
}

// MessageUpdate 工具执行过程中产出的更新信息。
// 可以是中间进度消息，也可以是最终结果。
type MessageUpdate struct {
	ToolID  string      // 产生该更新的工具 ID
	Message string      // 进度或结果消息内容
	Result  *ToolResult // 最终结果（非 nil 时表示工具已完成）
}

// ===================== 并发安全性判断 =====================

// concurrencySafeTools 记录所有可以安全并发执行的工具名称
var concurrencySafeTools = map[string]bool{
	"read_file": true,
}

// IsConcurrencySafe 判断指定工具是否可以安全地与其他工具并发执行。
// 默认只有明确标记的只读类工具才被视为并发安全。
func IsConcurrencySafe(toolName string) bool {
	_, exists := concurrencySafeTools[toolName]
	return exists
}

// ===================== 第一步：分批 =====================

// PartitionToolCalls 将一组工具调用按并发安全性划分为多个批次。
func PartitionToolCalls(toolCalls []ToolCall) []*ToolExecutionBatch {
	if len(toolCalls) == 0 {
		return nil
	}

	var batches []*ToolExecutionBatch
	var currentBatch *ToolExecutionBatch

	for index, call := range toolCalls {
		safe := IsConcurrencySafe(call.Function.Name)

		// 为每个工具调用创建 TrackedTool
		tracked := &TrackedTool{
			ID:                fmt.Sprintf("tool_%d", index),
			Name:              call.Function.Name,
			Args:              call.Function.Arguments,
			Status:            ToolStatusQueued,
			IsConcurrencySafe: safe,
		}

		// 如果当前批次为空，或并发安全性发生变化，则新建一个批次
		if currentBatch == nil || currentBatch.IsConcurrencySafe != safe {
			currentBatch = &ToolExecutionBatch{
				IsConcurrencySafe: safe,
				Tools:             []*TrackedTool{tracked},
			}
			batches = append(batches, currentBatch)
		} else {
			currentBatch.Tools = append(currentBatch.Tools, tracked)
		}
	}

	return batches
}

// ===================== 第二步 + 第三步：执行批次 =====================

// ExecuteBatches 按顺序执行所有批次。
// 并发安全的批次使用 goroutine 并行执行，不安全的批次逐个串行执行。
// 所有结果最终按原始工具顺序回写到 results 切片中。
func ExecuteBatches(batches []*ToolExecutionBatch, registry *Registry, toolCtx *ToolContext) []ToolResult {
	var allResults []ToolResult

	for _, batch := range batches {
		if batch.IsConcurrencySafe {
			// 并发执行：所有工具同时运行，结果按原始顺序收集
			batchResults := runConcurrently(batch.Tools, registry, toolCtx)
			allResults = append(allResults, batchResults...)
		} else {
			// 串行执行：逐个运行，保证写操作不冲突
			batchResults := runSerially(batch.Tools, registry, toolCtx)
			allResults = append(allResults, batchResults...)
		}
	}

	return allResults
}

// runConcurrently 并发执行一批并发安全的工具。
// 使用 goroutine 并行运行，但结果严格按原始顺序存放到切片中。
func runConcurrently(trackedTools []*TrackedTool, registry *Registry, toolCtx *ToolContext) []ToolResult {
	results := make([]ToolResult, len(trackedTools))
	var waitGroup sync.WaitGroup

	for index, tracked := range trackedTools {
		waitGroup.Add(1)
		// 捕获循环变量，启动 goroutine 并发执行
		go func(idx int, tool *TrackedTool) {
			defer waitGroup.Done()

			// 更新状态为执行中
			tool.Status = ToolStatusExecuting

			// 调用工具
			result := registry.RunTool(tool.Name, tool.Args, toolCtx)

			// 更新状态为已完成，保存结果
			tool.Status = ToolStatusCompleted
			tool.Result = &result

			// 按原始索引写入结果，保证顺序稳定
			results[idx] = result
		}(index, tracked)
	}

	waitGroup.Wait()
	return results
}

// runSerially 串行执行一批需要独占的工具。
// 逐个执行并收集结果，保证写操作之间不会相互干扰。
func runSerially(trackedTools []*TrackedTool, registry *Registry, toolCtx *ToolContext) []ToolResult {
	results := make([]ToolResult, 0, len(trackedTools))

	for _, tracked := range trackedTools {
		// 更新状态为执行中
		tracked.Status = ToolStatusExecuting

		// 调用工具
		result := registry.RunTool(tracked.Name, tracked.Args, toolCtx)

		// 更新状态为已完成，保存结果
		tracked.Status = ToolStatusCompleted
		tracked.Result = &result

		results = append(results, result)
	}

	return results
}

// ===================== 第四步：上下文修改器队列 =====================

// QueuedContextModifiers 暂存各工具产生的上下文修改器。
// key 为工具 ID，value 为该工具产生的修改器列表。
// 通过暂存而非立即执行，可以在所有工具完成后按原始顺序统一合并。
type QueuedContextModifiers struct {
	modifiers map[string][]ContextModifier // 按工具 ID 暂存修改器
	mu        sync.Mutex                   // 并发写入时的保护锁
}

// NewQueuedContextModifiers 创建新的上下文修改器队列
func NewQueuedContextModifiers() *QueuedContextModifiers {
	return &QueuedContextModifiers{
		modifiers: make(map[string][]ContextModifier),
	}
}

// Add 向队列中添加一个上下文修改器，关联到指定的工具 ID。
// 此方法是并发安全的，多个 goroutine 可以同时调用。
func (queue *QueuedContextModifiers) Add(toolID string, modifier ContextModifier) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	queue.modifiers[toolID] = append(queue.modifiers[toolID], modifier)
}

// ApplyInOrder 按照传入的工具原始顺序，依次应用所有暂存的上下文修改器。
// 这保证了即使工具并发执行完成的顺序不同，上下文修改仍然是确定性的。
func (queue *QueuedContextModifiers) ApplyInOrder(originalOrder []*TrackedTool, toolCtx *ToolContext) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	for _, tracked := range originalOrder {
		if modifiers, exists := queue.modifiers[tracked.ID]; exists {
			for _, modifier := range modifiers {
				modifier(toolCtx)
			}
		}
	}
}

// ===================== 顶层编排入口 =====================

// ExecuteToolCalls 工具执行运行时的顶层入口。
// 完整流程：分批 → 按批次执行（并发/串行） → 按原始顺序收集结果。
func ExecuteToolCalls(toolCalls []ToolCall, registry *Registry, toolCtx *ToolContext) []ToolResult {
	// 第一步：按并发安全性将工具调用分批
	batches := PartitionToolCalls(toolCalls)

	// 第二步 + 第三步：逐批执行，并发批次并行，独占批次串行
	results := ExecuteBatches(batches, registry, toolCtx)

	return results
}
