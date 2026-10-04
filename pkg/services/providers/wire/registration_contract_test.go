package wire

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

func TestNewServiceUsesCompletedOwnersAndSelectedLifecycle(t *testing.T) {
	t.Parallel()
	catalogCalls, executionCalls, closeCalls := 0, 0, 0
	nativeError := errors.New("selected execution error")
	closeError := errors.New("selected close error")
	root, err := NewService(
		completedCatalogFixture{resolve: func(id providers.ID) (providers.ID, error) {
			catalogCalls++
			if id != "fixture-alias" {
				t.Fatalf("catalog input = %q", id)
			}
			return "fixture-provider", nil
		}},
		completedExecutionFixture{execute: func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			executionCalls++
			if request.Provider != "fixture-provider" || request.AttemptID != "fixture-attempt" {
				t.Fatalf("execution request = %#v", request)
			}
			return providers.ExecuteResult{Content: "selected result"}, nativeError
		}},
		completedACPFixture{}, nil, logging.NoopLogger{},
		completedLifecycleFixture{close: func(context.Context) error {
			closeCalls++
			return closeError
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if catalogCalls != 0 || executionCalls != 0 || closeCalls != 0 {
		t.Fatal("construction invoked a supplied operation")
	}
	result, executeErr := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider: "fixture-alias", AttemptID: "fixture-attempt",
	})
	if result.Content != "selected result" || !errors.Is(executeErr, nativeError) || catalogCalls != 1 || executionCalls != 1 {
		t.Fatalf("Execute() = (%#v, %v), catalog=%d execution=%d", result, executeErr, catalogCalls, executionCalls)
	}
	lifecycle := root.(Lifecycle)
	if err := lifecycle.Close(context.Background()); !errors.Is(err, closeError) || closeCalls != 1 {
		t.Fatalf("Close() = %v, calls=%d", err, closeCalls)
	}
}

type completedCatalogFixture struct {
	CatalogService
	resolve func(providers.ID) (providers.ID, error)
}

func (fixture completedCatalogFixture) ResolveProviderID(id providers.ID) (providers.ID, error) {
	return fixture.resolve(id)
}

type completedExecutionFixture struct {
	execute func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)
}

func (fixture completedExecutionFixture) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	return fixture.execute(ctx, request)
}

func (completedExecutionFixture) Continue(context.Context, execution.ContinuationRequest) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency, Message: "provider continuation adapter is unavailable"}
}

type completedACPFixture struct{ ACPService }

func (completedACPFixture) Resolve(providers.ID) (providers.ID, bool) { return "", false }

type completedLifecycleFixture struct{ close func(context.Context) error }

func (fixture completedLifecycleFixture) Close(ctx context.Context) error { return fixture.close(ctx) }

func TestPrepareConfigurationDetachesExplicitCatalogValues(t *testing.T) {
	t.Parallel()
	input := Configuration{
		CatalogDescriptors: []providers.Descriptor{{ID: "fixture-provider", Aliases: []string{"fixture-alias"}}},
		CatalogOverrides:   []CatalogCapabilityOverride{{Provider: providers.IDCodex, Capabilities: []providers.Capability{providers.CapabilityPromptSubmission}}},
	}
	prepared, err := PrepareConfiguration(input)
	if err != nil {
		t.Fatal(err)
	}
	input.CatalogDescriptors[0].Aliases[0] = "mutated-alias"
	input.CatalogOverrides[0].Capabilities[0] = providers.CapabilityPermissionBypass
	if prepared.CatalogDescriptors[0].Aliases[0] != "fixture-alias" || prepared.CatalogOverrides[0].Capabilities[0] != providers.CapabilityPromptSubmission {
		t.Fatalf("configuration retained caller-owned values: %#v", prepared)
	}
}

