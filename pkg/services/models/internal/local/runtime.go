package local

import (
	"context"

	"errors"
	"fmt"

	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

type CacheLayout struct {
	ModelName string
	CachePath string
	Revision  string
	Files     []string
}

type Hooks = modelseffects.LocalRuntimeHooks

var ErrInvalidDependencies = errors.New("managed local runtime dependencies are invalid")

func missingDependencyError(name string) error {
	return fmt.Errorf("%w: %s is required", ErrInvalidDependencies, name)
}

type localModelResourceReservation struct {
	key      string
	count    int
	capacity int
}

type ResourceLimiter struct {
	mu           sync.Mutex
	entries      map[scopedResourceKey]*ResourceLimiterEntry
	closedScopes map[models.RuntimeScopeRef]bool
	closed       bool
	hooks        Hooks
	now          func() time.Time
}

// Scopes used to own separate limiters. A shared limiter retains that capacity
// boundary even when two scopes select the same model and backend.
type scopedResourceKey struct {
	scope    models.RuntimeScopeRef
	resource string
}

type ResourceLimiterEntry struct {
	mu       sync.Mutex
	cond     *sync.Cond
	capacity int
	inUse    int
	closed   bool
}

func NewResourceLimiter(hooks Hooks, now func() time.Time) (*ResourceLimiter, error) {
	if now == nil {
		return nil, missingDependencyError("resource limiter clock")
	}
	return &ResourceLimiter{
		entries:      make(map[scopedResourceKey]*ResourceLimiterEntry),
		closedScopes: make(map[models.RuntimeScopeRef]bool),
		hooks:        hooks,
		now:          now,
	}, nil
}

func newLocalModelResourceLimiterEntry(capacity int) *ResourceLimiterEntry {
	entry := &ResourceLimiterEntry{capacity: capacity}
	entry.cond = sync.NewCond(&entry.mu)
	return entry
}

// Acquire reserves local model capacity for a worker and returns an idempotent
// release function. A nil release means the worker has no local reservations.
func (l *ResourceLimiter) Acquire(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	factoryCfg *models.RuntimeConfig,
	workerDef *models.RuntimeWorker,
) (func(), error) {
	if l == nil || factoryCfg == nil || workerDef == nil {
		return nil, nil
	}
	reservations := localModelResourceReservations(factoryCfg, workerDef)
	if len(reservations) == 0 {
		return nil, nil
	}
	acquired, err := l.acquire(ctx, scope, reservations)
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() { releaseResourceReservations(acquired, reservations) })
	}, nil
}

func localModelResourceReservations(factoryCfg *models.RuntimeConfig, workerDef *models.RuntimeWorker) []localModelResourceReservation {
	if factoryCfg == nil || workerDef == nil || workerDef.ModelLocality != models.RuntimeModelLocalityLocal {
		return nil
	}

	resourcesByName := make(map[string]models.RuntimeResource, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourcesByName[resource.Name] = resource
	}

	combined := make(map[string]localModelResourceReservation)
	order := make([]string, 0, len(workerDef.Resources))
	for _, requirement := range workerDef.Resources {
		resource, ok := resourcesByName[requirement.Name]
		if !ok || !isProcessScopedLocalModelResource(resource) || requirement.Capacity <= 0 {
			continue
		}
		key := localModelResourceKey(resource)
		if key == "" {
			continue
		}
		if existing, ok := combined[key]; ok {
			existing.count += requirement.Capacity
			combined[key] = existing
			continue
		}
		combined[key] = localModelResourceReservation{
			key:      key,
			count:    requirement.Capacity,
			capacity: resource.Capacity,
		}
		order = append(order, key)
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

func isProcessScopedLocalModelResource(resource models.RuntimeResource) bool {
	return resource.Type == models.RuntimeResourceTypeModel &&
		strings.TrimSpace(resource.Model) != "" &&
		strings.TrimSpace(resource.Backend) != "" &&
		strings.TrimSpace(resource.LoadPolicy) != ""
}

func localModelResourceKey(resource models.RuntimeResource) string {
	model := strings.ToUpper(strings.TrimSpace(resource.Model))
	backend := strings.ToUpper(strings.TrimSpace(resource.Backend))
	loadPolicy := strings.ToUpper(strings.TrimSpace(resource.LoadPolicy))
	if model == "" || backend == "" || loadPolicy == "" {
		return ""
	}
	return strings.Join([]string{model, backend, loadPolicy}, "|")
}

// CloseScope retires reservations and wakes waiters without affecting peer scopes.
func (l *ResourceLimiter) CloseScope(scope models.RuntimeScopeRef) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closedScopes[scope] = true
	for key, entry := range l.entries {
		if key.scope == scope {
			entry.close()
			delete(l.entries, key)
		}
	}
}

// Close retires all process-owned reservation state.
func (l *ResourceLimiter) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	for key, entry := range l.entries {
		entry.close()
		delete(l.entries, key)
	}
	clear(l.closedScopes)
}

func (l *ResourceLimiter) acquire(ctx context.Context, scope models.RuntimeScopeRef,
	reservations []localModelResourceReservation,
) ([]*ResourceLimiterEntry, error) {
	waitStartedAt := l.now()
	if l.hooks.MarkResourceWaitStarted != nil {
		l.hooks.MarkResourceWaitStarted(ctx, waitStartedAt)
	}
	acquired := make([]*ResourceLimiterEntry, 0, len(reservations))
	for _, reservation := range reservations {
		entry, err := l.entry(scopedResourceKey{scope: scope, resource: reservation.key}, reservation.capacity)
		if err == nil {
			err = entry.acquire(ctx, reservation.count)
		}
		if err != nil {
			if l.hooks.MarkResourceWaitFinished != nil {
				l.hooks.MarkResourceWaitFinished(ctx, l.now(), false)
			}
			releaseResourceReservations(acquired, reservations)
			return nil, err
		}
		acquired = append(acquired, entry)
	}
	if l.hooks.MarkResourceWaitFinished != nil {
		l.hooks.MarkResourceWaitFinished(ctx, l.now(), true)
	}
	return acquired, nil
}

func releaseResourceReservations(entries []*ResourceLimiterEntry, reservations []localModelResourceReservation) {
	for i := len(entries) - 1; i >= 0; i-- {
		entries[i].release(reservations[i].count)
	}
}

func (l *ResourceLimiter) entry(key scopedResourceKey, capacity int) (*ResourceLimiterEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.closedScopes[key.scope] {
		return nil, models.ErrRuntimeScopeClosed
	}
	entry, ok := l.entries[key]
	if !ok {
		entry = newLocalModelResourceLimiterEntry(capacity)
		l.entries[key] = entry
		return entry, nil
	}
	entry.mu.Lock()
	if capacity > 0 && (entry.capacity == 0 || capacity < entry.capacity) {
		entry.capacity = capacity
		entry.cond.Broadcast()
	}
	entry.mu.Unlock()
	return entry, nil
}

func (e *ResourceLimiterEntry) close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	e.cond.Broadcast()
}

func (e *ResourceLimiterEntry) acquire(ctx context.Context, count int) error {
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

	for !e.closed && e.inUse+count > e.capacity {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("local model resource wait canceled: %w", err)
		}
		e.cond.Wait()
	}
	if e.closed {
		return models.ErrRuntimeScopeClosed
	}
	e.inUse += count
	return nil
}

func (e *ResourceLimiterEntry) release(count int) {
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
