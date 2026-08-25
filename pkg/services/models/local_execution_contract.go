package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Provider identifies the model provider used for inference dispatch.
type Provider string

const (
	ProviderClaude      Provider = "claude"
	ProviderCodex       Provider = "codex"
	ProviderAntigravity Provider = "antigravity"
	// Retired native provider values remain available for persisted-data
	// decoding; they are not selectable built-ins.
	ProviderGemini   Provider = "gemini"
	ProviderKiro     Provider = "kiro-cli"
	ProviderOpenCode Provider = "opencode"
	ProviderPi       Provider = "pi"
)

// ErrUnsupportedResponseMode reports that an infer/local-invocation result
// cannot satisfy the requested response mode. It is distinct from readiness
// blocked outcomes (ErrMissing, ErrLoading, ErrFailed, ErrUnsupported) so peers
// can branch on typed infer failures through the root contract.
var ErrUnsupportedResponseMode = errors.New("model invocation response mode is not supported")

var (
	// ErrInvalidInferenceDependencies classifies Models Inference construction
	// failures.
	ErrInvalidInferenceDependencies = errors.New("model inference dependencies are invalid")
	// ErrInferenceCancelled reports that model inference was cancelled through
	// its context or the explicit cancellation operation.
	ErrInferenceCancelled = errors.New("model inference cancelled")
	// ErrInferenceTimeout reports that inference exceeded its Models-owned
	// execution deadline.
	ErrInferenceTimeout = errors.New("model inference timed out")
	// ErrInferenceFailed reports a normalized provider/runtime inference
	// failure that is not one of the more specific readiness or lease failures.
	ErrInferenceFailed = errors.New("model inference failed")
	// ErrInferenceArtifactInvalid reports an empty or malformed opaque output
	// artifact reference.
	ErrInferenceArtifactInvalid = errors.New("model inference artifact is invalid")
	// ErrInvocationNotFound reports an unknown invocation capability.
	ErrInvocationNotFound = errors.New("model invocation not found")
	// ErrUnsupportedModelOperation reports that the scoped model does not
	// support the requested operation.
	ErrUnsupportedModelOperation = errors.New("model operation is not supported")
)

// ResponseMode selects the representation returned by direct invocation.
type ResponseMode string

const (
	ResponseModeAudioStream ResponseMode = "AUDIO_STREAM"
)

// ModelReference identifies a model by a configured name or a source URI.
// The reference is intentionally opaque to the Models root contract; source
// resolution belongs to a later invocation stage.
type ModelReference struct {
	NameOrURI string
}

// IsZero reports whether no model name or URI was supplied.
func (reference ModelReference) IsZero() bool {
	return strings.TrimSpace(reference.NameOrURI) == ""
}

// OutputMode selects the provider-neutral representation requested for a
// generic invocation result. An empty value is equivalent to AUTO.
type OutputMode string

const (
	OutputModeAuto     OutputMode = "AUTO"
	OutputModeInline   OutputMode = "INLINE"
	OutputModeJSON     OutputMode = "JSON"
	OutputModeArtifact OutputMode = "ARTIFACT"
)

// OperationParameter carries one ordered, named JSON parameter for a generic
// operation. The value intentionally uses native JSON-compatible Go values so
// transports do not need to agree on a backend-specific parameter type.
type OperationParameter struct {
	Name  string
	Value any
}

// Clone returns a detached operation parameter.
func (parameter OperationParameter) Clone() OperationParameter {
	parameter.Value = cloneInvocationJSONValue(parameter.Value)
	return parameter
}

// Options carries optional direct-invocation response behavior.
type Options struct {
	ResponseMode ResponseMode
}

// Request is the model-owned input for one direct invocation.
type Request struct {
	Operation string
	Content   []work.WorkContentPart
	Bindings  []ModelOperationBinding
	Options   *Options
}

// Result is the model-owned outcome of one direct invocation. Transport
// packages map it to public metadata or streamed response contracts.
type Result struct {
	ModelName         string
	Worker            string
	Operation         string
	ProviderLocality  string
	Content           []work.WorkContentPart
	Bindings          []ResolvedModelOperationBinding
	StreamFile        string
	StreamContentType string
}

