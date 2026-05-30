package interfaces

import (
	"encoding/json"
	"time"
)

// File directories

const (
	FactoryDir      = "factory"
	WorkflowsDir    = "workflows"
	WorkTypesDir    = "work-types"
	InputsDir       = "inputs"
	WorkersDir      = "workers"
	WorkstationsDir = "workstations"

	DefaultChannelName = "default"
	ArtifactsDirectory = "artifacts"
	StateDir           = "state"
	TransitionsDir     = "transitions"
	ArcsDir            = "arcs"
	GuardsDir          = "guards"
	ResourcesDir       = "resources"
	TokensDir          = "tokens"
	MarkingsDir        = "markings"

	FactoryAgentsFileName = "AGENTS.md"

	// relevant files.
	FactoryConfigFile         = "factory.json"
	CurrentFactoryPointerFile = ".current-factory"
	MetadataFile              = "metadata.json"
	MarkingFile               = "marking.json"
)

// Extensions

const (
	JsonExtension = ".json"
)

// Token fields
// These are fields that are used to color tokens as they go through the petri net.
const (
	WorkID     = "work_id"
	WorkTypeID = "work_type_id"
	TraceID    = "trace_id"
	ParentID   = "parent_id"
	Tags       = "tags"
	Relations  = "relations"
	Payload    = "payload"
)

// Internal time work contract.
const (
	SystemTimeWorkTypeID         = "__system_time"
	SystemTimePendingState       = "pending"
	SystemTimePendingPlaceID     = SystemTimeWorkTypeID + ":" + SystemTimePendingState
	SystemTimeExpiryTransitionID = SystemTimeWorkTypeID + ":expire"

	SystemTimeDashboardWorkTypeID         = "time"
	SystemTimeDashboardPendingPlaceID     = SystemTimeDashboardWorkTypeID + ":" + SystemTimePendingState
	SystemTimeDashboardExpiryTransitionID = SystemTimeDashboardWorkTypeID + ":expire"

	TimeWorkTagKeySource          = "agent_factory.source"
	TimeWorkTagKeyCronWorkstation = "agent_factory.cron.workstation"
	TimeWorkTagKeyNominalAt       = "agent_factory.time.nominal_at"
	TimeWorkTagKeyDueAt           = "agent_factory.time.due_at"
	TimeWorkTagKeyExpiresAt       = "agent_factory.time.expires_at"
	TimeWorkTagKeyJitter          = "agent_factory.time.jitter"

	TimeWorkSourceCron = "cron"
)

// IsSystemTimeWorkType reports whether a work type is the internal time-work
// type used for cron ticks.
func IsSystemTimeWorkType(workTypeID string) bool {
	return workTypeID == SystemTimeWorkTypeID
}

// IsSystemTimePlace reports whether a place belongs to the internal time-work
// state machine.
func IsSystemTimePlace(placeID string) bool {
	return placeID == SystemTimePendingPlaceID
}

// IsSystemTimeToken reports whether a token is an internal time-work token.
func IsSystemTimeToken(token *Token) bool {
	return token != nil && IsSystemTimeWorkType(token.Color.WorkTypeID)
}

// Executor configuration
// These are keys to store data for the messages between executors.
const (
	// These are well known tags that are used to store data for the messages between executors.
	RejectionFeedback = "_rejection_feedback"
)

// Resource constants

// Resource avialable states
const (
	ResourceStateAvailable = "available"
)

// WorkerType constants for worker AGENTS.md frontmatter.
const (
	WorkerTypeModel  = "MODEL_WORKER"
	WorkerTypeScript = "SCRIPT_WORKER"
	WorkerTypeHosted = "HOSTED_WORKER"
)

// Hosted worker provider constants for public hosted worker config.
const (
	HostedWorkerProviderLinear = "LINEAR"
)

