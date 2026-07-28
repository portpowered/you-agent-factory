package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

type registryRunnerRecorder struct {
	calls int
}

func (r *registryRunnerRecorder) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	r.calls++
	return workers.RunnerExecutionResult{}, nil
}

func TestRegistryCapabilityRunnerUsesManifestMaximumBeforeNativeExecution(t *testing.T) {
	t.Parallel()
	providers := builtInProviderRegistry(t)
	next := &registryRunnerRecorder{}
	runner := registryCapabilityRunner{next: next, providers: providers}

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		RunnerID: workers.RunnerIDGemini,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "session resume is not supported by the gemini runner") {
		t.Fatalf("Execute() error = %v", err)
	}
	if next.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", next.calls)
	}

	_, err = runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		RunnerID: workers.RunnerIDCodex,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityStructuredOutput,
		},
	})
	if err != nil {
		t.Fatalf("Execute(codex) error = %v", err)
	}
	if next.calls != 1 {
		t.Fatalf("native runner calls = %d, want 1", next.calls)
	}
}

func TestRegistrySelectionRejectsUnknownInsteadOfDefaulting(t *testing.T) {
	t.Parallel()
	providers := builtInProviderRegistry(t)

	_, err := resolveRuntimeRunnerSelection(providers, "", "", "unknown-provider")
	if err == nil || !strings.Contains(err.Error(), `provider "unknown-provider" is unknown`) {
		t.Fatalf("resolveRuntimeRunnerSelection() error = %v", err)
	}
}

func TestRuntimeSelectionCompatibilityWithoutRegistry(t *testing.T) {
	t.Parallel()

	selection, err := resolveRuntimeRunnerSelection(nil, "", "", workers.RunnerIDCodex)
	if err != nil {
		t.Fatalf("resolveRuntimeRunnerSelection() error = %v", err)
	}
	if selection.RunnerID != workers.RunnerIDCodex {
		t.Fatalf("resolveRuntimeRunnerSelection() = %#v", selection)
	}
	if err := validateRuntimeRunnerIdentity(nil, workers.RunnerIDCodex); err != nil {
		t.Fatalf("validateRuntimeRunnerIdentity(codex) error = %v", err)
	}
	if err := validateRuntimeRunnerIdentity(nil, "unknown-provider"); err == nil {
		t.Fatal("validateRuntimeRunnerIdentity(unknown) succeeded")
	}
}

func TestRegistryCapabilityValidationWithoutRegistryPreservesNativeRunner(t *testing.T) {
	t.Parallel()

	next := &registryRunnerRecorder{}
	runner := registryCapabilityRunner{next: next}
	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		RunnerID: workers.RunnerIDCodex,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityStructuredOutput,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if next.calls != 1 {
		t.Fatalf("native runner calls = %d, want 1", next.calls)
	}
}

func TestRegistryCapabilityRunnerRejectsUnknownRunnerBeforeNativeExecution(t *testing.T) {
	t.Parallel()

	next := &registryRunnerRecorder{}
	runner := registryCapabilityRunner{next: next, providers: builtInProviderRegistry(t)}
	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		RunnerID: "unknown-provider",
	})
	if err == nil || !strings.Contains(err.Error(), `provider "unknown-provider" is unknown`) {
		t.Fatalf("Execute() error = %v, want unknown-provider diagnostic", err)
	}
	if next.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", next.calls)
	}
}

func builtInProviderRegistry(t *testing.T) *providerregistry.Registry {
	t.Helper()
	registrations, err := providerregistry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := providerregistry.New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return providers
}

type conductorRouteRecordingRunner struct {
	calls int
}

func (r *conductorRouteRecordingRunner) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	r.calls++
	return workers.RunnerExecutionResult{Content: "native"}, nil
}

type successfulConductorIntegration struct {
	identity inference.Identity
	maximum  inference.CapabilitySet
	calls    int
}

func (i *successfulConductorIntegration) Identity() inference.Identity { return i.identity }

func (i *successfulConductorIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(i.maximum.Values()...)
}

