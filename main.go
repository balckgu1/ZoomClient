package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"zoomClient/clients"
	"zoomClient/ctxinsight"
	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/memory"
	"zoomClient/model"
	"zoomClient/prompt"
	"zoomClient/session"
	"zoomClient/skills"
	"zoomClient/tools"
	"zoomClient/utils"
	"zoomClient/web"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func main() {
	// 解析命令行参数
	flags := parseFlags()

	// 初始化日志
	logger.Init()
	defer logger.Sync()
	log := logger.Log

	// 加载配置
	utils.InitConfigWithDir(flags.ConfigDir)
	cfg := utils.GetConfig()

	// 初始化信号处理上下文，用于优雅关闭
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// 初始化发射器与 UI
	em, view, webSess := initEmitter(flags.OutputMode)

	// 初始化模型客户端
	client, modelname := initClient(flags.ModelType, cfg, em)
	if webSess != nil {
		webSess.Model = modelname
	}

	// 初始化模型注册表
	configDir := flags.ConfigDir
	if configDir == "" {
		configDir = "./config"
	}
	modelRegistry := model.NewRegistry(configDir + "/models.yaml")

	// 从 config.yaml 注册默认预设
	if cfg.OpenAI.ApiKey != "" || cfg.OpenAI.BaseURL != "" {
		modelRegistry.RegisterDefault(&model.Preset{
			Name:      "openai",
			Type:      "openai",
			BaseURL:   cfg.OpenAI.BaseURL,
			APIKey:    cfg.OpenAI.ApiKey,
			ModelName: cfg.OpenAI.ModelName,
		})
	}
	if cfg.Ollama.BaseURL != "" {
		modelRegistry.RegisterDefault(&model.Preset{
			Name:      "ollama",
			Type:      "ollama",
			BaseURL:   cfg.Ollama.BaseURL,
			ModelName: cfg.Ollama.ModelName,
		})
	}

	// 将当前模型类型设为激活预设
	modelRegistry.SetActive(flags.ModelType)

	// 构建工具上下文
	workDir := flags.WorkDir
	var err error
	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			log.Fatal("Failed to get working directory", zap.Error(err))
		}
	} else {
		workDir, err = filepath.Abs(workDir)
		if err != nil {
			log.Fatal("Failed to parse working directory to abslute path", zap.Error(err))
		}
		// 检查目录是否存在
		if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
			log.Fatal("Invalid work directory", zap.String("dir", workDir))
		}
	}

	toolCtx := &tools.ToolContext{
		WorkPath:           workDir,
		Ctx:                ctx,
		DefaultBashTimeout: time.Duration(cfg.Tools.DefaultBashTimeout) * time.Second,
		Logger:             log.Named("tool"),
		SessionID:          uuid.NewString(),
		AppState:           map[string]any{"turn": 0},
	}

	// 结构化提问通道：Web 模式通过 SSE 弹窗交互，CLI 模式通过标准输入交互
	if webSess != nil {
		toolCtx.UserAsker = web.NewWebQuestionAsker(webSess)
	} else {
		toolCtx.UserAsker = &tools.StdinUserAsker{}
	}

	// 加载skill
	skillregistry, err := skills.NewSkillRegistry(cfg.Skills.Dir)
	if err != nil {
		log.Warn("Load skills failed, continue with empty registry", zap.Error(err))
		skillregistry, _ = skills.NewSkillRegistry("")
	}

	// 构建系统提示词流水线
	promptBuilder := prompt.NewSystemPromptBuilder(skillregistry, cfg.Memory.Dir, modelname, toolCtx.WorkPath)
	pipeline := prompt.NewPipeline(promptBuilder)
	log.Info("MessagePipeline initialized")

	// 初始化会话状态
	state := &fsm.State{Messages: []clients.Message{}, TurnCount: 0}

	// 权限系统
	permitMgr := initPermissionManager(flags.OutputMode, cfg, webSess)
	log.Info("Permission system has been enabled",
		zap.String("Mode", string(permitMgr.GetMode())),
		zap.Int("Deny rules", len(cfg.Permission.DenyRules)),
		zap.Int("Allow rules", len(cfg.Permission.AllowRules)),
	)

	// 注册工具与子代理
	registry, todoManager, compactManager := initTools(cfg, client, modelname, skillregistry, toolCtx, state, permitMgr)
	registry.SetPermissionDecider(permitMgr.Decide)

	// 钩子系统
	hookRunner, metricsFile := initHookRunner(cfg)
	if metricsFile != nil {
		defer func() {
			if err := metricsFile.Close(); err != nil {
				log.Warn("metrics file close failed", zap.Error(err))
			}
		}()
	}
	log.Info("Hook system has been enabled")

	// 组装会话并启动
	sess := &AgentSession{
		State:          state,
		Cfg:            cfg,
		Client:         client,
		ModelName:      modelname,
		ModelRegistry:  modelRegistry,
		Pipeline:       pipeline,
		Registry:       registry,
		ToolCtx:        toolCtx,
		SkillRegistry:  skillregistry,
		TodoManager:    todoManager,
		CompactManager: compactManager,
		HookRunner:     hookRunner,
		Em:             em,
		PermissionMgr:  permitMgr,
		Temperature:    0.7,
		ReasonLevel:    "high",
		CtxRecorder:    ctxinsight.New(cfg.Compact.ContextLimit, modelname),
	}

	// Per-turn 同步增量记忆抽取器：回合结束时内联阻塞执行，跑完才结束该轮
	if cfg.Memory.AutoExtractOnTurn && cfg.Memory.Dir != "" {
		sess.MemoryExtractor = memory.NewExtractor(cfg.Memory.Dir, em)
		log.Info("Per-turn memory extraction enabled (synchronous)")
	}

	// hook1: 会话开始时
	hookRunner.HookRun(hook.EventSessionStart, map[string]any{"model": modelname, "pipeline": "active", "session_id": toolCtx.SessionID})
	if em != nil {
		em.EmitSessionStart(modelname, logger.LogFilePath)
	}
	log.Info("Agent REPL start")

	// 初始化会话管理器
	var sessMgr *session.Manager
	sessMgr, serr := session.NewManager(cfg.Session.Dir, client, modelname)
	if serr != nil {
		log.Warn("Session manager init failed, history disabled", zap.Error(serr))
	} else {
		initialRecord := sessMgr.CreateSession()
		sess.SessionMgr = sessMgr
		sess.SessionRecordID = initialRecord.ID
		sess.IsNewSession = true
		log.Info("Session history enabled", zap.String("dir", cfg.Session.Dir))
		// Web 模式：同时将记录 ID 绑定到 Web 会话
		if webSess != nil {
			webSess.RecordID = initialRecord.ID
		}
	}

	// 按模式运行 REPL
	switch flags.OutputMode {
	case "web":
		runWebREPL(ctx, sess, webSess, flags.WebPort, sessMgr)
	default:
		runCLIREPL(sess, view)
	}

	// 会话清理
	log.Info("Agent REPL End", zap.Int("total_turns", state.TurnCount))
	if em != nil {
		em.EmitSessionEnd(state.TurnCount)
	}
	hookRunner.HookRun(hook.EventSessionEnd, map[string]any{"total_turns": state.TurnCount})
}
