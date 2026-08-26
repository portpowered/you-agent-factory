// Package localai contains the private codecs for the pinned LocalAI backend
// protocol. LocalAI-native request details stay in this package; callers use
// the provider-neutral Models contracts at its boundary.
package localai

import (
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

const (
	// PinnedLocalAICommit identifies the LocalAI source that owns the protocol
	// consumed by the pinned backend artifacts.
	PinnedLocalAICommit = "b224c96db6f4b87306a33a808650bfce63b12588"
	// PinnedProtocolRevision identifies backend/backend.proto for that source.
	PinnedProtocolRevision = "ad62c6df07ae1169eb14411a565a689cd996b19c"
	// PinnedProtocolPath identifies the immutable protocol source file.
	PinnedProtocolPath = "backend/backend.proto"

	predictPromptField = "Prompt"
	predictImageField  = "Images"
	predictAudioField  = "Audios"
	predictVideoField  = "Videos"
)

var (
	// ErrInvalidOmniCapability identifies an incomplete codec capability fact.
	ErrInvalidOmniCapability = errors.New("LocalAI OMNI capability is invalid")
)

// OmniModalityCapability is the private record of one OMNI input slot in the
// pinned LocalAI PredictOptions protocol. Supported is a protocol fact, not a
// model-quality promise: it records whether the pinned request can represent
// and accept this modality.
type OmniModalityCapability struct {
	Slot          string
	Modality      models.Modality
	ProtocolField string
	Supported     bool
	Repeatable    bool
	MediaTypes    []string
}

// OmniConformanceRequest is the private request given to the deterministic
// pinned-protocol conformance fixture. It describes the exact field and media
// shape that the pinned PredictOptions contract must accept; it does not
// contain a live endpoint, model artifact, or generated LocalAI type.
type OmniConformanceRequest struct {
	ProtocolVersion  string
	ProtocolRevision string
	ProtocolPath     string
	LocalAICommit    string
	Slot             string
	Modality         models.Modality
	ProtocolField    string
	MediaType        string
	Repeatable       bool
}

// OmniProtocolConformanceProbe records whether one pinned protocol input
// shape is representable and accepted. The production implementation is an
// in-memory pinned-protocol fixture; tests can replace it with a recording
// fixture to prove capability narrowing without contacting a backend.
type OmniProtocolConformanceProbe interface {
	Accepts(OmniConformanceRequest) bool
}

// Clone returns a detached capability fact.
func (capability OmniModalityCapability) Clone() OmniModalityCapability {
	capability.MediaTypes = append([]string(nil), capability.MediaTypes...)
	return capability
}

// OmniCapability is the private capability result for the pinned OMNI
// protocol. Audio and video may be disabled by a conformance result without
// changing the provider-neutral public Models contract.
type OmniCapability struct {
	ProtocolVersion  string
	ProtocolRevision string
	ProtocolPath     string
	LocalAICommit    string
	Inputs           []OmniModalityCapability
}

// Clone returns a detached capability result.
func (capability OmniCapability) Clone() OmniCapability {
	cloned := capability
	cloned.Inputs = make([]OmniModalityCapability, len(capability.Inputs))
	for index, input := range capability.Inputs {
		cloned.Inputs[index] = input.Clone()
	}
	return cloned
}

// PinnedOmniCapability returns the capability facts recorded by the pinned
// protocol conformance fixture. The result, rather than a static media flag,
// controls the effective provider-neutral operation used by the codec.
func PinnedOmniCapability() OmniCapability {
	return CapabilityFromPinnedOmniProbe(ProbePinnedOmniProtocol())
}

func pinnedOmniProtocolShape() OmniCapability {
	return OmniCapability{
		ProtocolVersion:  modelseffects.PinnedHostProtocolVersion,
		ProtocolRevision: PinnedProtocolRevision,
		ProtocolPath:     PinnedProtocolPath,
		LocalAICommit:    PinnedLocalAICommit,
		Inputs: []OmniModalityCapability{
			{Slot: "prompt", Modality: models.ModalityText, ProtocolField: predictPromptField, Supported: true, MediaTypes: []string{"text/plain"}},
			{Slot: "image", Modality: models.ModalityImage, ProtocolField: predictImageField, Supported: true, Repeatable: true, MediaTypes: []string{"image/*"}},
			{Slot: "audio", Modality: models.ModalityAudio, ProtocolField: predictAudioField, Supported: true, MediaTypes: []string{"audio/*"}},
			{Slot: "video", Modality: models.ModalityVideo, ProtocolField: predictVideoField, Supported: true, MediaTypes: []string{"video/*"}},
			{Slot: "parameters", Modality: models.ModalityJSON, ProtocolField: "Metadata", Supported: true, MediaTypes: []string{"application/json"}},
		},
	}
}

// Supported reports whether a named slot is part of the recorded capability.
func (capability OmniCapability) Supported(slot string) bool {
	for _, input := range capability.Inputs {
		if strings.EqualFold(strings.TrimSpace(input.Slot), strings.TrimSpace(slot)) {
			return input.Supported
		}
	}
	return false
}

// ModalitySupported reports whether the recorded protocol can accept a
// modality for the corresponding OMNI slot.
func (capability OmniCapability) ModalitySupported(modality models.Modality) bool {
	for _, input := range capability.Inputs {
		if input.Modality == modality {
			return input.Supported
		}
	}
	return false
}

// Operation returns the effective provider-neutral OMNI operation for this
// capability result. Unsupported optional slots are removed so later generic
// invocation validation uses the same decision as the codec.
func (capability OmniCapability) Operation() models.Operation {
	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract(models.OperationOMNI)
	if !ok {
		return models.Operation{}
	}
	filtered := make([]models.OperationSlot, 0, len(operation.Inputs))
	for _, slot := range operation.Inputs {
		if capability.Supported(slot.Name) {
			filtered = append(filtered, slot)
		}
	}
	operation.Inputs = filtered
	return operation
}

// Validate checks that a capability result describes the required pinned
// protocol identity and every input has a stable provider-neutral mapping.
func (capability OmniCapability) Validate() error {
	if strings.TrimSpace(capability.ProtocolVersion) != modelseffects.PinnedHostProtocolVersion {
		return fmt.Errorf("%w: protocol version %q is not pinned", ErrInvalidOmniCapability, capability.ProtocolVersion)
	}
	if strings.TrimSpace(capability.ProtocolRevision) != PinnedProtocolRevision {
		return fmt.Errorf("%w: protocol revision %q is not pinned", ErrInvalidOmniCapability, capability.ProtocolRevision)
	}
	if strings.TrimSpace(capability.ProtocolPath) != PinnedProtocolPath ||
		strings.TrimSpace(capability.LocalAICommit) != PinnedLocalAICommit {
		return fmt.Errorf("%w: protocol source is not pinned", ErrInvalidOmniCapability)
	}
	seen := make(map[string]struct{}, len(capability.Inputs))
	for _, input := range capability.Inputs {
		slot := strings.TrimSpace(input.Slot)
		if slot == "" || input.Modality == "" || strings.TrimSpace(input.ProtocolField) == "" {
			return fmt.Errorf("%w: slot, modality, and protocol field are required", ErrInvalidOmniCapability)
		}
		if _, exists := seen[slot]; exists {
			return fmt.Errorf("%w: slot %q is repeated", ErrInvalidOmniCapability, slot)
		}
		seen[slot] = struct{}{}
	}
	for _, required := range []string{"prompt", "image", "parameters"} {
		if !capability.Supported(required) {
			return fmt.Errorf("%w: required slot %q is not supported", ErrInvalidOmniCapability, required)
		}
	}
	return nil
}

// OmniProtocolProbe is the stable, serializable evidence produced from the
// pinned protocol contract and its conformance fixture. It deliberately
// contains no live endpoint or backend artifact facts.
type OmniProtocolProbe struct {
	ProtocolVersion  string
	ProtocolRevision string
	ProtocolPath     string
	LocalAICommit    string
	PromptField      string
	ImageField       string
	AudioField       string
	VideoField       string
	AudioSupported   bool
	VideoSupported   bool
}

// ProbePinnedOmniProtocol records the pinned protocol's media capability by
// asking a deterministic conformance fixture whether the exact audio and
// video request shapes are representable and accepted. Keeping this probe
// independent of installed artifacts makes capability validation deterministic
// and keeps it inside the Models-owned LocalAI boundary.
func ProbePinnedOmniProtocol(probes ...OmniProtocolConformanceProbe) OmniProtocolProbe {
	capability := pinnedOmniProtocolShape()
	probe := OmniProtocolConformanceProbe(pinnedProtocolConformanceFixture{})
	if len(probes) > 0 && !isNilConformanceProbe(probes[0]) {
		probe = probes[0]
	}
	return OmniProtocolProbe{
		ProtocolVersion:  capability.ProtocolVersion,
		ProtocolRevision: capability.ProtocolRevision,
		ProtocolPath:     capability.ProtocolPath,
		LocalAICommit:    capability.LocalAICommit,
		PromptField:      protocolField(capability, models.ModalityText),
		ImageField:       protocolField(capability, models.ModalityImage),
		AudioField:       protocolField(capability, models.ModalityAudio),
		VideoField:       protocolField(capability, models.ModalityVideo),
		AudioSupported:   probe.Accepts(conformanceRequest(capability, models.ModalityAudio)),
		VideoSupported:   probe.Accepts(conformanceRequest(capability, models.ModalityVideo)),
	}
}

// CapabilityFromPinnedOmniProbe applies conformance evidence to the pinned
// protocol shape. Required text/image/parameter slots remain validated by
// OmniCapability.Validate; optional audio/video slots follow the measured
// acceptance result.
func CapabilityFromPinnedOmniProbe(probe OmniProtocolProbe) OmniCapability {
	capability := pinnedOmniProtocolShape()
	for index := range capability.Inputs {
		switch capability.Inputs[index].Modality {
		case models.ModalityAudio:
			capability.Inputs[index].Supported = probe.AudioSupported
		case models.ModalityVideo:
			capability.Inputs[index].Supported = probe.VideoSupported
		}
	}
	return capability.Clone()
}

func conformanceRequest(capability OmniCapability, modality models.Modality) OmniConformanceRequest {
	for _, input := range capability.Inputs {
		if input.Modality != modality {
			continue
		}
		mediaType := ""
		if len(input.MediaTypes) > 0 {
			mediaType = input.MediaTypes[0]
		}
		return OmniConformanceRequest{
			ProtocolVersion:  capability.ProtocolVersion,
			ProtocolRevision: capability.ProtocolRevision,
			ProtocolPath:     capability.ProtocolPath,
			LocalAICommit:    capability.LocalAICommit,
			Slot:             input.Slot,
			Modality:         input.Modality,
			ProtocolField:    input.ProtocolField,
			MediaType:        mediaType,
			Repeatable:       input.Repeatable,
		}
	}
	return OmniConformanceRequest{
		ProtocolVersion:  capability.ProtocolVersion,
		ProtocolRevision: capability.ProtocolRevision,
		ProtocolPath:     capability.ProtocolPath,
		LocalAICommit:    capability.LocalAICommit,
		Modality:         modality,
	}
}

type pinnedProtocolConformanceFixture struct{}

func (pinnedProtocolConformanceFixture) Accepts(request OmniConformanceRequest) bool {
	if request.ProtocolVersion != modelseffects.PinnedHostProtocolVersion ||
		request.ProtocolRevision != PinnedProtocolRevision ||
		request.ProtocolPath != PinnedProtocolPath ||
		request.LocalAICommit != PinnedLocalAICommit {
		return false
	}
	switch request.Modality {
	case models.ModalityAudio:
		return request.Slot == "audio" && request.ProtocolField == predictAudioField &&
			request.MediaType == "audio/*" && !request.Repeatable
	case models.ModalityVideo:
		return request.Slot == "video" && request.ProtocolField == predictVideoField &&
			request.MediaType == "video/*" && !request.Repeatable
	default:
		return false
	}
}

func isNilConformanceProbe(probe OmniProtocolConformanceProbe) bool {
	return probe == nil
}

func protocolField(capability OmniCapability, modality models.Modality) string {
	for _, input := range capability.Inputs {
		if input.Modality == modality {
			return input.ProtocolField
		}
	}
	return ""
}