func (i *successfulConductorIntegration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

func (i *successfulConductorIntegration) Capabilities(
	context.Context,
	inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

func (i *successfulConductorIntegration) Invoke(
	ctx context.Context,
	_ inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	i.calls++
	return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
		Content: "conductor-ok",
	})))
}

type failingConductorIntegration struct {
	identity inference.Identity
	failure  inference.Failure
}

func (i *failingConductorIntegration) Identity() inference.Identity { return i.identity }

func (*failingConductorIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(inference.CapabilityPromptSubmission)
}

func (*failingConductorIntegration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

func (i *failingConductorIntegration) Capabilities(
	context.Context,
	inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

func (i *failingConductorIntegration) Invoke(
	ctx context.Context,
	_ inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	return writer.Close(ctx, inference.FailedCompletion(i.failure))
}

func TestConductorInvocationRunnerRoutesExternalIntegrationsThroughConductor(t *testing.T) {
	t.Parallel()

	providers, integration := externalConductorRegistry(t)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	result, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:    work.WorkDispatch{DispatchID: "dispatch-conductor-1"},
		RunnerID:    "customer.provider",
		Model:       "fixture-model",
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Execute(external) error = %v", err)
	}
	if result.Content != "conductor-ok" {
		t.Fatalf("Execute(external) content = %q, want conductor-ok", result.Content)
	}
	if integration.calls != 1 {
		t.Fatalf("integration invoke calls = %d, want 1", integration.calls)
	}
	if native.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", native.calls)
	}
}

func TestConductorCollectingDestinationPublishesCanonicalResponseDrafts(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(workers.RunPayload{Status: string(workers.PhaseStarted)})
	if err != nil {
		t.Fatalf("marshal run payload: %v", err)
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID: "dispatch-conductor-progress", Kind: workers.KindRun, Phase: workers.PhaseStarted,
		Provenance: workers.Provenance{
			Provider: "customer.provider", Delivery: workers.DeliverySynthesized,
			Representation: workers.RepresentationNotification,
			Fidelity:       workers.FidelityLifecycleOnly,
		},
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("NewEventDraft() error = %v", err)
	}
	var published []workers.ProgressFragment
	destination := conductorCollectingDestination{
		dispatchID: "dispatch-conductor-progress",
		publish: func(fragment workers.ProgressFragment) {
			published = append(published, fragment)
		},
	}

	if err := destination.WriteEvent(context.Background(), event); err != nil {
		t.Fatalf("WriteEvent() error = %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("published response drafts = %d, want 1", len(published))
	}
	for index, fragment := range published {
		draft, ok := fragment.CanonicalDraft.(workers.Draft)
		if !ok || draft.DispatchID != "dispatch-conductor-progress" {
			t.Fatalf("published[%d] = %#v, want correlated canonical draft", index, fragment)
		}
	}
}

func TestConductorInvocationRunnerPreservesRetryableUnknownFailurePolicy(t *testing.T) {
	t.Parallel()

	const providerID = "customer.retryable"
	session := inference.NewProviderSession(providerID, "session_id", "session-retry-1", nil)
	integration := &failingConductorIntegration{
		identity: inference.Identity(providerID),
		failure: inference.NewFailure(inference.FailureInput{
			Kind:            inference.FailureUnknown,
			Message:         "provider is temporarily unavailable",
			Retryable:       true,
			ProviderSession: &session,
		}),
	}
	builtIns, err := providerregistry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	manifest := externalConductorManifest(t, providerID, "retryable")
	providers, err := providerregistry.New(append(
		builtIns,
		providerregistry.ExternalRegistration(manifest, integration),
	)...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	_, err = runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-retryable-1"},
		RunnerID: providerID,
	})
	var providerErr *workerprovider.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypeInternalServerError {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, workers.WorkFailureTypeInternalServerError)
	}
	decision := workerprovider.WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("failure decision = %#v, want retryable non-terminal non-throttle", decision)
	}
	if providerErr.ProviderSession == nil ||
		providerErr.ProviderSession.Provider != providerID ||
		providerErr.ProviderSession.ID != "session-retry-1" {
		t.Fatalf("provider session = %#v, want preserved retry session", providerErr.ProviderSession)
	}
	if native.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", native.calls)
	}
}

