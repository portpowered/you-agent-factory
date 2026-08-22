package wire

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"go.uber.org/zap/zapcore"
)

// TestProvideOperatorSettingsServiceLogsThroughTheCanonicalWireLogger proves the
// exact provider chain pkg/wire's generated injector uses to construct the
// Operator Settings service (provideOperatorSettingsLogger converting the
// canonical process logger, then provideOperatorSettingsService consuming it)
// actually threads a real logger into ResolveACPAgentProfile/
// UpdateACPAgentProfile operation logs, not just a test-injected spy that the
// production wiring never reaches.
func TestProvideOperatorSettingsServiceLogsThroughTheCanonicalWireLogger(t *testing.T) {
	t.Parallel()

	zapLogger, observed := testdeps.CapturingZapLogger(zapcore.InfoLevel)
	logger := provideOperatorSettingsLogger(zapLogger)

	edges := serviceedges.Edges{}
	files := provideOperatorSettingsFileSystem(edges)
	providersRoot, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	settings, err := provideOperatorSettingsService(
		files,
		provideOperatorSettingsCreateTemporaryFile(edges),
		provideOperatorSettingsProviderCatalog(providersRoot),
		provideOperatorConfigDecoder(),
		provideOperatorConfigDiagnosticsDecoder(),
		provideOperatorConfigEncoder(),
		provideOperatorSettingsIDGenerator(edges),
		providersRoot,
		logger,
	)
	if err != nil {
		t.Fatalf("provideOperatorSettingsService() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if _, err := settings.ResolveACPAgentProfile(path); err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}

	var messages []string
	for _, entry := range observed.All() {
		messages = append(messages, entry.Message)
	}
	if !slices.Contains(messages, "operator_settings.resolve_acp_agent_profile.started") {
		t.Fatalf("observed log messages = %v, want a start log proving the canonical wire logger reached the service", messages)
	}
	if !slices.Contains(messages, "operator_settings.resolve_acp_agent_profile.finished") {
		t.Fatalf("observed log messages = %v, want a finished log proving the canonical wire logger reached the service", messages)
	}
}
