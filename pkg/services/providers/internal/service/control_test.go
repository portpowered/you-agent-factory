package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
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

	catalogService, err := catalogwire.NewService(catalogwire.IdentityProbe, nil, nil)
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
	root, err := providerservice.NewWithACP(catalogService, executionService, &stubACPService{integrations: []providers.ACPIntegration{}}, nil, logger, &stubACPService{integrations: []providers.ACPIntegration{}})
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

	catalogService, err := catalogwire.NewService(catalogwire.IdentityProbe, nil, nil)
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
	root, err := providerservice.NewWithACP(catalogService, executionService, &stubACPService{integrations: []providers.ACPIntegration{}}, nil, logging.NoopLogger{}, &stubACPService{integrations: []providers.ACPIntegration{}})
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

func TestControlAttempt_InjectedLoggersStayIsolated(t *testing.T) {
	t.Parallel()
	// ControlAttempt owns its registry and does not consult either collaborator.
	// Supplying inert ports keeps this proof inside the root component.
	for _, attemptID := range []string{"first-root", "peer-root"} {
		t.Run(attemptID, func(t *testing.T) {
			t.Parallel()
			logger := &recordingControlLogger{}
			root, err := providerservice.NewWithACP(struct{ catalog.Service }{}, struct{ execution.Service }{},
				&stubACPService{}, nil, logger, &stubACPService{})
			if err != nil {
				t.Fatal(err)
			}
			quietPeer, err := providerservice.NewWithACP(struct{ catalog.Service }{}, struct{ execution.Service }{},
				&stubACPService{}, nil, logging.NoopLogger{}, &stubACPService{})
			if err != nil {
				t.Fatal(err)
			}
			request := providers.ControlAttemptRequest{
				Provider: providers.IDCodex, AttemptID: attemptID,
				Action: providers.ControlActionCancel,
			}
			if _, err := root.ControlAttempt(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			request.AttemptID = "quiet-peer"
			if _, err := quietPeer.ControlAttempt(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			for _, message := range []string{"provider control attempt accepted", "provider control attempt outcome"} {
				entries := logger.entriesFor(message)
				if len(entries) != 1 || entries[0].fields["attemptID"] != attemptID {
					t.Fatalf("selected sink contains peer output: %s = %#v", message, entries)
				}
				assertNoUnsafeControlLogFields(t, entries[0].fields)
			}
		})
	}
}

func TestControlAttempt_ProductionWiredRootIsDeterministicallyUnsupported(t *testing.T) {
	t.Parallel()

	logger := &recordingControlLogger{}
	root, err := newTestProvidersService(providerswire.IdentityCatalogProbe,
		platformclock.Real{}, logger, nil, nil, nil,
		nil,
		nil,
		nil,
		providerswire.Configuration{})
	if err != nil {
		t.Fatalf("newTestProvidersService() = %v", err)
	}

	result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
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

// newTestProvidersService assembles the fixture's explicit sibling owners.
func newTestProvidersService(probe providerswire.CatalogProbeOperation, scheduler platformclock.TimerSource, logger logging.Logger, commandFactory platformprocess.CommandFactory, locator platformprocess.ExecutableLocator, stdioPipes platformprocess.StdioPipeFactory, antigravity providerswire.AgyEffect, codex providerswire.CodexEffect, claude providerswire.ClaudeEffect, configuration providerswire.Configuration) (providers.Service, error) {
	config, err := providerswire.PrepareConfiguration(configuration)
	if err != nil {
		return nil, err
	}
	catalogService, err := providerswire.NewCatalogService(probe, config.CatalogDescriptors, config.CatalogOverrides)
	if err != nil {
		return nil, err
	}
	acpService, err := providerswire.NewACPService(config.ACPIntegrations, commandFactory, locator, stdioPipes, scheduler, logger)
	if err != nil {
		return nil, err
	}
	registrations, err := providerswire.ExecutionRegistrations(antigravity, codex, claude, acpService, config.ACPIntegrations, config.Registrations)
	if err != nil {
		return nil, err
	}
	executionService, err := providerswire.NewExecutionService(catalogService, registrations)
	if err != nil {
		return nil, err
	}
	return providerswire.NewService(catalogService, executionService, acpService, config.ACPIntegrations, logger, acpService)
}
