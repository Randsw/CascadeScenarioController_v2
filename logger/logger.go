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
//var SugarLog *zap.SugaredLogger

func InitLogger() {
	var err error
	var cfg zap.Config = zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	cfg.EncoderConfig.FunctionKey = "func"
	Zaplogger.logger, err = cfg.Build()
	//SugarLog = Zaplogger.Sugar()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer Zaplogger.logger.Sync()
}

func (z *Zaplog) Printf(message string, fields ...interface{}) {
	// var zapFields []zap.Field
	// for i, _ := range fields {
	// 	zapFields[i], _ = fields[i].(zap.Field)
	// }
	z.logger.Info(message)
}

func KafkaError(message string, fields ...interface{}) {
	// var zapFields []zap.Field
	// for i  := range fields {
	// 	zapFields[i], _ = fields[i].(zap.Field)
	// }
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
