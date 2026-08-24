package models

import (
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/backendregistry"
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

// IsManagedRuntimeBackend reports whether value is an enabled managed-runtime
// backend alias. It trims surrounding whitespace, compares case-insensitively,
// and performs no configuration lookup or external effect.
func IsManagedRuntimeBackend(value string) bool {
	return backendregistry.IsManagedRuntimeBackend(value)
}

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
	PullOutcomeAlreadyPresent              PullOutcome = "ALREADY_PRESENT"
	PullOutcomeAlreadyReady                PullOutcome = "ALREADY_READY"
	PullOutcomeInstalledSuccessfully       PullOutcome = "INSTALLED_SUCCESSFULLY"
	PullOutcomeSourceFetchFailed           PullOutcome = "SOURCE_FETCH_FAILED"
	PullOutcomeStillLoading                PullOutcome = "STILL_LOADING"
	PullOutcomeTimedOut                    PullOutcome = "TIMED_OUT"
	PullOutcomeCancelled                   PullOutcome = "CANCELLED"
	PullOutcomeUnsupportedRuntime          PullOutcome = "UNSUPPORTED_RUNTIME"
	PullOutcomeSourceResolutionFailed      PullOutcome = "SOURCE_RESOLUTION_FAILED"
	PullOutcomeIntegrityVerificationFailed PullOutcome = "INTEGRITY_VERIFICATION_FAILED"
	PullOutcomeAssemblyFailed              PullOutcome = "ASSEMBLY_FAILED"
	PullOutcomeCacheInstallationFailed     PullOutcome = "CACHE_INSTALLATION_FAILED"
	PullOutcomeReadinessEvaluationFailed   PullOutcome = "READINESS_EVALUATION_FAILED"
	PullOutcomeAssetPreparationFailed      PullOutcome = "ASSET_PREPARATION_FAILED"
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

// InvocationProtocolRequest carries the ordered, codec-normalized values for
// one generic protocol call. Slot order and detected media types are part of
// the request contract so adapters do not need to reconstruct CLI intent.
type InvocationProtocolRequest struct {
	Operation  string
	Prompt     string
	Inputs     []InvocationProtocolInput
	Parameters []OperationParameter
}

// InvocationProtocolInput is one provider-neutral protocol value.
type InvocationProtocolInput struct {
	Slot      string
	Modality  Modality
	MediaType string
	Content   string
	Reference string
}

// InvocationProtocolResponse is the detached text response returned by the
// generic protocol adapter.
type InvocationProtocolResponse struct {
	Text string
}

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
	Identity       string
	ReadinessState ReadinessState
	LifecycleState LifecycleState
	Locality       Locality
	// Revision is the installed managed-cache revision when cache facts are
	// available. It remains nil for cloud runtimes and models without an
	// installed managed cache.
	Revision *string
	// CachePath is the resolved managed-cache revision directory when cache
	// facts are available. It remains nil for cloud runtimes and models without
	// an installed managed cache.
	CachePath *string
	// CacheBytes is the exact recursive byte count of regular files in the
	// installed managed-cache revision. It remains nil when no installed cache
	// was observed.
	CacheBytes          *int64
	SupportedOperations []Operation
	Diagnostics         map[string]string
}

// Clone returns detached readiness facts safe for a peer to retain or mutate.
func (runtime Runtime) Clone() Runtime {
	runtime.Revision = cloneStringPointer(runtime.Revision)
	runtime.CachePath = cloneStringPointer(runtime.CachePath)
	runtime.CacheBytes = cloneInt64Pointer(runtime.CacheBytes)
	runtime.SupportedOperations = cloneOperations(runtime.SupportedOperations)
	runtime.Diagnostics = cloneStringMap(runtime.Diagnostics)
	return runtime
}

// ManagedRuntimeCacheFacts are the Models-owned observations used to derive
// managed local-model installation state. Expected artifacts come from the
// cache manifest or the active preparation plan; observed artifacts come from
// the cache inspection effect. Installed is retained as a compatibility fact
// for older adapters that cannot provide manifest details yet.
type ManagedRuntimeCacheFacts struct {
	Locality           Locality
	Supported          bool
	Installed          bool
	ManifestPresent    bool
	ManifestValid      bool
	ExpectedArtifacts  []AssetRequirement
	ObservedArtifacts  []AssetArtifact
	InstalledFileCount int
	PartialArtifacts   bool
	ActivePull         bool
	IntegrityVerified  bool
	FailureReason      string
}

