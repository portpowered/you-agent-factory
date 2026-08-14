package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestAuthorizeProviderTargetValidatesAndDetachesProviderSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		service              *Service
		identity             string
		request              workers.ExecuteRequest
		wantErr              error
		resolveResult        providers.ResolveIdentityResult
		resolveErr           error
		validateErr          error
		getErr               error
		providerCapabilities []providers.Capability
		wantProvider         string
		wantResumeProvider   string
		wantRunner           string
		wantAlias            string
	}{
		{
			name:       "non agent does not require providers",
			identity:   "script",
			request:    workers.ExecuteRequest{Target: workers.ExecutionTarget{RunnerID: "script"}},
			wantRunner: "script",
		},
		{
			name:     "nil service is unavailable",
			identity: runners.AgentIdentity,
			request:  agentProviderRequest(),
			service:  nil,
			wantErr:  workers.ErrExecuteUnavailable,
		},
		{
			name:     "nil providers service is unavailable",
			identity: runners.AgentIdentity,
			request:  agentProviderRequest(),
			service:  &Service{},
			wantErr:  workers.ErrExecuteUnavailable,
		},
		{
			name:     "empty identity is rejected",
			identity: runners.AgentIdentity,
			request:  workers.ExecuteRequest{},
			service:  &Service{providers: &providerAuthorizationFake{}},
			wantErr:  workers.ErrInvalidExecuteRequest,
		},
		{
			name:     "identity resolution failure is invalid request",
			identity: runners.AgentIdentity,
			request:  agentProviderRequestWithAlias("codex"),
			service: &Service{providers: &providerAuthorizationFake{
				resolveErr: errors.New("unknown provider"),
			}},
			wantErr: workers.ErrInvalidExecuteRequest,
		},
		{
			name:     "prerequisite failure is invalid request",
			identity: runners.AgentIdentity,
			request:  agentProviderRequestWithAlias("codex"),
			service: &Service{providers: &providerAuthorizationFake{
				resolveResult: providers.ResolveIdentityResult{ID: providers.IDCodex},
				validateErr:   errors.New("provider unavailable"),
			}},
			wantErr: workers.ErrInvalidExecuteRequest,
		},
		{
			name:     "antigravity skips catalog capability validation",
			identity: runners.AgentIdentity,
			request: func() workers.ExecuteRequest {
				request := agentProviderRequestWithAlias("agy")
				request.Target.Tools.RequiredOptionalCapabilities = []workers.RunnerOptionalCapability{
					workers.RunnerOptionalCapabilityImageInput,
				}
				request.Input.Resume = &workers.ProviderContinuationRef{
					Provider:          "before",
					ProviderSessionID: "session-1",
				}
				return request
			}(),
			service: &Service{providers: &providerAuthorizationFake{
				resolveResult: providers.ResolveIdentityResult{ID: providers.IDAntigravity},
				getErr:        errors.New("GetProvider must not be called for antigravity"),
			}},
			wantProvider:       string(providers.IDAntigravity),
			wantResumeProvider: string(providers.IDAntigravity),
			wantRunner:         workers.RunnerIDAntigravity,
			wantAlias:          "",
		},
		{
			name:     "success resolves capabilities and runner",
			identity: runners.AgentIdentity,
			request: func() workers.ExecuteRequest {
				request := agentProviderRequestWithAlias("openai-codex")
				request.Target.Tools.RequiredOptionalCapabilities = []workers.RunnerOptionalCapability{
					workers.RunnerOptionalCapabilityImageInput,
				}
				request.Input.Resume = &workers.ProviderContinuationRef{
					Provider:          "alias",
					ProviderSessionID: "session-2",
				}
				return request
			}(),
			service: &Service{providers: &providerAuthorizationFake{
				resolveResult:        providers.ResolveIdentityResult{ID: providers.IDCodex},
				providerCapabilities: []providers.Capability{providers.CapabilityImageInput},
			}},
			wantProvider:       string(providers.IDCodex),
			wantResumeProvider: string(providers.IDCodex),
			wantRunner:         workers.RunnerIDCodex,
			wantAlias:          "",
		},
		{
			name:     "resume provider supplies identity and authored runner remains",
			identity: runners.AgentIdentity,
			request: func() workers.ExecuteRequest {
				request := workers.ExecuteRequest{
					Target: workers.ExecutionTarget{
						RunnerID: runners.AgentIdentity,
					},
					Input: workers.ExecutionInput{Resume: &workers.ProviderContinuationRef{
						Provider:          string(providers.IDClaude),
						ProviderSessionID: "session-3",
					}},
				}
				return request
			}(),
			service: &Service{providers: &providerAuthorizationFake{
				resolveResult: providers.ResolveIdentityResult{ID: providers.IDClaude},
			}},
			wantProvider:       string(providers.IDClaude),
			wantResumeProvider: string(providers.IDClaude),
			wantRunner:         workers.RunnerIDClaude,
			wantAlias:          "",
		},
		{
			name:     "non agent runner value remains unchanged after authorization",
			identity: runners.AgentIdentity,
			request: func() workers.ExecuteRequest {
				request := agentProviderRequestWithAlias("codex")
				request.Target.RunnerID = "custom-agent"
				return request
			}(),
			service: &Service{providers: &providerAuthorizationFake{
				resolveResult: providers.ResolveIdentityResult{ID: providers.IDCodex},
			}},
			wantProvider: string(providers.IDCodex),
			wantRunner:   "custom-agent",
			wantAlias:    "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := test.request
			err := test.service.authorizeProviderTarget(context.Background(), &request, test.identity)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("authorizeProviderTarget() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("authorizeProviderTarget() error = %v", err)
			}
			if request.Target.Provider.ID != test.wantProvider ||
				request.Target.Provider.Alias != test.wantAlias ||
				request.Target.RunnerID != test.wantRunner {
				t.Fatalf("authorized target = %#v, want provider %q/alias %q/runner %q", request.Target, test.wantProvider, test.wantAlias, test.wantRunner)
			}
			if test.wantResumeProvider != "" && request.Input.Resume.Provider != test.wantResumeProvider {
				t.Fatalf("resume provider = %q, want %q", request.Input.Resume.Provider, test.wantResumeProvider)
			}
		})
	}
}

