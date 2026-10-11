package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

type startupJournal struct {
	path      string
	bindErr   error
	calls     int
	now       time.Time
	retention time.Duration
	err       error
}

func (j *startupJournal) Bind(path string) error {
	if j.bindErr != nil {
		return j.bindErr
	}
	j.path = path
	return nil
}

func (j *startupJournal) Activate(now time.Time, retention time.Duration) error {
	j.calls++
	j.now, j.retention = now, retention
	return j.err
}

type startupLogger struct {
	logging.NoopLogger
	lines []string
}

func (l *startupLogger) Info(message string, fields ...any) {
	l.lines = append(l.lines, fmt.Sprint(message, fields))
}

func (l *startupLogger) Warn(message string, fields ...any) {
	l.lines = append(l.lines, fmt.Sprint(message, fields))
}

func TestStartupActivatesOnceWithConfiguredRetention(t *testing.T) {
	t.Parallel()
	j := &startupJournal{}
	logger := &startupLogger{}
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.FixedZone("local", -7*60*60))
	clockCalls := 0
	s := NewStartup(true, j, func() time.Time { clockCalls++; return now }, 7*24*time.Hour, logger)
	if j.calls != 0 || clockCalls != 0 || len(logger.lines) != 0 {
		t.Fatal("construction activated the store")
	}
	var callers sync.WaitGroup
	for range 8 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			if err := s.Start(context.Background()); err != nil {
				t.Errorf("start: %v", err)
			}
		}()
	}
	callers.Wait()
	if j.calls != 1 || clockCalls != 1 || !j.now.Equal(now) || j.now.Location() != time.UTC || j.retention != 7*24*time.Hour {
		t.Fatal("startup repeated activation or changed the configured time/retention")
	}
	if len(logger.lines) != 2 || !strings.Contains(logger.lines[1], "store activated") {
		t.Fatal("successful activation lacks terminal diagnostic")
	}
}

func TestStartupQuarantinesStoreFailuresWithoutFailingApplication(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{ErrCorrupt, ErrUnavailable, errors.New("planted-secret private-store-path")} {
		j := &startupJournal{err: failure}
		logger := &startupLogger{}
		s := NewStartup(true, j, func() time.Time { return time.Unix(1, 0) }, DefaultRetention, logger)
		for range 2 {
			if err := s.Start(context.Background()); err != nil {
				t.Fatalf("message store failure escaped application startup: %v", err)
			}
		}
		if j.calls != 1 || len(logger.lines) != 2 || !strings.Contains(logger.lines[1], "store quarantined") {
			t.Fatal("quarantined startup retried or omitted its terminal diagnostic")
		}
		if strings.Contains(strings.Join(logger.lines, " "), "planted-secret") || strings.Contains(strings.Join(logger.lines, " "), "private-store-path") {
			t.Fatal("startup published unsafe collaborator diagnostics")
		}
		code := ErrUnavailable.Error()
		if errors.Is(failure, ErrCorrupt) {
			code = ErrCorrupt.Error()
		}
		if !strings.Contains(logger.lines[1], code) {
			t.Fatal("startup did not classify the safe store outcome")
		}
	}
}

func TestStartupDisabledAndCanceledDoNotTouchStore(t *testing.T) {
	t.Parallel()
	j := &startupJournal{}
	clockCalls := 0
	clock := func() time.Time { clockCalls++; return time.Unix(1, 0) }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewStartup(true, j, clock, DefaultRetention, logging.NoopLogger{})
	if err := s.Start(ctx); !errors.Is(err, context.Canceled) || clockCalls != 0 || j.calls != 0 {
		t.Fatalf("canceled startup consumed activation: %v", err)
	}
	if err := s.Start(context.Background()); err != nil || clockCalls != 1 || j.calls != 1 {
		t.Fatalf("canceled startup prevented a later activation: %v", err)
	}
	j = &startupJournal{}
	clockCalls = 0
	s = NewStartup(false, j, clock, DefaultRetention, logging.NoopLogger{})
	if err := s.Start(context.Background()); err != nil || j.calls != 0 || clockCalls != 0 {
		t.Fatalf("disabled messaging touched store: %v", err)
	}
}

