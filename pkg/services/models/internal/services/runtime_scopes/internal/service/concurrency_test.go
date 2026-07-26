package service_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func TestConcurrentOpenAndResolvePreserveScopeIsolation(t *testing.T) {
	t.Parallel()

	const (
		scopeCount        = 64
		resolveIterations = 20
	)

	service := newService(t, "concurrent-open")
	refs := make([]runtimescopes.Reference, scopeCount)
	errCh := make(chan error, scopeCount*resolveIterations)

	var opens sync.WaitGroup
	for scope := range scopeCount {
		opens.Add(1)
		go func() {
			defer opens.Done()
			want := fmt.Sprintf("factory-%d", scope)
			ref, err := service.Open(models.RuntimeBinding{
				CacheDirectory: fmt.Sprintf("cache-%d", scope),
				RuntimeConfig: func() *models.RuntimeConfig {
					return &models.RuntimeConfig{
						FactoryDirectory: want,
						Workers: []models.RuntimeWorker{{
							Name: want,
							Args: []string{want},
						}},
					}
				},
			})
			if err != nil {
				errCh <- fmt.Errorf("open scope %d: %w", scope, err)
				return
			}
			refs[scope] = ref
		}()
	}
	opens.Wait()

	seen := make(map[runtimescopes.Reference]int, scopeCount)
	for scope, ref := range refs {
		if ref == "" {
			t.Fatalf("scope %d received an empty reference", scope)
		}
		if prior, ok := seen[ref]; ok {
			t.Fatalf("scopes %d and %d received the same reference %q", prior, scope, ref)
		}
		seen[ref] = scope
	}

	var resolves sync.WaitGroup
	for scope, ref := range refs {
		resolves.Add(1)
		go func() {
			defer resolves.Done()
			want := fmt.Sprintf("factory-%d", scope)
			for range resolveIterations {
				binding, err := service.Resolve(ref)
				if err != nil {
					errCh <- fmt.Errorf("resolve scope %d: %w", scope, err)
					return
				}
				config := binding.RuntimeConfig()
				if config.FactoryDirectory != want ||
					len(config.Workers) != 1 ||
					config.Workers[0].Name != want ||
					len(config.Workers[0].Args) != 1 ||
					config.Workers[0].Args[0] != want {
					errCh <- fmt.Errorf("scope %d resolved another binding: %#v", scope, config)
					return
				}
			}
		}()
	}
	resolves.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestConcurrentResolveAndCloseReturnOnlyLiveOrStaleOutcomes(t *testing.T) {
	t.Parallel()

	const (
		resolvers         = 32
		resolveIterations = 50
	)

	service := newService(t, "concurrent-close")
	closingRef := openBinding(t, service, "closing")
	liveRef := openBinding(t, service, "live")
	start := make(chan struct{})
	errCh := make(chan error, (resolvers+1)*resolveIterations)

	var operations sync.WaitGroup
	for range resolvers {
		operations.Add(1)
		go func() {
			defer operations.Done()
			<-start
			for range resolveIterations {
				binding, err := service.Resolve(closingRef)
				switch {
				case err == nil:
					if got := binding.RuntimeConfig().FactoryDirectory; got != "closing" {
						errCh <- fmt.Errorf("closing scope resolved %q, want closing", got)
						return
					}
				case errors.Is(err, runtimescopes.ErrScopeUnknown):
				// Close linearized first, so the stale outcome is contract-valid.
				default:
					errCh <- fmt.Errorf("resolve closing scope: %w", err)
					return
				}
			}
		}()
	}

	operations.Add(1)
	go func() {
		defer operations.Done()
		<-start
		for range resolveIterations {
			binding, err := service.Resolve(liveRef)
			if err != nil {
				errCh <- fmt.Errorf("resolve unaffected scope: %w", err)
				return
			}
			if got := binding.RuntimeConfig().FactoryDirectory; got != "live" {
				errCh <- fmt.Errorf("unaffected scope resolved %q, want live", got)
				return
			}
		}
	}()

	close(start)
	if err := service.Close(closingRef); err != nil {
		t.Fatalf("Close closing scope: %v", err)
	}
	operations.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}

	if _, err := service.Resolve(closingRef); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
		t.Fatalf("Resolve closed scope error = %v, want ErrScopeUnknown", err)
	}
	assertFactoryDirectory(t, service, liveRef, "live")
}
