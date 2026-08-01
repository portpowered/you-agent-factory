package factorydefinitions

import "github.com/portpowered/infinite-you/pkg/services/work"

const ArgumentErrorCodeInvalidInterpolation = work.ArgumentErrorCodeInvalidInterpolation

// FileReader is the small, effect-free callback shape used by invocation
// policy. The owner wire package aliases it for construction boundaries; the
// root keeps this value type available to existing worker adapters without
// publishing an invocation service interface.
type FileReader func(string) ([]byte, error)
