package wire

import (
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"go.uber.org/zap/zapcore"
	"path/filepath"
)

// TestProvideOperatorSettingsServiceLogsThroughTheCanonicalWireLogger proves the
// exact provider chain pkg/wire's generated injector uses to construct the
// Operator Settings service (provideOperatorSettingsLogger converting the
// canonical process logger, then settingswire.NewService consuming it)
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
	settings, err := newOperatorSettingsTestService(
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
		t.Fatalf("newOperatorSettingsTestService() error = %v", err)
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

func TestProcessLoggerSelectionAndModelHostLogger(t *testing.T) {
	t.Parallel()
	backend, observed := testdeps.CapturingZapLogger(zapcore.DebugLevel)
	selected, err := provideProcessLogger(serviceedges.Edges{ProcessLogger: backend})
	if err != nil || selected != backend {
		t.Fatalf("selected backend=%v error=%v", selected, err)
	}
	provideModelHostLogger(selected).Info("owned host diagnostic", map[string]string{"correlation_id": "owned-scope"})
	records := observed.All()
	if len(records) != 1 || records[0].LoggerName != "modelhost" || records[0].ContextMap()["correlation_id"] != "owned-scope" {
		t.Fatalf("host adapter records=%v", records)
	}
	muted, err := provideProcessLogger(serviceedges.Edges{})
	if err != nil || muted == nil {
		t.Fatalf("default selection error=%v", err)
	}
	if muted.Core().Enabled(zapcore.InfoLevel) || !muted.Core().Enabled(zapcore.WarnLevel) {
		t.Fatal("default process logger must retain terminal-muted warn policy")
	}
	muted.Warn("default backend remains terminal-muted")
	if observed.Len() != 1 {
		t.Fatal("default selection reused caller backend")
	}
}

// newOperatorSettingsTestService mirrors the focused providers in servicesSet.
func newOperatorSettingsTestService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	catalog operatorsettings.ProviderCatalog,
	decode operatorsettings.ConfigDecoder,
	diagnostics operatorsettings.ConfigDiagnosticsDecoder,
	encode operatorsettings.ConfigEncoder,
	generateID operatorsettings.IDGenerator,
	providersRoot providers.Service,
	logger logging.Logger,
) (operatorsettings.Service, error) {
	document := settingswire.NewDocumentService(files, createTemp, decode, encode, catalog,
		provideOperatorSettingsDocumentPreserver(), diagnostics)
	resolution, err := settingswire.NewResolutionService(providersRoot)
	if err != nil {
		return nil, err
	}
	return settingswire.NewService(document, resolution, files, createTemp, decode, encode,
		generateID, logger, diagnostics)
}