// WorkstationType constants for workstation AGENTS.md frontmatter.
const (
	WorkstationTypeModel    = "MODEL_WORKSTATION"
	WorkstationTypeInvoke   = "MODEL_INVOKE"
	WorkstationTypeLogical  = "LOGICAL_MOVE"
	WorkstationTypeClassify = "CLASSIFIER_WORKSTATION"
)

// FactoryConfig is the specification of a factory as a JSON file.
type FactoryConfig struct {
	Name             string                          `json:"name"`
	Project          string                          `json:"project,omitempty"`
	Version          *FactoryVersion                 `json:"version,omitempty"`
	Runner           string                          `json:"runner,omitempty"`
	Guards           []FactoryGuardConfig            `json:"guards,omitempty"`
	InputTypes       []InputTypeConfig               `json:"input_types,omitempty"`
	WorkTypes        []WorkTypeConfig                `json:"work_types"`
	Resources        []ResourceConfig                `json:"resources"`
	ResourceManifest *PortableResourceManifestConfig `json:"resourceManifest,omitempty"`
	Workers          []WorkerConfig                  `json:"workers"`
	Workstations     []FactoryWorkstationConfig      `json:"workstations"`
}

// FactoryVersion is the durable optimistic-concurrency metadata stored with a
// persisted factory definition.
type FactoryVersion struct {
	Logical  int64     `json:"logical"`
	Physical time.Time `json:"physical"`
}

// InputTypeConfig declares a named input type that the factory accepts.
// When no input_types are declared, only the implicit "default" type is available.
type InputTypeConfig struct {
	Name string    `json:"name"`
	Type InputKind `json:"type"`
}

// InputKind identifies how the factory should parse and validate an incoming input.
type InputKind string

const (
	// InputKindDefault accepts a plain SubmitRequest with no structured validation.
	InputKindDefault InputKind = "default"
)

// WorkTypeHandlingBehaviorDefault marks the work type that receives simplified CLI prompt submissions.
const WorkTypeHandlingBehaviorDefault = "DEFAULT"

type WorkTypeConfig struct {
	Name              string   `json:"name"`
	States            []StateConfig `json:"states"`
	HandlingBehavior  []string `json:"handlingBehavior,omitempty"`
}

// StateConfig declares a state within a work type.
type StateConfig struct {
	Name string    `json:"name"`
	Type StateType `json:"type"`
}

type StateType string

const (
	StateTypeInitial    StateType = "INITIAL"
	StateTypeProcessing StateType = "PROCESSING"
	StateTypeTerminal   StateType = "TERMINAL"
	StateTypeFailed     StateType = "FAILED"
)

type ResourceConfig struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Capacity   int    `json:"capacity"`
	Model      string `json:"model,omitempty"`
	Backend    string `json:"backend,omitempty"`
	LoadPolicy string `json:"loadPolicy,omitempty"`
	Provider   string `json:"provider,omitempty"`
}

const (
	ResourceTypeModel          = "MODEL"
	ResourceTypeProviderQuota  = "PROVIDER_QUOTA"
	ResourceTypeInvocationSlot = "INVOCATION_SLOT"
)

// PortableResourceManifestConfig declares portability-only resources that are
// distinct from runtime-capacity resources.
type PortableResourceManifestConfig struct {
	RequiredTools []RequiredToolConfig `json:"requiredTools,omitempty"`
	BundledFiles  []BundledFileConfig  `json:"bundledFiles,omitempty"`
}

const (
	// BundledFileTypeScript is the canonical manifest type for portable script assets.
	BundledFileTypeScript = "SCRIPT"
	// BundledFileTypeDoc is the canonical manifest type for portable documentation assets.
	BundledFileTypeDoc = "DOC"
	// BundledFileTypeInput is the canonical manifest type for portable starter
	// work files that restore under factory/inputs/...
	BundledFileTypeInput = "INPUT"
	// BundledFileTypeRootHelper is the canonical manifest type for supported
	// project-root helper files such as Makefile.
	BundledFileTypeRootHelper = "ROOT_HELPER"
)

