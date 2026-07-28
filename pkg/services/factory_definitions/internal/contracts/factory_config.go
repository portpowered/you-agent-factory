// Package factorycontracts owns the canonical Factory definition, runtime,
// event, and projection value contracts shared across domain boundaries.
// Implementations and policy remain in their narrower Factory and Factory
// Session packages.
package factorycontracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/namevalue"
	factoryresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"gopkg.in/yaml.v3"
)

const NameValueTypeLocalizableAsset = namevalue.TypeLocalizableAsset

type NameValueConfig = namevalue.Config
type NameValueValidationError = namevalue.ValidationError

func ValidateNameValue(value NameValueConfig) error {
	return namevalue.Validate(value)
}

func ResolveNameValue(value NameValueConfig, locale string) string {
	return namevalue.Resolve(value, locale)
}

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
func IsSystemTimeToken(token *workerexecution.Token) bool {
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

// WorkerType constants for worker AGENTS.md frontmatter and runtime config.
const (
	WorkerTypeInference = "INFERENCE_WORKER"
	WorkerTypeAgent     = "AGENT_WORKER"
	WorkerTypeScript    = "SCRIPT_WORKER"
	WorkerTypePoller    = "POLLER_WORKER"
	// Legacy runtime identifiers retained during the migration window.
	WorkerTypeModel  = "MODEL_WORKER"
	WorkerTypeHosted = "HOSTED_WORKER"
)

// Hosted worker provider constants for public hosted worker config.
const (
	HostedWorkerProviderLinear = "LINEAR"
)

// WorkstationType constants for workstation AGENTS.md frontmatter.
const (
	WorkstationTypeInference = "INFERENCE_RUN"
	WorkstationTypeAgent     = "AGENT_RUN"
	WorkstationTypeScript    = "SCRIPT_RUN"
	WorkstationTypePoller    = "POLLER_RUN"
	// Legacy runtime identifiers retained during the migration window.
	WorkstationTypeModel    = "MODEL_WORKSTATION"
	WorkstationTypeInvoke   = "MODEL_INVOKE"
	WorkstationTypeLogical  = "LOGICAL_MOVE"
	WorkstationTypeClassify = "CLASSIFIER_WORKSTATION"
)

// FactoryConfig is the specification of a factory as a JSON file.
type FactoryConfig struct {
	Name                string                          `json:"name"`
	Description         *NameValueConfig                `json:"description,omitempty" yaml:"description,omitempty"`
	Project             string                          `json:"project,omitempty"`
	Version             *FactoryVersion                 `json:"version,omitempty"`
	Runner              string                          `json:"runner,omitempty"`
	Guards              []FactoryGuardConfig            `json:"guards,omitempty"`
	InputTypes          []InputTypeConfig               `json:"input_types,omitempty"`
	InvocationReturn    *InvocationReturnConfig         `json:"invocation_return,omitempty"`
	InvocationSignature *InvocationSignatureConfig      `json:"invocationSignature,omitempty"`
	Examples            []InvocationExampleConfig       `json:"examples,omitempty" yaml:"examples,omitempty"`
	Orchestrator        *FactoryOrchestratorConfig      `json:"orchestrator,omitempty"`
	WorkTypes           []WorkTypeConfig                `json:"work_types"`
	Resources           []factoryresource.Config        `json:"resources"`
	ResourceManifest    *PortableResourceManifestConfig `json:"resourceManifest,omitempty"`
	Layout              *FactoryLayoutConfig            `json:"layout,omitempty"`
	Workers             []workerconfig.Config           `json:"workers"`
	Workstations        []FactoryWorkstationConfig      `json:"workstations"`
}

// FactoryVersion is the durable optimistic-concurrency metadata stored with a
// persisted factory definition.
type FactoryVersion struct {
	Logical  int64     `json:"logical"`
	Physical time.Time `json:"physical"`
}

// ErrFactoryVersionStale reports that a complete current-Factory save was
// based on an older definition version than the durable current version.
var ErrFactoryVersionStale = errors.New("factory version is stale")

// ErrCurrentFactoryNotFound reports that no durable current-Factory pointer
// could be resolved for canonical definition readback.
var ErrCurrentFactoryNotFound = errors.New("current factory not found")

// ErrFactoryActivationRequiresIdle reports that a definition replacement was
// attempted while its Factory Runtime was not idle.
var ErrFactoryActivationRequiresIdle = errors.New("factory activation requires idle runtime")

// FactoryLayoutConfig carries non-executable portable graph editor layout
// metadata keyed by canonical graph ids.
type FactoryLayoutConfig struct {
	SchemaVersion int                             `json:"schemaVersion"`
	Nodes         []FactoryLayoutNodeConfig       `json:"nodes,omitempty"`
	Edges         []FactoryLayoutEdgeConfig       `json:"edges,omitempty"`
	Groups        []FactoryLayoutGroupConfig      `json:"groups,omitempty"`
	Annotations   []FactoryLayoutAnnotationConfig `json:"annotations,omitempty"`
	Viewport      *FactoryLayoutViewportConfig    `json:"viewport,omitempty"`
	Preferences   *FactoryLayoutPreferencesConfig `json:"preferences,omitempty"`
}

// FactoryLayoutAnnotationConfig is inert positioned canvas content. It is
// intentionally separate from Factory topology and runtime configuration.
type FactoryLayoutAnnotationConfig struct {
	ID       string                    `json:"id"`
	Kind     string                    `json:"kind"`
	Position FactoryLayoutPointConfig  `json:"position"`
	Size     *FactoryLayoutSizeConfig  `json:"size,omitempty"`
	Note     *FactoryLayoutNoteConfig  `json:"note,omitempty"`
	Image    *FactoryLayoutImageConfig `json:"image,omitempty"`
}

type FactoryLayoutNoteConfig struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body"`
	Tone  string `json:"tone"`
}