func TestRegistrationContractValuesDetachAndReportCapabilities(t *testing.T) {
	t.Parallel()

	set := NewCapabilitySet(CapabilityPromptSubmission, "custom")
	values := set.Values()
	values[0] = "mutated"
	if !set.Has(CapabilityPromptSubmission) || set.Has("missing") {
		t.Fatalf("CapabilitySet = %#v, want prompt capability and no missing capability", set.Values())
	}
	if set.Values()[0] != CapabilityPromptSubmission {
		t.Fatalf("CapabilitySet.Values() shares internal storage: %v", set.Values())
	}

	response := Response{Content: "detached"}
	completion := SuccessfulCompletion(response)
	if completion.Response == nil || completion.Response.Content != response.Content {
		t.Fatalf("SuccessfulCompletion() = %#v, want detached response", completion)
	}
	response.Content = "mutated"
	if completion.Response.Content == response.Content {
		t.Fatal("SuccessfulCompletion() shares response value state")
	}
}

func TestProgressingExternalIntegrationExercisesProtocolLifecycle(t *testing.T) {
	t.Parallel()

	integration := ProgressingExternalIntegration("sealed", "result")
	if integration.Identity() != "sealed" {
		t.Fatalf("Identity() = %q, want sealed", integration.Identity())
	}
	if !integration.MaximumCapabilities().Has(CapabilityPromptSubmission) {
		t.Fatalf("MaximumCapabilities() = %v, want prompt submission", integration.MaximumCapabilities().Values())
	}
	if _, err := integration.Discover(context.Background()); err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if _, err := integration.Capabilities(context.Background(), InvocationRequest{}); err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}

	writer := &recordingResponseWriter{}
	if err := integration.Invoke(context.Background(), InvocationRequest{}, writer); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if writer.events != 1 || writer.completion == nil || writer.completion.Response == nil || writer.completion.Response.Content != "result" {
		t.Fatalf("response writer = %#v, want one event and result completion", writer)
	}
	stats := integration.Stats()
	want := ProgressingIntegrationStats{DiscoverCalls: 1, CapabilityCalls: 1, InvokeCalls: 1, ProgressWrites: 1, TerminalCloses: 1}
	if !reflect.DeepEqual(stats, want) {
		t.Fatalf("Stats() = %#v, want %#v", stats, want)
	}
}

func TestRegistrationContractManifestCloneDetachesSlicesAndMaps(t *testing.T) {
	t.Parallel()

	values := map[string]string{"en": "Description"}
	manifest := Manifest{
		Aliases:       []string{"alias"},
		Documentation: []DocumentationLink{{Kind: "docs", URL: "https://example.invalid"}},
		Description:   LocalizedValue{Values: &values},
	}
	cloned := cloneManifest(manifest)
	manifest.Aliases[0] = "mutated"
	manifest.Documentation[0].URL = "mutated"
	values["en"] = "mutated"
	if cloned.Aliases[0] == "mutated" || cloned.Documentation[0].URL == "mutated" || (*cloned.Description.Values)["en"] == "mutated" {
		t.Fatalf("cloneManifest() shares mutable state: %#v", cloned)
	}
}

