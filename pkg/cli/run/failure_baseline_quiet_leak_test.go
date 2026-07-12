package run

import (
	"bytes"
	"context"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

// Hermetic S02 failure-baseline fixtures for one-shot run paths with quiet or
// CI-oriented suppression enabled. These lock the customer-visible contract
// that dashboard and operator startup chatter must not leak on stdout.

var quietLeakForbiddenMarkers = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
	"Opening dashboard",
	"Factory:",
	"Recording saved",
}

func assertQuietLeakContractForbidden(t *testing.T, output string) {
	t.Helper()

	for _, forbidden := range quietLeakForbiddenMarkers {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no quiet-leak marker %q", output, forbidden)
		}
	}
}

func TestFailureBaseline_QuietLeak_OneShotBatchRunSuppressesDashboardMarkers(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                        dir,
		Port:                       0,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		SuppressDashboardRendering: true,
		DisableDefaultRecording:    true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if output != "" {
		t.Fatalf("stdout = %q, want empty dashboard output with quiet suppression", output)
	}
	assertQuietLeakContractForbidden(t, output)
}

func TestFailureBaseline_QuietLeak_OneShotCleanInvocationSuppressesOperatorChatter(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                        dir,
		Port:                       0,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		CleanInvocation:            true,
		SuppressDashboardRendering: true,
		DisableDefaultRecording:    true,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if output != "mock worker accepted" {
		t.Fatalf("stdout = %q, want primary clean invocation output", output)
	}
	assertQuietLeakContractForbidden(t, output)
}

func TestFailureBaseline_QuietLeak_OneShotNamedGoalInvocationSuppressesOperatorChatter(t *testing.T) {
	preserveRunGlobals(t)

	text := "quiet-leak baseline goal prompt"
	var output bytes.Buffer
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "req-quiet-leak",
					TraceID:   "trace-quiet-leak",
					Status:    factoryapi.InvocationTerminalStatusCompleted,
					PrimaryResult: []interfaces.WorkContentPart{{
						Type: interfaces.WorkContentPartTypeText,
						Text: "goal quiet baseline completed",
					}},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                        "/tmp/builtin-goal",
		NamedFactoryName:           goal.PackagedFactoryName,
		InvocationPositionalText:   &text,
		StdinIsTTY:                 func() bool { return true },
		SuppressDashboardRendering: true,
		Output:                     &output,
		Port:                       7437,
		Logger:                     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "goal quiet baseline completed" {
		t.Fatalf("stdout = %q, want primary invocation result only", got)
	}
	assertQuietLeakContractForbidden(t, output.String())
}
