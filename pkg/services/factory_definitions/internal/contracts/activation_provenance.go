package factorycontracts

// FactoryActivationState describes whether the authored source currently
// agrees with the immutable Factory snapshot accepted by the live runtime.
// The comparison is a read-model concern; it never changes the loaded runtime
// or persists authored data.
type FactoryActivationState string

const (
	FactoryActivationStateActive                    FactoryActivationState = "ACTIVE"
	FactoryActivationStateAuthoredChanged           FactoryActivationState = "AUTHORED_CHANGED"
	FactoryActivationStateNotActivated              FactoryActivationState = "NOT_ACTIVATED"
	FactoryActivationStateAuthoredSourceUnavailable FactoryActivationState = "AUTHORED_SOURCE_UNAVAILABLE"
)

// FactoryActivationProvenance is the server-owned identity of one immutable
// Factory snapshot accepted by a runtime. It contains no authored Factory or
// instruction bytes.
type FactoryActivationProvenance struct {
	ActivationID       string
	LoadedSourceDigest string
	State              FactoryActivationState
}

// AuthoredSourceComparison is installed by the Definitions loader on a
// loaded source. Implementations read the selected authored source and return
// only a safe comparison state.
type AuthoredSourceComparison func() (FactoryActivationState, error)