type FactoryLayoutImageConfig struct {
	Source          FactoryLayoutImageSourceConfig `json:"source"`
	AlternativeText string                         `json:"alternativeText"`
}

// FactoryLayoutImageSourceConfig is the version-1 embedded image source. Its
// discriminator leaves room for future safe source variants without URLs.
type FactoryLayoutImageSourceConfig struct {
	Kind      string `json:"kind"`
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
}

type FactoryLayoutNodeConfig struct {
	ID         string                         `json:"id"`
	Position   FactoryLayoutPointConfig       `json:"position"`
	Size       *FactoryLayoutSizeConfig       `json:"size,omitempty"`
	Locked     *bool                          `json:"locked,omitempty"`
	EmptyState *FactoryLayoutEmptyStateConfig `json:"emptyState,omitempty"`
}

// FactoryLayoutEmptyStateConfig is inert presentation metadata for a canonical
// topology node. Exactly one of Text or Image is authored at the public boundary.
type FactoryLayoutEmptyStateConfig struct {
	Text  string                    `json:"text,omitempty"`
	Image *FactoryLayoutImageConfig `json:"image,omitempty"`
}

type FactoryLayoutEdgeConfig struct {
	ID            string                     `json:"id"`
	Waypoints     []FactoryLayoutPointConfig `json:"waypoints,omitempty"`
	LabelPosition *FactoryLayoutPointConfig  `json:"labelPosition,omitempty"`
}

type FactoryLayoutGroupConfig struct {
	ID            string                    `json:"id"`
	Label         string                    `json:"label,omitempty"`
	Bounds        FactoryLayoutBoundsConfig `json:"bounds"`
	NodeIDs       []string                  `json:"nodeIds"`
	ParentGroupID *string                   `json:"parentGroupId,omitempty"`
	Color         string                    `json:"color,omitempty"`
	Locked        *bool                     `json:"locked,omitempty"`
}

type FactoryLayoutViewportConfig struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

type FactoryLayoutPreferencesConfig struct {
	Direction string `json:"direction,omitempty"`
}

type FactoryLayoutPointConfig struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type FactoryLayoutSizeConfig struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type FactoryLayoutBoundsConfig struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
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

