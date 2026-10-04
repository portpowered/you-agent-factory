// Package wire is the Providers service composition boundary.
//
// Wire exposes focused inert constructors for completed private roles.
// Canonical composition assembles these once; the Providers root consumes them
// directly. Peers depend on Service rather than private construction roles. The
// process-edge registration contract in this package is for root composition,
// not a second peer-facing Providers service. Missing required construction
// ports fail with a deterministic construction error and a nil service.
package wire

import (
	"context"
	"fmt"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/catalogdata"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	acpwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/wire"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

// CommandRunner is the Providers-owned subprocess effect accepted at the
// composition boundary. Workers-specific runners are projected into this
// contract by pkg/wire.
type CommandRunner = providerservice.CommandRunner
type CommandRequest = providerservice.CommandRequest
type CommandResult = providerservice.CommandResult
type OutputChunkObserver = providerservice.OutputChunkObserver
type PTYAllocator = providerservice.PTYAllocator
type PTYSession = providerservice.PTYSession
type PTYSessionConfig = providerservice.PTYSessionConfig
type PTYProcessLaunch = providerservice.PTYProcessLaunch
type PTYSessionResult = providerservice.PTYSessionResult

const (
	DefaultPTYMaxCaptureBytes = providerservice.DefaultPTYMaxCaptureBytes
	MaxPTYMaxCaptureBytes     = providerservice.MaxPTYMaxCaptureBytes
	DefaultPTYIdleTimeout     = providerservice.DefaultPTYIdleTimeout
	DefaultPTYHardTimeout     = providerservice.DefaultPTYHardTimeout
)

// DefaultPTYSessionConfig returns the bounded native-session defaults used by
// the Providers Agy adapter.
func DefaultPTYSessionConfig() PTYSessionConfig {
	return providerservice.DefaultPTYSessionConfig()
}

const (
	OutputStreamStdout = providerservice.OutputStreamStdout
	OutputStreamStderr = providerservice.OutputStreamStderr
)

// NewAgyPTYAllocator constructs the Providers-owned PTY implementation.
func NewAgyPTYAllocator(host platformpty.Host, clock platformclock.Source, scheduler platformclock.TimerSource) (PTYAllocator, error) {
	return executionwire.NewAgyPTYAllocator(host, clock, scheduler)
}

// AgyEffect exposes the completed AGY execution collaborator to composition.
type AgyEffect = executionwire.AgyEffect

// Native effect aliases expose individually completed command collaborators.
type CodexEffect = executionwire.CodexEffect
type ClaudeEffect = executionwire.ClaudeEffect

// NewCodexEffect constructs one Codex command effect.
func NewCodexEffect(runner CommandRunner, clock platformclock.Source) CodexEffect {
	return executionwire.NewCodexEffect(runner, clock)
}

// NewClaudeEffect constructs one Claude command effect.
func NewClaudeEffect(runner CommandRunner, clock platformclock.Source) ClaudeEffect {
	return executionwire.NewClaudeEffect(runner, clock)
}

// NewAgyCommandEffect constructs one AGY print-mode effect.
func NewAgyCommandEffect(runner CommandRunner, clock platformclock.Source, scheduler platformclock.TimerSource) AgyEffect {
	return executionwire.NewAgyCommandEffect(runner, clock, scheduler)
}

// AgyPTYPolicy contains detached native-session policy, without host effects.
type AgyPTYPolicy = executionwire.AgyPTYPolicy

// NewAgyPTYEffect constructs one native effect over individually supplied ports.
func NewAgyPTYEffect(allocator PTYAllocator, locator platformprocess.ExecutableLocator, inspector platformfilesystem.PathInspector, clock platformclock.Source, policy AgyPTYPolicy) AgyEffect {
	return executionwire.NewAgyPTYEffect(allocator, locator, inspector, clock, policy)
}

// CatalogCapabilityOverride supplies an authoritative capability view for one
// already-registered provider route during process construction. It is used by
// hosts and functional tests whose selected route has narrower capabilities
// than its static publication; it cannot register a new provider identity.
type CatalogCapabilityOverride struct {
	Provider     providers.ID
	Capabilities []providers.Capability
}

// Clone returns detached override values for the construction boundary.
func (override CatalogCapabilityOverride) Clone() CatalogCapabilityOverride {
	return CatalogCapabilityOverride{
		Provider:     override.Provider,
		Capabilities: append([]providers.Capability(nil), override.Capabilities...),
	}
}

// CatalogProbeOperation is the completed catalog readiness projection.
type CatalogProbeOperation = catalog.ProbeOperation

// IdentityCatalogProbe preserves detached catalog facts without readiness I/O.
func IdentityCatalogProbe(ctx context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
	return catalogwire.IdentityProbe(ctx, descriptor)
}

// Configuration contains detached registration and catalog values. It owns no
// service collaborators or runtime resources.
type Configuration struct {
	CatalogDescriptors []providers.Descriptor
	CatalogOverrides   []CatalogCapabilityOverride
	ACPIntegrations    []providers.ACPIntegration
	Registrations      ProviderRegistrations
}

// PrepareConfiguration projects package defaults and explicit construction data.
// It performs no readiness, command, channel or lifecycle effects.
func PrepareConfiguration(config Configuration) (Configuration, error) {
	packaged, err := PackagedACPIntegrations()
	if err != nil {
		return Configuration{}, err
	}
	integrations := effectiveACPIntegrations(packaged, config.ACPIntegrations)
	descriptors, err := packagedACPDescriptors(integrations)
	if err != nil {
		return Configuration{}, err
	}
	for _, registration := range config.Registrations {
		descriptors = append(descriptors, registrationDescriptor(registration.Manifest))
	}
	detached := make([]providers.Descriptor, len(config.CatalogDescriptors))
	for i, descriptor := range config.CatalogDescriptors {
		detached[i] = descriptor.Clone()
	}
	overrides := make([]CatalogCapabilityOverride, len(config.CatalogOverrides))
	for i, override := range config.CatalogOverrides {
		overrides[i] = override.Clone()
	}
	return Configuration{
		CatalogDescriptors: append(detached, descriptors...),
		CatalogOverrides:   overrides,
		ACPIntegrations:    integrations,
		Registrations:      append(ProviderRegistrations(nil), config.Registrations...),
	}, nil
}

// These aliases expose completed private roles to canonical composition.
type CatalogService = catalog.Service
type ExecutionService = execution.ContinuationService
type ACPService = acp.ContinuationService
type Lifecycle = providerservice.Lifecycle
type ExecutionRegistration = execution.Registration

// NewCatalogService constructs only the catalog over the supplied projection.
func NewCatalogService(probe CatalogProbeOperation, descriptors []providers.Descriptor, overrides []CatalogCapabilityOverride) (CatalogService, error) {
	projected := make([]catalog.CapabilityOverride, len(overrides))
	for i, override := range overrides {
		projected[i] = catalog.CapabilityOverride{Provider: override.Provider, Capabilities: append([]providers.Capability(nil), override.Capabilities...)}
	}
	return catalogwire.NewService(probe, descriptors, projected)
}

// NewACPService constructs only the configured ACP owner, without starting peers.
func NewACPService(integrations []providers.ACPIntegration, commandFactory platformprocess.CommandFactory, locator platformprocess.ExecutableLocator, stdioPipes platformprocess.StdioPipeFactory, scheduler platformclock.TimerSource, logger logging.Logger) (ACPService, error) {
	return acpwire.NewService(integrations, commandFactory, locator, stdioPipes, scheduler, logger)
}

// NewExecutionService constructs only normalized execution over completed routes.
func NewExecutionService(catalogService CatalogService, registrations []ExecutionRegistration) (ExecutionService, error) {
	return executionwire.NewService(catalogService, registrations...)
}

// NewService receives completed siblings and the exact close capability. It
// neither constructs a secondary graph nor selects execution effects.
func NewService(catalogService CatalogService, executionService ExecutionService, acpService ACPService, packagedACP []providers.ACPIntegration, logger logging.Logger, lifecycle Lifecycle) (providers.Service, error) {
	root, err := providerservice.NewWithACP(catalogService, executionService, acpService, packagedACP, logger, lifecycle)
	if err != nil {
		return nil, err
	}
	return root, nil
}

func packagedACPDescriptors(integrations []providers.ACPIntegration) ([]providers.Descriptor, error) {
	catalog, err := modelproviders.Catalog()
	if err != nil {
		return nil, fmt.Errorf("load packaged provider catalog for ACP descriptors: %w", err)
	}
	published := make(map[string]struct{}, len(catalog.Providers))
	for _, manifest := range catalog.Providers {
		published[manifest.Id] = struct{}{}
	}
	descriptors := make([]providers.Descriptor, 0, len(integrations))
	for _, integration := range integrations {
		if _, exists := published[integration.Name.String()]; exists {
			continue
		}
		descriptors = append(descriptors, acpDescriptor(integration))
	}
	return descriptors, nil
}

// PackagedACPIntegrations returns the detached data-backed ACP defaults used by
// Providers. Composition uses this exact source when materializing a new
// operator configuration so init and runtime discovery cannot drift.
func PackagedACPIntegrations() ([]providers.ACPIntegration, error) {
	return ACPIntegrationsFromRuntimeCatalog(modelproviders.RuntimeACPJSON())
}

// ACPIntegrationsFromRuntimeCatalog projects a generated package-owned
// runtime catalog into detached Providers integrations. The production
// catalog uses RuntimeACPJSON; the parameter keeps the composition boundary
// able to validate and diagnose alternate generated documents without starting
// any provider process.
func ACPIntegrationsFromRuntimeCatalog(document []byte) ([]providers.ACPIntegration, error) {
	return catalogdata.DecodeACPIntegrations(document)
}

// ExecutionRegistrations binds completed native and ACP effects to detached
// route values, preserving identity collision and capability validation.
func ExecutionRegistrations(antigravity AgyEffect, codex CodexEffect, claude ClaudeEffect, acpService ACPService, acpIntegrations []providers.ACPIntegration, externalRegistrations ProviderRegistrations) ([]ExecutionRegistration, error) {
	registrations := executionwire.BuiltInRegistrations(antigravity, codex, claude)
	for _, integration := range acpIntegrations {
		registrations = append(registrations, executionwire.NewACPRegistration(integration.Name, acpService))
	}
	for _, registration := range externalRegistrations {
		for _, existing := range registrations {
			if existing.Provider == providers.ID(registration.Manifest.ID) {
				return nil, fmt.Errorf("provider registry validation failed for %q: identity collision", registration.Manifest.ID)
			}
		}
		if err := validateExternalRegistrationCapabilities(registration); err != nil {
			return nil, err
		}
		attempt, err := externalRegistrationAttempt(registration)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, attempt)
	}
	return registrations, nil
}

