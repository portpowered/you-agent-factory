package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

type startupJournal struct {
	calls     int
	now       time.Time
	retention time.Duration
	err       error
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
