package runtimebinding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

const resourceCapacitySetOperation = "resource.capacity.set"

type liveChangeCapacityApplication struct {
	runtime  factory.ResourceCapacityService
	admitted factory.AdmittedResourceCapacityService
}

// NewLiveChangeApplication binds resource-specific live-change policy to the
// runtime capability without exposing the engine or Petri marking to Sessions.
func NewLiveChangeApplication(runtime factory.Service) factorysessions.LiveChangeApplication {
	capacity, _ := runtime.(factory.ResourceCapacityService)
	if capacity == nil {
		return nil
	}
	admitted, _ := runtime.(factory.AdmittedResourceCapacityService)
	return &liveChangeCapacityApplication{runtime: capacity, admitted: admitted}
}

// NewLiveChangeAdmission binds the runtime's shared admission gate to the
// Factory Session coordinator. A nil result is intentional for runtimes that
// do not expose mutable resource capacity.
func NewLiveChangeAdmission(runtime factory.Service) factorysessions.LiveChangeAdmission {
	admission, _ := runtime.(factory.ResourceCapacityAdmission)
	if admission == nil {
		return nil
	}
	return liveChangeCapacityAdmission{admission: admission}
}

type liveChangeCapacityAdmission struct {
	admission factory.ResourceCapacityAdmission
}

func (a liveChangeCapacityAdmission) AcquireLiveChange(ctx context.Context, _ string) (func(), error) {
	return a.admission.AcquireResourceCapacityAdmission(ctx)
}

func (a *liveChangeCapacityApplication) PreflightLiveChange(ctx context.Context, request factorysessions.LiveChangeApplicationRequest) (factorysessions.LiveChangePreflightResult, error) {
	capacityRequest, err := capacityRequestFromLiveChange(request.Request)
	if err != nil {
		return factorysessions.LiveChangePreflightResult{}, err
	}
	result, err := a.preview(ctx, capacityRequest)
	if err != nil {
		return factorysessions.LiveChangePreflightResult{}, capacityError(err)
	}
	return factorysessions.LiveChangePreflightResult{
		Admissible:       true,
		NoOp:             result.Outcome == factory.ResourceCapacityOutcomeNoOp,
		Factory:          result.Factory,
		ResourceCapacity: &result,
	}, nil
}

func (a *liveChangeCapacityApplication) ApplyLiveChange(ctx context.Context, request factorysessions.LiveChangeApplicationRequest) (factorysessions.LiveChangeApplicationResult, error) {
	capacityRequest, err := capacityRequestFromLiveChange(request.Request)
	if err != nil {
		return factorysessions.LiveChangeApplicationResult{}, err
	}
	result, err := a.apply(ctx, capacityRequest)
	if err != nil {
		return factorysessions.LiveChangeApplicationResult{}, capacityError(err)
	}
	return factorysessions.LiveChangeApplicationResult{
		Factory:          result.Factory,
		ResourceCapacity: &result,
	}, nil
}

func (a *liveChangeCapacityApplication) preview(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if a == nil || a.runtime == nil {
		return factory.ResourceCapacityResult{}, &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "resource capacity application is unavailable",
		}
	}
	if a.admitted != nil {
		return a.admitted.PreviewResourceCapacityAdmitted(ctx, request)
	}
	return a.runtime.PreviewResourceCapacity(ctx, request)
}

func (a *liveChangeCapacityApplication) apply(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if a == nil || a.runtime == nil {
		return factory.ResourceCapacityResult{}, &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "resource capacity application is unavailable",
		}
	}
	if a.admitted != nil {
		return a.admitted.SetResourceCapacityAdmitted(ctx, request)
	}
	return a.runtime.SetResourceCapacity(ctx, request)
}

func capacityRequestFromLiveChange(request factorysessions.LiveChangeRequest) (factory.ResourceCapacityRequest, error) {
	if request.Operation != resourceCapacitySetOperation {
		return factory.ResourceCapacityRequest{}, &factorysessions.LiveChangeError{
			Code:  factorysessions.LiveChangeErrorInvalidRequest,
			Field: "operation", Message: "unsupported live change operation",
		}
	}
	if strings.TrimSpace(request.TargetID) == "" {
		return factory.ResourceCapacityRequest{}, &factorysessions.LiveChangeError{
			Code:  factorysessions.LiveChangeErrorInvalidRequest,
			Field: "targetId", Message: "resource id is required",
		}
	}
	if string(request.RequestedValue) == "" || string(request.RequestedValue) == "null" {
		return factory.ResourceCapacityRequest{}, &factorysessions.LiveChangeError{
			Code:  factorysessions.LiveChangeErrorInvalidRequest,
			Field: "requestedValue", Message: "capacity must be an integer",
		}
	}
	var capacity int
	if err := json.Unmarshal(request.RequestedValue, &capacity); err != nil {
		return factory.ResourceCapacityRequest{}, &factorysessions.LiveChangeError{
			Code:  factorysessions.LiveChangeErrorInvalidRequest,
			Field: "requestedValue", Message: "capacity must be an integer", Cause: err,
		}
	}
	return factory.ResourceCapacityRequest{ResourceID: request.TargetID, RequestedCapacity: capacity}, nil
}

func capacityError(err error) error {
	if typed := new(factorysessions.LiveChangeError); errors.As(err, &typed) {
		return typed
	}
	var capacityErr *factory.ResourceCapacityError
	if !errors.As(err, &capacityErr) {
		return err
	}
	result := &factory.ResourceCapacityResult{
		ResourceID:        capacityErr.ResourceID,
		PreviousCapacity:  capacityErr.CurrentCapacity,
		RequestedCapacity: capacityErr.RequestedCapacity,
		EffectiveCapacity: capacityErr.CurrentCapacity,
		InUseCount:        capacityErr.InUseCount,
		AvailableCount:    capacityErr.AvailableCount,
		MinimumCapacity:   capacityErr.MinimumCapacity,
	}
	switch capacityErr.Code {
	case factory.ResourceCapacityErrorNotFound:
		return &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorTargetNotFound,
			Message: "resource was not found", ResourceCapacity: result,
		}
	case factory.ResourceCapacityErrorValidation:
		return &factorysessions.LiveChangeError{
			Code:  factorysessions.LiveChangeErrorInvalidRequest,
			Field: "requestedValue", Message: "capacity must be a non-negative integer", ResourceCapacity: result,
		}
	case factory.ResourceCapacityErrorCapacityInUse:
		return &factorysessions.LiveChangeError{
			Code:             factorysessions.LiveChangeErrorCapacityInUse,
			Message:          fmt.Sprintf("resource %q capacity cannot be reduced below %d units in use", capacityErr.ResourceID, capacityErr.MinimumCapacity),
			ResourceCapacity: result,
		}
	default:
		return err
	}
}
