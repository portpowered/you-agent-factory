package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
)

func newActivationGatewayTestGateway(t *testing.T) factorysessions.DefinitionActivationGateway {
	t.Helper()

	clock := platformclock.Real{}
	registry := sessionregistry.New()
	responses := responsestream.NewRegistry(func() *responsestream.SessionResponseStream {
		return responsestream.NewSessionResponseStream(clock)
	}, clock)
	state := sessionruntime.New(
		registry,
		responses,
		nil,
		clock,
		func() string { return "response-event-test-id" },
		func() string { return "session-test-id" },
	)
	session := livesession.New(
		factorysessions.DefaultSessionID,
		"/factory",
		"/factory",
		"/factory/runtime",
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		nil,
		true,
		"factory",
		clock,
		func() string { return "session-test-id" },
		func() string { return "response-event-test-id" },
	)
	registry.Upsert(session, true)

	gateway := factorysessionservice.NewDefinitionActivationGatewayForTest(state)
	if gateway == nil {
		t.Fatal("NewDefinitionActivationGatewayForTest() returned nil")
	}
	return gateway
}

func TestDefinitionActivationGatewaySerializesActivationLock(t *testing.T) {
	t.Parallel()

	gateway := newActivationGatewayTestGateway(t)

	const workers = 8
	var active int32
	var peak int32
	var orderMu sync.Mutex
	order := make([]int, 0, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = gateway.WithActivationLock(func() error {
				current := atomic.AddInt32(&active, 1)
				for {
					peakValue := atomic.LoadInt32(&peak)
					if current <= peakValue || atomic.CompareAndSwapInt32(&peak, peakValue, current) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				orderMu.Lock()
				order = append(order, worker)
				orderMu.Unlock()
				atomic.AddInt32(&active, -1)
				return nil
			})
		}()
	}
	close(start)
	wg.Wait()

	if peak != 1 {
		t.Fatalf("peak concurrent activation holders = %d, want 1 serialized critical section", peak)
	}
	if len(order) != workers {
		t.Fatalf("recorded activation order length = %d, want %d", len(order), workers)
	}
}

func TestDefinitionActivationGatewayRejectsIdleViolation(t *testing.T) {
	t.Parallel()

	gateway := newActivationGatewayTestGateway(t)

	err := gateway.RequireIdleRuntimeForSession(context.Background(), factorysessions.DefaultSessionID)
	if err == nil {
		t.Fatal("RequireIdleRuntimeForSession() error = nil, want idle rejection without live runtime snapshot")
	}
}

func TestApplyNamedReplacement_RestoresPointerWhenRuntimeReplacementFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		seedAlpha   bool
		wantPointer string
	}{
		{name: "existing pointer", seedAlpha: true, wantPointer: "alpha"},
		{name: "absent pointer", seedAlpha: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rootDir := t.TempDir()
			for _, name := range []string{"alpha", "beta"} {
				if err := os.MkdirAll(filepath.Join(rootDir, name), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", name, err)
				}
				if err := os.WriteFile(
					filepath.Join(rootDir, name, factorydefinitions.FactoryConfigFile),
					[]byte(`{"name":"`+name+`"}`),
					0o644,
				); err != nil {
					t.Fatalf("write %s factory: %v", name, err)
				}
			}
			paths := testCurrentPointerStore{}
			if test.seedAlpha {
				if err := paths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
					t.Fatalf("write alpha pointer: %v", err)
				}
			}

			replacementErr := errors.New("controlled runtime replacement failure")
			err := factorysessionservice.ApplyNamedReplacement(
				context.Background(),
				"session",
				&livesession.LiveSession{ID: "session"},
				true,
				rootDir,
				"beta",
				emptyRuntimeRecord{},
				func(context.Context, string) error { return nil },
				func(context.Context) error { return nil },
				func(context.Context, *livesession.LiveSession, string, runtimeports.RuntimeInstance) error {
					return replacementErr
				},
				func(string, string, runtimeports.RuntimeInstance) error { return nil },
				paths.ReadCurrentPointer,
				paths.WriteCurrentPointer,
				paths.RemoveCurrentPointer,
			)
			if !errors.Is(err, replacementErr) {
				t.Fatalf("ApplyNamedReplacement error = %v, want replacement failure", err)
			}
			pointer, pointerErr := paths.ReadCurrentPointer(rootDir)
			if test.seedAlpha {
				if pointerErr != nil || pointer != test.wantPointer {
					t.Fatalf("pointer after failed replacement = %q, error=%v, want %q", pointer, pointerErr, test.wantPointer)
				}
				return
			}
			if !errors.Is(pointerErr, os.ErrNotExist) {
				t.Fatalf("pointer after failed replacement = %q, error=%v, want absent", pointer, pointerErr)
			}
		})
	}
}

type emptyRuntimeRecord struct {
	factory.RuntimeRecord
}

type testCurrentPointerStore struct{}

func (testCurrentPointerStore) ReadCurrentPointer(rootDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(rootDir, ".current-factory"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (testCurrentPointerStore) WriteCurrentPointer(rootDir, name string) error {
	return os.WriteFile(filepath.Join(rootDir, ".current-factory"), []byte(name), 0o644)
}

func (testCurrentPointerStore) RemoveCurrentPointer(rootDir string) error {
	return os.Remove(filepath.Join(rootDir, ".current-factory"))
}
