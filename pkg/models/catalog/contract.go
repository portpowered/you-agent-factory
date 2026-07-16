// Package catalog owns provider-neutral model discovery contracts.
package catalog

import (
	"errors"

	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
)

var ErrUnsupportedOperation = errors.New("model invocation operation is not supported")

type Status string

const (
	StatusReady       Status = "READY"
	StatusUnavailable Status = "UNAVAILABLE"
)

type LoadState string

const (
	LoadStateUnloaded      LoadState = "UNLOADED"
	LoadStateNotApplicable LoadState = "NOT_APPLICABLE"
)

type ResourceSummary struct {
	Name       string
	Type       string
	Capacity   int
	Model      *string
	Backend    *string
	LoadPolicy *string
	Provider   *string
}

type Capability struct {
	Worker           string
	ProviderLocality managedruntime.Locality
	ModelProvider    *string
	Operations       []managedruntime.Operation
	ResourceNames    []string
}

type Summary struct {
	Name             string
	ProviderLocality managedruntime.Locality
	Status           Status
	LoadState        LoadState
	Operations       []managedruntime.Operation
	Modalities       []string
	Resources        []ResourceSummary
	ManagedRuntime   managedruntime.Runtime
}

type Detail struct {
	Summary
	Capabilities []Capability
	Diagnostics  map[string]string
}

type Entry struct {
	Summary Summary
	Detail  Detail
}

type List struct {
	Results []Summary
}
