package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type localModelResourceReservation struct {
	key      string
	count    int
	capacity int
}

type localModelResourceLimiter struct {
	mu      sync.Mutex
	entries map[string]*localModelResourceLimiterEntry
}

type localModelResourceLimiterEntry struct {
	mu       sync.Mutex
	cond     *sync.Cond
	capacity int
	inUse    int
}

type localModelLimitedRunner struct {
	inner        workers.Runner
	limiter      *localModelResourceLimiter
	reservations []localModelResourceReservation
}

func newLocalModelResourceLimiter() *localModelResourceLimiter {
	return &localModelResourceLimiter{
		entries: make(map[string]*localModelResourceLimiterEntry),
	}
}

func newLocalModelResourceLimiterEntry(capacity int) *localModelResourceLimiterEntry {
	entry := &localModelResourceLimiterEntry{capacity: capacity}
	entry.cond = sync.NewCond(&entry.mu)
	return entry
}

func (l *localModelResourceLimiter) wrapRunner(
	inner workers.Runner,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
) workers.Runner {
	if inner == nil || l == nil || factoryCfg == nil || workerDef == nil {
		return inner
	}
	reservations := localModelResourceReservations(factoryCfg, workerDef)
	if len(reservations) == 0 {
		return inner
	}
	return &localModelLimitedRunner{
		inner:        inner,
		limiter:      l,
		reservations: reservations,
	}
}

func localModelResourceReservations(factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.WorkerConfig) []localModelResourceReservation {
	combined := make(map[string]localModelResourceReservation)
	order := make([]string, 0, len(workerDef.Resources))
	for _, match := range eligibleLocalModelResourceMatches(factoryCfg, workerDef) {
		if match.requirement.Capacity <= 0 {
			continue
		}
		if existing, ok := combined[match.key]; ok {
			existing.count += match.requirement.Capacity
			combined[match.key] = existing
			continue
		}
		combined[match.key] = localModelResourceReservation{
			key:      match.key,
			count:    match.requirement.Capacity,
			capacity: match.resource.Capacity,
		}
		order = append(order, match.key)
	}

	if len(order) == 0 {
		return nil
	}
	out := make([]localModelResourceReservation, 0, len(order))
	for _, key := range order {
		out = append(out, combined[key])
	}
	return out
}

func isProcessScopedLocalModelResource(resource interfaces.ResourceConfig) bool {
	return resource.Type == interfaces.ResourceTypeModel &&
		strings.TrimSpace(resource.Model) != "" &&
		strings.TrimSpace(resource.Backend) != "" &&
		strings.TrimSpace(resource.LoadPolicy) != ""
}

func localModelResourceKey(resource interfaces.ResourceConfig) string {
	model := strings.ToUpper(strings.TrimSpace(resource.Model))
	backend := strings.ToUpper(strings.TrimSpace(resource.Backend))
	loadPolicy := strings.ToUpper(strings.TrimSpace(resource.LoadPolicy))
	if model == "" || backend == "" || loadPolicy == "" {
		return ""
	}
	return strings.Join([]string{model, backend, loadPolicy}, "|")
}

func (l *localModelResourceLimiter) acquire(ctx context.Context, reservations []localModelResourceReservation) error {
	if l == nil || len(reservations) == 0 {
		return nil
	}

	waitStartedAt := time.Now()
	markModelExecutionResourceWaitStarted(ctx, waitStartedAt)
	acquired := make([]localModelResourceReservation, 0, len(reservations))
	for _, reservation := range reservations {
		entry := l.entry(reservation.key, reservation.capacity)
		if err := entry.acquire(ctx, reservation.count); err != nil {
			markModelExecutionResourceWaitFinished(ctx, time.Now(), false)
			l.release(acquired)
			return err
		}
		acquired = append(acquired, reservation)
	}
	markModelExecutionResourceWaitFinished(ctx, time.Now(), true)
	return nil
}

func (l *localModelResourceLimiter) release(reservations []localModelResourceReservation) {
	if l == nil || len(reservations) == 0 {
		return
	}
	for i := len(reservations) - 1; i >= 0; i-- {
		reservation := reservations[i]
		entry := l.entry(reservation.key, reservation.capacity)
		entry.release(reservation.count)
	}
}

func (l *localModelResourceLimiter) entry(key string, capacity int) *localModelResourceLimiterEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok {
		entry = newLocalModelResourceLimiterEntry(capacity)
		l.entries[key] = entry
		return entry
	}

	entry.mu.Lock()
	if capacity > 0 && (entry.capacity == 0 || capacity < entry.capacity) {
		entry.capacity = capacity
		entry.cond.Broadcast()
	}
	entry.mu.Unlock()
	return entry
}

func (e *localModelResourceLimiterEntry) acquire(ctx context.Context, count int) error {
	if e == nil || count <= 0 {
		return nil
	}

	stopBroadcast := context.AfterFunc(ctx, func() {
		e.mu.Lock()
		e.cond.Broadcast()
		e.mu.Unlock()
	})
	defer stopBroadcast()

	e.mu.Lock()
	defer e.mu.Unlock()

	for e.inUse+count > e.capacity {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("local model resource wait canceled: %w", err)
		}
		e.cond.Wait()
	}
	e.inUse += count
	return nil
}

func (e *localModelResourceLimiterEntry) release(count int) {
	if e == nil || count <= 0 {
		return
	}
	e.mu.Lock()
	e.inUse -= count
	if e.inUse < 0 {
		e.inUse = 0
	}
	e.mu.Unlock()
	e.cond.Broadcast()
}

func (r *localModelLimitedRunner) Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	if err := r.limiter.acquire(ctx, r.reservations); err != nil {
		return interfaces.RunnerExecutionResult{}, err
	}
	defer r.limiter.release(r.reservations)
	return r.inner.Execute(ctx, request)
}
