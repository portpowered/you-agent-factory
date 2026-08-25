package wire

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// statelessProviderContract supplies the native Providers operations that are
// not relevant to these Workers execution assertions. Keeping the test seam
// execute-shaped means the core Workers tests do not need the legacy
// inference-shaped adapter used by later caller-family migrations.
type statelessProviderContract struct{}

func TestNewServiceWithContentMaterializerRejectsACPURLWithoutSafetyCapability(t *testing.T) {
	t.Parallel()

	input := newStatelessConstructionInputs()
	provider := input.agentDependencies.Providers.(*statelessTestProviders)
	command := input.scriptDependencies.CommandRunner.(*statelessTestCommandRunner)
	local := input.inferenceDependencies.Models.(*statelessTestLocalInvoker)
	materializeCalls := atomic.Int32{}
	service, err := NewServiceWithContentMaterializer(
		input.agentDependencies,
		input.scriptConfig,
		input.scriptDependencies,
		input.inferenceConfig,
		input.inferenceDependencies,
		nil,
		nil,
		func() time.Time { return time.Unix(1, 0) },
		nil,
		nil,
		nil,
		nil,
		work.ContentMaterializeFunc(func(context.Context, string) (string, work.ContentCleanup, error) {
			materializeCalls.Add(1)
			return "", nil, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewServiceWithContentMaterializer() error = %v", err)
	}

	request := statelessHappyPathCases()[2].request
	request.Target.ExecutorProvider = workers.ExecutorProviderACP
	request.Input.Work = []workers.WorkInput{{
		Name: "private-image",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeImage,
			URL:  "http://127.0.0.1/secret.png",
		}},
	}}
	result, err := service.Execute(context.Background(), request)
	if err == nil || !errors.Is(err, workers.ErrInvalidExecuteRequest) ||
		!strings.Contains(err.Error(), "cannot validate ACP remote URL safety") {
		t.Fatalf("Execute() error = %v, want stable missing-safety-capability error", err)
	}
	if result.Correlation != (workers.ExecutionCorrelation{}) ||
		result.Outcome != "" || len(result.Output.Primary) != 0 || result.Failure != nil {
		t.Fatalf("Execute() result = %#v, want no started result", result)
	}
	if materializeCalls.Load() != 0 || command.calls.Load() != 0 || local.calls.Load() != 0 ||
		provider.executeCalls.Load() != 0 {
		t.Fatalf(
			"unsafe ACP URL effects = materialize %d command %d model %d provider %d, want zero",
			materializeCalls.Load(),
			command.calls.Load(),
			local.calls.Load(),
			provider.executeCalls.Load(),
		)
	}
}

func (statelessProviderContract) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{Providers: []providers.Descriptor{{ID: providers.IDCodex}}}, nil
}

func (statelessProviderContract) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	if request.ID != providers.IDCodex {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: providers.IDCodex}}, nil
}

func (statelessProviderContract) ResolveIdentity(
	_ context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	identity := strings.ToLower(strings.TrimSpace(request.Identity))
	if identity == "openai" {
		identity = string(providers.IDCodex)
	}
	if identity != string(providers.IDCodex) {
		return providers.ResolveIdentityResult{}, providers.ErrUnknownProvider
	}
	return providers.ResolveIdentityResult{ID: providers.IDCodex}, nil
}

func (statelessProviderContract) ResolveSelection(
	ctx context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	identity := request.Workstation
	if identity == "" {
		identity = request.Factory
	}
	if identity == "" {
		identity = request.ModelProvider
	}
	resolved, err := (statelessProviderContract{}).ResolveIdentity(
		ctx,
		providers.ResolveIdentityRequest{Identity: identity},
	)
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return providers.ResolveSelectionResult{Provider: resolved.ID}, nil
}

func (statelessProviderContract) ControlAttempt(
	_ context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return providers.ControlAttemptResult{
		Provider:  request.Provider,
		AttemptID: request.AttemptID,
		Action:    request.Action,
		Outcome:   providers.ControlOutcomeUnsupported,
	}, nil
}

func (statelessProviderContract) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

func (statelessProviderContract) ContinueReference(
	_ context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	return providers.ContinueReferenceResult{
		Reference: reference.ContinuationRef(),
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}
