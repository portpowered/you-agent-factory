package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// nextIssuerID gives each process-local registry a distinct token namespace.
// The counter is hashed before it enters a reference, so callers cannot select
// a registry or scope by its allocation position.
var nextIssuerID atomic.Uint64

type service struct {
	mu           sync.RWMutex
	issuerToken  string
	nextScopeID  uint64
	liveBindings map[runtimescopes.Reference]models.RuntimeBinding
}

var _ runtimescopes.Service = (*service)(nil)

// New constructs an inert, process-local Runtime Scopes registry.
func New() runtimescopes.Service {
	issuerID := nextIssuerID.Add(1)
	return &service{
		issuerToken:  opaqueToken("issuer", issuerID),
		liveBindings: make(map[runtimescopes.Reference]models.RuntimeBinding),
	}
}

func (s *service) Open(binding models.RuntimeBinding) (runtimescopes.Reference, error) {
	if s == nil {
		return "", fmt.Errorf("%w: service is required", runtimescopes.ErrScopeUnknown)
	}
	if err := models.ValidateRuntimeBinding(binding); err != nil {
		return "", err
	}

	snapshot := cloneRuntimeConfig(binding.RuntimeConfig())

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextScopeID++
	ref := runtimescopes.Reference(s.issuerToken + "." + opaqueToken("scope", s.nextScopeID))
	s.liveBindings[ref] = detachedBinding(binding.CacheDirectory, snapshot)
	return ref, nil
}

func (s *service) Resolve(ref runtimescopes.Reference) (models.RuntimeBinding, error) {
	if s == nil {
		return models.RuntimeBinding{}, fmt.Errorf("%w: service is required", runtimescopes.ErrScopeUnknown)
	}

	s.mu.RLock()
	binding, ok := s.liveBindings[ref]
	s.mu.RUnlock()
	if !ok {
		return models.RuntimeBinding{}, fmt.Errorf("%w: reference does not identify a live scope", runtimescopes.ErrScopeUnknown)
	}
	return detachedBinding(binding.CacheDirectory, binding.RuntimeConfig()), nil
}

func detachedBinding(cacheDirectory string, config *models.RuntimeConfig) models.RuntimeBinding {
	snapshot := cloneRuntimeConfig(config)
	return models.RuntimeBinding{
		CacheDirectory: cacheDirectory,
		RuntimeConfig: func() *models.RuntimeConfig {
			return cloneRuntimeConfig(snapshot)
		},
	}
}

func cloneRuntimeConfig(config *models.RuntimeConfig) *models.RuntimeConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	cloned.Resources = append([]models.RuntimeResource(nil), config.Resources...)
	cloned.Workers = make([]models.RuntimeWorker, len(config.Workers))
	for i := range config.Workers {
		cloned.Workers[i] = config.Workers[i].Clone()
	}
	return &cloned
}

func opaqueToken(namespace string, id uint64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("models-runtime-scope:%s:%d", namespace, id)))
	return hex.EncodeToString(sum[:16])
}
