package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log 全局 Zap 日志实例
var Log *zap.Logger

// LogFilePath 日志文件路径，CLI 前端用它在会话开始时提示用户。
const LogFilePath = "./logs/backend.log"

// Init 初始化全局日志记录器。
//
// 设计：日志只写到 ./logs/backend.log，避免污染 CLI 前端的标准输出；
// 但 ERROR / FATAL 级别同时镜像到 stderr，保证程序异常时用户也能看到。
// 这样实现了"日志在后端文件，前端只显示用户可见信息"的分流。
func Init() {
	if err := os.MkdirAll("./logs", 0o755); err != nil {
		panic("failed to create logs dir: " + err.Error())
	}

	encoderCfg := zap.NewDevelopmentEncoderConfig()
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder // 文件无颜色
	encoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
	encoder := zapcore.NewConsoleEncoder(encoderCfg)

	// 文件 sink：全量 DEBUG 起
	file, err := os.OpenFile(LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		panic("failed to open log file: " + err.Error())
	}
	fileCore := zapcore.NewCore(encoder, zapcore.AddSync(file), zap.DebugLevel)

	// stderr 兜底 sink：仅 ERROR 及以上，方便致命错误时仍能看到
	stderrEncoderCfg := zap.NewDevelopmentEncoderConfig()
	stderrEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	stderrEncoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
	stderrEncoder := zapcore.NewConsoleEncoder(stderrEncoderCfg)
	stderrCore := zapcore.NewCore(stderrEncoder, zapcore.AddSync(os.Stderr), zap.ErrorLevel)

	core := zapcore.NewTee(fileCore, stderrCore)
	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0))
}

// Sync 刷新日志缓冲区，程序退出前调用
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