func TestExternalRegistrationAttemptMapsSuccessAndRejectsInvalidRegistration(t *testing.T) {
	t.Parallel()

	descriptor := registrationDescriptor(Manifest{
		ID:          "sealed",
		DisplayName: LocalizedValue{Value: "Sealed"},
		Aliases:     []string{"sealed-alias"},
		MaximumExecutionCapabilities: ExecutionCapabilities{
			PromptSubmission: true, ImageInput: true, SessionResume: true, StructuredOutput: true, PermissionBypass: true,
		},
	})
	if descriptor.ID != "sealed" || len(descriptor.Aliases) != 1 || len(descriptor.Capabilities) != 5 || !slices.Contains(descriptor.Capabilities, providers.CapabilityPermissionBypass) {
		t.Fatalf("registrationDescriptor() = %#v, want identity, alias, and five capabilities including permission bypass", descriptor)
	}

	integration := &permissionBypassIntegration{
		ProgressingIntegration: ProgressingExternalIntegration("sealed", "attempt result"),
	}
	attempt, err := externalRegistrationAttempt(Registration{
		Manifest:    Manifest{ID: "sealed"},
		Integration: integration,
	})
	if err != nil {
		t.Fatalf("externalRegistrationAttempt() error = %v", err)
	}
	result, err := attempt.Attempt(context.Background(), providers.ExecuteRequest{
		Provider: providers.ID("sealed"), AttemptID: "attempt-1", Model: "model-1", UserMessage: "hello", SkipPermissions: true,
	})
	if err != nil || result.Content != "attempt result" || result.Diagnostics == nil {
		t.Fatalf("external attempt = (%#v, %v), want result with diagnostics", result, err)
	}
	if !integration.Stats().LastSkipPermissions {
		t.Fatal("external invocation lost skip permissions")
	}

	if _, err := externalRegistrationAttempt(Registration{Manifest: Manifest{ID: "missing"}}); err == nil || !strings.Contains(err.Error(), "integration is required") {
		t.Fatalf("missing integration error = %v, want validation error", err)
	}
	if _, err := externalRegistrationAttempt(Registration{
		Manifest:    Manifest{ID: "manifest"},
		Integration: ProgressingExternalIntegration("different", "ignored"),
	}); err == nil || !strings.Contains(err.Error(), "does not match manifest") {
		t.Fatalf("identity mismatch error = %v, want validation error", err)
	}
}

func TestExternalRegistrationCompletesUnsupportedContinuationWithoutInvocation(t *testing.T) {
	t.Parallel()
	integration := ProgressingExternalIntegration("sealed", "must not execute")
	registration, err := externalRegistrationAttempt(Registration{
		Manifest: Manifest{ID: "sealed"}, Integration: integration,
	})
	if err != nil {
		t.Fatalf("externalRegistrationAttempt() = %v", err)
	}
	before := integration.Stats()
	request := execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{Provider: "sealed", AttemptID: "continue-1"},
		ResumeSession:  &providers.SessionRef{Provider: "sealed", Kind: providers.SessionIDKind, ID: "session-1"},
	}
	for range 2 {
		result, err := registration.Continue(context.Background(), request)
		var failure providers.ExecuteFailure
		if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindDependency ||
			failure.Message != "provider continuation adapter is unavailable" {
			t.Fatalf("Continue() error = %#v, want existing unsupported-continuation failure", err)
		}
		if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
			t.Fatalf("Continue() result = %#v, want empty result", result)
		}
	}
	if after := integration.Stats(); !reflect.DeepEqual(after, before) {
		t.Fatalf("unsupported continuation invoked integration: before %#v, after %#v", before, after)
	}
}

func TestNewServiceRejectsManifestIntegrationPermissionBypassMismatch(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		ID:                         "mismatch-provider",
		ImplementationAvailability: ImplementationExternallySupplied,
		TechnicalSupportLevel:      SupportProduction,
		MaximumExecutionCapabilities: ExecutionCapabilities{
			PromptSubmission: true,
			PermissionBypass: true,
		},
	}
	integration := ProgressingExternalIntegration("mismatch-provider", "must not execute")
	_, err := newTestProvidersService(IdentityCatalogProbe,
		platformclock.Real{}, logging.NoopLogger{}, nil, nil, nil,
		nil,
		nil,
		nil,
		Configuration{Registrations: []Registration{Registration{
			Manifest:    manifest,
			Integration: integration,
		}}})
	if err == nil || !strings.Contains(err.Error(), `integration maximum capability "permission_bypass" contradicts`) {
		t.Fatalf("newTestProvidersService() error = %v, want manifest/integration permission-bypass mismatch", err)
	}
	if stats := integration.Stats(); stats.DiscoverCalls != 0 || stats.CapabilityCalls != 0 || stats.InvokeCalls != 0 {
		t.Fatalf("mismatched integration stats = %#v, want no provider calls during rejected construction", stats)
	}
}

type recordingResponseWriter struct {
	events     int
	completion *Completion
}

func (writer *recordingResponseWriter) WriteEvent(context.Context, EventDraft) error {
	writer.events++
	return nil
}

