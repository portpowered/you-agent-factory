package modelhost

import (
	"errors"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// FailureClass is a provider-neutral outcome for model host operations.
type FailureClass string

const (
	FailureClassNone               FailureClass = ""
	FailureClassMissingAssets      FailureClass = "missing_assets"
	FailureClassLoadingTimeout     FailureClass = "loading_timeout"
	FailureClassProcessCrash       FailureClass = "process_crash"
	FailureClassUnsupportedRuntime FailureClass = "unsupported_runtime"
	FailureClassCancelled          FailureClass = "cancelled"
	FailureClassCapacityExhausted  FailureClass = "capacity_exhausted"
)

// ReadinessStateForFailureClass maps a failure class to managed-runtime readiness.
func ReadinessStateForFailureClass(class FailureClass) factoryapi.ManagedRuntimeReadinessState {
	switch class {
	case FailureClassMissingAssets:
		return factoryapi.ManagedRuntimeReadinessStateMISSING
	case FailureClassLoadingTimeout:
		return factoryapi.ManagedRuntimeReadinessStateLOADING
	case FailureClassProcessCrash:
		return factoryapi.ManagedRuntimeReadinessStateFAILED
	case FailureClassCancelled:
		return factoryapi.ManagedRuntimeReadinessStateFAILED
	case FailureClassCapacityExhausted:
		return factoryapi.ManagedRuntimeReadinessStateFAILED
	case FailureClassUnsupportedRuntime:
		return factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED
	default:
		return factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED
	}
}

// FailureClassForReadinessState derives the primary failure class for a readiness state.
func FailureClassForReadinessState(readiness factoryapi.ManagedRuntimeReadinessState) FailureClass {
	switch readiness {
	case factoryapi.ManagedRuntimeReadinessStateREADY:
		return FailureClassNone
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		return FailureClassMissingAssets
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		return FailureClassLoadingTimeout
	case factoryapi.ManagedRuntimeReadinessStateFAILED:
		return FailureClassProcessCrash
	case factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED:
		return FailureClassUnsupportedRuntime
	default:
		return FailureClassUnsupportedRuntime
	}
}

// FailureClassFromError classifies operational errors into provider-neutral classes.
func FailureClassFromError(err error) FailureClass {
	if err == nil {
		return FailureClassNone
	}
	if errors.Is(err, ErrCancelled) {
		return FailureClassCancelled
	}
	if errors.Is(err, ErrUnsupportedRuntime) {
		return FailureClassUnsupportedRuntime
	}
	if errors.Is(err, ErrCapacityExhausted) {
		return FailureClassCapacityExhausted
	}
	if errors.Is(err, ErrMissingAssets) {
		return FailureClassMissingAssets
	}
	if errors.Is(err, ErrLoadingTimeout) {
		return FailureClassLoadingTimeout
	}
	if errors.Is(err, ErrProcessCrash) {
		return FailureClassProcessCrash
	}
	return FailureClassUnsupportedRuntime
}