func validateExternalRegistrationCapabilities(registration Registration) error {
	if registration.Integration == nil {
		return nil
	}
	manifestSupportsBypass := registration.Manifest.MaximumExecutionCapabilities.PermissionBypass
	integrationSupportsBypass := registration.Integration.MaximumCapabilities().Has(CapabilityPermissionBypass)
	if manifestSupportsBypass == integrationSupportsBypass {
		return nil
	}
	return fmt.Errorf(
		"provider registry validation failed for %q: integration maximum capability %q contradicts manifest maximum execution capability permissionBypass",
		registration.Manifest.ID,
		CapabilityPermissionBypass,
	)
}

func registrationDescriptor(manifest Manifest) providers.Descriptor {
	capabilities := []providers.Capability{}
	if manifest.MaximumExecutionCapabilities.PromptSubmission {
		capabilities = append(capabilities, providers.CapabilityPromptSubmission)
	}
	if manifest.MaximumExecutionCapabilities.ImageInput {
		capabilities = append(capabilities, providers.CapabilityImageInput)
	}
	if manifest.MaximumExecutionCapabilities.SessionResume {
		capabilities = append(capabilities, providers.CapabilitySessionResume)
	}
	if manifest.MaximumExecutionCapabilities.StructuredOutput {
		capabilities = append(capabilities, providers.CapabilityStructuredOutput)
	}
	if manifest.MaximumExecutionCapabilities.PermissionBypass {
		capabilities = append(capabilities, providers.CapabilityPermissionBypass)
	}
	return providers.Descriptor{
		ID: providers.ID(manifest.ID), Aliases: append([]string(nil), manifest.Aliases...),
		DisplayName: manifest.DisplayName.Value, Availability: providers.AvailabilitySelectable,
		Readiness: providers.ReadinessReady, Capabilities: capabilities,
	}
}