func (writer *recordingResponseWriter) Close(_ context.Context, completion Completion) error {
	writer.completion = &completion
	return nil
}

var _ ResponseWriter = (*recordingResponseWriter)(nil)

type permissionBypassIntegration struct {
	*ProgressingIntegration
}

func (*permissionBypassIntegration) MaximumCapabilities() CapabilitySet {
	return NewCapabilitySet(CapabilityPromptSubmission, CapabilityPermissionBypass)
}

func (integration *permissionBypassIntegration) Capabilities(context.Context, InvocationRequest) (CapabilitySet, error) {
	return integration.MaximumCapabilities(), nil
}

func TestNewServiceUsesCompletedConstructionPorts(t *testing.T) {
	t.Parallel()

	service, err := newTestProvidersService(IdentityCatalogProbe,
		platformclock.Real{}, logging.NoopLogger{}, nil, nil, nil,
		nil,
		nil,
		nil,
		Configuration{})
	if err != nil {
		t.Fatalf("newTestProvidersService() error = %v, want successful construction with required ports", err)
	}
	if service == nil {
		t.Fatal("newTestProvidersService() returned nil service, want non-nil providers.Service")
	}
	var root providers.Service = service
	if root == nil {
		t.Fatal("constructed root is not assignable to providers.Service")
	}
}

// newTestProvidersService assembles the fixture's explicit sibling owners.
func newTestProvidersService(probe CatalogProbeOperation, scheduler platformclock.TimerSource, logger logging.Logger, commandFactory platformprocess.CommandFactory, locator platformprocess.ExecutableLocator, stdioPipes platformprocess.StdioPipeFactory, antigravity AgyEffect, codex CodexEffect, claude ClaudeEffect, configuration Configuration) (providers.Service, error) {
	config, err := PrepareConfiguration(configuration)
	if err != nil {
		return nil, err
	}
	catalogService, err := NewCatalogService(probe, config.CatalogDescriptors, config.CatalogOverrides)
	if err != nil {
		return nil, err
	}
	acpService, err := NewACPService(config.ACPIntegrations, commandFactory, locator, stdioPipes, scheduler, logger)
	if err != nil {
		return nil, err
	}
	// Absent fixture routes receive completed command effects with a disabled edge.
	if antigravity == nil {
		antigravity = NewAgyCommandEffect((disabledNativeRunner{"Antigravity"}).commandEffect(), platformclock.Real{}, scheduler)
	}
	if codex == nil {
		codex = NewCodexEffect((disabledNativeRunner{"Codex"}).commandEffect(), platformclock.Real{})
	}
	if claude == nil {
		claude = NewClaudeEffect((disabledNativeRunner{"Claude"}).commandEffect(), platformclock.Real{})
	}
	registrations, err := ExecutionRegistrations(antigravity, codex, claude, acpService, config.ACPIntegrations, config.Registrations)
	if err != nil {
		return nil, err
	}
	executionService, err := NewExecutionService(catalogService, registrations)
	if err != nil {
		return nil, err
	}
	return NewService(catalogService, executionService, acpService, config.ACPIntegrations, logger, acpService)
}

type disabledNativeRunner struct{ name string }

func (runner disabledNativeRunner) Run(context.Context, providers.CommandRequest) (providers.CommandResult, error) {
	return providers.CommandResult{}, providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: runner.name + " native execution is unavailable",
	}
}

// RunStreaming supplies the buffered fixture's completed stdout chunk.
func (runner disabledNativeRunner) RunStreaming(ctx context.Context, request providers.CommandRequest, observe providers.OutputChunkObserver) (providers.CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if len(result.Stdout) > 0 && observe != nil {
		if observeErr := observe(providers.OutputStreamStdout, result.Stdout); err == nil {
			err = observeErr
		}
	}
	return result, err
}

func (runner disabledNativeRunner) commandEffect() providers.CommandRunner {
	return providers.CommandRunner{Run: runner.Run, RunStreaming: runner.RunStreaming}
}
