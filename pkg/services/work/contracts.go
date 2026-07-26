package work

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrMoveWorkRequestAlreadyApplied is the typed state-access failure returned
// when an operator move requestId was already applied. Peers branch with
// errors.Is on MoveWorkForSession / MoveWorkAndRead.
var ErrMoveWorkRequestAlreadyApplied = errors.New("operator move request was already applied")

// InvocationReturnConfig selects the Work result returned by one Factory
// invocation.
type InvocationReturnConfig struct {
	Policy        string `json:"policy"`
	WorkTypeName  string `json:"workTypeName,omitempty"`
	TerminalState string `json:"terminalState,omitempty"`
	WorkName      string `json:"workName,omitempty"`
}

const (
	InvocationReturnPolicySubmittedWorkTerminal = "SUBMITTED_WORK_TERMINAL"
	InvocationReturnPolicyExplicit              = "EXPLICIT"

	InvocationParameterTypeHintString        = "STRING"
	InvocationParameterTypeHintPath          = "PATH"
	InvocationParameterTypeHintFilePath      = "FILE_PATH"
	InvocationParameterTypeHintDirectoryPath = "DIRECTORY_PATH"
	InvocationParameterTypeHintNumberString  = "NUMBER_STRING"
	InvocationParameterTypeHintBooleanString = "BOOLEAN_STRING"

	InvocationParameterValueModeExact        = "EXACT"
	InvocationParameterValueModeRepeated     = "REPEATED"
	InvocationParameterValueModeVariadic     = "VARIADIC"
	InvocationParameterValueModeFileContents = "FILE_CONTENTS"

	InvocationParameterBindingKindPositional = "POSITIONAL"
	InvocationParameterBindingKindNamed      = "NAMED"
	InvocationParameterBindingKindStdin      = "STDIN"
	InvocationParameterBindingKindNamedRest  = "NAMED_REST"

	InvocationUnknownNamedArgumentPolicyReject  = "REJECT"
	InvocationUnknownNamedArgumentPolicyAllow   = "ALLOW"
	InvocationUnknownNamedArgumentPolicyCollect = "COLLECT"

	InvocationOutputContractModeInline = "INLINE"
	InvocationOutputContractModeFile   = "FILE"
	InvocationOutputContractModeJSON   = "JSON"
)

type InvocationSignatureConfig struct {
	Parameters                 []InvocationParameterConfig     `json:"parameters,omitempty"`
	UnknownNamedArgumentPolicy string                          `json:"unknownNamedArgumentPolicy,omitempty"`
	OutputContract             *InvocationOutputContractConfig `json:"outputContract,omitempty"`
}

type InvocationParameterConfig struct {
	Name          string                             `json:"name"`
	Description   string                             `json:"description,omitempty"`
	ExternalName  string                             `json:"externalName,omitempty"`
	Aliases       []string                           `json:"aliases,omitempty"`
	TypeHint      string                             `json:"typeHint,omitempty"`
	ValueMode     string                             `json:"valueMode,omitempty"`
	Required      bool                               `json:"required,omitempty"`
	Sensitive     bool                               `json:"sensitive,omitempty"`
	Choices       []string                           `json:"choices,omitempty"`
	DefaultValue  string                             `json:"defaultValue,omitempty"`
	DefaultValues []string                           `json:"defaultValues,omitempty"`
	Bindings      []InvocationParameterBindingConfig `json:"bindings,omitempty"`
}

type InvocationParameterBindingConfig struct {
	Kind     string `json:"kind"`
	Position int    `json:"position,omitempty"`
}

type InvocationOutputContractConfig struct {
	Mode          string `json:"mode,omitempty"`
	PathParameter string `json:"pathParameter,omitempty"`
	ContentType   string `json:"contentType,omitempty"`
	FileExtension string `json:"fileExtension,omitempty"`
	Description   string `json:"description,omitempty"`
}

// InvocationWorldStateProvider supplies the Work-owned projection required by
// invocation return-policy evaluation.
type InvocationWorldStateProvider interface {
	InvocationWorldState() InvocationWorldState
}

// InvocationWorldState is the narrow selected-tick projection consumed by Work
// result selection. Factory Runtime projections adapt to this contract.
type InvocationWorldState struct {
	PayloadLineage           WorkPayloadLineageProjection
	WorkRequestsByID         map[string]InvocationWorkRequest
	WorkItemsByID            map[string]FactoryWorkItem
	FailedWorkItemsByID      map[string]FactoryWorkItem
	TerminalWorkByID         map[string]InvocationTerminalWork
	WorkStateChangesByWorkID map[string][]InvocationWorkStateChange
	FactoryState             string
	JavaScriptRuntime        *InvocationJavaScriptRuntime
	SessionBracket           *InvocationSessionBracket
}