func externalRegistrationAttempt(registration Registration) (execution.Registration, error) {
	if registration.Integration == nil {
		return execution.Registration{}, fmt.Errorf("provider registry validation failed for %q: integration is required", registration.Manifest.ID)
	}
	if got := string(registration.Integration.Identity()); got != registration.Manifest.ID {
		return execution.Registration{}, fmt.Errorf("provider registry validation failed for %q: integration identity %q does not match manifest", registration.Manifest.ID, got)
	}
	return execution.Registration{
		Provider: providers.ID(registration.Manifest.ID),
		Attempt: func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			writer := &externalResponseWriter{}
			invocation := InvocationRequest{
				ID: request.AttemptID, ModelID: request.Model,
				ReasoningEffort: request.ReasoningEffort,
				SkipPermissions: request.SkipPermissions, Prompt: request.UserMessage,
			}
			if err := validateExternalInvocationCapabilities(ctx, registration.Manifest.ID, registration.Integration, invocation); err != nil {
				return providers.ExecuteResult{}, err
			}
			err := registration.Integration.Invoke(ctx, invocation, writer)
			if err != nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: err.Error()}
			}
			if writer.completion == nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: "external provider completed without a terminal result"}
			}
			if writer.completion.Err != nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: writer.completion.Err.Error()}
			}
			if writer.completion.Response == nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: "external provider completed without a response"}
			}
			result := providers.ExecuteResult{
				Content: writer.completion.Response.Content,
				Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
					"completion_evidence": "provider_response",
				}},
			}
			if writer.progress > 0 {
				result.Diagnostics.Progress = []providers.ExecuteProgress{{Phase: "updated", Detail: "external provider progress"}}
			}
			return result, nil
		},
	}, nil
}

