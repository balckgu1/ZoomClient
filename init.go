package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/emitter"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/memory"
	"zoomClient/metrics"
	"zoomClient/permission"
	"zoomClient/skills"
	"zoomClient/subagent"
	"zoomClient/task"
	"zoomClient/tools"
	"zoomClient/ui"
	"zoomClient/utils"
	"zoomClient/web"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// cliFlags 保存解析后的命令行标志。
type cliFlags struct {
	ModelType  string
	OutputMode string
	ConfigDir  string
	WebPort    int
	WorkDir    string
}

// parseFlags 解析 os.Args 中的命令行标志。
func parseFlags() cliFlags {
	return parseFlagsFrom(os.Args[1:])
}

// parseFlagsFrom 按给定参数解析标志。使用独立的 FlagSet 而非全局 flag.CommandLine，
// 既保留 -h 的用法输出，也便于单元测试（不受进程全局状态影响）。
func parseFlagsFrom(args []string) cliFlags {
	var f cliFlags
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&f.ModelType, "m", "openai", "Model backend type: ollama | openai | anthropic | gemini, default: openai")
	fs.StringVar(&f.OutputMode, "mode", "cli", "Output mode: cli (terminal) | web (browser UI)")
	// webAlias 是历史别名，等价于 -mode web，保留以兼容旧脚本与旧版文档。
	webAlias := fs.Bool("web", false, "Shorthand for -mode web (deprecated, use -mode web)")
	fs.StringVar(&f.ConfigDir, "config-dir", "", "Config directory path (default: ./config)")
	fs.StringVar(&f.WorkDir, "workdir", "", "Project working directory (default: current directory)")
	fs.IntVar(&f.WebPort, "port", 8080, "Web server port (web mode only)")
	// Parse 出错时 ExitOnError 会直接退出，与改造前的 flag.Parse 行为一致；
	// 因此下面的错误分支实际不可达，显式处理只是为了满足 errcheck（check-blank）。
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	// 显式传入 -mode 时以 -mode 为准，否则 -web 等价于 -mode web。
	modeSet := false
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "mode" {
			modeSet = true
		}
	})
	if *webAlias && !modeSet {
		f.OutputMode = "web"
	}
	return f
}

// initEmitter 根据输出模式创建相应的发射器和可选会话。
// CLI 模式返回 ui.Renderer，供 runCLIREPL 渲染与读取输入。
func initEmitter(outputMode string) (emitter.Emitter, *ui.Renderer, *web.Session) {
	log := logger.Log
	switch strings.ToLower(outputMode) {
	case "web":
		sess := web.NewSession(uuid.NewString(), "")
		return web.NewSseEmitter(sess), nil, sess
	case "cli", "":
		view := ui.New()
		return view, view, nil
	default:
		log.Fatal("Unsupported output mode", zap.String("--mode", outputMode))
		return nil, nil, nil
	}
}