func TestConductorInvocationRunnerRoutesCodexThroughConductor(t *testing.T) {
	t.Parallel()

	providers, integration := externalConductorRegistry(t)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-codex-1"},
		RunnerID: workers.RunnerIDCodex,
	})
	if err == nil {
		t.Fatal("Execute(codex) error = nil, want conductor dependency failure without Providers service")
	}
	if native.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", native.calls)
	}
	if integration.calls != 0 {
		t.Fatalf("integration invoke calls = %d, want 0 without Providers wiring", integration.calls)
	}
}

func TestConductorInvocationRunnerResolvesModelProviderWhenRunnerIDEmpty(t *testing.T) {
	t.Parallel()

	providers := builtInProviderRegistry(t)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-codex-model-provider"},
		ModelProvider: workers.RunnerIDCodex,
		UserMessage:   "hello codex",
	})
	if err == nil {
		t.Fatal("Execute(modelProvider=codex) error = nil, want conductor dependency failure without Providers service")
	}
	if native.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", native.calls)
	}
}

func TestConductorInvocationRunnerRoutesMigratedGeminiThroughConductor(t *testing.T) {
	t.Parallel()

	commandRunner := &builtInConductorCommandRunner{
		result: workers.CommandResult{Stdout: []byte("gemini via conductor")},
	}
	providers := geminiConductorRegistry(t, commandRunner)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	result, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:    work.WorkDispatch{DispatchID: "dispatch-gemini-conductor"},
		RunnerID:    "gemini",
		Model:       "gemini-2.5-flash",
		UserMessage: "hello gemini",
	})
	if err != nil {
		t.Fatalf("Execute(gemini) error = %v", err)
	}
	if result.Content != "gemini via conductor" {
		t.Fatalf("Execute(gemini) content = %q, want gemini via conductor", result.Content)
	}
	if native.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0 for migrated Gemini", native.calls)
	}
	if commandRunner.calls != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1", commandRunner.calls)
	}
	if providers.UsesNativeRunner(workers.RunnerIDCodex) {
		t.Fatal("UsesNativeRunner(codex) = true, want conductor route")
	}
	if providers.UsesNativeRunner("claude") {
		t.Fatal("UsesNativeRunner(claude) = true, want conductor route")
	}
	if providers.UsesNativeRunner("gemini") {
		t.Fatal("UsesNativeRunner(gemini) = true, want conductor route")
	}
}

func TestConductorInvocationRunnerRoutesMigratedCursorThroughConductor(t *testing.T) {
	t.Parallel()

	stdout := cursorConductorSuccessStdout("cursor via conductor", "cursor-session-123")
	platformRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	providers := cursorConductorRegistryWithRunner(t, platformRunner)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	result, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cursor-conductor"},
		RunnerID:      workers.RunnerIDCursorCLI,
		ModelProvider: workers.RunnerIDCursorCLI,
		Model:         "cursor-model",
		UserMessage:   "hello cursor",
	})
	if err != nil {
		t.Fatalf("Execute(cursor) error = %v", err)
	}
	if result.Content != "cursor via conductor" {
		t.Fatalf("Execute(cursor) content = %q, want cursor via conductor", result.Content)
	}
	if native.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0 for migrated Cursor", native.calls)
	}
	if platformRunner.CallCount() != 1 || platformRunner.LastRequest().Command != "agent" {
		t.Fatalf("Cursor command calls = %d request = %#v", platformRunner.CallCount(), platformRunner.LastRequest())
	}
	if providers.UsesNativeRunner("cursor") || providers.UsesNativeRunner("agent") {
		t.Fatal("UsesNativeRunner(cursor) = true, want conductor route")
	}
}

