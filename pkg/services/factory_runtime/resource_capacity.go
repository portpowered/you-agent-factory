package factory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ResourceCapacityRequest is the runtime-owned intent to change one resource
// pool. ResourceID is the canonical authored resource identifier; display
// names are never used as an implicit fallback at this boundary.
type ResourceCapacityRequest struct {
	ResourceID        string
	RequestedCapacity int
}

// ResourceCapacityOutcome describes the result of a capacity decision.
type ResourceCapacityOutcome string

const (
	ResourceCapacityOutcomeApplied ResourceCapacityOutcome = "APPLIED"
	ResourceCapacityOutcomeNoOp    ResourceCapacityOutcome = "NO_OP"
)

// ResourceCapacityResult is a detached capacity decision and its resulting
// runtime accounting. Factory is the effective authored snapshot when the
// runtime can provide one.
type ResourceCapacityResult struct {
	ResourceID        string
	ResourceName      string
	PreviousCapacity  int
	RequestedCapacity int
	EffectiveCapacity int
	InUseCount        int
	AvailableCount    int
	MinimumCapacity   int
	Outcome           ResourceCapacityOutcome
	Factory           *interfaces.FactorySnapshot
}

// ResourceCapacityErrorCode identifies a safe, stable capacity rejection.
type ResourceCapacityErrorCode string

const (
	ResourceCapacityErrorValidation    ResourceCapacityErrorCode = "VALIDATION"
	ResourceCapacityErrorNotFound      ResourceCapacityErrorCode = "NOT_FOUND"
	ResourceCapacityErrorCapacityInUse ResourceCapacityErrorCode = "RESOURCE_CAPACITY_IN_USE"
)

var (
	ErrResourceCapacityValidation = errors.New("resource capacity request is invalid")
	ErrResourceCapacityNotFound   = errors.New("resource was not found")
	ErrResourceCapacityInUse      = errors.New("resource capacity is in use")
)

var resourceCapacityErrorSentinels = map[ResourceCapacityErrorCode]error{
	ResourceCapacityErrorValidation:    ErrResourceCapacityValidation,
	ResourceCapacityErrorNotFound:      ErrResourceCapacityNotFound,
	ResourceCapacityErrorCapacityInUse: ErrResourceCapacityInUse,
}

// ResourceCapacityError is the typed, safe error returned when a capacity
// decision cannot be admitted. The counts make a capacity-in-use response
// actionable without exposing Petri tokens or runtime implementation state.
type ResourceCapacityError struct {
	Code              ResourceCapacityErrorCode
	ResourceID        string
	CurrentCapacity   int
	RequestedCapacity int
	InUseCount        int
	AvailableCount    int
	MinimumCapacity   int
	Message           string
}

func (e *ResourceCapacityError) Error() string {
	if e == nil {
		return "resource capacity error"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code == ResourceCapacityErrorCapacityInUse {
		return fmt.Sprintf("resource %q has %d units in use; requested capacity %d is below the minimum %d", e.ResourceID, e.InUseCount, e.RequestedCapacity, e.MinimumCapacity)
	}
	return string(e.Code)
}

func (e *ResourceCapacityError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	return resourceCapacityErrorSentinels[e.Code] == target
}

// ResourceCapacityService is the narrow runtime capability used by Factory
// Sessions and future orchestrators for resource capacity decisions.
type ResourceCapacityService interface {
	PreviewResourceCapacity(context.Context, ResourceCapacityRequest) (ResourceCapacityResult, error)
	SetResourceCapacity(context.Context, ResourceCapacityRequest) (ResourceCapacityResult, error)
}

// AdmittedResourceCapacityService is the internal companion used while a
// ResourceCapacityAdmission lease is held. Its methods do not reacquire the
// shared engine admission gate.
type AdmittedResourceCapacityService interface {
	PreviewResourceCapacityAdmitted(context.Context, ResourceCapacityRequest) (ResourceCapacityResult, error)
	SetResourceCapacityAdmitted(context.Context, ResourceCapacityRequest) (ResourceCapacityResult, error)
}

// ResourceCapacityAdmission serializes capacity mutation and Petri dispatch
// acquisition at one runtime boundary.
type ResourceCapacityAdmission interface {
	AcquireResourceCapacityAdmission(context.Context) (release func(), err error)
}

// ResourceCapacityLeaseRequest identifies one resource unit a dispatch needs
// for the duration of its external effect. ResourceID is the canonical
// authored identifier; display names are never used as a fallback.
type ResourceCapacityLeaseRequest struct {
	ResourceID string
}

// ResourceCapacityLease is the detached handle returned after one resource
// unit has been admitted. The runtime keeps the unit in use until Release is
// called; Release is idempotent so terminal and cancellation cleanup can share
// one defer without risking a duplicate token.
type ResourceCapacityLease struct {
	ResourceID      string
	FactoryRevision int
	release         func()
	releaseOnce     sync.Once
}

// NewResourceCapacityLease constructs a runtime-owned lease handle. The
// release callback is intentionally supplied by the runtime implementation;
// callers should normally receive leases from ResourceCapacityLeaseAdmission
// and only invoke Release.
func NewResourceCapacityLease(resourceID string, factoryRevision int, release func()) *ResourceCapacityLease {
	return &ResourceCapacityLease{
		ResourceID:      strings.TrimSpace(resourceID),
		FactoryRevision: factoryRevision,
		release:         release,
	}
}

// Release returns the leased unit to the runtime. It is safe to call more than
// once and safe to call on a nil lease.
func (l *ResourceCapacityLease) Release() {
	if l == nil {
		return
	}
	l.releaseOnce.Do(func() {
		if l.release != nil {
			l.release()
		}
	})
}

// ResourceCapacityLeaseAdmission extends the shared resource admission
// boundary with waitable unit leases. Implementations must serialize lease
// acquisition and capacity mutation with ResourceCapacityAdmission.
type ResourceCapacityLeaseAdmission interface {
	ResourceCapacityAdmission
	AcquireResourceCapacityLease(context.Context, ResourceCapacityLeaseRequest) (*ResourceCapacityLease, error)
}

// ResourceCapacityRevisionService exposes the session's effective Factory
// revision to resource-bound dispatches. The setter is monotonic so replay or
// a late duplicate cannot move a running runtime back to an older revision.
type ResourceCapacityRevisionService interface {
	CurrentFactoryRevision() int
	SetFactoryRevision(int)
}
