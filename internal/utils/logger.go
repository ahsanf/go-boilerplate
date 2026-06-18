package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the raw zap logger kept for compatibility with libraries that
// expect *zap.Logger (e.g. PubSub subscriber constructors).
var Logger *zap.Logger

// Log is the structured app logger; use this in services and handlers.
var Log *AppLogger

// AppLogger wraps zap and exposes
//
//	logger.Info(funcName, className, traceId)
//	logger.Info(funcName, className, traceId, "custom message")
//
// When no message is given it defaults to "call {funcName} from {className}".
type AppLogger struct {
	zl *zap.Logger
}

func (a *AppLogger) Info(funcName, className, traceId string, message ...string) {
	a.zl.Info(logMsg(funcName, className, message...), logFields(funcName, className, traceId)...)
}

func (a *AppLogger) Warn(funcName, className, traceId string, message ...string) {
	a.zl.Warn(logMsg(funcName, className, message...), logFields(funcName, className, traceId)...)
}

func (a *AppLogger) Error(funcName, className, traceId string, message ...string) {
	a.zl.Error(logMsg(funcName, className, message...), logFields(funcName, className, traceId)...)
}

func (a *AppLogger) Fatal(funcName, className, traceId string, message ...string) {
	a.zl.Fatal(logMsg(funcName, className, message...), logFields(funcName, className, traceId)...)
}

// Zap returns the underlying *zap.Logger for use with libraries.
func (a *AppLogger) Zap() *zap.Logger { return a.zl }

func (a *AppLogger) Sync() error { return a.zl.Sync() }

// ── Package-level helpers ─────────────────────────────────────────────────────
// These delegate to the global Log so callers can write utils.Info(...) instead
// of utils.Log.Info(...).

func LogInfo(funcName, className, traceId string, message ...string) {
	Log.Info(funcName, className, traceId, message...)
}

func LogWarn(funcName, className, traceId string, message ...string) {
	Log.Warn(funcName, className, traceId, message...)
}

func LogError(funcName, className, traceId string, message ...string) {
	Log.Error(funcName, className, traceId, message...)
}

func LogFatal(funcName, className, traceId string, message ...string) {
	Log.Fatal(funcName, className, traceId, message...)
}

func LogSync() error { return Log.Sync() }

// InitLogger initialises Log and Logger.
// Must be called after LoadConfig().
func InitLogger() {
	consoleCore := zapcore.NewCore(
		buildConsoleEncoder(),
		zapcore.AddSync(os.Stdout),
		zapcore.DebugLevel,
	)
	fileCore := zapcore.NewCore(
		buildJSONEncoder(),
		zapcore.AddSync(openLogFile()),
		zapcore.InfoLevel,
	)

	zl := zap.New(
		zapcore.NewTee(consoleCore, fileCore),
		zap.WithCaller(false),
	)

	Logger = zl
	Log = &AppLogger{zl: zl}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func logMsg(funcName, className string, message ...string) string {
	if len(message) > 0 && message[0] != "" {
		return message[0]
	}
	return fmt.Sprintf("call %s from %s", funcName, className)
}

func logFields(funcName, className, traceId string) []zap.Field {
	f := []zap.Field{
		zap.String("function", funcName),
		zap.String("class", className),
	}
	if traceId != "" {
		f = append(f, zap.String("trace_id", traceId))
	}
	return f
}

func buildConsoleEncoder() zapcore.Encoder {
	cfg := zap.NewDevelopmentEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.ConsoleSeparator = " "
	return zapcore.NewConsoleEncoder(cfg)
}

func buildJSONEncoder() zapcore.Encoder {
	cfg := zap.NewProductionEncoderConfig()
	cfg.TimeKey = "timestamp"
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	return zapcore.NewJSONEncoder(cfg)
}

func openLogFile() *os.File {
	if err := os.MkdirAll("logs", 0o755); err != nil {
		panic("cannot create logs dir: " + err.Error())
	}
	now := time.Now()
	name := fmt.Sprintf("%d-%d-%d_api.log", now.Year(), int(now.Month()), now.Day())
	f, err := os.OpenFile(filepath.Join("logs", name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		panic("cannot open log file: " + err.Error())
	}
	return f
}