const (
	// BundledFileEncodingUTF8 declares plain UTF-8 inline content.
	BundledFileEncodingUTF8 = "utf-8"
)

// RequiredToolConfig declares one validation-only external tool dependency.
type RequiredToolConfig struct {
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Purpose     string   `json:"purpose,omitempty"`
	VersionArgs []string `json:"versionArgs,omitempty"`
}

// BundledFileConfig declares one portable file payload and its factory-relative
// restoration target.
type BundledFileConfig struct {
	Type       string                   `json:"type"`
	TargetPath string                   `json:"targetPath"`
	Content    BundledFileContentConfig `json:"content"`
}

// BundledFileContentConfig declares the bundled inline file payload.
type BundledFileContentConfig struct {
	Encoding string `json:"encoding"`
	Inline   string `json:"inline"`
}

// WorkstationLimits holds execution limits from workstation configuration.
type WorkstationLimits struct {
	MaxRetries       int    `json:"max_retries,omitempty" yaml:"maxRetries,omitempty"`
	MaxExecutionTime string `json:"max_execution_time,omitempty" yaml:"maxExecutionTime,omitempty"`
}

type WorkflowConfig struct {
	Name  string             `json:"name"`
	Paths []TransitionConfig `json:"transitions"`
}

// FactoryWorkstationConfig is the factory.json workstation topology entry.
// It also carries flattened runtime workstation fields when factory.json embeds
// AGENTS.md-equivalent workstation configuration directly.
type FactoryWorkstationConfig struct {
	ID                    string                      `json:"id" yaml:"id,omitempty"`
	Name                  string                      `json:"name" yaml:"name,omitempty"`
	Kind                  WorkstationKind             `json:"behavior,omitempty" yaml:"behavior,omitempty"`
	Type                  string                      `json:"type,omitempty" yaml:"type,omitempty"`
	Operation             string                      `json:"operation,omitempty" yaml:"operation,omitempty"`
	OperationBindings     []ModelOperationBinding     `json:"operationBindings,omitempty" yaml:"operationBindings,omitempty"`
	WorkerTypeName        string                      `json:"worker" yaml:"worker,omitempty"`
	Runner                string                      `json:"runner,omitempty" yaml:"runner,omitempty"`
	OpenCodeAgent         string                      `json:"openCodeAgent,omitempty" yaml:"openCodeAgent,omitempty"`
	PromptFile            string                      `json:"prompt_file,omitempty" yaml:"promptFile,omitempty"`
	OutputSchema          string                      `json:"output_schema,omitempty" yaml:"outputSchema,omitempty"`
	Timeout               string                      `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Limits                WorkstationLimits           `json:"limits,omitempty" yaml:"limits,omitempty"`
	Cron                  *CronConfig                 `json:"cron,omitempty" yaml:"cron,omitempty"`
	Inputs                []IOConfig                  `json:"inputs" yaml:"inputs,omitempty"`
	Outputs               []IOConfig                  `json:"outputs" yaml:"outputs,omitempty"`
	ClassificationRoutes  []ClassificationRouteConfig `json:"classification_routes,omitempty" yaml:"classificationRoutes,omitempty"`
	OnContinue            []IOConfig                  `json:"on_continue,omitempty" yaml:"onContinue,omitempty"`
	OnRejection           []IOConfig                  `json:"on_rejection,omitempty" yaml:"onRejection,omitempty"`
	OnFailure             []IOConfig                  `json:"on_failure,omitempty" yaml:"onFailure,omitempty"`
	Resources             []ResourceConfig            `json:"resources,omitempty" yaml:"resources,omitempty"`
	CopyReferencedScripts bool                        `json:"copy_referenced_scripts,omitempty" yaml:"-"`
	Guards                []GuardConfig               `json:"guards,omitempty" yaml:"guards,omitempty"`
	StopWords             []string                    `json:"stop_words,omitempty" yaml:"stopWords,omitempty"`
	RuntimeStopWords      []string                    `json:"runtime_stop_words,omitempty" yaml:"-"`
	Body                  string                      `json:"body,omitempty" yaml:"-"`
	PromptTemplate        string                      `json:"prompt_template,omitempty" yaml:"-"`
	WorkingDirectory      string                      `json:"working_directory,omitempty" yaml:"workingDirectory,omitempty"`
	Worktree              string                      `json:"worktree,omitempty" yaml:"worktree,omitempty"`
	Env                   map[string]string           `json:"env,omitempty" yaml:"env,omitempty"`
}

// ClassificationRouteConfig declares one authored classifier label and the
// destinations that label should dispatch to on successful classification.
type ClassificationRouteConfig struct {
	Label   string     `json:"label" yaml:"label,omitempty"`
	Outputs []IOConfig `json:"outputs" yaml:"outputs,omitempty"`
}

// CronConfig declares the trigger contract for cron workstations.
// Cron workstations reuse workstation inputs and outputs for token readiness
// and routing; this object only owns trigger timing.
type CronConfig struct {
	Schedule       string `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	TriggerAtStart bool   `json:"triggerAtStart,omitempty" yaml:"triggerAtStart,omitempty"`
	Jitter         string `json:"jitter,omitempty" yaml:"jitter,omitempty"`
	ExpiryWindow   string `json:"expiryWindow,omitempty" yaml:"expiryWindow,omitempty"`

	unsupportedInterval bool
}

