package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

type recordedControlLogEntry struct {
	message string
	fields  map[string]any
}

// recordingControlLogger implements logging.Logger for asserting the exact
// safe fields ControlAttempt emits without depending on a concrete logging
// backend.
type recordingControlLogger struct {
	mu      sync.Mutex
	entries []recordedControlLogEntry
}

func (l *recordingControlLogger) Debug(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}
func (l *recordingControlLogger) Info(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}
func (l *recordingControlLogger) Warn(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}
func (l *recordingControlLogger) Error(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}
func (l *recordingControlLogger) Verbose(message string, keysAndValues ...any) {
	l.record(message, keysAndValues)
}

func (l *recordingControlLogger) record(message string, keysAndValues []any) {
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
	l.entries = append(l.entries, recordedControlLogEntry{message: message, fields: fields})
}

func (l *recordingControlLogger) entriesFor(message string) []recordedControlLogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var matches []recordedControlLogEntry
	for _, entry := range l.entries {
		if entry.message == message {
			matches = append(matches, entry)
		}
	}
	return matches
}

func assertNoUnsafeControlLogFields(t *testing.T, fields map[string]any) {
	t.Helper()
	for key := range fields {
		switch key {
		case "provider", "attemptID", "action", "outcome":
			continue
		default:
			t.Fatalf("unexpected log field %q leaked into control operation log: %#v", key, fields)
		}
	}
}

func mustControlRootService(t *testing.T, logger *recordingControlLogger) *providerservice.Service {
	t.Helper()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionCalls := 0
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				executionCalls++
				return providers.ExecuteResult{}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logger)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return root
}

func TestControlAttempt_DeterministicUnsupportedForEveryAction(t *testing.T) {
	t.Parallel()

	logger := &recordingControlLogger{}
	root := mustControlRootService(t, logger)

	for _, action := range []providers.ControlAction{
		providers.ControlActionPause,
		providers.ControlActionCancel,
		providers.ControlActionTerminate,
	} {
		result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-1",
			Action:    action,
		})
		if err != nil {
			t.Fatalf("ControlAttempt(%q) error = %v, want nil", action, err)
		}
		if result.Provider != providers.IDCodex ||
			result.AttemptID != "attempt-1" ||
			result.Action != action ||
			result.Outcome != providers.ControlOutcomeUnsupported {
			t.Fatalf("ControlAttempt(%q) = %#v, want unsupported echo", action, result)
		}
	}
}

func TestControlAttempt_ValidationFailsBeforeOutcome(t *testing.T) {
	t.Parallel()

	logger := &recordingControlLogger{}
	root := mustControlRootService(t, logger)

	_, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "",
		AttemptID: "attempt-1",
		Action:    providers.ControlActionPause,
	})
	if !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("ControlAttempt(empty provider) error = %v, want ErrInvalidID", err)
	}

	_, err = root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "   ",
		Action:    providers.ControlActionPause,
	})
	if !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("ControlAttempt(blank attempt id) error = %v, want ErrInvalidControlRequest", err)
	}

	_, err = root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-1",
		Action:    providers.ControlAction("resume"),
	})
	if !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("ControlAttempt(unknown action) error = %v, want ErrInvalidControlRequest", err)
	}

	rejected := logger.entriesFor("provider control attempt rejected")
	if len(rejected) != 3 {
		t.Fatalf("rejected log entries = %d, want 3", len(rejected))
	}
	for _, entry := range rejected {
		if entry.fields["outcome"] != "invalid" {
			t.Fatalf("rejected log fields = %#v, want outcome=invalid", entry.fields)
		}
		assertNoUnsafeControlLogFields(t, entry.fields)
	}

	if accepted := logger.entriesFor("provider control attempt accepted"); len(accepted) != 0 {
		t.Fatalf("accepted log entries = %d, want 0 for invalid requests", len(accepted))
	}
}

func TestControlAttempt_InvokesNoExecutionAdapter(t *testing.T) {
	t.Parallel()

	catalogService, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionCalls := 0
	executionService, err := executionwire.NewService(
		catalogService,
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				executionCalls++
				return providers.ExecuteResult{}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("executionwire.NewService() = %v", err)
	}
	root, err := providerservice.New(catalogService, executionService, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	if _, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-1",
		Action:    providers.ControlActionCancel,
	}); err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if executionCalls != 0 {
		t.Fatalf("execution adapter calls = %d, want 0", executionCalls)
	}
}

func TestControlAttempt_LogsSafeAcceptedIntentAndTerminalOutcome(t *testing.T) {
	t.Parallel()

	logger := &recordingControlLogger{}
	root := mustControlRootService(t, logger)

	if _, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-9",
		Action:    providers.ControlActionTerminate,
	}); err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}

	accepted := logger.entriesFor("provider control attempt accepted")
	if len(accepted) != 1 ||
		accepted[0].fields["provider"] != string(providers.IDCodex) ||
		accepted[0].fields["attemptID"] != "attempt-9" ||
		accepted[0].fields["action"] != string(providers.ControlActionTerminate) {
		t.Fatalf("accepted log = %#v", accepted)
	}
	assertNoUnsafeControlLogFields(t, accepted[0].fields)

	outcome := logger.entriesFor("provider control attempt outcome")
	if len(outcome) != 1 ||
		outcome[0].fields["provider"] != string(providers.IDCodex) ||
		outcome[0].fields["attemptID"] != "attempt-9" ||
		outcome[0].fields["action"] != string(providers.ControlActionTerminate) ||
		outcome[0].fields["outcome"] != string(providers.ControlOutcomeUnsupported) {
		t.Fatalf("outcome log = %#v", outcome)
	}
	assertNoUnsafeControlLogFields(t, outcome[0].fields)
}

func TestControlAttempt_ProductionWiredRootIsDeterministicallyUnsupported(t *testing.T) {
	t.Parallel()

	logger := &recordingControlLogger{}
	root, err := providerswire.NewService(providerswire.WithLogger(logger))
	if err != nil {
		t.Fatalf("providerswire.NewService() = %v", err)
	}

	result, err := providers.ControlAttempt(context.Background(), root, providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "wired-attempt",
		Action:    providers.ControlActionPause,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v, want nil", err)
	}
	if result.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt() outcome = %q, want unsupported", result.Outcome)
	}
	if len(logger.entriesFor("provider control attempt outcome")) != 1 {
		t.Fatalf("outcome log entries = %d, want 1 for production-wired root", len(logger.entriesFor("provider control attempt outcome")))
	}
}
