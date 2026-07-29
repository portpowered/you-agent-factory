package structured_test

import (
	"context"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/structured"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestExecutorSupportsNoProvidersAfterPiCutover(t *testing.T) {
	t.Parallel()

	executor := structured.NewExecutor()
	for _, provider := range []string{
		string(modelprovider.ProviderPi),
		string(modelprovider.ProviderClaude),
		string(modelprovider.ProviderCodex),
		string(modelprovider.ProviderCursor),
		string(modelprovider.ProviderOpenCode),
		string(modelprovider.ProviderGemini),
		string(modelprovider.ProviderKiro),
		"unknown-provider",
	} {
		if executor.Supports(provider) {
			t.Fatalf("Supports(%q) = true, want false after Pi cutover", provider)
		}
	}
	var nilExecutor *structured.Executor
	if nilExecutor.Supports(string(modelprovider.ProviderPi)) {
		t.Fatal("nil executor unexpectedly supports Pi")
	}
}

func TestExecutorRejectsConductorRoutedProvidersAtScriptWrapBoundary(t *testing.T) {
	t.Parallel()

	provider := workerprovider.NewScriptWrapProviderWithDependencies(
		false, nil, &recordingRunner{}, nil, nil, structured.NewExecutor(), nil,
	)
	for _, modelProvider := range []string{
		string(modelprovider.ProviderClaude),
		string(modelprovider.ProviderCodex),
		string(modelprovider.ProviderCursor),
		string(modelprovider.ProviderOpenCode),
		string(modelprovider.ProviderPi),
	} {
		_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
			Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cutover"},
			ModelProvider: modelProvider,
			UserMessage:   "prompt",
		})
		if err == nil {
			t.Fatalf("Infer(%s) error = nil, want conductor routing rejection", modelProvider)
		}
	}
}

type recordingRunner struct {
	request workerprovider.CommandRequest
	result  workerprovider.CommandResult
	calls   int
	err     error
}

func (r *recordingRunner) Run(_ context.Context, req workerprovider.CommandRequest) (workerprovider.CommandResult, error) {
	r.calls++
	r.request = req
	return r.result, r.err
}