// ModelInvocationRef is an opaque Models-owned invocation capability.
type ModelInvocationRef struct {
	value string
}

// Parse restores an invocation reference received from a trusted boundary.
func (ModelInvocationRef) Parse(value string) (ModelInvocationRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ModelInvocationRef{}, ErrInvocationNotFound
	}
	return ModelInvocationRef{value: value}, nil
}

// String serializes the opaque invocation reference.
func (ref ModelInvocationRef) String() string {
	return ref.value
}

// IsZero reports whether no invocation reference was supplied.
func (ref ModelInvocationRef) IsZero() bool {
	return strings.TrimSpace(ref.value) == ""
}

// InferenceArtifactRef is an opaque reference to a Models-owned output
// artifact. It does not reveal a cache or filesystem location.
type InferenceArtifactRef struct {
	value string
}

// Parse restores an artifact reference received from a trusted boundary.
func (InferenceArtifactRef) Parse(value string) (InferenceArtifactRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return InferenceArtifactRef{}, ErrInferenceArtifactInvalid
	}
	return InferenceArtifactRef{value: value}, nil
}

// String serializes the opaque artifact reference.
func (ref InferenceArtifactRef) String() string {
	return ref.value
}

// IsZero reports whether no artifact reference was supplied.
func (ref InferenceArtifactRef) IsZero() bool {
	return strings.TrimSpace(ref.value) == ""
}

// InferenceInput is detached Models-owned invocation input.
type InferenceInput struct {
	// Name is the operation slot that receives this value. It is empty only for
	// the legacy prepared primitive, whose single input predates named slots.
	Name string
	// Modality is the provider-neutral content category for this value.
	Modality    Modality
	ContentType string
	// MediaType preserves the concrete media metadata supplied by the caller,
	// such as image/png or audio/wav.
	MediaType string
	Content   string
	// Artifact is an opaque Models-owned input artifact reference. It is
	// optional when content is carried inline.
	Artifact *InferenceArtifactRef
}

// InferenceContent is detached Models-owned invocation output.
type InferenceContent struct {
	// Name is the output slot name when the provider returned named output.
	Name        string
	Modality    Modality
	ContentType string
	MediaType   string
	Content     string
}

// ASRBackendRequest is the detached, provider-neutral request supplied to a
// Models-owned ASR backend effect. The LocalAI wire representation remains
// private to the Models codec boundary.
type ASRBackendRequest struct {
	Audio      []byte
	MediaType  string
	Prompt     string
	Parameters map[string]any
}

// ASRBackendSegment is one timestamped segment returned by an ASR backend.
type ASRBackendSegment struct {
	ID    int32
	Start int64
	End   int64
	Text  string
}

// ASRBackendResponse carries decoded ASR facts and any opaque artifacts that
// the backend effect has associated with the operation.
type ASRBackendResponse struct {
	Text      string
	Segments  []ASRBackendSegment
	Artifacts []InferenceArtifact
}

// EmbeddingBackendRequest is the detached, provider-neutral request supplied
// to a Models-owned embedding backend effect. The backend protocol remains
// private to the Models codec boundary.
type EmbeddingBackendRequest struct {
	Text       string
	Parameters map[string]any
}

// EmbeddingBackendResponse carries one decoded embedding vector returned by an
// embedding backend effect. Models validates the vector before publishing it.
type EmbeddingBackendResponse struct {
	Embeddings []float64
}

// InferenceOutput is one ordered, slot-named generic invocation output. An
// output may carry inline content, an opaque artifact, or both metadata forms.
type InferenceOutput struct {
	Name        string
	Modality    Modality
	ContentType string
	MediaType   string
	Content     string
	Artifact    *InferenceArtifact
}

// Clone returns a detached generic invocation output.
func (output InferenceOutput) Clone() InferenceOutput {
	if output.Artifact != nil {
		artifact := output.Artifact.Clone()
		output.Artifact = &artifact
	}
	return output
}