// ManagedRuntimeHostFacts are the detached runtime-host observations layered
// over verified installation facts. A host cannot make an uninstalled model
// ready; the projection falls back to cache state when the facts conflict.
type ManagedRuntimeHostFacts struct {
	Observed       bool
	ReadinessState ReadinessState
	LifecycleState LifecycleState
}

// ManagedRuntimeStateProjection is the canonical compatible state pair for a
// managed runtime. FailureReason is safe diagnostic context owned by Models,
// not a transport error or provider-native message.
type ManagedRuntimeStateProjection struct {
	ReadinessState ReadinessState
	LifecycleState LifecycleState
	FailureReason  string
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

const genericInvocationOutputLimitBytes int64 = 16 << 20

// NormalizeGenericInvocationOutputs validates and detaches named outputs from
// one generic runtime response. It orders outputs by the declared operation
// contract, merges inline content with matching artifact metadata, rejects
// malformed or oversized responses, and never returns a partial output slice.
func NormalizeGenericInvocationOutputs(
	operation Operation,
	content []InferenceContent,
	artifacts []InferenceArtifact,
) ([]InferenceOutput, error) {
	declared, order, err := declaredOutputSlots(operation.Outputs)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 && len(artifacts) == 0 {
		return emptyGenericOutputs(operation.Outputs)
	}

	outputs := make(map[string]*InferenceOutput, len(content)+len(artifacts))
	if err := collectGenericContentOutputs(outputs, operation.Outputs, declared, content, len(artifacts) == 0); err != nil {
		return nil, err
	}
	if err := collectGenericArtifactOutputs(outputs, operation.Outputs, declared, artifacts, len(content) == 0); err != nil {
		return nil, err
	}
	if err := validateRequiredGenericOutputs(operation.Outputs, outputs); err != nil {
		return nil, err
	}
	return orderedGenericOutputs(order, outputs), nil
}

func declaredOutputSlots(slots []OperationSlot) (map[string]OperationSlot, []string, error) {
	declared := make(map[string]OperationSlot, len(slots))
	order := make([]string, 0, len(slots))
	for _, slot := range slots {
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			return nil, nil, malformedInvocationOutputFailure("operation declares an unnamed output slot", "")
		}
		if _, exists := declared[name]; exists {
			return nil, nil, malformedInvocationOutputFailure("operation declares a duplicate output slot", name)
		}
		declared[name] = slot
		order = append(order, name)
	}
	return declared, order, nil
}

func emptyGenericOutputs(slots []OperationSlot) ([]InferenceOutput, error) {
	for _, slot := range slots {
		if slot.Required != nil && *slot.Required {
			return nil, malformedInvocationOutputFailure("required output is missing", slot.Name)
		}
	}
	return nil, nil
}

func collectGenericContentOutputs(
	outputs map[string]*InferenceOutput,
	declaredSlots []OperationSlot,
	declared map[string]OperationSlot,
	content []InferenceContent,
	single bool,
) error {
	for index, item := range content {
		name, slot, err := outputSlot(item.Name, declaredSlots, declared, single && len(content) == 1)
		if err != nil {
			return err
		}
		if err := validateOutputContent(item, name, slot, index); err != nil {
			return err
		}
		output := InferenceOutput{
			Name: name, Modality: outputModality(item.Modality, slot),
			ContentType: item.ContentType, MediaType: item.MediaType, Content: item.Content,
		}
		if err := appendGenericOutput(outputs, name, slot, output); err != nil {
			return err
		}
	}
	return nil
}

