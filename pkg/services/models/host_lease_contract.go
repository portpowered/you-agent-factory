package models

import (
	"fmt"
	"strings"
)

// InspectRuntimeRequest is the plain host readiness-inspect request. Peers
// identify a model by Name without importing models/internal/host.
type InspectRuntimeRequest struct {
	Name string
}

// ValidateInspectRuntimeRequest checks the plain readiness-inspect request.
// Empty names fail closed as ErrNotFound without touching nested host packages.
func ValidateInspectRuntimeRequest(request InspectRuntimeRequest) error {
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// AcquireLeaseRequest is the plain host/lease acquire request. Peers identify a
// model and optional holder without importing nested host supervisor or
// lease-manager implementation types.
type AcquireLeaseRequest struct {
	ModelName string
	Holder    string
}

// ValidateAcquireLeaseRequest checks the plain lease-acquire request. Empty
// model names fail closed as ErrNotFound without touching nested host packages.
func ValidateAcquireLeaseRequest(request AcquireLeaseRequest) error {
	if strings.TrimSpace(request.ModelName) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// ReleaseLeaseRequest is the plain host/lease release request. Peers identify a
// lease by LeaseID without importing nested host supervisor types.
type ReleaseLeaseRequest struct {
	LeaseID string
}

// ValidateReleaseLeaseRequest checks the plain lease-release request. Empty
// lease identifiers fail closed as ErrHostLeaseNotFound.
func ValidateReleaseLeaseRequest(request ReleaseLeaseRequest) error {
	if strings.TrimSpace(request.LeaseID) == "" {
		return fmt.Errorf("%w: empty lease id", ErrHostLeaseNotFound)
	}
	return nil
}

// HostLeaseOptions configures lease acquisition on the Models root host/lease
// slice. Peers supply Holder without nested lease-manager types.
type HostLeaseOptions struct {
	Holder string
}

// HostLease grants disposable call capacity for one loaded managed runtime.
// Peers consume this Models-owned vocabulary without importing
// models/internal/host supervisor, process, or lease-manager types.
type HostLease struct {
	ID       string
	Identity HostIdentity
	Endpoint string
	Holder   string
}