func TestValidateProviderCapabilities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		required    []workers.RunnerOptionalCapability
		descriptor  providers.Descriptor
		getErr      error
		wantErr     error
		wantFailure workers.WorkFailureType
	}{
		{
			name:     "no provider capabilities are required",
			required: []workers.RunnerOptionalCapability{workers.RunnerOptionalCapabilityWorkingDirectory},
		},
		{
			name:     "provider lookup failure",
			required: []workers.RunnerOptionalCapability{workers.RunnerOptionalCapabilityImageInput},
			getErr:   errors.New("catalog unavailable"),
			wantErr:  workers.ErrInvalidExecuteRequest,
		},
		{
			name:        "missing capability is permanent bad request",
			required:    []workers.RunnerOptionalCapability{workers.RunnerOptionalCapabilityStructuredOutput},
			wantErr:     providers.ErrCapabilityMismatch,
			wantFailure: workers.WorkFailureTypePermanentBadRequest,
		},
		{
			name:       "all mapped capabilities are present",
			required:   []workers.RunnerOptionalCapability{workers.RunnerOptionalCapabilityImageInput, workers.RunnerOptionalCapabilitySessionResume, workers.RunnerOptionalCapabilityStructuredOutput},
			descriptor: providers.Descriptor{Capabilities: []providers.Capability{providers.CapabilityImageInput, providers.CapabilitySessionResume, providers.CapabilityStructuredOutput}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &providerAuthorizationFake{
				descriptor: test.descriptor,
				getErr:     test.getErr,
			}
			err := validateProviderCapabilities(context.Background(), fake, providers.IDCodex, test.required)
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("validateProviderCapabilities() error = %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("validateProviderCapabilities() error = %v, want %v", err, test.wantErr)
			}
			if test.wantFailure != "" {
				var providerErr *workers.ProviderError
				if !errors.As(err, &providerErr) || providerErr.Type != test.wantFailure {
					t.Fatalf("provider failure = %#v, want type %q", providerErr, test.wantFailure)
				}
			}
		})
	}
}