// UnmarshalJSON decodes the supported cron contract while preserving whether a
// removed interval field was supplied so config validation can report it with a
// precise workstation path.
func (c *CronConfig) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	type cronConfigPayload struct {
		Schedule       string `json:"schedule,omitempty"`
		TriggerAtStart bool   `json:"triggerAtStart,omitempty"`
		Jitter         string `json:"jitter,omitempty"`
		ExpiryWindow   string `json:"expiryWindow,omitempty"`
	}
	var payload cronConfigPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	c.Schedule = payload.Schedule
	c.TriggerAtStart = payload.TriggerAtStart
	c.Jitter = payload.Jitter
	c.ExpiryWindow = payload.ExpiryWindow
	_, c.unsupportedInterval = fields["interval"]
	return nil
}

// HasUnsupportedInterval reports whether the decoded config supplied the
// removed cron.interval field.
func (c *CronConfig) HasUnsupportedInterval() bool {
	return c != nil && c.unsupportedInterval
}

// WorkstationKind identifies the scheduling semantics of a workstation.
type WorkstationKind string

const (
	// WorkstationKindStandard is the default fire-once workstation type.
	WorkstationKindStandard WorkstationKind = "standard"
	// WorkstationKindRepeater re-fires a transition after a non-terminal result.
	WorkstationKindRepeater WorkstationKind = "repeater"
	// WorkstationKindCron declares a timed trigger workstation.
	WorkstationKindCron WorkstationKind = "cron"
	// WorkstationKindPoller declares a long-lived ingress poller workstation.
	WorkstationKindPoller WorkstationKind = "poller"
)

// GuardType identifies a built-in guard type in customer-facing config.
type GuardType string

const (
	GuardTypeVisitCount          GuardType = "visit_count"
	GuardTypeMatchesFields       GuardType = "matches_fields"
	GuardTypeAllChildrenComplete GuardType = "all_children_complete"
	GuardTypeAnyChildFailed      GuardType = "any_child_failed"
	GuardTypeSameName            GuardType = "same_name"
	GuardTypeSameTraceID         GuardType = "same_trace_id"
	GuardTypeInferenceThrottle   GuardType = "inference_throttle_guard"
)

// FactoryGuardConfig declares a root-level factory guard using customer-facing names.
type FactoryGuardConfig struct {
	Type          GuardType `json:"type" yaml:"type"`
	ModelProvider string    `json:"model_provider,omitempty" yaml:"modelProvider,omitempty"`
	Model         string    `json:"model,omitempty" yaml:"model,omitempty"`
	RefreshWindow string    `json:"refresh_window,omitempty" yaml:"refreshWindow,omitempty"`
}