// initClient 创建 ChatClient 并根据后端类型解析模型名称。
func initClient(modelType string, cfg *utils.Config, em emitter.Emitter) (clients.ChatClient, string) {
	log := logger.Log
	switch strings.ToLower(modelType) {
	case "openai":
		apiKey := cfg.OpenAI.ApiKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if apiKey == "" {
			if em != nil {
				em.EmitError("config", "Please set the API key")
			}
			log.Fatal("No API key OPENAI_API_KEY")
		}
		baseURL := cfg.OpenAI.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		modelname := cfg.OpenAI.ModelName
		if modelname == "" {
			modelname = "gpt-4o"
		}
		client := clients.NewOpenAIClient(baseURL, apiKey)
		log.Info("OpenAI backend has been selected", zap.String("model", modelname), zap.String("base_url", baseURL))
		return client, modelname
	case "ollama", "":
		if cfg.Ollama.BaseURL == "" {
			log.Fatal("BaseURL is empty", zap.String("base_url", cfg.Ollama.BaseURL))
		}
		client := clients.NewOllamaClient(cfg.Ollama.BaseURL)
		if cfg.Ollama.ModelName == "" {
			log.Fatal("modelName is empty", zap.String("model_name", cfg.Ollama.ModelName))
		}
		log.Info("Olama backend has been selected", zap.String("model", cfg.Ollama.ModelName))
		return client, cfg.Ollama.ModelName
	case "anthropic":
		apiKey := cfg.Anthropic.ApiKey
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if apiKey == "" {
			if em != nil {
				em.EmitError("config", "Please set the API key ANTHROPIC_API_KEY")
			}
			log.Fatal("No API key ANTHROPIC_API_KEY")
		}
		client := clients.NewAnthropicClient(apiKey)
		modelname := cfg.Anthropic.ModelName
		if modelname == "" {
			modelname = "claude-opus-4-8"
		}
		log.Info("Anthropic backend has been selected", zap.String("model", modelname))
		return client, modelname
	case "gemini":
		apiKey := cfg.Gemini.ApiKey
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if apiKey == "" {
			if em != nil {
				em.EmitError("config", "Please set the API key GEMINI_API_KEY")
			}
			log.Fatal("No API key GEMINI_API_KEY")
		}
		client := clients.NewGeminiClient(apiKey)
		modelname := cfg.Gemini.ModelName
		if modelname == "" {
			modelname = "gemini-3.5-flash"
		}
		log.Info("Gemini backend has been selected", zap.String("model", modelname))
		return client, modelname
	default:
		if em != nil {
			em.EmitError("config", "Unsupported model backend types: "+modelType)
		}
		log.Fatal("Unsupported model backend types", zap.String("-m", modelType))
		return nil, ""
	}
}

// initTools 创建工具注册表并注册所有工具。
func initTools(cfg *utils.Config, client clients.ChatClient, modelname string,
	skillregistry *skills.SkillRegistry, toolCtx *tools.ToolContext, state *fsm.State,
	permitMgr *permission.Manager) (*tools.ToolRegister, *tools.TodoManager, *compact.CompactManager) {

	// 日志记录器
	log := logger.Log

	registry := tools.NewToolRegister()

	// 注册基础工具
	registry.Register(tools.WriteFileTool{})
	registry.Register(tools.EditFileTool{})
	registry.Register(tools.ReadFileTool{})
	registry.Register(tools.ListDirectory{})
	registry.Register(tools.RunBashTool{})
	registry.Register(tools.GlobSearch{})

	// 注册 load_skills 工具
	registry.Register(skills.NewLoadSkillTool(skillregistry))

	// 注册记忆工具
	registry.Register(memory.NewSaveMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewSearchMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewUpdateMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewDeleteMemoryTool(cfg.Memory.Dir))

	// 实例化并注册待办管理器
	todoManager := tools.NewTodoManager()
	registry.Register(todoManager)

	// 实例化并注册任务管理器
	taskManager := task.NewTaskManager(cfg.Tasks.Dir)
	registry.Register(task.NewCreateTaskTool(taskManager))
	registry.Register(task.NewListTasksTool(taskManager))
	registry.Register(task.NewGetTaskTool(taskManager))
	registry.Register(task.NewClaimTaskTool(taskManager))
	registry.Register(task.NewCompleteTaskTool(taskManager))

	// 实例化并注册压缩管理器
	compactManager := compact.NewCompactManager(compact.DefaultConfig(*cfg), client, modelname)
	registry.Register(compact.NewCompactTool(compactManager))

	// 创建子代理并注册子任务工具
	options := map[string]interface{}{
		"temperature": cfg.Subagent.Temperature,
	}
	subAgent := subagent.NewSubAgent(client, modelname, cfg.Subagent.DefaultSystemPrompt, cfg.Subagent.ForkSubtaskPromptPrefix,
		subagent.BuildSubAgentRegistry(), toolCtx, cfg.Subagent.DefaultMaxTurns, options)

	subAgentRunner := func(prompt string, parentMessages []clients.Message) (string, error) {
		if parentMessages == nil {
			return subAgent.Run(prompt)
		}
		return subAgent.RunWithFork(prompt, parentMessages)
	}
	parentMessagesProvider := func() []clients.Message {
		return state.Messages
	}
	registry.Register(subagent.NewTaskTool(subAgentRunner, parentMessagesProvider))

	// 与子代理的独立注册表共享权限策略
	if subAgent.Registry != nil {
		subAgent.Registry.SetPermissionDecider(permitMgr.Decide)
	}

	log.Info("Registered tool list", zap.Any("tools", registry.GetAllNames()))
	return registry, todoManager, compactManager
}

