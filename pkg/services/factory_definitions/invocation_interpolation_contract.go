package factorydefinitions

import "github.com/portpowered/infinite-you/pkg/services/work"

const ArgumentErrorCodeInvalidInterpolation = work.ArgumentErrorCodeInvalidInterpolation

// FileReader resolves FILE_CONTENTS arguments at an explicit IO boundary.
type FileReader func(string) ([]byte, error)

// InvocationInterpolationService owns runtime interpolation of normalized
// invocation arguments into effective Factory Definition values.
type InvocationInterpolationService interface {
	ValidateInvocationInterpolation(*FactoryConfig, *work.InvocationArguments, FileReader) error
	InterpolateWorkerConfig(FactoryWorkerConfig, *work.InvocationArguments, FileReader) (FactoryWorkerConfig, error)
	InterpolateWorkstationConfig(FactoryWorkstationConfig, *work.InvocationArguments, FileReader) (FactoryWorkstationConfig, error)
}
