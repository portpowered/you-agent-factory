package responseevents

import shared "github.com/portpowered/infinite-you/pkg/interfaces/responseevents"

type Delivery = shared.Delivery
type Representation = shared.Representation
type Fidelity = shared.Fidelity
type Provenance = shared.Provenance

const (
	DeliveryNativeStream       = shared.DeliveryNativeStream
	DeliveryNativeFinal        = shared.DeliveryNativeFinal
	DeliverySynthesized        = shared.DeliverySynthesized
	DeliveryReplay             = shared.DeliveryReplay
	RepresentationDelta        = shared.RepresentationDelta
	RepresentationSnapshot     = shared.RepresentationSnapshot
	RepresentationNotification = shared.RepresentationNotification
	FidelityLossless           = shared.FidelityLossless
	FidelityNormalized         = shared.FidelityNormalized
	FidelityLossy              = shared.FidelityLossy
	FidelityFinalOnly          = shared.FidelityFinalOnly
	FidelityLifecycleOnly      = shared.FidelityLifecycleOnly
)
