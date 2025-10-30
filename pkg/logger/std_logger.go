package logger

import (
	"fmt"
	"log"
	"os"
)

// stdLogger простая реализация Logger через стандартный log
type stdLogger struct {
	logger *log.Logger
}

// NewLogger создает новый логгер
func NewLogger() Logger {
	return &stdLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
	}
}

func (l *stdLogger) format(level, msg string, fields map[string]interface{}) string {
	formatted := fmt.Sprintf("[%s] %s", level, msg)
	if fields != nil {
		for key, value := range fields {
			formatted += fmt.Sprintf(" %s=%v", key, value)
		}
	}
	return formatted
}

func (l *stdLogger) Debug(msg string, fields map[string]interface{}) {
	l.logger.Output(2, l.format("DEBUG", msg, fields))
}

func (l *stdLogger) Info(msg string, fields map[string]interface{}) {
	l.logger.Output(2, l.format("INFO", msg, fields))
}

func (l *stdLogger) Warn(msg string, fields map[string]interface{}) {
	l.logger.Output(2, l.format("WARN", msg, fields))
}

func (l *stdLogger) Error(msg string, fields map[string]interface{}) {
	l.logger.Output(2, l.format("ERROR", msg, fields))
}

func (l *stdLogger) Fatal(msg string, fields map[string]interface{}) {
	l.logger.Output(2, l.format("FATAL", msg, fields))
	os.Exit(1)
}
