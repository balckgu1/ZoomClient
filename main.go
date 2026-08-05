package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"zoomClient/fsm"
	"zoomClient/hook"
	"zoomClient/logger"
	"zoomClient/model"
	"zoomClient/prompt"
	"zoomClient/session"
	"zoomClient/skills"
	"zoomClient/tools"
	"zoomClient/utils"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func main() {
	// Parse flags
	flags := parseFlags()

	// Initialize logger
	logger.Init()
	defer logger.Sync()
	log := logger.Log

	// Load Config
	utils.InitConfigWithDir(flags.ConfigDir)
	cfg := utils.GetConfig()

	// Context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Initialize emitter & UI
	em, view, webSess := initEmitter(flags.OutputMode)

	// Initialize model client
	client, modelname := initClient(flags.ModelType, cfg, em)
	if webSess != nil {
		webSess.Model = modelname
	}

	// Initialize Model Registry
	configDir := flags.ConfigDir
	if configDir == "" {
		configDir = "./config"
	}
	modelRegistry := model.NewRegistry(configDir + "/models.yaml")

	// Register default presets from config.yaml
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

	// Set active preset to current model type
	modelRegistry.SetActive(flags.ModelType)

	// Build tool context
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
		// check the directory exists
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

	// Load skills
	skillregistry, err := skills.NewSkillRegistry(cfg.Skills.Dir)
	if err != nil {
		log.Warn("Load skills failed, continue with empty registry", zap.Error(err))
		skillregistry, _ = skills.NewSkillRegistry("")
	}

	// Build system prompt pipeline
	promptBuilder := prompt.NewSystemPromptBuilder(skillregistry, cfg.Memory.Dir, modelname, toolCtx.WorkPath)
	pipeline := prompt.NewPipeline(promptBuilder)
	log.Info("MessagePipeline initialized")

	// Initialize Session state
	state := &fsm.State{Messages: []fsm.Message{}, TurnCount: 0}

	// Permission system
	permitMgr := initPermissionManager(flags.OutputMode, cfg, webSess)
	log.Info("Permission system has been enabled",
		zap.String("Mode", string(permitMgr.GetMode())),
		zap.Int("Deny rules", len(cfg.Permission.DenyRules)),
		zap.Int("Allow rules", len(cfg.Permission.AllowRules)),
	)

	// Register tools and subagent
	registry, todoManager, compactManager := initTools(cfg, client, modelname, skillregistry, toolCtx, state, permitMgr)
	registry.SetPermissionDecider(permitMgr.Decide)

	// Hook system
	hookRunner := initHookRunner()
	log.Info("Hook system has been enabled")

	// Assemble session & start
	sess := &AgentSession{
		State: state, Cfg: cfg, Client: client, ModelName: modelname,
		ModelRegistry: modelRegistry,
		Pipeline:      pipeline, Registry: registry, ToolCtx: toolCtx,
		TodoManager: todoManager, CompactManager: compactManager,
		HookRunner: hookRunner, Em: em, PermissionMgr: permitMgr,
	}

	hookRunner.Run(hook.EventSessionStart, map[string]any{"model": modelname, "pipeline": "active"})
	if em != nil {
		em.EmitSessionStart(modelname, logger.LogFilePath)
	}
	log.Info("Agent REPL start")

	// Initialize session manager (all modes)
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
		// Web mode: also bind record ID to web session
		if webSess != nil {
			webSess.RecordID = initialRecord.ID
		}
	}

	// Run REPL by mode
	switch flags.OutputMode {
	case "web":
		runWebREPL(ctx, sess, webSess, flags.WebPort, sessMgr)
	default:
		runCLIREPL(sess, view)
	}

	// Session cleanup
	log.Info("Agent REPL End", zap.Int("total_turns", state.TurnCount))
	if em != nil {
		em.EmitSessionEnd(state.TurnCount)
	}
	hookRunner.Run(hook.EventSessionEnd, map[string]any{"total_turns": state.TurnCount})
}