func collectGenericArtifactOutputs(
	outputs map[string]*InferenceOutput,
	declaredSlots []OperationSlot,
	declared map[string]OperationSlot,
	artifacts []InferenceArtifact,
	single bool,
) error {
	for _, artifact := range artifacts {
		name, slot, err := outputSlot(artifact.Name, declaredSlots, declared, single && len(artifacts) == 1)
		if err != nil {
			return err
		}
		if err := validateOutputArtifact(artifact, name, slot); err != nil {
			return err
		}
		cloned := artifact.Clone()
		if existing := outputs[name]; existing != nil {
			if existing.Artifact != nil {
				return malformedInvocationOutputFailure("output slot has duplicate artifact metadata", name)
			}
			if slot.Repeatable {
				return malformedInvocationOutputFailure("repeatable output slots require ordered runtime output support", name)
			}
			existing.Artifact = &cloned
			continue
		}
		if err := appendGenericOutput(outputs, name, slot, InferenceOutput{
			Name: name, Modality: outputModality("", slot),
			MediaType: cloned.MediaType, Artifact: &cloned,
		}); err != nil {
			return err
		}
	}
	return nil
}

func appendGenericOutput(
	outputs map[string]*InferenceOutput,
	name string,
	slot OperationSlot,
	output InferenceOutput,
) error {
	if existing := outputs[name]; existing != nil {
		if slot.Repeatable {
			return malformedInvocationOutputFailure("repeatable output slots require ordered runtime output support", name)
		}
		return malformedInvocationOutputFailure("output slot was returned more than once", name)
	}
	outputs[name] = &output
	return nil
}

func validateRequiredGenericOutputs(
	slots []OperationSlot,
	outputs map[string]*InferenceOutput,
) error {
	for _, slot := range slots {
		if slot.Required != nil && *slot.Required {
			if _, exists := outputs[slot.Name]; !exists {
				return malformedInvocationOutputFailure("required output is missing", slot.Name)
			}
		}
	}
	return nil
}

func orderedGenericOutputs(order []string, outputs map[string]*InferenceOutput) []InferenceOutput {
	result := make([]InferenceOutput, 0, len(outputs))
	for _, name := range order {
		if output := outputs[name]; output != nil {
			result = append(result, output.Clone())
		}
	}
	return result
}

func outputSlot(
	name string,
	outputs []OperationSlot,
	declared map[string]OperationSlot,
	single bool,
) (string, OperationSlot, error) {
	name = strings.TrimSpace(name)
	if name == "" && single && len(outputs) == 1 {
		name = outputs[0].Name
	}
	slot, ok := declared[name]
	if !ok {
		return "", OperationSlot{}, malformedInvocationOutputFailure(
			fmt.Sprintf("runtime returned unknown output slot %q", name), name,
		)
	}
	return name, slot, nil
}

func validateOutputContent(
	content InferenceContent,
	name string,
	slot OperationSlot,
	index int,
) error {
	if int64(len([]byte(content.Content))) > genericInvocationOutputLimitBytes {
		return malformedInvocationOutputFailure(
			fmt.Sprintf("output %q exceeds the response size limit", name), name,
		)
	}
	if content.Modality != "" && content.Modality != slot.Modality {
		return malformedInvocationOutputFailure(
			fmt.Sprintf("output %d for slot %q has unsupported modality %q", index, name, content.Modality), name,
		)
	}
	if media := strings.TrimSpace(content.MediaType); media != "" && !matchesMediaType(media, slot.MediaTypes) {
		return malformedInvocationOutputFailure(
			fmt.Sprintf("output slot %q returned unsupported media type", name), name,
		)
	}
	return nil
}

func validateOutputArtifact(artifact InferenceArtifact, name string, slot OperationSlot) error {
	if artifact.Artifact.IsZero() {
		return malformedInvocationOutputFailure("runtime returned an invalid artifact reference", name)
	}
	if artifact.SizeBytes < 0 || artifact.SizeBytes > genericInvocationOutputLimitBytes {
		return malformedInvocationOutputFailure(
			fmt.Sprintf("output %q exceeds the response size limit", name), name,
		)
	}
	if media := strings.TrimSpace(artifact.MediaType); media != "" && !matchesMediaType(media, slot.MediaTypes) {
		return malformedInvocationOutputFailure(
			fmt.Sprintf("output slot %q returned unsupported artifact media type", name), name,
		)
	}
	return nil
}

func outputModality(modality Modality, slot OperationSlot) Modality {
	if modality != "" {
		return modality
	}
	return slot.Modality
}

func malformedInvocationOutputFailure(message, slot string) error {
	return &InvocationFailure{
		Class:   InvocationFailureClassMalformedResponse,
		Message: message,
		Slot:    slot,
		Cause:   ErrInferenceFailed,
	}
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