type InvocationReturnConfig = work.InvocationReturnConfig

// Invocation return policies are authored Factory configuration values. Public
// transports map these stable domain values to their generated enum types.
const (
	InvocationReturnPolicySubmittedWorkTerminal = work.InvocationReturnPolicySubmittedWorkTerminal
	InvocationReturnPolicyExplicit              = work.InvocationReturnPolicyExplicit
)

// Invocation signature values are authored Factory configuration vocabulary.
// Keep validation and runtime policy on these values independent of generated
// transport enum declarations.
const (
	InvocationParameterTypeHintString        = work.InvocationParameterTypeHintString
	InvocationParameterTypeHintPath          = work.InvocationParameterTypeHintPath
	InvocationParameterTypeHintFilePath      = work.InvocationParameterTypeHintFilePath
	InvocationParameterTypeHintDirectoryPath = work.InvocationParameterTypeHintDirectoryPath
	InvocationParameterTypeHintNumberString  = work.InvocationParameterTypeHintNumberString
	InvocationParameterTypeHintBooleanString = work.InvocationParameterTypeHintBooleanString
	InvocationParameterTypeHintJSON          = work.InvocationParameterTypeHintJSON

	InvocationParameterValueModeExact        = work.InvocationParameterValueModeExact
	InvocationParameterValueModeRepeated     = work.InvocationParameterValueModeRepeated
	InvocationParameterValueModeVariadic     = work.InvocationParameterValueModeVariadic
	InvocationParameterValueModeFileContents = work.InvocationParameterValueModeFileContents

	InvocationParameterBindingKindPositional = work.InvocationParameterBindingKindPositional
	InvocationParameterBindingKindNamed      = work.InvocationParameterBindingKindNamed
	InvocationParameterBindingKindStdin      = work.InvocationParameterBindingKindStdin
	InvocationParameterBindingKindNamedRest  = work.InvocationParameterBindingKindNamedRest

	InvocationUnknownNamedArgumentPolicyReject  = work.InvocationUnknownNamedArgumentPolicyReject
	InvocationUnknownNamedArgumentPolicyAllow   = work.InvocationUnknownNamedArgumentPolicyAllow
	InvocationUnknownNamedArgumentPolicyCollect = work.InvocationUnknownNamedArgumentPolicyCollect

	InvocationOutputContractModeInline = work.InvocationOutputContractModeInline
	InvocationOutputContractModeFile   = work.InvocationOutputContractModeFile
	InvocationOutputContractModeJSON   = work.InvocationOutputContractModeJSON
)

type InvocationSignatureConfig = work.InvocationSignatureConfig
type InvocationParameterConfig = work.InvocationParameterConfig
type InvocationParameterBindingConfig = work.InvocationParameterBindingConfig
type InvocationOutputContractConfig = work.InvocationOutputContractConfig
type InvocationExampleConfig struct {
	Name        string                     `json:"name" yaml:"name"`
	Description NameValueConfig            `json:"description" yaml:"description"`
	Args        InvocationExampleArguments `json:"args" yaml:"args"`
}

// InvocationExampleArguments is the structured, inert argument payload stored
// in a Factory example. Values are deliberately limited to the same scalar and
// repeated string forms accepted by invocation requests.
type InvocationExampleArguments map[string]interface{}

func (a *InvocationExampleArguments) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	values := make(InvocationExampleArguments, len(raw))
	for name, value := range raw {
		var scalar string
		if err := json.Unmarshal(value, &scalar); err == nil {
			values[name] = scalar
			continue
		}
		var repeated []string
		if err := json.Unmarshal(value, &repeated); err != nil {
			return fmt.Errorf("args.%s must be a string or array of strings", name)
		}
		values[name] = repeated
	}
	*a = values
	return nil
}