func (s InvocationWorldState) InvocationWorldState() InvocationWorldState { return s }

type InvocationWorkRequest struct {
	WorkItems []FactoryWorkItem
	TraceID   string
}

type InvocationTerminalWork struct {
	WorkItem FactoryWorkItem
	Status   string
}

type InvocationWorkStateChange struct {
	WorkID       string
	WorkTypeName string
	ToState      string
	ToPlaceID    string
	RequestID    string
}

type InvocationJavaScriptRuntime struct {
	Dispatches []InvocationDispatchState
}

type InvocationDispatchState struct {
	ID             string
	Status         string
	RelatedWorkIDs []string
}

type InvocationSessionBracket struct {
	SessionID              string
	LifecycleControlStatus string
	FinalStatus            string
	FailureReason          string
}

// SubmitRequest is the internal normalized item used to create work tokens.
type SubmitRequest struct {
	RequestID                string               `json:"requestId,omitempty"`
	WorkID                   string               `json:"workId,omitempty"`
	Name                     string               `json:"name,omitempty"`
	WorkTypeID               string               `json:"workTypeName"`
	TargetState              string               `json:"targetState,omitempty"`
	ChainingTraceDepth       int                  `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string               `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string             `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string               `json:"traceId"`
	Content                  []WorkContentPart    `json:"content,omitempty"`
	Payload                  []byte               `json:"payload"`
	Tags                     map[string]string    `json:"tags"`
	Relations                []Relation           `json:"relations"`
	ExecutionID              string               `json:"executionId,omitempty"`
	InvocationArguments      *InvocationArguments `json:"-"`
}

// WorkRequestType identifies the canonical request contract accepted by factory submit surfaces.
type WorkRequestType string

const WorkRequestTypeFactoryRequestBatch WorkRequestType = "FACTORY_REQUEST_BATCH"

// WorkRequest is the plain Work-owned admission request contract. Peers pass an
// already-decoded request (identity, type, works, and relations) through the
// root Service admission slice; path or protocol decoding stays at adapters.
type WorkRequest struct {
	RequestID              string          `json:"requestId"`
	CurrentChainingTraceID string          `json:"currentChainingTraceId,omitempty"`
	Type                   WorkRequestType `json:"type"`
	Works                  []Work          `json:"works,omitempty"`
	Relations              []WorkRelation  `json:"relations,omitempty"`
}

// WorkRequestEventPayload is the Work-owned payload recorded when a request
// enters a Factory. FactoryEvent context remains authoritative for request,
// trace, and work identity; these fields preserve the public event wire shape.
type WorkRequestEventPayload struct {
	ParentLineage []string                   `json:"parentLineage,omitempty"`
	Relations     []WorkRequestEventRelation `json:"relations,omitempty"`
	Source        string                     `json:"source,omitempty"`
	Type          WorkRequestType            `json:"type"`
	Works         []WorkRequestEventWork     `json:"works,omitempty"`
}

// WorkRequestEventWork preserves the event representation of submitted work
// until Factory context fallbacks are applied by the consuming reducer.
type WorkRequestEventWork struct {
	Name                     string            `json:"name"`
	WorkID                   string            `json:"workId,omitempty"`
	RequestID                string            `json:"requestId,omitempty"`
	WorkTypeID               string            `json:"workTypeName,omitempty"`
	State                    *WorkEventState   `json:"state,omitempty"`
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	Payload                  json.RawMessage   `json:"payload,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
}

// WorkEventState is the state reference embedded in the public Work event
// shape. Replay needs the authored name; the optional durable ID and category
// remain available for compatible decoding.
type WorkEventState struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// WorkRequestEventRelation preserves both name- and ID-based relationship
// references accepted by historical Work request events.
type WorkRequestEventRelation struct {
	Type           WorkRelationType `json:"type"`
	SourceWorkName string           `json:"sourceWorkName"`
	TargetWorkID   string           `json:"targetWorkId,omitempty"`
	TargetWorkName string           `json:"targetWorkName"`
	RequiredState  string           `json:"requiredState,omitempty"`
}

// RelationshipChangeRequestEventPayload records one Work-owned relationship
// in canonical Factory history.
type RelationshipChangeRequestEventPayload struct {
	Relation WorkRequestEventRelation `json:"relation"`
}

