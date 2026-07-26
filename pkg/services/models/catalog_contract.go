package models

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupportedOperation classifies a catalog or invocation path that Models
// does not support for the requested peer operation.
var ErrUnsupportedOperation = errors.New("model invocation operation is not supported")

// ErrUnavailable classifies catalog list/get when Models cannot serve the
// catalog (for example missing runtime scope). It is distinct from ErrNotFound
// and ErrUnsupportedOperation so peers can branch on typed outcomes.
var ErrUnavailable = errors.New("models catalog is unavailable")

// Status names catalog readiness for one model summary/detail.
type Status string

const (
	StatusReady       Status = "READY"
	StatusUnavailable Status = "UNAVAILABLE"
)

// LoadState names whether a model runtime is loaded for catalog projection.
type LoadState string

const (
	LoadStateUnloaded      LoadState = "UNLOADED"
	LoadStateNotApplicable LoadState = "NOT_APPLICABLE"
)

// ListModelsRequest is the plain catalog list request vocabulary. List currently
// takes no filters; peers use this type without nested catalog assemblers.
type ListModelsRequest struct {
	Scope RuntimeScopeRef
}

// GetModelRequest is the plain catalog get request. Peers identify a model by
// Name and may require one configured operation without importing
// models/internal/catalog.
type GetModelRequest struct {
	Scope     RuntimeScopeRef
	Name      string
	Operation string
}

// Validate checks request fields whose validity does not depend on private
// scope ownership or catalog state.
func (request GetModelRequest) Validate() error {
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// ValidateGetModelRequest checks the plain get-model request. Empty names fail
// closed as ErrNotFound without touching nested catalog implementation.
//
// Deprecated: call GetModelRequest.Validate.
func ValidateGetModelRequest(request GetModelRequest) error {
	return request.Validate()
}

// GetModelReadinessRequest identifies the scoped model whose current readiness
// facts a peer needs.
type GetModelReadinessRequest struct {
	Scope     RuntimeScopeRef
	Name      string
	Operation string
}

// Validate checks request fields whose validity does not depend on private
// scope ownership or catalog state.
func (request GetModelReadinessRequest) Validate() error {
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// ResourceSummary describes one resource attached to a catalog model summary.
type ResourceSummary struct {
	Name       string
	Type       string
	Capacity   int
	Model      *string
	Backend    *string
	LoadPolicy *string
	Provider   *string
}

// Capability describes one worker capability projected onto a catalog detail.
type Capability struct {
	Worker           string
	ProviderLocality Locality
	ModelProvider    *string
	Operations       []Operation
	ResourceNames    []string
}

// SourceMetadata describes a configured model source without exposing source
// resolvers, download clients, cache records, or filesystem locations.
type SourceMetadata struct {
	Provider  string
	Reference string
	Revision  string
}

// Summary is the Models-owned catalog list/get summary value, including
// Status/LoadState and ManagedRuntime readiness vocabulary.
type Summary struct {
	Name             string
	ProviderLocality Locality
	Status           Status
	LoadState        LoadState
	Operations       []Operation
	Modalities       []string
	Resources        []ResourceSummary
	ManagedRuntime   Runtime
}

// Clone returns a detached catalog summary.
func (summary Summary) Clone() Summary {
	summary.Operations = cloneOperations(summary.Operations)
	summary.Modalities = append([]string(nil), summary.Modalities...)
	summary.Resources = cloneResourceSummaries(summary.Resources)
	summary.ManagedRuntime = summary.ManagedRuntime.Clone()
	return summary
}

// Detail is the Models-owned catalog get result value.
type Detail struct {
	Summary
	Capabilities []Capability
	Sources      []SourceMetadata
	Diagnostics  map[string]string
}

// Clone returns a detached catalog detail.
func (detail Detail) Clone() Detail {
	detail.Summary = detail.Summary.Clone()
	capabilities := detail.Capabilities
	detail.Capabilities = make([]Capability, len(capabilities))
	for i, capability := range capabilities {
		detail.Capabilities[i] = capability
		detail.Capabilities[i].Operations = cloneOperations(capability.Operations)
		detail.Capabilities[i].ResourceNames = append([]string(nil), capability.ResourceNames...)
	}
	detail.Sources = append([]SourceMetadata(nil), detail.Sources...)
	detail.Diagnostics = cloneStringMap(detail.Diagnostics)
	return detail
}

func cloneResourceSummaries(resources []ResourceSummary) []ResourceSummary {
	cloned := make([]ResourceSummary, len(resources))
	for i, resource := range resources {
		cloned[i] = resource
		cloned[i].Model = cloneStringPointer(resource.Model)
		cloned[i].Backend = cloneStringPointer(resource.Backend)
		cloned[i].LoadPolicy = cloneStringPointer(resource.LoadPolicy)
		cloned[i].Provider = cloneStringPointer(resource.Provider)
	}
	return cloned
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// Entry pairs summary and detail for one catalog model.
type Entry struct {
	Summary Summary
	Detail  Detail
}

// List is the Models-owned catalog list result.
type List struct {
	Results []Summary
}

// ListModelsResult is the detached result of one scoped catalog list.
type ListModelsResult struct {
	Models []Summary
}

// GetModelResult is the detached result of one scoped catalog lookup.
type GetModelResult struct {
	Model Detail
}

// GetModelReadinessResult is the detached result of one scoped readiness
// lookup.
type GetModelReadinessResult struct {
	ModelName string
	Readiness Runtime
}
