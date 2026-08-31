package main

import (
	"flag"
	"io"
	"os"
	"strings"

	"zoomClient/clients"
	"zoomClient/compact"
	"zoomClient/emitter"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/memory"
	"zoomClient/permission"
	"zoomClient/skills"
	"zoomClient/subagent"
	"zoomClient/tools"
	"zoomClient/ui"
	"zoomClient/utils"
	"zoomClient/web"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// cliFlags holds parsed command-line flags.
type cliFlags struct {
	ModelType  string
	OutputMode string
	ConfigDir  string
	WebPort    int
	WorkDir    string
}

func parseFlags() cliFlags {
	var f cliFlags
	flag.StringVar(&f.ModelType, "m", "openai", "Model backend type: ollama | openai | anthropic | gemini, default: openai")
	flag.StringVar(&f.OutputMode, "mode", "cli", "Output mode: cli (terminal) | api (NDJSON for Tauri sidecar) | web (browser UI)")
	flag.StringVar(&f.ConfigDir, "config-dir", "", "Config directory path (default: ./config)")
	flag.StringVar(&f.WorkDir, "workdir", "", "Project working directory (default: current directory)")
	flag.IntVar(&f.WebPort, "port", 8080, "Web server port (web mode only)")
	flag.Parse()
	return f
}

// initEmitter creates the appropriate emitter and optional session based on output mode.
// CLI 模式返回 ui.Renderer（同时充当 emitter），供 runCLIREPL 渲染与读取输入。
func initEmitter(outputMode string) (emitter.Emitter, *ui.Renderer, *web.Session) {
	log := logger.Log
	switch strings.ToLower(outputMode) {
	case "api":
		return emitter.NewApiEmitter(os.Stdout), nil, nil
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

// initClient creates the ChatClient and resolves model name based on backend type.
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

// initTools creates the tool registry and registers all tools.
func initTools(cfg *utils.Config, client clients.ChatClient, modelname string,
	skillregistry *skills.SkillRegistry, toolCtx *tools.ToolContext, state *fsm.State,
	permitMgr *permission.Manager) (*tools.ToolRegister, *tools.TodoManager, *compact.CompactManager) {
	log := logger.Log

	registry := tools.NewToolRegister()

	// Register basic tools
	registry.Register(tools.WriteFileTool{})
	registry.Register(tools.EditFileTool{})
	registry.Register(tools.ReadFileTool{})
	registry.Register(tools.ListDirectory{})
	registry.Register(tools.RunBashTool{})
	registry.Register(tools.GlobSearch{})

	// Register load_skills tool
	registry.Register(skills.NewLoadSkillTool(skillregistry))

	// Register memory tools
	registry.Register(memory.NewSaveMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewSearchMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewUpdateMemoryTool(cfg.Memory.Dir))
	registry.Register(memory.NewDeleteMemoryTool(cfg.Memory.Dir))

	// Instantiate and register todo manager
	todoManager := tools.NewTodoManager()
	registry.Register(todoManager)

	// Instantiate and register compact manager
	compactManager := compact.NewCompactManager(compact.DefaultConfig(*cfg), client, modelname)
	registry.Register(compact.NewCompactTool(compactManager))

	// Create subagent and register sub_task tool
	options := map[string]interface{}{
		"temperature": cfg.Subagent.Temperature,
	}
	subAgent := subagent.NewSubAgent(client, modelname, cfg.Subagent.DefaultSystemPrompt, cfg.Subagent.ForkSubtaskPromptPrefix,
		subagent.BuildSubAgentRegistry(), toolCtx, cfg.Subagent.DefaultMaxTurns, options)

	subAgentRunner := func(prompt string, parentMessages []fsm.Message) (string, error) {
		if parentMessages == nil {
			return subAgent.Run(prompt)
		}
		return subAgent.RunWithFork(prompt, parentMessages)
	}
	parentMessagesProvider := func() []fsm.Message {
		return state.Messages
	}
	registry.Register(subagent.NewTaskTool(subAgentRunner, parentMessagesProvider))

	// Share permission policy with subagent's independent registry
	if subAgent.Registry != nil {
		subAgent.Registry.SetPermissionDecider(permitMgr.Decide)
	}

	log.Info("Registered tool list", zap.Any("tools", registry.GetAllNames()))
	return registry, todoManager, compactManager
}

// initPermissionManager creates the permission manager based on output mode.
func initPermissionManager(outputMode string, cfg *utils.Config, webSess *web.Session) *permission.Manager {
	var asker permission.Asker
	switch outputMode {
	case "web":
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

// buildHookRunner constructs a hook runner
func initHookRunner() *hook.Runner {
	// Build a new hook runner instance
	runner := hook.NewRunner()

	// Register hooks for session start
	runner.HookRegister(hook.EventSessionStart, hook.OnSessionStart)

	runner.HookRegister(hook.EventPreToolUse, hook.PreToolBlockDangerous)
	runner.HookRegister(hook.EventPreToolUse, hook.PreToolRateLimit)
	runner.HookRegister(hook.EventPreToolUse, hook.PreToolSensitiveFileGuard)

	runner.HookRegister(hook.EventPostToolUse, hook.PostToolAuditLog)
	runner.HookRegister(hook.EventToolError, hook.OnToolErrorRecovery)

	runner.HookRegister(hook.EventSessionEnd, hook.OnSessionEnd)
	return runner
}

// buildAsker selects the interaction method when ask is triggered based on config.
//   - interactive=true + CLI mode   → StdinAsker（终端文本交互）
//   - interactive=false             → DenyAsker（安全默认值）
func buildAsker(interactive bool, apiMode bool, w io.Writer) permission.Asker {
	if !interactive {
		return permission.DenyAsker{}
	}
	return permission.NewStdinAsker()
}
