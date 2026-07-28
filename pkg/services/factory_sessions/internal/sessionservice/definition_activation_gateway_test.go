package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
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
