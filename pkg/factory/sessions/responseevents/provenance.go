package responseevents

// Delivery describes how the response event entered the Factory vocabulary.
type Delivery string

const (
	DeliveryNativeStream Delivery = "NATIVE_STREAM"
	DeliveryNativeFinal  Delivery = "NATIVE_FINAL"
	DeliverySynthesized  Delivery = "SYNTHESIZED"
	DeliveryReplay       Delivery = "REPLAY"
)

// Representation describes the shape fidelity model used for the public payload.
type Representation string

const (
	RepresentationDelta        Representation = "DELTA"
	RepresentationSnapshot     Representation = "SNAPSHOT"
	RepresentationNotification Representation = "NOTIFICATION"
)

// Fidelity describes how closely the public payload preserves provider detail.
type Fidelity string

const (
	FidelityLossless      Fidelity = "LOSSLESS"
	FidelityNormalized    Fidelity = "NORMALIZED"
	FidelityLossy         Fidelity = "LOSSY"
	FidelityFinalOnly     Fidelity = "FINAL_ONLY"
	FidelityLifecycleOnly Fidelity = "LIFECYCLE_ONLY"
)

// Provenance records provider-neutral fidelity metadata for one response event.
// It exposes diagnostic identity without promoting provider-native schemas into
// the public vocabulary.
type Provenance struct {
	Delivery           Delivery       `json:"delivery"`
	Fidelity           Fidelity       `json:"fidelity"`
	NativeEventSubtype string         `json:"nativeEventSubtype,omitempty"`
	NativeEventType    string         `json:"nativeEventType"`
	Provider           string         `json:"provider"`
	Representation     Representation `json:"representation"`
}