type GuardMatchConfig struct {
	InputKey string `json:"input_key,omitempty" yaml:"inputKey,omitempty"`
}

// GuardConfig declares a guard on a workstation using customer-facing names.
type GuardConfig struct {
	Type        GuardType         `json:"type" yaml:"type"`
	Workstation string            `json:"workstation,omitempty" yaml:"workstation,omitempty"`
	MaxVisits   int               `json:"max_visits,omitempty" yaml:"maxVisits,omitempty"`
	MatchConfig *GuardMatchConfig `json:"match_config,omitempty" yaml:"matchConfig,omitempty"`
}

type IOConfig struct {
	WorkTypeName string            `json:"work_type" yaml:"workType"`
	StateName    string            `json:"state" yaml:"state"`
	Guard        *InputGuardConfig `json:"guard,omitempty" yaml:"guard,omitempty"`
}

// InputGuardConfig declares a guard on a specific input.
type InputGuardConfig struct {
	Type        GuardType `json:"type" yaml:"type"`
	MatchInput  string    `json:"match_input,omitempty" yaml:"matchInput,omitempty"`
	ParentInput string    `json:"parent_input,omitempty" yaml:"parentInput,omitempty"`
	SpawnedBy   string    `json:"spawned_by,omitempty" yaml:"spawnedBy,omitempty"`
}

type TransitionConfig struct {
	FromWorkstationName string `json:"from"`
	ToWorkstationName   string `json:"to"`
}

// ModelOperationBinding declares how one MODEL_INVOKE workstation input slot
// resolves content from runtime input or authored workstation configuration.
type ModelOperationBinding struct {
	Slot           string                         `json:"slot" yaml:"slot"`
	Selector       *ModelOperationBindingSelector `json:"selector,omitempty" yaml:"selector,omitempty"`
	Config         []WorkContentPart              `json:"config,omitempty" yaml:"config,omitempty"`
	DefaultContent []WorkContentPart              `json:"defaultContent,omitempty" yaml:"defaultContent,omitempty"`
}

// ModelOperationBindingSelector matches one input content part deterministically
// against ordered runtime input content.
type ModelOperationBindingSelector struct {
	Slot  string `json:"slot,omitempty" yaml:"slot,omitempty"`
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	Type  string `json:"type,omitempty" yaml:"type,omitempty"`
	Role  string `json:"role,omitempty" yaml:"role,omitempty"`
}

// ModelOperationBindingSource records where one slot binding was resolved from.
type ModelOperationBindingSource string

const (
	ModelOperationBindingSourceInput   ModelOperationBindingSource = "INPUT"
	ModelOperationBindingSourceConfig  ModelOperationBindingSource = "CONFIG"
	ModelOperationBindingSourceDefault ModelOperationBindingSource = "DEFAULT"
	ModelOperationBindingSourceOmitted ModelOperationBindingSource = "OMITTED"
)

// ResolvedModelOperationBinding stores one resolved slot binding before model
// execution begins.
type ResolvedModelOperationBinding struct {
	Slot    string                      `json:"slot"`
	Source  ModelOperationBindingSource `json:"source"`
	Content []WorkContentPart           `json:"content,omitempty"`
}