// WorkRequestSubmittedWork identifies one accepted work item in a batch upsert.
type WorkRequestSubmittedWork struct {
	Name         string
	WorkTypeName string
	WorkID       string
}

// WorkRequestSubmitResult is the plain Work-owned admission result. Peers
// consume detached acceptance facts (request/work identity and Accepted) without
// importing Work implementation packages.
type WorkRequestSubmitResult struct {
	RequestID    string
	TraceID      string
	WorkID       string
	Name         string
	WorkTypeName string
	Accepted     bool
	Works        []WorkRequestSubmittedWork
}

// Work is one public item inside a WorkRequest batch.
type Work struct {
	Name                     string               `json:"name"`
	WorkID                   string               `json:"workId,omitempty"`
	RequestID                string               `json:"requestId,omitempty"`
	WorkTypeID               string               `json:"workTypeName,omitempty"`
	State                    string               `json:"state,omitempty"`
	ChainingTraceDepth       int                  `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string               `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string             `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string               `json:"traceId,omitempty"`
	Content                  []WorkContentPart    `json:"content,omitempty"`
	Payload                  any                  `json:"payload,omitempty"`
	Tags                     map[string]string    `json:"tags,omitempty"`
	ExecutionID              string               `json:"-"`
	RuntimeRelations         []Relation           `json:"-"`
	InvocationArguments      *InvocationArguments `json:"-"`
}

// WorkContentPart is the backend-owned canonical work content shape mirrored from the public API contract.
type WorkContentPart struct {
	Type        WorkContentPartType `json:"type"`
	Text        string              `json:"text,omitempty"`
	URL         string              `json:"url,omitempty"`
	File        string              `json:"file,omitempty"`
	JSON        json.RawMessage     `json:"json,omitempty"`
	Slot        string              `json:"slot,omitempty"`
	Label       string              `json:"label,omitempty"`
	Role        string              `json:"role,omitempty"`
	ContentType string              `json:"contentType,omitempty"`
	ArtifactID  string              `json:"artifactId,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
}

// WorkContentPartType identifies one canonical content part kind.
type WorkContentPartType string

const (
	WorkContentPartTypeText   WorkContentPartType = "text"
	WorkContentPartTypeImage  WorkContentPartType = "image"
	WorkContentPartTypeAudio  WorkContentPartType = "AUDIO"
	WorkContentPartTypeJSON   WorkContentPartType = "JSON"
	WorkContentPartTypeBinary WorkContentPartType = "BINARY"
)

// Normalized returns the stable backend-owned kind for supported public aliases.
func (t WorkContentPartType) Normalized() WorkContentPartType {
	switch t {
	case "TEXT":
		return WorkContentPartTypeText
	case "IMAGE":
		return WorkContentPartTypeImage
	default:
		return t
	}
}

// InvocationArguments carries transport-independent invocation parameter normalization data.
type InvocationArguments struct {
	Arguments map[string]InvocationArgument `json:"-"`
}

type InvocationArgument struct {
	Values    []string                   `json:"-"`
	ValueMode string                     `json:"-"`
	Sensitive bool                       `json:"-"`
	Sources   []InvocationArgumentSource `json:"-"`
}

type InvocationArgumentSource struct {
	Kind   string `json:"-"`
	Name   string `json:"-"`
	Redact bool   `json:"-"`
}

type WorkRelationType string

const (
	WorkRelationDependsOn   WorkRelationType = "DEPENDS_ON"
	WorkRelationParentChild WorkRelationType = "PARENT_CHILD"
)

type WorkRelation struct {
	Type           WorkRelationType `json:"type"`
	SourceWorkName string           `json:"sourceWorkName"`
	TargetWorkName string           `json:"targetWorkName"`
	RequiredState  string           `json:"requiredState,omitempty"`
}

type WorkRequestNormalizeOptions struct {
	DefaultWorkTypeID string
	ValidWorkTypes    map[string]bool
	ValidStatesByType map[string]map[string]bool
	IDGenerator       RequestIDGenerator
}

// Relation defines a typed relationship between runtime work items.
type Relation struct {
	Type          RelationType `json:"type"`
	TargetWorkID  string       `json:"target_work_id"`
	RequiredState string       `json:"required_state,omitempty"`
}

type RelationType string

const (
	RelationDependsOn   RelationType = "DEPENDS_ON"
	RelationParentChild RelationType = "PARENT_CHILD"
	RelationSpawnedBy   RelationType = "SPAWNED_BY"
)