func TestConductorInvocationRunnerPreservesKiroResumeSessionWithoutReplacement(t *testing.T) {
	t.Parallel()

	const sessionID = "675f9238-5f05-456c-9a9f-f8fe486f49e4"
	commandRunner := &builtInConductorCommandRunner{
		result: workers.CommandResult{Stdout: []byte("kiro resumed via conductor")},
	}
	registrations, err := providerregistry.BuiltInRegistrations(
		providerregistry.BuiltInDependencies{CommandRunner: commandRunner},
	)
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := providerregistry.New(registrations...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	result, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-kiro-resume"},
		RunnerID:      workers.RunnerIDKiro,
		ModelProvider: workers.RunnerIDKiro,
		UserMessage:   "continue",
		SessionID:     sessionID,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
		},
	})
	if err != nil {
		t.Fatalf("Execute(kiro resume) error = %v", err)
	}
	if result.Content != "kiro resumed via conductor" {
		t.Fatalf("Execute(kiro resume) content = %q", result.Content)
	}
	if commandRunner.calls != 1 {
		t.Fatalf("Kiro command runner calls = %d, want 1", commandRunner.calls)
	}
	if native.calls != 0 {
		t.Fatalf("native runner calls = %d, want 0", native.calls)
	}
	if !containsRunnerArgPair(commandRunner.request.Args, "--resume-id", sessionID) {
		t.Fatalf("Kiro command args = %#v, want --resume-id %s", commandRunner.request.Args, sessionID)
	}
	if result.ProviderSession == nil ||
		result.ProviderSession.Provider != workers.RunnerIDKiro ||
		result.ProviderSession.Kind != "session_id" ||
		result.ProviderSession.ID != sessionID {
		t.Fatalf("Kiro provider session = %#v, want preserved kiro/session_id/%s", result.ProviderSession, sessionID)
	}
}

func TestInvocationRequestFromRunnerPreservesExecutionContext(t *testing.T) {
	t.Parallel()

	want := workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-context-1",
			WorkstationName: "gemini-workstation",
			ProjectID:       "project-context",
			InputTokens:     []any{"dispatch-token"},
		},
		WorkerType:         "gemini-worker",
		WorkstationType:    "model-workstation",
		RunnerID:           workers.RunnerIDGemini,
		ProjectID:          "project-context",
		InputTokens:        []any{"request-token"},
		Model:              "gemini-2.5-flash",
		ModelProvider:      "gemini",
		SystemPrompt:       "system context",
		UserMessage:        "user context",
		OutputSchema:       `{"type":"object"}`,
		EnvVars:            map[string]string{"GEMINI_CONTEXT": "configured"},
		ProcessEnvironment: []string{"PATH=/fixture", "INHERITED=present"},
		Worktree:           "worktrees/gemini-context",
		WorkingDirectory:   "C:/fixture/gemini-context",
	}

	got := invocationRequestFromRunner(want).Execution()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Execution() = %#v, want %#v", got, want)
	}
}

func TestConductorInvocationRunnerBypassedWhenProviderOverrideDisablesRegistryDecorators(t *testing.T) {
	t.Parallel()

	providers, _ := externalConductorRegistry(t)
	service := &Service{
		providerRegistry:    providers,
		invocationConductor: conductor.New(providers),
	}
	decorators := service.runtimeRunnerDecorators(nil, nil, nil, nil, false, nil)
	for _, decorator := range decorators {
		runner := decorator(&conductorRouteRecordingRunner{}, nil)
		if _, ok := runner.(conductorInvocationRunner); ok {
			t.Fatal("ProviderOverride path attached conductorInvocationRunner")
		}
		if _, ok := runner.(registryCapabilityRunner); ok {
			t.Fatal("ProviderOverride path attached registryCapabilityRunner")
		}
	}
}

