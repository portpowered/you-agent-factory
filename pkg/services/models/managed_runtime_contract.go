package models

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNotFound reports that a requested model is absent from configuration.
	ErrNotFound = errors.New("model not found")
	// ErrMissing reports that infer/local invocation requires uninstalled
	// runtime assets. Distinct from ErrLoading, ErrFailed, and ErrUnsupported.
	ErrMissing = errors.New("managed runtime missing")
	// ErrLoading reports that infer/local invocation must wait for runtime
	// preparation. Distinct from ErrMissing, ErrFailed, and ErrUnsupported.
	ErrLoading = errors.New("managed runtime loading")
	// ErrFailed reports that infer/local invocation is blocked by a failed
	// runtime. Distinct from ErrMissing, ErrLoading, and ErrUnsupported.
	ErrFailed = errors.New("managed runtime failed")
	// ErrUnsupported reports that infer/local invocation targets an unsupported
	// runtime. Distinct from ErrMissing, ErrLoading, ErrFailed, and
	// ErrUnsupportedResponseMode.
	ErrUnsupported = errors.New("managed runtime unsupported")
)

// ReadinessState names the managed-runtime readiness vocabulary.
type ReadinessState string

const (
	ReadinessStateReady       ReadinessState = "READY"
	ReadinessStateMissing     ReadinessState = "MISSING"
	ReadinessStateLoading     ReadinessState = "LOADING"
	ReadinessStateFailed      ReadinessState = "FAILED"
	ReadinessStateUnsupported ReadinessState = "UNSUPPORTED"
)

// LifecycleState names the durable install/load position of a managed runtime.
type LifecycleState string

const (
	LifecycleStateNotInstalled  LifecycleState = "NOT_INSTALLED"
	LifecycleStateInstalling    LifecycleState = "INSTALLING"
	LifecycleStateInstalled     LifecycleState = "INSTALLED"
	LifecycleStateLoading       LifecycleState = "LOADING"
	LifecycleStateLoaded        LifecycleState = "LOADED"
	LifecycleStateNotApplicable LifecycleState = "NOT_APPLICABLE"
)

// PullOutcome classifies the source-agnostic result of an asset pull.
type PullOutcome string

const (
	PullOutcomeAlreadyPresent        PullOutcome = "ALREADY_PRESENT"
	PullOutcomeAlreadyReady          PullOutcome = "ALREADY_READY"
	PullOutcomeInstalledSuccessfully PullOutcome = "INSTALLED_SUCCESSFULLY"
	PullOutcomeSourceFetchFailed     PullOutcome = "SOURCE_FETCH_FAILED"
	PullOutcomeStillLoading          PullOutcome = "STILL_LOADING"
	PullOutcomeTimedOut              PullOutcome = "TIMED_OUT"
	PullOutcomeUnsupportedRuntime    PullOutcome = "UNSUPPORTED_RUNTIME"
)

// Locality identifies whether a model executes locally or through a remote provider.
type Locality string

const (
	LocalityLocal Locality = "LOCAL"
	LocalityCloud Locality = "CLOUD"
)

// Operation describes one provider-neutral model capability.
type Operation struct {
	Name    string
	Inputs  []OperationSlot
	Outputs []OperationSlot
}

const (
	// OperationOMNI accepts multimodal prompt content and returns generated
	// text. It is intentionally a provider-neutral operation identifier.
	OperationOMNI = "OMNI"
	// OperationEMBED converts text into an embedding value.
	OperationEMBED = "EMBED"
	// OperationTTS converts text into audio.
	OperationTTS = "TTS"
	// OperationASR converts audio into a transcript and segments.
	OperationASR = "ASR"
)

// Modality identifies the provider-neutral content category of an operation
// slot. It deliberately does not name a backend representation.
type Modality string

const (
	ModalityText   Modality = "TEXT"
	ModalityImage  Modality = "IMAGE"
	ModalityAudio  Modality = "AUDIO"
	ModalityVideo  Modality = "VIDEO"
	ModalityJSON   Modality = "JSON"
	ModalityBinary Modality = "BINARY"
)