// InferenceArtifact contains peer-required output metadata without paths,
// readers, storage records, or runtime artifact handles.
type InferenceArtifact struct {
	Artifact   InferenceArtifactRef
	Name       string
	MediaType  string
	SizeBytes  int64
	Properties map[string]string
}

// Clone returns detached artifact metadata safe for a peer to retain.
func (artifact InferenceArtifact) Clone() InferenceArtifact {
	artifact.Properties = cloneStringMap(artifact.Properties)
	return artifact
}

func cloneInvocationJSONValue(value any) any {
	switch typed := value.(type) {
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneInvocationJSONValue(item)
		}
		return cloned
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = cloneInvocationJSONValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return value
	}
}

// ModelInvocationStatus is the observable lifecycle of an invocation.
type ModelInvocationStatus string

const (
	ModelInvocationStatusAccepted  ModelInvocationStatus = "ACCEPTED"
	ModelInvocationStatusCompleted ModelInvocationStatus = "COMPLETED"
	ModelInvocationStatusFailed    ModelInvocationStatus = "FAILED"
	ModelInvocationStatusCancelled ModelInvocationStatus = "CANCELLED"
)

// InvocationLeaseDisposition reports what happened to capacity after an
// invocation outcome.
type InvocationLeaseDisposition string

const (
	InvocationLeaseRetained InvocationLeaseDisposition = "RETAINED"
	InvocationLeaseReleased InvocationLeaseDisposition = "RELEASED"
	InvocationLeaseExpired  InvocationLeaseDisposition = "EXPIRED"
)

// InvokeModelRequest asks Models to run one operation under an issued lease.
type InvokeModelRequest struct {
	Scope     RuntimeScopeRef
	Lease     ModelLeaseRef
	Holder    string
	ModelName string
	// Model, Inputs, Parameters, OutputMode, and Offline are the additive
	// generic invocation vocabulary. Lease/ModelName/Input/ResponseMode remain
	// available for the prepared primitive and existing TTS callers.
	Model        ModelReference
	Operation    string
	ResponseMode ResponseMode
	Input        InferenceInput
	Inputs       []InferenceInput
	Parameters   []OperationParameter
	OutputMode   OutputMode
	Offline      bool
}

// Validate checks the peer-controlled invocation identity and capacity input.
func (request InvokeModelRequest) Validate() error {
	if request.usesGenericInvocationShape() {
		return request.ValidateGeneric()
	}
	if request.Scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if request.Lease.IsZero() {
		return ErrHostLeaseNotFound
	}
	if strings.TrimSpace(request.Holder) == "" {
		return ErrHostInvalidHolder
	}
	if strings.TrimSpace(request.ModelName) == "" {
		return ErrNotFound
	}
	if strings.TrimSpace(request.Operation) == "" {
		return ErrUnsupportedModelOperation
	}
	return nil
}

// ValidateGeneric checks the detached request shape without resolving a model,
// downloading assets, starting a backend, or acquiring a lease.
func (request InvokeModelRequest) ValidateGeneric() error {
	if request.Scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if strings.TrimSpace(request.Holder) == "" {
		return ErrHostInvalidHolder
	}
	if request.Model.IsZero() {
		return newInvocationFailure(
			InvocationFailureClassInvalidModelReference,
			"model name or URI is required",
		)
	}
	for _, input := range request.Inputs {
		if strings.TrimSpace(input.Name) == "" {
			return newInvocationFailure(
				InvocationFailureClassInvalidSlot,
				"input slot name is required",
			)
		}
	}
	for _, parameter := range request.Parameters {
		if strings.TrimSpace(parameter.Name) == "" {
			return newInvocationFailure(
				InvocationFailureClassInvalidParameter,
				"parameter name is required",
			)
		}
		if !isJSONCompatibleInvocationValue(parameter.Value) {
			return newInvocationFailure(
				InvocationFailureClassInvalidParameter,
				fmt.Sprintf("parameter %q value must be JSON-compatible", parameter.Name),
			)
		}
	}
	if !validOutputMode(request.OutputMode) {
		return newInvocationFailure(
			InvocationFailureClassInvalidParameter,
			fmt.Sprintf("unsupported output mode %q", request.OutputMode),
		)
	}
	return nil
}