// initPermissionManager 根据输出模式创建权限管理器。
func initPermissionManager(outputMode string, cfg *utils.Config, webSess *web.Session) *permission.Manager {
	var asker permission.Asker
	switch outputMode {
	case "web":
		// Web 模式创建一个新的 asker
		asker = web.NewWebAsker(webSess)
	default:
		// CLI 模式使用 StdinAsker 进行终端文本交互
		asker = permission.NewStdinAsker()
	}
	return permission.NewManager(
		permission.Mode(cfg.Permission.Mode),
		permission.BuildPermissionRules(cfg.Permission.DenyRules),
		permission.BuildPermissionRules(cfg.Permission.AllowRules),
		asker,
	)
}

// defaultMetricsPath 指标 JSONL 默认输出路径
const defaultMetricsPath = "logs/metrics.jsonl"

// openMetricsFile 打开指标 JSONL 输出文件（目录不存在时自动创建）。
// path 为空时使用默认路径；返回实际使用的路径供日志输出。
func openMetricsFile(path string) (*os.File, string, error) {
	if path == "" {
		path = defaultMetricsPath
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, path, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, path, err
	}
	return f, path, nil
}

// initHookRunner 构建 hook Runner；启用指标采集时一并创建采集器并最先注册，
// 返回写句柄（未启用或打开失败时为 nil），由调用方在退出时关闭。
func initHookRunner(cfg *utils.Config) (*hook.Runner, io.WriteCloser) {
	log := logger.Log
	// 构建新的 hook runner 实例
	runner := hook.NewRunner()

	// 指标采集（可选）：纯观察者，必须先于业务 handler 注册，
	// 否则业务 handler 返回 Block/Retry 时 Runner 短路会跳过采集造成漏记
	var metricsWC io.WriteCloser
	if mcfg := cfg.Observability.Metrics; mcfg.Enabled {
		f, path, err := openMetricsFile(mcfg.Path)
		if err != nil {
			log.Warn("Metrics collector disabled", zap.String("path", path), zap.Error(err))
		} else {
			metricsWC = f
			metrics.NewCollector(f).Register(runner)
			log.Info("Metrics collector enabled", zap.String("path", path))
		}
	}

	// 注册会话开始的 hooks
	runner.HookRegister(hook.EventSessionStart, hook.OnSessionStart)

	// 注册 LLM 调用相关的 hooks（聊天前后 + 错误重试）
	runner.HookRegister(hook.EventPreChat, hook.PreChatAuditLog)
	runner.HookRegister(hook.EventPostChat, hook.PostChatValidate)
	runner.HookRegister(hook.EventLLMError, hook.OnLLMErrorRetry)

	// 注册工具使用的 hooks
	runner.HookRegister(hook.EventPreToolUse, hook.PreToolBlockDangerous)
	runner.HookRegister(hook.EventPreToolUse, hook.PreToolRateLimit)
	runner.HookRegister(hook.EventPreToolUse, hook.PreToolSensitiveFileGuard)

	// 注册工具使用完成与错误的 hooks
	runner.HookRegister(hook.EventPostToolUse, hook.PostToolAuditLog)
	runner.HookRegister(hook.EventToolError, hook.OnToolErrorRecovery)

	// 注册会话结束的 hooks
	runner.HookRegister(hook.EventSessionEnd, hook.OnSessionEnd)
	return runner, metricsWC
}

// buildAsker 在触发询问时根据配置选择交互方式。
//   - interactive=true + CLI mode   → StdinAsker（终端文本交互）
//   - interactive=false             → DenyAsker（安全默认值）
func buildAsker(interactive bool, apiMode bool, w io.Writer) permission.Asker {
	if !interactive {
		return permission.DenyAsker{}
	}
	return permission.NewStdinAsker()
}
