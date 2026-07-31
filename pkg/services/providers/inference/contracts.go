// Package inference contains the compatibility contract for externally
// supplied provider registrations. Runtime peers should use providers.Service;
// this protocol is restricted to the process-edge extension seam.
package inference

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type Provider = workers.Provider
type Identity string
type Capability string

const CapabilityPromptSubmission Capability = "prompt_submission"

type CapabilitySet struct{ values []Capability }

func NewCapabilitySet(values ...Capability) CapabilitySet {
	return CapabilitySet{values: append([]Capability(nil), values...)}
}
func (set CapabilitySet) Values() []Capability      { return slices.Clone(set.values) }
func (set CapabilitySet) Has(value Capability) bool { return slices.Contains(set.values, value) }

type Discovery struct{}

type ProviderSession struct {
	Provider string
	Kind     string
	ID       string
}

type InvocationRequest struct {
	ID              string
	ModelID         string
	ReasoningEffort string
	Prompt          string
}

type Response struct {
	Content         string
	ProviderSession *ProviderSession
}

type Completion struct {
	Response *Response
	Err      error
}

func SuccessfulCompletion(response Response) Completion {
	clone := response
	return Completion{Response: &clone}
}

type EventDraft struct{}

type ResponseWriter interface {
	WriteEvent(context.Context, EventDraft) error
	Close(context.Context, Completion) error
}

type Integration interface {
	Identity() Identity
	MaximumCapabilities() CapabilitySet
	Discover(context.Context) (Discovery, error)
	Capabilities(context.Context, InvocationRequest) (CapabilitySet, error)
	Invoke(context.Context, InvocationRequest, ResponseWriter) error
}

type ImplementationAvailability string
type TechnicalSupportLevel string

const (
	ImplementationExternallySupplied ImplementationAvailability = "externally-supplied"
	SupportProduction                TechnicalSupportLevel      = "production"
)

type LocalizedValue struct {
	ID      *string            `json:"id,omitempty"`
	Locales *[]string          `json:"locales,omitempty"`
	Type    string             `json:"type"`
	Value   string             `json:"value"`
	Values  *map[string]string `json:"values,omitempty"`
}
type Deprecation struct {
	DeprecatedSince string `json:"deprecatedSince"`
}
type DiscoveryPrerequisites struct {
	ConfigurationKeys []string `json:"configurationKeys"`
	EndpointKinds     []string `json:"endpointKinds"`
	ExecutableNames   []string `json:"executableNames"`
}
type DocumentationLink struct{ Kind, URL string }
type ExecutionCapabilities struct {
	ImageInput, PromptSubmission, SessionResume, StructuredOutput, ToolExecution, WorkingDirectory, Worktree bool
}
type ResponseFidelityCapabilities struct {
	FileChanges, MessageDeltas, MessageSnapshots, NativeStreaming, Plans, ProviderReconnect, ReasoningSummaries, StableItemIDs, ToolLifecycle, ToolOutputDeltas, Usage bool
}
type Manifest struct {
	Aliases                             []string                     `json:"aliases"`
	Deprecation                         *Deprecation                 `json:"deprecation,omitempty"`
	Description                         LocalizedValue               `json:"description"`
	Discovery                           DiscoveryPrerequisites       `json:"discovery"`
	DisplayName                         LocalizedValue               `json:"displayName"`
	Documentation                       []DocumentationLink          `json:"documentation"`
	ID                                  string                       `json:"id"`
	ImplementationAvailability          ImplementationAvailability   `json:"implementationAvailability"`
	MaximumExecutionCapabilities        ExecutionCapabilities        `json:"maximumExecutionCapabilities"`
	MaximumResponseFidelityCapabilities ResponseFidelityCapabilities `json:"maximumResponseFidelityCapabilities"`
	TechnicalSupportLevel               TechnicalSupportLevel        `json:"technicalSupportLevel"`
}
type Registration struct {
	Manifest    Manifest
	Integration Integration
}
type ProviderRegistrations []Registration

type ProgressingIntegrationStats struct {
	DiscoverCalls, CapabilityCalls, InvokeCalls, ProgressWrites, TerminalCloses int
}

type ProgressingIntegration struct {
	identity Identity
	content  string
	mu       sync.Mutex
	stats    ProgressingIntegrationStats
}

func ProgressingExternalIntegration(identity Identity, content string) *ProgressingIntegration {
	return &ProgressingIntegration{identity: identity, content: content}
}
func (integration *ProgressingIntegration) Identity() Identity { return integration.identity }
func (*ProgressingIntegration) MaximumCapabilities() CapabilitySet {
	return NewCapabilitySet(CapabilityPromptSubmission)
}
func (integration *ProgressingIntegration) Discover(context.Context) (Discovery, error) {
	integration.mu.Lock()
	defer integration.mu.Unlock()
	integration.stats.DiscoverCalls++
	return Discovery{}, nil
}
func (integration *ProgressingIntegration) Capabilities(context.Context, InvocationRequest) (CapabilitySet, error) {
	integration.mu.Lock()
	defer integration.mu.Unlock()
	integration.stats.CapabilityCalls++
	return integration.MaximumCapabilities(), nil
}
func (integration *ProgressingIntegration) Invoke(ctx context.Context, _ InvocationRequest, writer ResponseWriter) error {
	integration.mu.Lock()
	integration.stats.InvokeCalls++
	integration.mu.Unlock()
	if err := writer.WriteEvent(ctx, EventDraft{}); err != nil {
		return err
	}
	integration.mu.Lock()
	integration.stats.ProgressWrites++
	integration.mu.Unlock()
	if err := writer.Close(ctx, SuccessfulCompletion(Response{Content: integration.content})); err != nil {
		return err
	}
	integration.mu.Lock()
	integration.stats.TerminalCloses++
	integration.mu.Unlock()
	return nil
}
func (integration *ProgressingIntegration) Stats() ProgressingIntegrationStats {
	integration.mu.Lock()
	defer integration.mu.Unlock()
	return integration.stats
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.Aliases = slices.Clone(manifest.Aliases)
	manifest.Documentation = slices.Clone(manifest.Documentation)
	if manifest.Description.Values != nil {
		cloned := maps.Clone(*manifest.Description.Values)
		manifest.Description.Values = &cloned
	}
	return manifest
}