// FactoryWorkItem describes a unit of work at a point in history.
type FactoryWorkItem struct {
	ID                       string            `json:"id"`
	WorkTypeID               string            `json:"workTypeId"`
	State                    string            `json:"state,omitempty"`
	DisplayName              string            `json:"displayName,omitempty"`
	ChainingTraceDepth       int               `json:"chainingTraceDepth,omitempty"`
	CurrentChainingTraceID   string            `json:"currentChainingTraceId,omitempty"`
	PreviousChainingTraceIDs []string          `json:"previousChainingTraceIds,omitempty"`
	TraceID                  string            `json:"traceId,omitempty"`
	Content                  []WorkContentPart `json:"content,omitempty"`
	ParentID                 string            `json:"parentId,omitempty"`
	PlaceID                  string            `json:"placeId,omitempty"`
	Tags                     map[string]string `json:"tags,omitempty"`
}

type FactoryRelation struct {
	Type           string `json:"type"`
	SourceWorkID   string `json:"sourceWorkId,omitempty"`
	SourceWorkName string `json:"sourceWorkName,omitempty"`
	TargetWorkID   string `json:"targetWorkId"`
	TargetWorkName string `json:"targetWorkName,omitempty"`
	RequiredState  string `json:"requiredState,omitempty"`
	RequestID      string `json:"requestId,omitempty"`
	TraceID        string `json:"traceId,omitempty"`
}

// WorkRequestRecord stores the batch-level request observed before runtime injection.
type WorkRequestRecord struct {
	RequestID     string
	Type          WorkRequestType
	TraceID       string
	Source        string
	ParentLineage []string
	WorkItems     []FactoryWorkItem
	Relations     []FactoryRelation
}

// GeneratedSubmissionBatchMetadata captures request-level metadata for generated work.
type GeneratedSubmissionBatchMetadata struct {
	Source        string   `json:"source"`
	ParentLineage []string `json:"parentLineage"`
}

// GeneratedSubmissionBatch carries a canonical generated request with runtime submissions.
type GeneratedSubmissionBatch struct {
	Request     WorkRequest                      `json:"request"`
	Metadata    GeneratedSubmissionBatchMetadata `json:"metadata"`
	Submissions []SubmitRequest                  `json:"submissions"`
}

// FactorySubmissionRecord stores when a submit request became visible to the runtime.
type FactorySubmissionRecord struct {
	SubmissionID string
	ObservedTick int
	Request      SubmitRequest
	Source       string
}

// WorkStateChangeSource identifies who initiated a work-state change.
type WorkStateChangeSource string

const (
	WorkStateChangeSourceAPI              WorkStateChangeSource = "api"
	WorkStateChangeSourceCLI              WorkStateChangeSource = "cli"
	WorkStateChangeSourceCascadingFailure WorkStateChangeSource = "cascading-failure"
)

type WorkStateChangeRecord struct {
	WorkID, WorkTypeID, WorkTypeName string
	FromState, ToState               string
	FromPlaceID, ToPlaceID           string
	Source                           WorkStateChangeSource
	RequestID, TriggerWorkID, Reason string
}

// OperatorMoveResult is the existing detached move success shape returned by
// the root Service state-access slice (MoveWorkForSession). Peers consume Work
// identity and from/to state facts without importing Factory Runtime types.
type OperatorMoveResult struct {
	WorkID, WorkTypeID     string
	FromState, ToState     string
	FromPlaceID, ToPlaceID string
	TokenID                string
}

// CloneTags returns a detached copy of Work tag metadata while preserving nil.
func CloneTags(tags map[string]string) map[string]string {
	if tags == nil {
		return nil
	}
	cloned := make(map[string]string, len(tags))
	for key, value := range tags {
		cloned[key] = value
	}
	return cloned
}

// CloneRelations returns a detached copy of Work relations while preserving nil.
func CloneRelations(relations []Relation) []Relation {
	if relations == nil {
		return nil
	}
	cloned := make([]Relation, len(relations))
	copy(cloned, relations)
	return cloned
}

// ClonePayload returns detached Work payload bytes while preserving nil.
func ClonePayload(payload []byte) []byte {
	if payload == nil {
		return nil
	}
	return append([]byte(nil), payload...)
}