// OperationSlot describes one named input or output of a model operation.
type OperationSlot struct {
	Name         string
	ContentTypes []string
	Modality     Modality
	Required     *bool
	Repeatable   bool
	MediaTypes   []string
}

// Clone returns a detached operation contract.
func (operation Operation) Clone() Operation {
	operation.Inputs = cloneOperationSlots(operation.Inputs)
	operation.Outputs = cloneOperationSlots(operation.Outputs)
	return operation
}

// GenericOperationCatalog is the stateless Models-root value that publishes
// canonical provider-neutral operation contracts.
type GenericOperationCatalog struct{}

// GenericOperationContracts returns the canonical provider-neutral operation
// shapes in stable order. Each call returns fresh slices so callers cannot
// mutate the shared contract definitions.
func (GenericOperationCatalog) GenericOperationContracts() []Operation {
	return []Operation{
		genericOMNIOperation(),
		genericEMBEDOperation(),
		genericTTSOperation(),
		genericASROperation(),
	}
}

// GenericOperationContract returns one detached canonical operation shape by
// identifier. Identifiers are case-insensitive at this value-only lookup
// boundary; returned contracts remain uppercase.
func (catalog GenericOperationCatalog) GenericOperationContract(name string) (Operation, bool) {
	name = strings.ToUpper(strings.TrimSpace(name))
	for _, operation := range catalog.GenericOperationContracts() {
		if operation.Name == name {
			return operation, true
		}
	}
	return Operation{}, false
}

func genericOperationSlot(
	name string,
	modality Modality,
	required bool,
	repeatable bool,
	mediaTypes ...string,
) OperationSlot {
	requiredValue := required
	return OperationSlot{
		Name:         name,
		ContentTypes: []string{string(modality)},
		Modality:     modality,
		Required:     &requiredValue,
		Repeatable:   repeatable,
		MediaTypes:   append([]string(nil), mediaTypes...),
	}
}

func genericOMNIOperation() Operation {
	return Operation{
		Name: OperationOMNI,
		Inputs: []OperationSlot{
			genericOperationSlot("prompt", ModalityText, true, false, "text/plain"),
			genericOperationSlot("image", ModalityImage, false, true, "image/*"),
			genericOperationSlot("audio", ModalityAudio, false, false, "audio/*"),
			genericOperationSlot("video", ModalityVideo, false, false, "video/*"),
			genericOperationSlot("parameters", ModalityJSON, false, false, "application/json"),
		},
		Outputs: []OperationSlot{
			genericOperationSlot("text", ModalityText, true, false, "text/plain"),
			genericOperationSlot("usage", ModalityJSON, false, false, "application/json"),
		},
	}
}

func genericEMBEDOperation() Operation {
	return Operation{
		Name: OperationEMBED,
		Inputs: []OperationSlot{
			genericOperationSlot("text", ModalityText, true, false, "text/plain"),
			genericOperationSlot("parameters", ModalityJSON, false, false, "application/json"),
		},
		Outputs: []OperationSlot{
			genericOperationSlot("embedding", ModalityJSON, true, false, "application/json"),
		},
	}
}

func genericTTSOperation() Operation {
	return Operation{
		Name: OperationTTS,
		Inputs: []OperationSlot{
			genericOperationSlot("text", ModalityText, true, false, "text/plain"),
			genericOperationSlot("voice", ModalityAudio, false, false, "audio/*"),
			genericOperationSlot("parameters", ModalityJSON, false, false, "application/json"),
		},
		Outputs: []OperationSlot{
			genericOperationSlot("audio", ModalityAudio, true, false, "audio/*"),
		},
	}
}