func TestStartupSelectsExplicitProfileOnce(t *testing.T) {
	t.Parallel()
	path, err := filepath.Abs(filepath.Join("profile-a", "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	j := &startupJournal{}
	s := NewStartup(true, j, func() time.Time { return time.Unix(1, 0) }, DefaultRetention, logging.NoopLogger{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.StartAt(ctx, path, true, DefaultRetention); !errors.Is(err, context.Canceled) || j.path != "" {
		t.Fatal("cancellation consumed profile selection", err)
	}
	for range 2 {
		if err := s.StartAt(context.Background(), path, true, DefaultRetention); err != nil {
			t.Fatal(err)
		}
	}
	if j.path != path || j.calls != 1 {
		t.Fatal("startup did not retain the selected profile")
	}
	other := filepath.Join(filepath.Dir(path), "other.jsonl")
	if err := s.StartAt(context.Background(), other, true, DefaultRetention); !errors.Is(err, agentmessages.ErrBadRequest) || j.path != path || j.calls != 1 {
		t.Fatal("startup accepted a different profile", err)
	}
}

func TestStartupRejectsInvalidOrConflictingSelectionBeforeActivation(t *testing.T) {
	t.Parallel()
	path, err := filepath.Abs("messages.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	j := &startupJournal{bindErr: agentmessages.ErrBadRequest}
	s := NewStartup(true, j, func() time.Time { return time.Unix(1, 0) }, DefaultRetention, logging.NoopLogger{})
	for _, invalid := range []string{"", "relative/messages.jsonl", path} {
		if err := s.StartAt(context.Background(), invalid, true, DefaultRetention); !errors.Is(err, agentmessages.ErrBadRequest) || j.calls != 0 || j.path != "" {
			t.Fatal("invalid selection activated a profile", err)
		}
	}
	j.bindErr = nil
	if err := s.StartAt(context.Background(), path, true, DefaultRetention); err != nil || j.calls != 1 {
		t.Fatal("selection rejection prevented a valid later start", err)
	}
}

func TestStartupDisabledProfileSkipsStoreAndClock(t *testing.T) {
	t.Parallel()
	path, err := filepath.Abs("messages.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	j := &startupJournal{bindErr: errors.New("must not bind")}
	s := NewStartup(false, j, func() time.Time { t.Error("disabled startup read clock"); return time.Time{} }, DefaultRetention, logging.NoopLogger{})
	if err := s.StartAt(context.Background(), path, false, DefaultRetention); err != nil || j.calls != 0 || j.path != "" {
		t.Fatal("disabled startup touched the journal", err)
	}
}

func TestStartupUsesSelectedSettingsInsteadOfConstructionDefaults(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := &startupJournal{}
	s := NewStartup(false, j, func() time.Time { return time.Unix(1, 0) }, DefaultRetention, logging.NoopLogger{})
	retention := 2 * 24 * time.Hour
	if err := s.StartAt(t.Context(), path, true, retention); err != nil {
		t.Fatal(err)
	}
	if j.path != path || j.calls != 1 || j.retention != retention {
		t.Fatal("selected enabled/retention settings were ignored")
	}
	if err := s.Start(t.Context()); err != nil || j.calls != 1 {
		t.Fatal("legacy startup reapplied construction defaults", err)
	}
	for _, settings := range []startupSettings{{false, retention}, {true, DefaultRetention}} {
		if err := s.StartAt(t.Context(), path, settings.enabled, settings.retention); !errors.Is(err, agentmessages.ErrBadRequest) {
			t.Fatal("active store accepted different settings", err)
		}
	}
	if j.calls != 1 || j.retention != retention {
		t.Fatal("rejected settings changed the store")
	}
}

func TestStartupSelectedDisabledAndQuarantinedSettingsRemainImmutable(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "messages.jsonl")
			j := &startupJournal{err: ErrCorrupt}
			s := NewStartup(true, j, func() time.Time { return time.Unix(1, 0) }, DefaultRetention, logging.NoopLogger{})
			retention := 24 * time.Hour
			for range 2 {
				if err := s.StartAt(t.Context(), path, enabled, retention); err != nil {
					t.Fatal("selected startup failed unrelated execution", err)
				}
			}
			wantCalls := 0
			if enabled {
				wantCalls = 1
			}
			if j.calls != wantCalls {
				t.Fatal("disabled store activated or quarantined store retried")
			}
			if err := s.StartAt(t.Context(), path, !enabled, retention); !errors.Is(err, agentmessages.ErrBadRequest) {
				t.Fatal("selected enablement changed", err)
			}
			if err := s.StartAt(t.Context(), path, enabled, DefaultRetention); !errors.Is(err, agentmessages.ErrBadRequest) {
				t.Fatal("selected retention changed", err)
			}
			if j.calls != wantCalls {
				t.Fatal("settings rejection retried activation")
			}
		})
	}
}

func TestStartupRejectedSettingsDoNotConsumeSelection(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := &startupJournal{bindErr: agentmessages.ErrBadRequest}
	s := NewStartup(false, j, func() time.Time { return time.Unix(1, 0) }, DefaultRetention, logging.NoopLogger{})
	for _, retention := range []time.Duration{0, -24 * time.Hour, 24*time.Hour - time.Nanosecond, 24 * time.Hour} {
		if err := s.StartAt(t.Context(), path, true, retention); !errors.Is(err, agentmessages.ErrBadRequest) {
			t.Fatal("invalid setting or binding was accepted", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.StartAt(ctx, path, true, 24*time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled settings selection was accepted", err)
	}
	if j.calls != 0 || j.path != "" {
		t.Fatal("rejected settings touched the journal")
	}
	j.bindErr = nil
	if err := s.StartAt(t.Context(), path, false, DefaultRetention); err != nil || j.calls != 0 || j.path != "" {
		t.Fatal("rejected enabled selection replaced later disabled settings", err)
	}
}

func TestStartupConcurrentSettingsSelectOneImmutableConfiguration(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	j := &startupJournal{}
	s := NewStartup(false, j, func() time.Time { return time.Unix(1, 0) }, DefaultRetention, logging.NoopLogger{})
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, retention := range []time.Duration{24 * time.Hour, DefaultRetention} {
		go func() {
			<-start
			results <- s.StartAt(t.Context(), path, true, retention)
		}()
	}
	close(start)
	success, rejected := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, agentmessages.ErrBadRequest):
			rejected++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 || j.calls != 1 {
		t.Fatal("competing startup settings did not select exactly one activation")
	}
	if err := s.StartAt(t.Context(), path, true, j.retention); err != nil || j.calls != 1 {
		t.Fatal("selected settings were not idempotent", err)
	}
}