type externalResponseWriter struct {
	completion *Completion
	progress   int
}

func (writer *externalResponseWriter) WriteEvent(context.Context, EventDraft) error {
	writer.progress++
	return nil
}

func (writer *externalResponseWriter) Close(_ context.Context, completion Completion) error {
	clone := completion
	writer.completion = &clone
	return nil
}

func effectiveACPIntegrations(packaged, configured []providers.ACPIntegration) []providers.ACPIntegration {
	values := make([]providers.ACPIntegration, len(packaged))
	for index, value := range packaged {
		values[index] = value.Clone()
	}
	for _, value := range configured {
		found := false
		for i := range values {
			if values[i].Name == value.Name {
				replacement := value.Clone()
				if replacement.Command == values[i].Command {
					// Persisted operator settings predate the generated runtime
					// projection and only carry the legacy command shape. Preserve
					// package-owned runtime facts when the saved command is still
					// the reviewed package command.
					if replacement.Aliases == nil {
						replacement.Aliases = append([]string(nil), values[i].Aliases...)
					}
					if replacement.Arguments == nil {
						replacement.Arguments = append([]string(nil), values[i].Arguments...)
					}
					if replacement.RuntimePosture == "" {
						replacement.RuntimePosture = values[i].RuntimePosture
					}
					if replacement.ImplementationProfile == "" {
						replacement.ImplementationProfile = values[i].ImplementationProfile
					}
				}
				values[i] = replacement
				found = true
				break
			}
		}
		if !found {
			values = append(values, value.Clone())
		}
	}
	return values
}

func acpDescriptor(integration providers.ACPIntegration) providers.Descriptor {
	return providers.Descriptor{ID: integration.Name, Aliases: append([]string(nil), integration.Aliases...), DisplayName: integration.Name.String(), Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessUnverified, Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilitySessionResume}}
}
