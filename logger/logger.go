package logger

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Zaplog struct {
	logger *zap.Logger
}

var Zaplogger Zaplog

func InitLogger() {
	var err error
	var cfg zap.Config = zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	cfg.EncoderConfig.FunctionKey = "func"
	Zaplogger.logger, err = cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
}

// Gracefully close logger
func CloseLogger() {
	err := Zaplogger.logger.Sync()
	if err != nil {
		log.Printf("can't sync zap logger: %v", err)
	}
}

func (z *Zaplog) Printf(message string, fields ...interface{}) {
	z.logger.Info(message)
}

func KafkaError(message string, fields ...interface{}) {
	Zaplogger.logger.Error(message)
}

func KafkaInfo(message string, fields ...interface{}) {
	Zaplogger.logger.Info(message)
}

func Info(message string, fields ...zap.Field) {
	Zaplogger.logger.Info(message, fields...)
}

func Warn(message string, fields ...zap.Field) {
	Zaplogger.logger.Warn(message, fields...)
}

func Debug(message string, fields ...zap.Field) {
	Zaplogger.logger.Debug(message, fields...)
}

func Error(message string, fields ...zap.Field) {
	Zaplogger.logger.Error(message, fields...)
}

func Fatal(message string, fields ...zap.Field) {
	Zaplogger.logger.Fatal(message, fields...)
}
