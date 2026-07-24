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
type ListModelsRequest struct{}

// GetModelRequest is the plain catalog get request. Peers identify a model by
// Name without importing models/internal/catalog.
type GetModelRequest struct {
	Name string
}

// ValidateGetModelRequest checks the plain get-model request. Empty names fail
// closed as ErrNotFound without touching nested catalog implementation.
func ValidateGetModelRequest(request GetModelRequest) error {
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

// Detail is the Models-owned catalog get result value.
type Detail struct {
	Summary
	Capabilities []Capability
	Diagnostics  map[string]string
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
