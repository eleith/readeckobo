package logger

import (
	"fmt"
	"log"
	"strings"
)

type Level int

const (
	ERROR Level = iota
	WARN
	INFO
	DEBUG
)

func ParseLevel(lvl string) (Level, error) {
	switch strings.ToLower(lvl) {
	case "error":
		return ERROR, nil
	case "warn":
		return WARN, nil
	case "info":
		return INFO, nil
	case "debug":
		return DEBUG, nil
	}
	return INFO, fmt.Errorf("invalid log level: %s", lvl)
}

// Logger is a simple leveled logger.
type Logger struct {
	level Level
}

// New creates a new Logger.
func New(level Level) *Logger {
	return &Logger{level: level}
}

// Errorf prints a formatted error message.
func (l *Logger) Errorf(format string, v ...interface{}) {
	if l.level >= ERROR {
		log.Printf(format, v...)
	}
}

// Warnf prints a formatted warning message.
func (l *Logger) Warnf(format string, v ...interface{}) {
	if l.level >= WARN {
		log.Printf(format, v...)
	}
}

// Infof prints a formatted info message.
func (l *Logger) Infof(format string, v ...interface{}) {
	if l.level >= INFO {
		log.Printf(format, v...)
	}
}

// Debugf prints a formatted debug message.
func (l *Logger) Debugf(format string, v ...interface{}) {
	if l.level >= DEBUG {
		log.Printf(format, v...)
	}
}