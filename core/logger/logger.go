package logger

import "fmt"

type Level string

const (
	Info  Level = "info"
	Warn  Level = "warn"
	Error Level = "error"
)

type Msg struct {
	Level Level
	Msg   string
}

type Logger struct {
	Logs []Msg
}

func NewLogger() *Logger {
	return &Logger{
		Logs: []Msg{},
	}
}

func (l *Logger) LogInfo(format string, args ...any) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.Logs = append(l.Logs, Msg{
		Level: Info,
		Msg:   msg,
	})
}

func (l *Logger) LogWarn(format string, args ...any) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.Logs = append(l.Logs, Msg{
		Level: Warn,
		Msg:   msg,
	})
}

func (l *Logger) LogError(format string, args ...any) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.Logs = append(l.Logs, Msg{
		Level: Error,
		Msg:   msg,
	})
}