// ContentFromWorkerOutput maps one worker response body onto canonical Work
// content. Structured content is decoded when present; otherwise the raw body
// becomes one text part.
func ContentFromWorkerOutput(raw string) ([]WorkContentPart, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var content []WorkContentPart
	if err := json.Unmarshal([]byte(trimmed), &content); err == nil && len(content) > 0 {
		return SupportedContentParts(content), nil
	}

	var envelope struct {
		Content []WorkContentPart `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && len(envelope.Content) > 0 {
		return SupportedContentParts(envelope.Content), nil
	}

	return []WorkContentPart{{
		Type: WorkContentPartTypeText,
		Text: raw,
	}}, nil
}

// SupportedContentParts normalizes public aliases and removes unknown part
// kinds while preserving input order.
func SupportedContentParts(parts []WorkContentPart) []WorkContentPart {
	supported := make([]WorkContentPart, 0, len(parts))
	for _, part := range parts {
		part.Type = part.Type.Normalized()
		switch part.Type {
		case WorkContentPartTypeText,
			WorkContentPartTypeImage,
			WorkContentPartTypeAudio,
			WorkContentPartTypeJSON,
			WorkContentPartTypeBinary:
			supported = append(supported, part)
		}
	}
	return supported
}

// WorkDispatch is the canonical dispatch-owned runtime payload.
type WorkDispatch struct {
	DispatchID               string              `json:"dispatch_id"`
	TransitionID             string              `json:"transition_id"`
	WorkerType               string              `json:"worker_type,omitempty"`
	WorkstationName          string              `json:"workstation_name,omitempty"`
	ProjectID                string              `json:"project_id,omitempty"`
	CurrentChainingTraceID   string              `json:"current_chaining_trace_id,omitempty"`
	PreviousChainingTraceIDs []string            `json:"previous_chaining_trace_ids,omitempty"`
	Execution                ExecutionMetadata   `json:"execution,omitempty"`
	InputTokens              []any               `json:"input_tokens"`
	InputBindings            map[string][]string `json:"input_bindings,omitempty"`
}

type ExecutionMetadata struct {
	DispatchCreatedTick int      `json:"dispatch_created_tick,omitempty"`
	CurrentTick         int      `json:"current_tick,omitempty"`
	RequestID           string   `json:"request_id,omitempty"`
	TraceID             string   `json:"trace_id,omitempty"`
	WorkIDs             []string `json:"work_ids,omitempty"`
	ReplayKey           string   `json:"replay_key,omitempty"`
}

func CloneExecutionMetadata(metadata ExecutionMetadata) ExecutionMetadata {
	clone := metadata
	clone.WorkIDs = cloneStringSlice(metadata.WorkIDs)
	return clone
}

func CloneWorkDispatch(dispatch WorkDispatch) WorkDispatch {
	clone := dispatch
	clone.PreviousChainingTraceIDs = cloneStringSlice(dispatch.PreviousChainingTraceIDs)
	clone.Execution = CloneExecutionMetadata(dispatch.Execution)
	clone.InputTokens = cloneAnySlice(dispatch.InputTokens)
	clone.InputBindings = cloneStringSliceMap(dispatch.InputBindings)
	return clone
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(values))
	for key, items := range values {
		clone[key] = cloneStringSlice(items)
	}
	return clone
}

func cloneAnySlice(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	clone := make([]any, len(values))
	for index, value := range values {
		clone[index] = cloneAnyValue(value)
	}
	return clone
}

func CloneWorkContentParts(parts []WorkContentPart) []WorkContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]WorkContentPart, len(parts))
	for i, part := range parts {
		cloned[i] = part
		cloned[i].JSON = append([]byte(nil), part.JSON...)
		cloned[i].Metadata = cloneAnyMap(part.Metadata)
	}
	return cloned
}

// CloneInvocationArguments returns a detached copy of runtime-only invocation
// argument metadata.
func CloneInvocationArguments(args *InvocationArguments) *InvocationArguments {
	if args == nil || len(args.Arguments) == 0 {
		return nil
	}
	clone := &InvocationArguments{Arguments: make(map[string]InvocationArgument, len(args.Arguments))}
	for name, argument := range args.Arguments {
		next := InvocationArgument{
			Values: cloneStringSlice(argument.Values), ValueMode: argument.ValueMode, Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = append([]InvocationArgumentSource(nil), argument.Sources...)
		}
		clone.Arguments[name] = next
	}
	return clone
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = cloneAnyValue(value)
	}
	return clone
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case []any:
		return cloneAnySlice(typed)
	case map[string]any:
		return cloneAnyMap(typed)
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case map[string]string:
		return cloneStringMap(typed)
	case map[string][]string:
		return cloneStringSliceMap(typed)
	default:
		return value
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
