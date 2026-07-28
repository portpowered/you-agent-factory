package invocationreturnpolicy

import (
	"encoding/json"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/lineagegraph"
)

var (
	ErrInvalidInvocationInput    = errors.New("invalid invocation input")
	ErrUnsupportedReturnPolicy = errors.New("unsupported invocation return policy")
)

type InvocationReturnConfig struct {
	Policy        string
	WorkTypeName  string
	TerminalState string
	WorkName      string
}

type InvocationSignatureConfig struct {
	Parameters                 []InvocationParameterConfig
	UnknownNamedArgumentPolicy string
}

type InvocationParameterConfig struct {
	Name          string
	ExternalName  string
	Aliases       []string
	TypeHint      string
	ValueMode     string
	Required      bool
	Sensitive     bool
	Choices       []string
	DefaultValue  string
	DefaultValues []string
	Bindings      []InvocationParameterBindingConfig
}

type InvocationParameterBindingConfig struct {
	Kind     string
	Position int
}

type InvocationArguments struct {
	Arguments map[string]InvocationArgument
}

type InvocationArgument struct {
	Values    []string
	ValueMode string
	Sensitive bool
	Sources   []InvocationArgumentSource
}

type InvocationArgumentSource struct {
	Kind   string
	Name   string
	Redact bool
}

type ContentPartType string

const (
	ContentPartTypeText   ContentPartType = "text"
	ContentPartTypeImage  ContentPartType = "image"
	ContentPartTypeAudio  ContentPartType = "AUDIO"
	ContentPartTypeJSON   ContentPartType = "JSON"
	ContentPartTypeBinary ContentPartType = "BINARY"
)

func (t ContentPartType) Normalized() ContentPartType {
	switch t {
	case "TEXT":
		return ContentPartTypeText
	case "IMAGE":
		return ContentPartTypeImage
	default:
		return t
	}
}

type ContentPart struct {
	Type        ContentPartType `json:"type"`
	Text        string          `json:"text,omitempty"`
	URL         string          `json:"url,omitempty"`
	File        string          `json:"file,omitempty"`
	JSON        json.RawMessage `json:"json,omitempty"`
	Slot        string          `json:"slot,omitempty"`
	Label       string          `json:"label,omitempty"`
	Role        string          `json:"role,omitempty"`
	ContentType string          `json:"contentType,omitempty"`
	ArtifactID  string          `json:"artifactId,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

type WorkItem struct {
	ID                       string
	WorkTypeID               string
	State                    string
	DisplayName              string
	ChainingTraceDepth       int
	CurrentChainingTraceID   string
	PreviousChainingTraceIDs []string
	TraceID                  string
	Content                  []ContentPart
	ParentID                 string
	PlaceID                  string
	Tags                     map[string]string
}

type InvocationWorldStateProvider interface {
	InvocationWorldState() InvocationWorldState
}

type InvocationWorldState struct {
	PayloadLineage           lineagegraph.WorkPayloadLineageProjection
	WorkRequestsByID         map[string]InvocationWorkRequest
	WorkItemsByID            map[string]WorkItem
	FailedWorkItemsByID      map[string]WorkItem
	TerminalWorkByID         map[string]InvocationTerminalWork
	WorkStateChangesByWorkID map[string][]InvocationWorkStateChange
	FactoryState             string
	JavaScriptRuntime        *InvocationJavaScriptRuntime
	SessionBracket           *InvocationSessionBracket
}

func (s InvocationWorldState) InvocationWorldState() InvocationWorldState { return s }

type InvocationWorkRequest struct {
	WorkItems []WorkItem
	TraceID   string
}

type InvocationTerminalWork struct {
	WorkItem WorkItem
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