func TestConductorInvocationRunnerRejectsCapabilityEscalationBeforeProviderIO(t *testing.T) {
	t.Parallel()

	providers, integration := externalConductorRegistry(t)
	native := &conductorRouteRecordingRunner{}
	runner := conductorInvocationRunner{
		next:      native,
		conductor: conductor.New(providers),
		providers: providers,
	}

	_, err := runner.Execute(context.Background(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-escalate-1"},
		RunnerID: "customer.provider",
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilitySessionResume,
		},
	})
	if err == nil {
		t.Fatal("Execute(escalation) error = nil, want rejection")
	}
	var providerErr *workerprovider.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute(escalation) error = %v, want *ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want permanent bad request", providerErr.Type)
	}
	if !strings.Contains(providerErr.Error(), "session_resume") &&
		!strings.Contains(providerErr.Message, "session_resume") {
		t.Fatalf("provider error = %#v, want session_resume diagnostic", providerErr)
	}
	if integration.calls != 0 || native.calls != 0 {
		t.Fatalf("provider I/O occurred: integration=%d native=%d", integration.calls, native.calls)
	}
}

func TestNewRuntimeWithSelectionComposesConductorFromRegistry(t *testing.T) {
	t.Parallel()

	providers, _ := externalConductorRegistry(t)
	service := &Service{
		providerRegistry:    providers,
		invocationConductor: conductor.New(providers),
	}
	if service.invocationConductor == nil {
		t.Fatal("invocationConductor = nil")
	}
	decorators := service.runtimeRunnerDecorators(nil, nil, nil, nil, true, nil)
	var sawConductor bool
	for _, decorator := range decorators {
		runner := decorator(&conductorRouteRecordingRunner{}, nil)
		if _, ok := runner.(conductorInvocationRunner); ok {
			sawConductor = true
		}
	}
	if !sawConductor {
		t.Fatal("runtimeRunnerDecorators omitted conductorInvocationRunner")
	}
}

func externalConductorRegistry(t *testing.T) (*providerregistry.Registry, *successfulConductorIntegration) {
	t.Helper()
	builtIns, err := providerregistry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	manifest := externalConductorManifest(t, "customer.provider", "customer")
	integration := &successfulConductorIntegration{
		identity: inference.Identity(manifest.ID),
		maximum: inference.NewCapabilitySet(
			inference.CapabilityPromptSubmission,
		),
	}
	providers, err := providerregistry.New(append(
		builtIns,
		providerregistry.ExternalRegistration(manifest, integration),
	)...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers, integration
}

func externalConductorManifest(t *testing.T, identity, alias string) providerregistry.Manifest {
	t.Helper()
	var catalog struct {
		Providers []providerregistry.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = providerregistry.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = providerregistry.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = providerregistry.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = providerregistry.ResponseFidelityCapabilities{}
	return manifest
}

func cursorConductorSuccessStdout(content, sessionID string) []byte {
	return []byte(`{"type":"result","subtype":"success","is_error":false,"result":"` + content + `","session_id":"` + sessionID + `"}` + "\n")
}

func cursorConductorRegistryWithRunner(
	t *testing.T,
	runner platformprocess.CommandRunner,
) *providerregistry.Registry {
	t.Helper()
	providersService, err := providerswire.NewService(providerswire.WithCommandRunner(runner))
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	builtIns, err := providerregistry.BuiltInRegistrations(providerregistry.BuiltInDependencies{
		ProvidersService: providersService,
	})
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := providerregistry.New(builtIns...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers
}

func geminiConductorRegistry(t *testing.T, runner workers.CommandRunner) *providerregistry.Registry {
	t.Helper()
	builtIns, err := providerregistry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	replaced, err := providerregistry.ReplaceCatalogIntegration(
		builtIns,
		"gemini",
		gemini.NewIntegration(gemini.IntegrationDependencies{CommandRunner: runner}),
	)
	if err != nil {
		t.Fatalf("ReplaceCatalogIntegration(gemini) error = %v", err)
	}
	providers, err := providerregistry.New(replaced...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers
}

type builtInConductorCommandRunner struct {
	calls   int
	request workers.CommandRequest
	result  workers.CommandResult
}

func (r *builtInConductorCommandRunner) Run(
	_ context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	r.calls++
	r.request = request
	return r.result, nil
}

func containsRunnerArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
