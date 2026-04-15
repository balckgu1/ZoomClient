package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log 全局 Zap 日志实例
var Log *zap.Logger

// Init 初始化全局日志记录器（开发模式：彩色、人类可读格式）
func Init() {
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder // 彩色日志级别
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
	var err error
	Log, err = cfg.Build(zap.AddCallerSkip(0))
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
}

// Sync 刷新日志缓冲区，程序退出前调用
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