// WorkerConfig is the canonical worker configuration used by factory.json,
// worker AGENTS.md frontmatter, and loaded runtime config.
type WorkerConfig struct {
	Name             string                    `json:"name" yaml:"name,omitempty"`
	Type             string                    `json:"type" yaml:"type"`
	Provider         string                    `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model            string                    `json:"model,omitempty" yaml:"model,omitempty"`
	ModelProvider    string                    `json:"modelProvider,omitempty" yaml:"modelProvider,omitempty"`
	ModelLocality    string                    `json:"modelLocality,omitempty" yaml:"modelLocality,omitempty"`
	ExecutorProvider string                    `json:"executorProvider,omitempty" yaml:"executorProvider,omitempty"`
	Operations       []ModelOperation          `json:"operations,omitempty" yaml:"operations,omitempty"`
	Command          string                    `json:"command,omitempty" yaml:"command,omitempty"`
	Args             []string                  `json:"args,omitempty" yaml:"args,omitempty"`
	Resources        []ResourceConfig          `json:"resources,omitempty" yaml:"resources,omitempty"`
	Timeout          string                    `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	StopToken        string                    `json:"stopToken,omitempty" yaml:"stopToken,omitempty"`
	SkipPermissions  bool                      `json:"skipPermissions,omitempty" yaml:"skipPermissions,omitempty"`
	OpenCodeAgent    string                    `json:"openCodeAgent,omitempty" yaml:"openCodeAgent,omitempty"`
	Auth             *HostedWorkerAuthConfig   `json:"auth,omitempty" yaml:"auth,omitempty"`
	Linear           *HostedLinearWorkerConfig `json:"linear,omitempty" yaml:"linear,omitempty"`
	Body             string                    `json:"body,omitempty" yaml:"-"`

	// Internal-only runtime fields retained during contract cleanup.
	SessionID   string `json:"-" yaml:"-"`
	Concurrency int    `json:"-" yaml:"-"`
}

type HostedWorkerAuthConfig struct {
	SecretRef string `json:"secretRef,omitempty" yaml:"secretRef,omitempty"`
}

type HostedLinearWorkerConfig struct {
	PollInterval string                          `json:"pollInterval,omitempty" yaml:"pollInterval,omitempty"`
	TeamIDs      []string                        `json:"teamIds,omitempty" yaml:"teamIds,omitempty"`
	StateIDs     []string                        `json:"stateIds,omitempty" yaml:"stateIds,omitempty"`
	Mapping      HostedLinearWorkerMappingConfig `json:"mapping,omitempty" yaml:"mapping,omitempty"`
	Claim        *HostedLinearWorkerClaimConfig  `json:"claim,omitempty" yaml:"claim,omitempty"`
}

type HostedLinearWorkerMappingConfig struct {
	WorkType string `json:"workType,omitempty" yaml:"workType,omitempty"`
	State    string `json:"state,omitempty" yaml:"state,omitempty"`
}

type HostedLinearWorkerClaimConfig struct {
	AssigneeField string `json:"assigneeField,omitempty" yaml:"assigneeField,omitempty"`
}

// TimeoutDuration parses Timeout as a time.Duration. It returns zero when the
// value is empty or invalid.
func (w *WorkerConfig) TimeoutDuration() time.Duration {
	if w.Timeout == "" {
		return 0
	}
	d, _ := time.ParseDuration(w.Timeout)
	return d
}

const (
	ModelLocalityLocal = "LOCAL"
	ModelLocalityCloud = "CLOUD"
)

const (
	ModelOperationContentTypeText   = "TEXT"
	ModelOperationContentTypeImage  = "IMAGE"
	ModelOperationContentTypeAudio  = "AUDIO"
	ModelOperationContentTypeJSON   = "JSON"
	ModelOperationContentTypeBinary = "BINARY"
)

// ModelOperation declares one provider-agnostic capability exposed by a model worker.
type ModelOperation struct {
	Name    string               `json:"name" yaml:"name"`
	Inputs  []ModelOperationSlot `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs []ModelOperationSlot `json:"outputs,omitempty" yaml:"outputs,omitempty"`
}

// ModelOperationSlot declares one named operation input or output slot.
type ModelOperationSlot struct {
	Name         string   `json:"name" yaml:"name"`
	ContentTypes []string `json:"contentTypes,omitempty" yaml:"contentTypes,omitempty"`
	Required     bool     `json:"required,omitempty" yaml:"required,omitempty"`
}
