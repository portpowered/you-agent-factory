package service_test

import (
	"context"
	"sync"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
)

type recordedLogEntry struct {
	message string
	fields  map[string]any
}

// recordingLogger implements logging.Logger for asserting the exact safe
// fields the registry emits without depending on a concrete logging backend.
type recordingLogger struct {
	mu      sync.Mutex
	entries []recordedLogEntry
}

func (l *recordingLogger) Debug(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Info(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Warn(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Error(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) Verbose(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingLogger) record(message string, keysAndValues []any) {
	fields := make(map[string]any, len(keysAndValues)/2)
	for index := 0; index+1 < len(keysAndValues); index += 2 {
		key, ok := keysAndValues[index].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[index+1]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, recordedLogEntry{message: message, fields: fields})
}

func (l *recordingLogger) entriesFor(message string) []recordedLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var matches []recordedLogEntry
	for _, entry := range l.entries {
		if entry.message == message {
			matches = append(matches, entry)
		}
	}
	return matches
}

func assertNoPayloadOrCredentialKeys(t *testing.T, fields map[string]any) {
	t.Helper()
	for key := range fields {
		switch key {
		case "sessionID", "outcome", "state", "filter_state_count", "result_count":
			continue
		default:
			t.Fatalf("unexpected log field %q leaked into operation log: %#v", key, fields)
		}
	}
}

func TestRegistryLogsReserveOutcomes(t *testing.T) {
	logger := &recordingLogger{}
	registry := service.New(logger)
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "   "}); err == nil {
		t.Fatalf("Reserve() with invalid ID unexpectedly succeeded")
	}
	rejected := logger.entriesFor("worker session reserve rejected")
	if len(rejected) != 1 || rejected[0].fields["outcome"] != "invalid" {
		t.Fatalf("reserve-rejected log = %#v", rejected)
	}
	assertNoPayloadOrCredentialKeys(t, rejected[0].fields)

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	accepted := logger.entriesFor("worker session reserve")
	if len(accepted) != 1 || accepted[0].fields["sessionID"] != "worker-1" || accepted[0].fields["outcome"] != "reserved" {
		t.Fatalf("reserve-accepted log = %#v", accepted)
	}
	assertNoPayloadOrCredentialKeys(t, accepted[0].fields)

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err == nil {
		t.Fatalf("duplicate Reserve() unexpectedly succeeded")
	}
	accepted = logger.entriesFor("worker session reserve")
	if len(accepted) != 2 || accepted[1].fields["sessionID"] != "worker-1" || accepted[1].fields["outcome"] != "duplicate" {
		t.Fatalf("reserve-duplicate log = %#v", accepted)
	}
	assertNoPayloadOrCredentialKeys(t, accepted[1].fields)
}

func TestRegistryLogsListOutcomes(t *testing.T) {
	logger := &recordingLogger{}
	registry := service.New(logger)
	ctx := context.Background()

	if _, err := registry.List(ctx, workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{"INTERRUPTED"}}}); err == nil {
		t.Fatalf("List() with invalid filter unexpectedly succeeded")
	}
	rejected := logger.entriesFor("worker session list rejected")
	if len(rejected) != 1 || rejected[0].fields["outcome"] != "invalid" {
		t.Fatalf("list-rejected log = %#v", rejected)
	}
	assertNoPayloadOrCredentialKeys(t, rejected[0].fields)

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if _, err := registry.List(ctx, workersessions.ListRequest{}); err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	succeeded := logger.entriesFor("worker session list")
	if len(succeeded) != 1 || succeeded[0].fields["outcome"] != "success" || succeeded[0].fields["result_count"] != 1 {
		t.Fatalf("list-success log = %#v", succeeded)
	}
	assertNoPayloadOrCredentialKeys(t, succeeded[0].fields)
}
