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

func (l *stdLogger) format(level, msg string, keysAndValues ...interface{}) string {
	formatted := fmt.Sprintf("[%s] %s", level, msg)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			formatted += fmt.Sprintf(" %v=%v", keysAndValues[i], keysAndValues[i+1])
		}
	}
	return formatted
}

func (l *stdLogger) Debug(msg string, keysAndValues ...interface{}) {
	l.logger.Output(2, l.format("DEBUG", msg, keysAndValues...))
}

func (l *stdLogger) Info(msg string, keysAndValues ...interface{}) {
	l.logger.Output(2, l.format("INFO", msg, keysAndValues...))
}

func (l *stdLogger) Warn(msg string, keysAndValues ...interface{}) {
	l.logger.Output(2, l.format("WARN", msg, keysAndValues...))
}

func (l *stdLogger) Error(msg string, keysAndValues ...interface{}) {
	l.logger.Output(2, l.format("ERROR", msg, keysAndValues...))
}

func (l *stdLogger) Fatal(msg string, keysAndValues ...interface{}) {
	l.logger.Output(2, l.format("FATAL", msg, keysAndValues...))
	os.Exit(1)
}