// PrepareGenericInvocation selects and validates the effective operation for a
// generic request without consulting a scope, filesystem, network, process,
// or lease. It returns detached request values so a caller can safely retain
// the prepared inputs and parameters while it performs later stages.
func PrepareGenericInvocation(
	request InvokeModelRequest,
	definition ModelDefinition,
) (InvokeModelRequest, Operation, error) {
	if err := request.ValidateGeneric(); err != nil {
		return InvokeModelRequest{}, Operation{}, err
	}
	operations := uniqueOperations(definition.Operations)
	operation, err := selectGenericOperation(request, operations)
	if err != nil {
		return InvokeModelRequest{}, Operation{}, err
	}
	if err := validateGenericInputs(request, operation); err != nil {
		return InvokeModelRequest{}, Operation{}, err
	}
	if err := validateGenericParameters(request, operation); err != nil {
		return InvokeModelRequest{}, Operation{}, err
	}
	prepared := request
	prepared.Operation = operation.Name
	prepared.Inputs = cloneInferenceInputs(request.Inputs)
	prepared.Parameters = cloneInvocationParameters(request.Parameters)
	return prepared, operation.Clone(), nil
}

func selectGenericOperation(
	request InvokeModelRequest,
	operations []Operation,
) (Operation, error) {
	validNames := operationNames(operations)
	requested := strings.TrimSpace(request.Operation)
	if requested == "" {
		if len(operations) == 1 {
			return operations[0].Clone(), nil
		}
		message := "operation is ambiguous"
		if len(operations) == 0 {
			message = "model does not expose an operation"
		}
		return Operation{}, invocationContractFailure(
			request,
			InvocationFailureClassInvalidOperation,
			message+validNamesSuffix(validNames),
			"",
			validNames,
		)
	}
	for _, operation := range operations {
		if strings.EqualFold(operation.Name, requested) {
			return operation.Clone(), nil
		}
	}
	return Operation{}, invocationContractFailure(
		request,
		InvocationFailureClassInvalidOperation,
		fmt.Sprintf("unknown operation %q%s", requested, validNamesSuffix(validNames)),
		"",
		validNames,
	)
}

