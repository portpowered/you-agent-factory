package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

type service struct {
	mu           sync.RWMutex
	issuerToken  string
	nextScopeID  uint64
	liveBindings map[runtimescopes.Reference]models.RuntimeBinding
	closedScopes map[runtimescopes.Reference]struct{}
}

var _ runtimescopes.Service = (*service)(nil)

// New constructs an inert, process-local Runtime Scopes registry.
func New(issuerID string) runtimescopes.Service {
	return &service{
		issuerToken:  opaqueToken("issuer", issuerID),
		liveBindings: make(map[runtimescopes.Reference]models.RuntimeBinding),
		closedScopes: make(map[runtimescopes.Reference]struct{}),
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
	scopeIdentity := fmt.Sprintf("%s:%d", s.issuerToken, s.nextScopeID)
	ref := runtimescopes.Reference(s.issuerToken + "." + opaqueToken("scope", scopeIdentity))
	s.liveBindings[ref] = detachedBinding(binding.CacheDirectory, snapshot)
	return ref, nil
}

func (s *service) Resolve(ref runtimescopes.Reference) (models.RuntimeBinding, error) {
	if s == nil {
		return models.RuntimeBinding{}, fmt.Errorf("%w: service is required", runtimescopes.ErrScopeUnknown)
	}
	if s.isForeign(ref) {
		return models.RuntimeBinding{}, fmt.Errorf("%w: reference belongs to another service", runtimescopes.ErrScopeForeign)
	}

	s.mu.RLock()
	binding, ok := s.liveBindings[ref]
	_, closed := s.closedScopes[ref]
	s.mu.RUnlock()
	if closed {
		return models.RuntimeBinding{}, fmt.Errorf("%w: reference identifies a closed scope", runtimescopes.ErrScopeClosed)
	}
	if !ok {
		return models.RuntimeBinding{}, fmt.Errorf("%w: reference does not identify a live scope", runtimescopes.ErrScopeUnknown)
	}
	return detachedBinding(binding.CacheDirectory, binding.RuntimeConfig()), nil
}

func (s *service) Close(ref runtimescopes.Reference) error {
	if s == nil {
		return fmt.Errorf("%w: service is required", runtimescopes.ErrScopeUnknown)
	}
	if s.isForeign(ref) {
		return fmt.Errorf("%w: reference belongs to another service", runtimescopes.ErrScopeForeign)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, closed := s.closedScopes[ref]; closed {
		return fmt.Errorf("%w: reference identifies a closed scope", runtimescopes.ErrScopeClosed)
	}
	if _, ok := s.liveBindings[ref]; !ok {
		return fmt.Errorf("%w: reference does not identify a live scope", runtimescopes.ErrScopeUnknown)
	}
	delete(s.liveBindings, ref)
	s.closedScopes[ref] = struct{}{}
	return nil
}

func (s *service) isForeign(ref runtimescopes.Reference) bool {
	issuerToken, scopeToken, ok := strings.Cut(string(ref), ".")
	if !ok || !validToken(issuerToken) || !validToken(scopeToken) {
		return false
	}
	return issuerToken != s.issuerToken
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

func opaqueToken(namespace string, identity string) string {
	sum := sha256.Sum256([]byte("models-runtime-scope:" + namespace + ":" + identity))
	return hex.EncodeToString(sum[:16])
}

func validToken(value string) bool {
	if len(value) != 32 || value == strings.Repeat("0", 32) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
