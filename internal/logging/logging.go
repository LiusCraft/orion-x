package logging

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Level  string
	Format string
}

var (
	baseLogger *zap.Logger
	sugar      *zap.SugaredLogger
	traceID    atomic.Value
	turnID     uint64
)

func init() {
	baseLogger = zap.NewNop()
	sugar = baseLogger.Sugar()
}

func InitFromEnv() error {
	cfg := Config{
		Level:  os.Getenv("LOG_LEVEL"),
		Format: os.Getenv("LOG_FORMAT"),
	}
	return Init(cfg)
}

func Init(cfg Config) error {
	level := strings.ToLower(strings.TrimSpace(cfg.Level))
	if level == "" {
		level = "info"
	}

	format := strings.ToLower(strings.TrimSpace(cfg.Format))
	if format == "" {
		format = "console"
	}

	var zapCfg zap.Config
	switch format {
	case "json":
		zapCfg = zap.NewProductionConfig()
	case "console":
		zapCfg = zap.NewDevelopmentConfig()
		zapCfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	default:
		return fmt.Errorf("invalid LOG_FORMAT: %s", cfg.Format)
	}

	atomLevel := zap.NewAtomicLevel()
	if err := atomLevel.UnmarshalText([]byte(level)); err != nil {
		return fmt.Errorf("invalid LOG_LEVEL: %s", cfg.Level)
	}
	zapCfg.Level = atomLevel

	logger, err := zapCfg.Build(
		zap.AddCaller(),
		zap.AddCallerSkip(1),
	)
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}

	baseLogger = logger
	sugar = logger.Sugar()
	return nil
}

func Sync() {
	if baseLogger != nil {
		_ = baseLogger.Sync()
	}
}

func SetTraceID(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	traceID.Store(id)
}

func NewTraceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "trace-unknown"
	}
	return hex.EncodeToString(buf)
}

func StartTurn() uint64 {
	return atomic.AddUint64(&turnID, 1)
}

func Debugf(format string, args ...interface{}) {
	withFields().Debugf(format, args...)
}

func Infof(format string, args ...interface{}) {
	withFields().Infof(format, args...)
}

func Warnf(format string, args ...interface{}) {
	withFields().Warnf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	withFields().Errorf(format, args...)
}

func Fatalf(format string, args ...interface{}) {
	withFields().Fatalf(format, args...)
}

func withFields() *zap.SugaredLogger {
	tid, _ := traceID.Load().(string)
	if tid == "" {
		tid = "trace-unknown"
	}
	currentTurn := atomic.LoadUint64(&turnID)
	return sugar.With(
		"trace_id", tid,
		"turn_id", currentTurn,
		"log_id", fmt.Sprintf("%s-%d", tid, currentTurn),
	)
}