func validateGenericInputs(request InvokeModelRequest, operation Operation) error {
	slots := make(map[string]OperationSlot, len(operation.Inputs))
	validNames := make([]string, 0, len(operation.Inputs))
	for _, slot := range operation.Inputs {
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			continue
		}
		slots[name] = slot
		validNames = append(validNames, name)
	}
	sort.Strings(validNames)
	counts := make(map[string]int, len(request.Inputs))
	for index, input := range request.Inputs {
		name := strings.TrimSpace(input.Name)
		slot, ok := slots[name]
		if !ok {
			return invocationContractFailure(
				request,
				InvocationFailureClassInvalidSlot,
				fmt.Sprintf("unknown input slot %q%s", name, validNamesSuffix(validNames)),
				name,
				validNames,
			)
		}
		counts[name]++
		if !slot.Repeatable && counts[name] > 1 {
			return invocationContractFailure(
				request,
				InvocationFailureClassSlotArity,
				fmt.Sprintf("input slot %q accepts at most one value", name),
				name,
				[]string{"1"},
			)
		}
		if err := validateGenericInput(request, index, input, slot); err != nil {
			return err
		}
	}
	missing := make([]string, 0)
	for _, slot := range operation.Inputs {
		if slot.Required != nil && *slot.Required && counts[slot.Name] == 0 {
			missing = append(missing, slot.Name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return invocationContractFailure(
			request,
			InvocationFailureClassInvalidSlot,
			"required input slot is missing: "+strings.Join(missing, ", "),
			missing[0],
			validNames,
		)
	}
	return nil
}

func validateGenericInput(
	request InvokeModelRequest,
	index int,
	input InferenceInput,
	slot OperationSlot,
) error {
	name := strings.TrimSpace(input.Name)
	if input.Artifact != nil && input.Artifact.IsZero() {
		return invocationContractFailure(
			request,
			InvocationFailureClassArtifact,
			fmt.Sprintf("input slot %q has an invalid artifact reference", name),
			name,
			nil,
		)
	}
	if input.Modality == "" {
		return invocationContractFailure(
			request,
			InvocationFailureClassMediaCapability,
			fmt.Sprintf("input %d for slot %q must declare a modality", index, name),
			name,
			nil,
		)
	}
	if input.Modality != slot.Modality {
		return invocationContractFailure(
			request,
			InvocationFailureClassMediaCapability,
			fmt.Sprintf("input slot %q does not accept modality %q", name, input.Modality),
			name,
			[]string{string(slot.Modality)},
		)
	}
	if contentType := strings.TrimSpace(input.ContentType); contentType != "" &&
		!slotAcceptsContentType(slot, contentType) {
		return invocationContractFailure(
			request,
			InvocationFailureClassMediaCapability,
			fmt.Sprintf("input slot %q does not accept content type %q", name, contentType),
			name,
			slot.MediaTypes,
		)
	}
	if mediaType := strings.TrimSpace(input.MediaType); mediaType != "" &&
		!matchesMediaType(mediaType, slot.MediaTypes) {
		return invocationContractFailure(
			request,
			InvocationFailureClassMediaCapability,
			fmt.Sprintf("input slot %q does not accept media type %q", name, mediaType),
			name,
			slot.MediaTypes,
		)
	}
	return nil
}

func validateGenericParameters(request InvokeModelRequest, operation Operation) error {
	if len(request.Parameters) == 0 {
		return nil
	}
	acceptsParameters := false
	for _, slot := range operation.Inputs {
		if strings.EqualFold(strings.TrimSpace(slot.Name), "parameters") {
			acceptsParameters = true
			break
		}
	}
	if !acceptsParameters {
		return invocationContractFailure(
			request,
			InvocationFailureClassInvalidParameter,
			"operation does not accept named parameters",
			"",
			nil,
		)
	}
	seen := make(map[string]struct{}, len(request.Parameters))
	for _, parameter := range request.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if _, exists := seen[name]; exists {
			return invocationContractFailure(
				request,
				InvocationFailureClassInvalidParameter,
				fmt.Sprintf("parameter %q is repeated", name),
				"",
				[]string{name},
			)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func invocationContractFailure(
	request InvokeModelRequest,
	class InvocationFailureClass,
	message string,
	slot string,
	validNames []string,
) error {
	return &InvocationFailure{
		Class:      class,
		Message:    message,
		Model:      request.Model,
		Operation:  strings.TrimSpace(request.Operation),
		Slot:       slot,
		ValidNames: append([]string(nil), validNames...),
	}
}

func validNamesSuffix(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return "; valid: " + strings.Join(names, ", ")
}

func operationNames(operations []Operation) []string {
	seen := make(map[string]struct{}, len(operations))
	result := make([]string, 0, len(operations))
	for _, operation := range operations {
		name := strings.TrimSpace(operation.Name)
		if name == "" {
			continue
		}
		canonical := strings.ToUpper(name)
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		result = append(result, canonical)
	}
	sort.Strings(result)
	return result
}

func uniqueOperations(operations []Operation) []Operation {
	seen := make(map[string]struct{}, len(operations))
	result := make([]Operation, 0, len(operations))
	for _, operation := range operations {
		name := strings.ToUpper(strings.TrimSpace(operation.Name))
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		operation.Name = name
		seen[name] = struct{}{}
		result = append(result, operation.Clone())
	}
	return result
}

func cloneInferenceInputs(inputs []InferenceInput) []InferenceInput {
	if inputs == nil {
		return nil
	}
	result := make([]InferenceInput, len(inputs))
	for index, input := range inputs {
		result[index] = input.Clone()
	}
	return result
}

func cloneInvocationParameters(parameters []OperationParameter) []OperationParameter {
	if parameters == nil {
		return nil
	}
	result := make([]OperationParameter, len(parameters))
	for index, parameter := range parameters {
		result[index] = parameter.Clone()
	}
	return result
}

func isJSONCompatibleInvocationValue(value any) bool {
	_, err := json.Marshal(value)
	return err == nil
}

func slotAcceptsContentType(slot OperationSlot, contentType string) bool {
	for _, declared := range slot.ContentTypes {
		if strings.EqualFold(strings.TrimSpace(declared), contentType) ||
			strings.EqualFold(strings.TrimSpace(declared), string(slot.Modality)) {
			return true
		}
	}
	return matchesMediaType(contentType, slot.MediaTypes)
}

func matchesMediaType(value string, patterns []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" || pattern == "*/*" || pattern == value {
			return true
		}
		if strings.HasSuffix(pattern, "/*") &&
			strings.HasPrefix(value, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}

// InferenceInput.Clone returns a detached generic input value.
func (input InferenceInput) Clone() InferenceInput {
	cloned := input
	if input.Artifact != nil {
		artifact := *input.Artifact
		cloned.Artifact = &artifact
	}
	return cloned
}

func (request InvokeModelRequest) usesGenericInvocationShape() bool {
	return !request.Model.IsZero() || request.Inputs != nil || request.Parameters != nil || request.OutputMode != "" || request.Offline
}

// UsesGenericInvocationShape reports whether this request uses the additive
// generic model-reference/input/output vocabulary rather than the prepared
// lease primitive's legacy fields.
func (request InvokeModelRequest) UsesGenericInvocationShape() bool {
	return request.usesGenericInvocationShape()
}

func validOutputMode(mode OutputMode) bool {
	switch mode {
	case "", OutputModeAuto, OutputModeInline, OutputModeJSON, OutputModeArtifact:
		return true
	default:
		return false
	}
}

// GenericInvocationRequest is the descriptive name for the additive generic
// request. It aliases the existing request carrier so prepared primitive
// callers and generic callers cannot drift into separate request vocabularies.
type GenericInvocationRequest = InvokeModelRequest

// InvokeModelResult contains detached inference and lease-lifecycle facts.
type InvokeModelResult struct {
	Invocation ModelInvocationRef
	Scope      RuntimeScopeRef
	Lease      ModelLeaseRef
	ModelName  string
	Operation  string
	Status     ModelInvocationStatus
	Content    []InferenceContent
	Artifacts  []InferenceArtifact
	// Outputs is the additive generic result projection. Content and Artifacts
	// remain populated by the prepared primitive for legacy compatibility.
	Outputs          []InferenceOutput
	LeaseDisposition InvocationLeaseDisposition
	// CancellationOutcome is populated when context cancellation ends an
	// accepted invocation, matching explicit CancelInvocation vocabulary.
	CancellationOutcome InvocationCancellationOutcome
}

// Clone returns a detached invocation result safe for a peer to retain.
func (result InvokeModelResult) Clone() InvokeModelResult {
	result.Content = append([]InferenceContent(nil), result.Content...)
	artifacts := result.Artifacts
	result.Artifacts = make([]InferenceArtifact, len(artifacts))
	for i := range artifacts {
		result.Artifacts[i] = artifacts[i].Clone()
	}
	outputs := result.Outputs
	result.Outputs = make([]InferenceOutput, len(outputs))
	for i := range outputs {
		result.Outputs[i] = outputs[i].Clone()
	}
	return result
}

// GenericInvocationResult is the descriptive name for the additive generic
// result carrier.
type GenericInvocationResult = InvokeModelResult

// CancelInvocationRequest identifies one invocation within its issuing scope.
type CancelInvocationRequest struct {
	Scope      RuntimeScopeRef
	Invocation ModelInvocationRef
}

// Validate checks the cancellation capability input.
func (request CancelInvocationRequest) Validate() error {
	if request.Scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if request.Invocation.IsZero() {
		return ErrInvocationNotFound
	}
	return nil
}

// InvocationCancellationOutcome distinguishes first, repeated, and late
// cancellation without parsing implementation-specific strings.
type InvocationCancellationOutcome string

const (
	InvocationCancellationRequested        InvocationCancellationOutcome = "CANCELLED"
	InvocationCancellationAlreadyCancelled InvocationCancellationOutcome = "ALREADY_CANCELLED"
	InvocationCancellationAlreadyCompleted InvocationCancellationOutcome = "ALREADY_COMPLETED"
)

// CancelInvocationResult reports the observable cancellation and capacity
// outcome for an issued invocation.
type CancelInvocationResult struct {
	Invocation       ModelInvocationRef
	Status           ModelInvocationStatus
	Outcome          InvocationCancellationOutcome
	LeaseDisposition InvocationLeaseDisposition
}

// ResolvedModelOperationBinding is the provider-neutral binding projection
// returned to model-invocation callers.
type ResolvedModelOperationBinding struct {
	Slot    string
	Source  string
	Content []work.WorkContentPart
}

type ModelOperationBinding struct {
	Slot           string                         `json:"slot" yaml:"slot"`
	Selector       *ModelOperationBindingSelector `json:"selector,omitempty" yaml:"selector,omitempty"`
	Config         []work.WorkContentPart         `json:"config,omitempty" yaml:"config,omitempty"`
	DefaultContent []work.WorkContentPart         `json:"defaultContent,omitempty" yaml:"defaultContent,omitempty"`
}

type ModelOperationBindingSelector struct {
	Slot  string `json:"slot,omitempty" yaml:"slot,omitempty"`
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	Type  string `json:"type,omitempty" yaml:"type,omitempty"`
	Role  string `json:"role,omitempty" yaml:"role,omitempty"`
}

// TargetError retains model and worker identity while preserving the domain
// failure that an outward adapter classifies for its customer-facing surface.
type TargetError struct {
	ModelName  string
	WorkerName string
	Operation  string
	Cause      error
}

func (e *TargetError) Error() string {
	if e == nil || e.Cause == nil {
		return ""
	}
	return e.Cause.Error()
}

func (e *TargetError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// LocalInvocationRequest is the plain infer request on the Models root.
// Peers supply Worker/dispatch/bindings vocabulary without importing nested
// inference or local-execution implementation types.
type LocalInvocationRequest struct {
	Scope            RuntimeScopeRef
	Holder           string
	Worker           LocalWorker
	Resources        []LocalResource
	Dispatch         work.WorkDispatch
	ModelOperation   string
	ModelBindings    []ResolvedModelOperationBinding
	WorkingDirectory string
}

// LocalWorker is the Models-owned projection of the authored Worker fields
// required to select and invoke a managed local runtime.
type LocalWorker struct {
	Name          string
	Type          string
	Model         string
	ModelLocality string
	Resources     []LocalResource
}

func (worker LocalWorker) UsesManagedRuntime() bool {
	return RuntimeWorker{Type: worker.Type, ModelLocality: worker.ModelLocality}.UsesManagedRuntime()
}

// LocalResource is the Models-owned projection of a Factory resource.
type LocalResource struct {
	ID         string
	Name       string
	Type       string
	Capacity   int
	Model      string
	Backend    string
	LoadPolicy string
	Provider   string
}

// LocalInvocationResult is the plain infer result on the Models root. Handled
// true means Models owned the invocation; false means Models declined.
type LocalInvocationResult struct {
	Handled bool
	Content string
}

// ValidateLocalInvocationRequest checks the plain infer/local-invocation
// request. Managed-runtime workers with an empty Model fail closed.
func ValidateLocalInvocationRequest(request LocalInvocationRequest) error {
	if request.Worker.UsesManagedRuntime() && strings.TrimSpace(request.Worker.Model) == "" {
		return fmt.Errorf("%w: empty managed runtime model name", ErrNotFound)
	}
	return nil
}
