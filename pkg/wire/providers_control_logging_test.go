package wire

import (
	"context"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/testdeps"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"go.uber.org/zap/zapcore"
)

// TestProvideProvidersServiceLogsThroughTheCanonicalWireLogger proves the
// exact provider chain pkg/wire's generated injector uses to construct the
// application Providers root (provideOperatorSettingsLogger converting the
// canonical process logger, then provideProvidersService/
// provideConfiguredProvidersService threading it into providerswire.WithLogger)
// actually reaches ControlAttempt's accepted-intent and terminal-outcome logs,
// not just a test-injected spy that the production wiring never reaches.
func TestProvideProvidersServiceLogsThroughTheCanonicalWireLogger(t *testing.T) {
	t.Parallel()

	zapLogger, observed := testdeps.CapturingZapLogger(zapcore.InfoLevel)
	logger := provideOperatorSettingsLogger(zapLogger)

	providersRoot, err := provideProvidersService(serviceedges.Edges{}, logger)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}

	result, err := providersRoot.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "wire-graph-attempt",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt() error = %v", err)
	}
	if result.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt() outcome = %q, want unsupported", result.Outcome)
	}

	var messages []string
	for _, entry := range observed.All() {
		messages = append(messages, entry.Message)
	}
	if !slices.Contains(messages, "provider control attempt accepted") {
		t.Fatalf("observed log messages = %v, want an accepted log proving the canonical wire logger reached ControlAttempt", messages)
	}
	if !slices.Contains(messages, "provider control attempt outcome") {
		t.Fatalf("observed log messages = %v, want an outcome log proving the canonical wire logger reached ControlAttempt", messages)
	}
}
