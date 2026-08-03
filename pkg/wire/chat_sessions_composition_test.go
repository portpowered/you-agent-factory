package wire

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap/zapcore"
)

// staticFactoryDefinitionsService is a minimal factorydefinitions.Service
// double covering only ListEffectiveFactories, the sole collaborator method
// the Chat Sessions Factory target-catalog operation depends on. Factory
// Definitions' full production Service is only constructible from a live
// Factory Session's SessionHost/DefinitionActivationGateway/Validator
// (pkg/wire/factory_definition_service_provider.go's
// provideFactoryDefinitionsFactory), so this composition test proves the
// Chat Sessions side of the wiring (real canonical Operator Settings root,
// real canonical logger, direct single injection, no dependency bag) with a
// focused double standing in for the Factory Definitions root, matching the
// same fake-collaborator convention used by
// pkg/services/chat_sessions/internal/service's own unit tests.
type staticFactoryDefinitionsService struct {
	factorydefinitions.Service

	mu      sync.Mutex
	calls   int
	entries []factorydefinitions.EffectiveFactoryCatalogEntry
}

func (s *staticFactoryDefinitionsService) ListEffectiveFactories(
	context.Context,
	factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return factorydefinitions.ListEffectiveFactoriesResult{Entries: s.entries}, nil
}

func (s *staticFactoryDefinitionsService) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *staticFactoryDefinitionsService) setEntries(entries []factorydefinitions.EffectiveFactoryCatalogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = entries
}

// TestProvideChatSessionsFactoryTargetCatalogServiceComposesThroughTheCanonicalWireGraph
// proves the exact provider chain pkg/wire registers for the Chat Sessions
// Factory target-catalog root (provideChatSessionsFactoryTargetCatalogService
// consuming the same provideOperatorSettingsService chain and canonical
// process logger as every other canonical consumer) performs direct single
// injection with no dependency bag, threads a real logger into the
// operation's started/finished logs, observes live Factory Definitions
// drift on the very next call, and never invokes anything beyond the one
// read-only collaborator method it depends on.
func TestProvideChatSessionsFactoryTargetCatalogServiceComposesThroughTheCanonicalWireGraph(t *testing.T) {
	t.Parallel()

	zapLogger, observed := testdeps.CapturingZapLogger(zapcore.InfoLevel)
	logger := provideOperatorSettingsLogger(zapLogger)

	edges := serviceedges.Edges{}
	files := provideOperatorSettingsFileSystem(edges)
	providersRoot, err := provideProvidersService(edges, logger)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	providerRegistry, err := provideProviderRegistry(edges, providersRoot)
	if err != nil {
		t.Fatalf("provideProviderRegistry() error = %v", err)
	}
	operatorSettings, err := provideOperatorSettingsService(
		files,
		provideOperatorSettingsCreateTemporaryFile(edges),
		provideOperatorSettingsProviderCatalog(providerRegistry),
		provideOperatorConfigDecoder(),
		provideOperatorConfigEncoder(),
		provideOperatorSettingsIDGenerator(edges),
		providersRoot,
		logger,
	)
	if err != nil {
		t.Fatalf("provideOperatorSettingsService() error = %v", err)
	}

	factoryDefinitions := &staticFactoryDefinitionsService{
		entries: []factorydefinitions.EffectiveFactoryCatalogEntry{
			{Name: "@you/factory-builder", Definition: &factorydefinitions.FactoryConfig{Name: "Factory Builder"}},
		},
	}

	catalog, err := provideChatSessionsFactoryTargetCatalogService(operatorSettings, factoryDefinitions, logger)
	if err != nil {
		t.Fatalf("provideChatSessionsFactoryTargetCatalogService() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	req := chatsessions.ResolveFactoryTargetCatalogRequest{OperatorSettingsPath: path}

	if _, err := catalog.ResolveFactoryTargetCatalog(context.Background(), req); err != nil {
		t.Fatalf("ResolveFactoryTargetCatalog() error = %v", err)
	}
	if factoryDefinitions.callCount() != 1 {
		t.Fatalf("ListEffectiveFactories call count = %d, want exactly 1 for one resolution", factoryDefinitions.callCount())
	}

	var messages []string
	for _, entry := range observed.All() {
		messages = append(messages, entry.Message)
	}
	if !slices.Contains(messages, "chat_sessions.resolve_factory_target_catalog.started") {
		t.Fatalf("observed log messages = %v, want a start log proving the canonical wire logger reached the service", messages)
	}
	if !slices.Contains(messages, "chat_sessions.resolve_factory_target_catalog.finished") {
		t.Fatalf("observed log messages = %v, want a finished log proving the canonical wire logger reached the service", messages)
	}

	// Drift: Factory Builder becomes uninstalled between calls. The same
	// long-lived catalog root must observe it on the very next call instead
	// of replaying a cached prior resolution.
	factoryDefinitions.setEntries(nil)
	if _, err := catalog.ResolveFactoryTargetCatalog(context.Background(), req); err == nil {
		t.Fatal("ResolveFactoryTargetCatalog() after uninstalling the default target = nil error, want live drift to be observed")
	}
	if factoryDefinitions.callCount() != 2 {
		t.Fatalf("ListEffectiveFactories call count = %d, want exactly 2 after a second resolution", factoryDefinitions.callCount())
	}
}
