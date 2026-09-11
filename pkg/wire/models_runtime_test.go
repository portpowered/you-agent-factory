package wire

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelservice "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestModelsInvokeStandaloneScopeProjectsOperatorModelOverlay(t *testing.T) {
	t.Parallel()

	scope, err := (modelservice.RuntimeScopeRef{}).Parse("wire:models:standalone-overlay")
	if err != nil {
		t.Fatalf("parse standalone overlay scope: %v", err)
	}
	home := t.TempDir()
	fixtureSource := "hf://fixture/models/llm.gguf@0000000000000000000000000000000000000000"
	var openRequest modelservice.OpenRuntimeScopeRequest
	root := modelsCLICompositionRootStub{
		openRuntime: func(_ context.Context, request modelservice.OpenRuntimeScopeRequest) (modelservice.OpenRuntimeScopeResult, error) {
			openRequest = request
			return modelservice.OpenRuntimeScopeResult{Scope: scope}, nil
		},
		closeRuntime: func(_ context.Context, request modelservice.CloseRuntimeScopeRequest) (modelservice.CloseRuntimeScopeResult, error) {
			return modelservice.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
		},
	}
	loader := func(path string) (operatorsettings.Config, error) {
		if path != operatorsettings.DefaultConfigPath(home) {
			t.Fatalf("operator config path = %q, want %q", path, operatorsettings.DefaultConfigPath(home))
		}
		return operatorsettings.Config{Models: map[string]operatorsettings.ModelConfig{
			modelservice.BuiltInModelNameLLM: {Source: &fixtureSource},
		}}, nil
	}
	composition, err := provideModelsCLIComposition(
		root,
		&modelsCLICompositionScopeSourceStub{err: factorydefinitions.ErrFactoryLayoutNotFound},
		loader,
	)
	if err != nil {
		t.Fatalf("provideModelsCLIComposition() error = %v", err)
	}
	opener, ok := composition.(modelscli.CompositionInvokeScopeWithModelCacheOpener)
	if !ok {
		t.Fatal("Models CLI composition does not expose cache-aware invoke scope opener")
	}
	opened, err := opener.CompositionOpenInvokeScopeWithModelCache(context.Background(), modelscli.InvokeScopeRequest{
		Config: modelscli.InvokeConfig{HomeDir: home},
	})
	if err != nil {
		t.Fatalf("CompositionOpenInvokeScopeWithModelCache() error = %v", err)
	}
	if overlay := openRequest.Config.OperatorModels[modelservice.BuiltInModelNameLLM]; overlay.Source == nil || *overlay.Source != fixtureSource {
		t.Fatalf("standalone Models operator overlay = %#v, want fixture source %q", overlay, fixtureSource)
	}
	if err := opened.Close(context.Background()); err != nil {
		t.Fatalf("close standalone overlay scope: %v", err)
	}
}
