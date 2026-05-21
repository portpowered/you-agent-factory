package scheduler

import (
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type capturingLogger struct {
	entries []logEntry
}

type logEntry struct {
	level string
	msg   string
	args  []any
}

func (l *capturingLogger) Debug(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "debug", msg: msg, args: keysAndValues})
}
func (l *capturingLogger) Info(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "info", msg: msg, args: keysAndValues})
}
func (l *capturingLogger) Warn(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "warn", msg: msg, args: keysAndValues})
}
func (l *capturingLogger) Error(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "error", msg: msg, args: keysAndValues})
}
func (l *capturingLogger) Verbose(msg string, keysAndValues ...any) {
	l.entries = append(l.entries, logEntry{level: "verbose", msg: msg, args: keysAndValues})
}

func (l *capturingLogger) entryMatches(e *logEntry, substr string) bool {
	if strings.Contains(e.msg, substr) {
		return true
	}
	for _, arg := range e.args {
		if s, ok := arg.(string); ok && strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func (l *capturingLogger) findEntry(substr string) *logEntry {
	for i := range l.entries {
		if l.entryMatches(&l.entries[i], substr) {
			return &l.entries[i]
		}
	}
	return nil
}

func (l *capturingLogger) countEntries(substr string) int {
	count := 0
	for i := range l.entries {
		if l.entryMatches(&l.entries[i], substr) {
			count++
		}
	}
	return count
}

func makeTestSnapshot(tokens map[string]*interfaces.Token) petri.MarkingSnapshot {
	placeTokens := make(map[string][]string)
	for id, tok := range tokens {
		if tok.CreatedAt.IsZero() {
			tok.CreatedAt = time.Now()
		}
		if tok.EnteredAt.IsZero() {
			tok.EnteredAt = time.Now()
		}
		placeTokens[tok.PlaceID] = append(placeTokens[tok.PlaceID], id)
	}
	return petri.MarkingSnapshot{Tokens: tokens, PlaceTokens: placeTokens}
}

func schedulerCronTimeToken(id string, workstation string, dueAt time.Time, expiresAt time.Time) *interfaces.Token {
	return &interfaces.Token{
		ID:      id,
		PlaceID: interfaces.SystemTimePendingPlaceID,
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   interfaces.DataTypeWork,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: workstation,
				interfaces.TimeWorkTagKeyDueAt:           dueAt.Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.Format(time.RFC3339Nano),
			},
		},
	}
}

func transitionIDs(enabled []interfaces.EnabledTransition) []string {
	ids := make([]string, len(enabled))
	for i := range enabled {
		ids[i] = enabled[i].TransitionID
	}
	return ids
}

func tokenIDs(tokens []interfaces.Token) []string {
	ids := make([]string, len(tokens))
	for i := range tokens {
		ids[i] = tokens[i].ID
	}
	return ids
}
