package sessionpersistence

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type zapLogger struct {
	logger *zap.Logger
}

func (l zapLogger) Info(msg string, fields map[string]string) {
	if l.logger == nil {
		return
	}
	l.logger.Info(msg, mapToZapFields(fields)...)
}

func mapToZapFields(fields map[string]string) []zap.Field {
	zapFields := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		zapFields = append(zapFields, zap.String(key, value))
	}
	return zapFields
}

// NewZapObserver builds an observer backed by a zap logger and optional core.
func NewZapObserver(core zapcore.Core) (Observer, *zap.Logger) {
	logger := zap.New(core)
	return Observer{Logger: zapLogger{logger: logger}}, logger
}

// FieldValueFromObservedLogs returns the string value for a field name from the
// last logged invalidation entry.
func FieldValueFromObservedLogs(observed *observer.ObservedLogs, key string) string {
	logged := observed.FilterMessage("session persistence invalidation").All()
	if len(logged) == 0 {
		return ""
	}
	context := logged[len(logged)-1].ContextMap()
	value, ok := context[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return strings.TrimSpace(text)
}