func TestProviderCapabilityAndRunnerHelpers(t *testing.T) {
	t.Parallel()

	capabilityTests := []struct {
		input providers.Capability
		want  workers.RunnerOptionalCapability
		ok    bool
	}{
		{input: providers.CapabilityImageInput, want: workers.RunnerOptionalCapabilityImageInput, ok: true},
		{input: providers.CapabilitySessionResume, want: workers.RunnerOptionalCapabilitySessionResume, ok: true},
		{input: providers.CapabilityStructuredOutput, want: workers.RunnerOptionalCapabilityStructuredOutput, ok: true},
		{input: providers.Capability("unknown")},
	}
	for _, test := range capabilityTests {
		got, ok := providerCapabilityForRunnerCapability(test.want)
		if test.input == "unknown" {
			got, ok = providerCapabilityForRunnerCapability("unknown")
		}
		if ok != test.ok || (ok && got != test.input) {
			t.Fatalf("providerCapabilityForRunnerCapability(%q) = %q, %t; want %q, %t", test.want, got, ok, test.input, test.ok)
		}
	}

	descriptor := providers.Descriptor{Capabilities: []providers.Capability{providers.CapabilityImageInput}}
	if !providerDescriptorHasCapability(descriptor, providers.CapabilityImageInput) {
		t.Fatal("providerDescriptorHasCapability() = false for present capability")
	}
	if providerDescriptorHasCapability(descriptor, providers.CapabilitySessionResume) {
		t.Fatal("providerDescriptorHasCapability() = true for absent capability")
	}
	if providerDescriptorHasCapability(providers.Descriptor{}, providers.CapabilityImageInput) {
		t.Fatal("providerDescriptorHasCapability() = true for empty descriptor")
	}

	if got := runnerIDForProvider(providers.IDAntigravity); got != workers.RunnerIDAntigravity {
		t.Fatalf("runnerIDForProvider(antigravity) = %q, want %q", got, workers.RunnerIDAntigravity)
	}
	if got := runnerIDForProvider(providers.IDClaude); got != string(providers.IDClaude) {
		t.Fatalf("runnerIDForProvider(claude) = %q, want %q", got, providers.IDClaude)
	}
}

func agentProviderRequest() workers.ExecuteRequest {
	return agentProviderRequestWithAlias("")
}

func agentProviderRequestWithAlias(alias string) workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Target: workers.ExecutionTarget{
			RunnerID: runners.AgentIdentity,
			Provider: workers.ProviderReference{Alias: alias},
		},
	}
}

type providerAuthorizationFake struct {
	providers.Service
	resolveResult        providers.ResolveIdentityResult
	resolveErr           error
	validateErr          error
	descriptor           providers.Descriptor
	getErr               error
	providerCapabilities []providers.Capability
}

func (fake *providerAuthorizationFake) ResolveIdentity(
	context.Context,
	providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	return fake.resolveResult, fake.resolveErr
}

func (fake *providerAuthorizationFake) ValidatePrerequisites(
	context.Context,
	providers.ValidatePrerequisitesRequest,
) error {
	return fake.validateErr
}

func (fake *providerAuthorizationFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if fake.getErr != nil {
		return providers.GetProviderResult{}, fake.getErr
	}
	descriptor := fake.descriptor
	if len(fake.providerCapabilities) > 0 {
		descriptor.Capabilities = append([]providers.Capability(nil), fake.providerCapabilities...)
	}
	return providers.GetProviderResult{Provider: descriptor}, nil
}
