package sessioncursor

import (
	"fmt"
	"strings"

	"go.uber.org/zap/zaptest/observer"
)

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