func genericASROperation() Operation {
	return Operation{
		Name: OperationASR,
		Inputs: []OperationSlot{
			genericOperationSlot("audio", ModalityAudio, true, false, "audio/*"),
			genericOperationSlot("prompt", ModalityText, false, false, "text/plain"),
			genericOperationSlot("parameters", ModalityJSON, false, false, "application/json"),
		},
		Outputs: []OperationSlot{
			genericOperationSlot("transcript", ModalityText, true, false, "text/plain"),
			genericOperationSlot("segments", ModalityJSON, true, false, "application/json"),
		},
	}
}

func cloneOperations(operations []Operation) []Operation {
	if operations == nil {
		return nil
	}
	cloned := make([]Operation, len(operations))
	for i, operation := range operations {
		cloned[i] = operation
		cloned[i].Inputs = cloneOperationSlots(operation.Inputs)
		cloned[i].Outputs = cloneOperationSlots(operation.Outputs)
	}
	return cloned
}

func cloneOperationSlots(slots []OperationSlot) []OperationSlot {
	if slots == nil {
		return nil
	}
	cloned := make([]OperationSlot, len(slots))
	for i, slot := range slots {
		cloned[i] = slot
		cloned[i].ContentTypes = append([]string(nil), slot.ContentTypes...)
		cloned[i].MediaTypes = append([]string(nil), slot.MediaTypes...)
		if slot.Required != nil {
			required := *slot.Required
			cloned[i].Required = &required
		}
	}
	return cloned
}

// Runtime is the model-owned readiness projection consumed by service and
// transport adapters.
type Runtime struct {
	Identity            string
	ReadinessState      ReadinessState
	LifecycleState      LifecycleState
	Locality            Locality
	SupportedOperations []Operation
	Diagnostics         map[string]string
}

// Clone returns detached readiness facts safe for a peer to retain or mutate.
func (runtime Runtime) Clone() Runtime {
	runtime.SupportedOperations = cloneOperations(runtime.SupportedOperations)
	runtime.Diagnostics = cloneStringMap(runtime.Diagnostics)
	return runtime
}

// InvocationError carries managed-runtime readiness context without exposing a
// transport-specific projection or error type.
type InvocationError struct {
	Identity       string
	ReadinessState ReadinessState
	LifecycleState LifecycleState
	Cause          error
}

func (e *InvocationError) Error() string {
	if e == nil {
		return ""
	}
	action := invocationAction(e.ReadinessState)
	if action == "" {
		action = "resolve managed runtime readiness before invoking"
	}
	return fmt.Sprintf(
		"managed runtime %q readiness is %s (lifecycle %s): %s",
		e.Identity,
		e.ReadinessState,
		e.LifecycleState,
		action,
	)
}

func (e *InvocationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ManagedRuntimeReadinessState exposes readiness through the narrow model seam.
func (e *InvocationError) ManagedRuntimeReadinessState() ReadinessState {
	if e == nil {
		return ""
	}
	return e.ReadinessState
}

// InvocationError classifies a model-owned runtime projection. A
// ready runtime returns nil.
func (runtime Runtime) InvocationError() error {
	var cause error
	switch runtime.ReadinessState {
	case ReadinessStateReady:
		return nil
	case ReadinessStateMissing:
		cause = ErrMissing
	case ReadinessStateLoading:
		cause = ErrLoading
	case ReadinessStateFailed:
		cause = ErrFailed
	default:
		cause = ErrUnsupported
	}
	return &InvocationError{
		Identity:       runtime.Identity,
		ReadinessState: runtime.ReadinessState,
		LifecycleState: runtime.LifecycleState,
		Cause:          cause,
	}
}

func invocationAction(readiness ReadinessState) string {
	switch readiness {
	case ReadinessStateMissing:
		return "pull or install the managed runtime before invoking"
	case ReadinessStateLoading:
		return "wait for the managed runtime to finish loading before invoking"
	case ReadinessStateFailed:
		return "resolve the managed runtime failure before invoking"
	case ReadinessStateUnsupported:
		return "use a supported managed runtime for invocation"
	default:
		return ""
	}
}
