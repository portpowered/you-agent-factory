package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type blockingRunner struct {
	mu          sync.Mutex
	current     int
	maxObserved int
	enterCh     chan struct{}
	releaseCh   chan struct{}
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{
		enterCh:   make(chan struct{}, 8),
		releaseCh: make(chan struct{}),
	}
}

func (r *blockingRunner) Execute(_ context.Context, _ interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	r.mu.Lock()
	r.current++
	if r.current > r.maxObserved {
		r.maxObserved = r.current
	}
	r.mu.Unlock()

	r.enterCh <- struct{}{}
	<-r.releaseCh

	r.mu.Lock()
	r.current--
	r.mu.Unlock()
	return interfaces.RunnerExecutionResult{Content: "ok"}, nil
}

func (r *blockingRunner) MaxObserved() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxObserved
}

func TestLocalModelResourceLimiter_BoundsSharedLocalModelConcurrencyAcrossSessions(t *testing.T) {
	limiter := newLocalModelResourceLimiter()
	factoryCfg := &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
	}
	workerDef := &interfaces.WorkerConfig{
		Name:          "tts-worker",
		ModelLocality: interfaces.ModelLocalityLocal,
		Resources:     []interfaces.ResourceConfig{{Name: "omnivoice-cache", Capacity: 1}},
	}

	inner := newBlockingRunner()
	first := limiter.wrapRunner(inner, factoryCfg, workerDef)
	second := limiter.wrapRunner(inner, factoryCfg, workerDef)

	ctx := context.Background()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_, _ = first.Execute(ctx, interfaces.RunnerExecutionRequest{})
	}()

	select {
	case <-inner.enterCh:
	case <-time.After(2 * time.Second):
		t.Fatal("first execution did not enter runner")
	}

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		_, _ = second.Execute(ctx, interfaces.RunnerExecutionRequest{})
	}()

	select {
	case <-inner.enterCh:
		t.Fatal("second execution entered before shared local model resource was released")
	case <-time.After(150 * time.Millisecond):
	}

	close(inner.releaseCh)

	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second execution did not complete after release")
	}
	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first execution did not complete after release")
	}

	if got := inner.MaxObserved(); got != 1 {
		t.Fatalf("max observed local-model concurrency = %d, want 1", got)
	}
}
