package admission

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

type profileStartup struct {
	calls     int
	path      string
	enabled   bool
	retention time.Duration
	err       error
}

func (s *profileStartup) StartAt(_ context.Context, path string, enabled bool, retention time.Duration) error {
	s.calls++
	s.path, s.enabled, s.retention = path, enabled, retention
	return s.err
}

func configuredFixture(t *testing.T, settings *operatorsettings.MessagingSettings, startup *profileStartup) (*Configured, *engineFixture) {
	t.Helper()
	f := newEngineFixture()
	f.engine.validator = Validator{}
	f.engine.logger = logging.NoopLogger{}
	s, err := NewConfigured(f.engine, startup, func(string) (operatorsettings.Config, error) {
		return operatorsettings.Config{Messaging: settings}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

func TestSelectedPolicyControlsAdmissionAndExpiry(t *testing.T) {
	t.Parallel()
	bodyBytes, expiry, sends, retention := 16, 60, 1, 2
	startup := &profileStartup{}
	s, f := configuredFixture(t, &operatorsettings.MessagingSettings{
		RetentionDays: &retention,
		Limits:        &operatorsettings.MessagingLimits{MaxBodyBytes: &bodyBytes, DefaultExpirySeconds: &expiry, SendsPerChainPerHour: &sends},
	}, startup)
	path := filepath.Join(t.TempDir(), "config.json")
	if _, err := s.Send(t.Context(), engineRequest("first", "first"), f.caller, ""); !errors.Is(err, agentmessages.ErrDisabled) {
		t.Fatalf("unselected: %v", err)
	}
	if err := s.Activate(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if startup.path != filepath.Join(filepath.Dir(path), "messages", "messages.jsonl") || !startup.enabled || startup.retention != 48*time.Hour {
		t.Fatal("incorrect selected store policy")
	}
	if _, err := s.Send(t.Context(), engineRequest("large", "this body is too large"), f.caller, ""); !errors.Is(err, agentmessages.ErrLimitExceeded) {
		t.Fatalf("body limit: %v", err)
	}
	message, err := s.Send(t.Context(), engineRequest("first", "first"), f.caller, "")
	if err != nil || message.ExpiresAt.Sub(message.SentAt) != time.Minute {
		t.Fatalf("selected expiry: %+v %v", message, err)
	}
	if _, err := s.Send(t.Context(), engineRequest("second", "second"), f.caller, ""); !errors.Is(err, agentmessages.ErrLimitExceeded) {
		t.Fatalf("selected quota: %v", err)
	}
	if retry, err := s.Send(t.Context(), engineRequest("first", "first"), f.caller, ""); err != nil || retry.MessageID != message.MessageID {
		t.Fatalf("quota spent on replay: %+v %v", retry, err)
	}
}

func TestProfileSelectionIsImmutableAndConcurrent(t *testing.T) {
	t.Parallel()
	startup := &profileStartup{}
	s, _ := configuredFixture(t, nil, startup)
	path := filepath.Join(t.TempDir(), "config.json")
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := s.Activate(t.Context(), path); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if startup.calls != 1 {
		t.Fatal("same profile activated more than once")
	}
	if err := s.Activate(t.Context(), filepath.Join(t.TempDir(), "config.json")); !errors.Is(err, agentmessages.ErrBadRequest) {
		t.Fatalf("foreign rebind: %v", err)
	}
	if _, err := s.List(t.Context(), agentmessages.ListRequest{}); err != nil {
		t.Fatalf("rejected rebind quarantined original inbox: %v", err)
	}
}

func TestInvalidOrDisabledSettingsLeaveHostActivationHealthy(t *testing.T) {
	t.Parallel()
	disabled, invalid := false, 0
	for _, cell := range []struct {
		name         string
		settings     *operatorsettings.MessagingSettings
		want         error
		startupCalls int
	}{
		{"disabled", &operatorsettings.MessagingSettings{Enabled: &disabled}, agentmessages.ErrDisabled, 1},
		{"invalid", &operatorsettings.MessagingSettings{RetentionDays: &invalid}, agentmessages.ErrStoreUnavailable, 0},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			startup := &profileStartup{}
			s, f := configuredFixture(t, cell.settings, startup)
			path := filepath.Join(t.TempDir(), "config.json")
			if err := s.Activate(t.Context(), path); err != nil {
				t.Fatalf("optional Messaging failed host startup: %v", err)
			}
			if _, err := s.Send(t.Context(), engineRequest("first", "first"), f.caller, ""); !errors.Is(err, cell.want) {
				t.Fatalf("safe refusal: %v", err)
			}
			if len(f.ledger.transactions) != 0 || startup.calls != cell.startupCalls {
				t.Fatal("refused settings affected journal")
			}
		})
	}
}

func TestCanceledProfileSelectionCanRetry(t *testing.T) {
	t.Parallel()
	startup := &profileStartup{err: context.Canceled}
	s, _ := configuredFixture(t, nil, startup)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := s.Activate(t.Context(), path); !errors.Is(err, context.Canceled) {
		t.Fatalf("activation cancellation: %v", err)
	}
	startup.err = nil
	if err := s.Activate(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(t.Context(), agentmessages.ListRequest{}); err != nil {
		t.Fatalf("cancellation consumed selection: %v", err)
	}
}
