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

// InvocationSensitiveJSONSpan identifies the byte range in one rendered,
// public Factory snapshot string that came from a declared sensitive
// invocation argument. The range is value-free provenance: it contains no
// invocation value and is only used at the snapshot representation boundary.
// Start and End are zero-based byte offsets into the UTF-8 string, with End
// exclusive.
type InvocationSensitiveJSONSpan struct {
	JSONPointer string
	Start       int
	End         int
}

// InvocationSensitiveJSONSpanSource is an optional capability implemented by
// invocation-bound loaded Factory sources. Sources without invocation-bound
// sensitive values do not need to implement it.
type InvocationSensitiveJSONSpanSource interface {
	InvocationSensitiveJSONSpans() []InvocationSensitiveJSONSpan
}