func (a *InvocationExampleArguments) UnmarshalYAML(node *yaml.Node) error {
	var raw map[string]interface{}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	values := make(InvocationExampleArguments, len(raw))
	for name, value := range raw {
		switch typed := value.(type) {
		case string:
			values[name] = typed
		case []interface{}:
			repeated := make([]string, len(typed))
			for index, item := range typed {
				text, ok := item.(string)
				if !ok {
					return fmt.Errorf("args.%s[%d] must be a string", name, index)
				}
				repeated[index] = text
			}
			values[name] = repeated
		default:
			return fmt.Errorf("args.%s must be a string or array of strings", name)
		}
	}
	*a = values
	return nil
}

type WorkTypeConfig struct {
	ID               string           `json:"id,omitempty" yaml:"id,omitempty"`
	Name             string           `json:"name"`
	Description      *NameValueConfig `json:"description,omitempty" yaml:"description,omitempty"`
	States           []StateConfig    `json:"states"`
	HandlingBehavior []string         `json:"handlingBehavior,omitempty"`
}

// StateConfig declares a state within a work type.
type StateConfig struct {
	ID   string    `json:"id,omitempty" yaml:"id,omitempty"`
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
	ID         string                   `json:"id,omitempty"`
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

// WorkPropagationMode selects how downstream work receives payload content.
type WorkPropagationMode string

const (
	// WorkPropagationModeOutputAsPayload uses workstation output as downstream payload.
	WorkPropagationModeOutputAsPayload WorkPropagationMode = "OUTPUT_AS_PAYLOAD"
	// WorkPropagationModePreserveInput keeps the consumed input payload for downstream work.
	WorkPropagationModePreserveInput WorkPropagationMode = "PRESERVE_INPUT"
)

// WorkPropagationConfig declares workstation payload propagation policy.
type WorkPropagationConfig struct {
	Mode WorkPropagationMode `json:"mode,omitempty" yaml:"mode,omitempty"`
}

type WorkflowConfig struct {
	Name  string             `json:"name"`
	Paths []TransitionConfig `json:"transitions"`
}

// FactoryWorkstationConfig is the factory.json workstation topology entry. ID is
// the durable public workstation identifier for graph and layout references.
// It also carries flattened runtime workstation fields when factory.json embeds
// AGENTS.md-equivalent workstation configuration directly.
type FactoryWorkstationConfig struct {
	ID                    string                      `json:"id" yaml:"id,omitempty"`
	Name                  string                      `json:"name" yaml:"name,omitempty"`
	Description           *NameValueConfig            `json:"description,omitempty" yaml:"description,omitempty"`
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
	WorkPropagation       *WorkPropagationConfig      `json:"workPropagation,omitempty" yaml:"workPropagation,omitempty"`
	Cron                  *CronConfig                 `json:"cron,omitempty" yaml:"cron,omitempty"`
	Inputs                []IOConfig                  `json:"inputs" yaml:"inputs,omitempty"`
	Outputs               []IOConfig                  `json:"outputs" yaml:"outputs,omitempty"`
	ClassificationRoutes  []ClassificationRouteConfig `json:"classification_routes,omitempty" yaml:"classificationRoutes,omitempty"`
	OnContinue            []IOConfig                  `json:"on_continue,omitempty" yaml:"onContinue,omitempty"`
	OnRejection           []IOConfig                  `json:"on_rejection,omitempty" yaml:"onRejection,omitempty"`
	OnFailure             []IOConfig                  `json:"on_failure,omitempty" yaml:"onFailure,omitempty"`
	Resources             []factoryresource.Config    `json:"resources,omitempty" yaml:"resources,omitempty"`
	CopyReferencedScripts bool                        `json:"copy_referenced_scripts,omitempty" yaml:"-"`
	Guards                []GuardConfig               `json:"guards,omitempty" yaml:"guards,omitempty"`
	StopWords             []string                    `json:"stop_words,omitempty" yaml:"stopWords,omitempty"`
	RuntimeStopWords      []string                    `json:"runtime_stop_words,omitempty" yaml:"-"`
	OutcomeFormat         string                      `json:"outcomeFormat,omitempty" yaml:"outcomeFormat,omitempty"`
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
	Config         []work.WorkContentPart         `json:"config,omitempty" yaml:"config,omitempty"`
	DefaultContent []work.WorkContentPart         `json:"defaultContent,omitempty" yaml:"defaultContent,omitempty"`
}

// ModelOperationBindingSelector matches one input content part deterministically
// against ordered runtime input content.
type ModelOperationBindingSelector struct {
	Slot  string `json:"slot,omitempty" yaml:"slot,omitempty"`
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	Type  string `json:"type,omitempty" yaml:"type,omitempty"`
	Role  string `json:"role,omitempty" yaml:"role,omitempty"`
}

const (
	OrchestratorKindPetri      = "PETRI"
	OrchestratorKindJavaScript = "JAVASCRIPT"
	OrchestratorInlineEncoding = "utf-8"
)

// FactoryOrchestratorConfig is the authored orchestrator identity for one factory.
type FactoryOrchestratorConfig struct {
	Kind       string                               `json:"kind"`
	Petri      *FactoryOrchestratorPetriConfig      `json:"petri,omitempty"`
	JavaScript *FactoryOrchestratorJavaScriptConfig `json:"javascript,omitempty"`
}

// FactoryOrchestratorPetriConfig carries Petri-specific orchestrator options.
type FactoryOrchestratorPetriConfig struct{}

// FactoryOrchestratorJavaScriptConfig carries JavaScript workflow source identity and policy.
type FactoryOrchestratorJavaScriptConfig struct {
	Dialect       string                                        `json:"dialect,omitempty"`
	SourceRef     string                                        `json:"sourceRef,omitempty"`
	InlineSource  *FactoryOrchestratorJavaScriptInlineSource    `json:"inlineSource,omitempty"`
	SourceHash    string                                        `json:"sourceHash,omitempty"`
	Entrypoint    string                                        `json:"entrypoint,omitempty"`
	Metadata      map[string]string                             `json:"metadata,omitempty"`
	ArgsSchema    json.RawMessage                               `json:"argsSchema,omitempty"`
	DefaultPolicy json.RawMessage                               `json:"defaultPolicy,omitempty"`
	Agents        map[string]FactoryOrchestratorJavaScriptAgent `json:"agents,omitempty"`
}

// FactoryOrchestratorJavaScriptAgent declares stable defaults for a named child role.
type FactoryOrchestratorJavaScriptAgent struct {
	Preset string `json:"preset"`
}

// FactoryOrchestratorJavaScriptInlineSource carries inline workflow source text.
type FactoryOrchestratorJavaScriptInlineSource struct {
	Encoding string `json:"encoding"`
	Inline   string `json:"inline"`
}

// EffectiveOrchestratorKind returns the resolved orchestrator kind for a factory.
// Missing orchestrator blocks default to PETRI for compatibility with legacy Petri factories.
func EffectiveOrchestratorKind(cfg *FactoryConfig) string {
	if cfg == nil || cfg.Orchestrator == nil {
		return OrchestratorKindPetri
	}
	kind := strings.TrimSpace(cfg.Orchestrator.Kind)
	if kind == "" {
		return OrchestratorKindPetri
	}
	return kind
}

// IsJavaScriptOrchestratorFactory reports whether the factory resolves to a JavaScript orchestrator.
func IsJavaScriptOrchestratorFactory(cfg *FactoryConfig) bool {
	return EffectiveOrchestratorKind(cfg) == OrchestratorKindJavaScript
}

// IsPetriOrchestratorFactory reports whether the factory resolves to a Petri orchestrator.
func IsPetriOrchestratorFactory(cfg *FactoryConfig) bool {
	return EffectiveOrchestratorKind(cfg) == OrchestratorKindPetri
}

// StrictPublicFactoryOrchestratorKind canonicalizes supported orchestrator kinds.
func StrictPublicFactoryOrchestratorKind(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryOrchestratorKindAliases, false)
}

var publicFactoryOrchestratorKindAliases = map[string]string{
	OrchestratorKindPetri:      OrchestratorKindPetri,
	OrchestratorKindJavaScript: OrchestratorKindJavaScript,
}
