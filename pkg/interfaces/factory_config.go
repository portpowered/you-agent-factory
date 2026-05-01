package interfaces

import "encoding/json"

// FactoryConfig is the specification of a factory as a JSON file.
type FactoryConfig struct {
	Project          string                          `json:"project,omitempty"`
	InputTypes       []InputTypeConfig               `json:"input_types,omitempty"`
	WorkTypes        []WorkTypeConfig                `json:"work_types"`
	Resources        []ResourceConfig                `json:"resources"`
	ResourceManifest *PortableResourceManifestConfig `json:"resourceManifest,omitempty"`
	Workers          []WorkerConfig                  `json:"workers"`
	Workstations     []FactoryWorkstationConfig      `json:"workstations"`
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

type WorkTypeConfig struct {
	Name   string        `json:"name"`
	States []StateConfig `json:"states"`
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
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
}

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
	ID                    string            `json:"id" yaml:"id,omitempty"`
	Name                  string            `json:"name" yaml:"name,omitempty"`
	Kind                  WorkstationKind   `json:"kind,omitempty" yaml:"kind,omitempty"`
	Type                  string            `json:"type,omitempty" yaml:"type,omitempty"`
	WorkerTypeName        string            `json:"worker" yaml:"worker,omitempty"`
	PromptFile            string            `json:"prompt_file,omitempty" yaml:"promptFile,omitempty"`
	OutputSchema          string            `json:"output_schema,omitempty" yaml:"outputSchema,omitempty"`
	Timeout               string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Limits                WorkstationLimits `json:"limits,omitempty" yaml:"limits,omitempty"`
	Cron                  *CronConfig       `json:"cron,omitempty" yaml:"cron,omitempty"`
	Inputs                []IOConfig        `json:"inputs" yaml:"inputs,omitempty"`
	Outputs               []IOConfig        `json:"outputs" yaml:"outputs,omitempty"`
	OnContinue            *IOConfig         `json:"on_continue,omitempty" yaml:"onContinue,omitempty"`
	OnRejection           *IOConfig         `json:"on_rejection,omitempty" yaml:"onRejection,omitempty"`
	OnFailure             *IOConfig         `json:"on_failure,omitempty" yaml:"onFailure,omitempty"`
	Resources             []ResourceConfig  `json:"resources,omitempty" yaml:"resources,omitempty"`
	CopyReferencedScripts bool              `json:"copy_referenced_scripts,omitempty" yaml:"-"`
	Guards                []GuardConfig     `json:"guards,omitempty" yaml:"guards,omitempty"`
	StopWords             []string          `json:"stop_words,omitempty" yaml:"stopWords,omitempty"`
	RuntimeStopWords      []string          `json:"runtime_stop_words,omitempty" yaml:"-"`
	Body                  string            `json:"body,omitempty" yaml:"-"`
	PromptTemplate        string            `json:"prompt_template,omitempty" yaml:"-"`
	WorkingDirectory      string            `json:"working_directory,omitempty" yaml:"workingDirectory,omitempty"`
	Worktree              string            `json:"worktree,omitempty" yaml:"worktree,omitempty"`
	Env                   map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
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
)

// GuardType identifies a built-in guard type in customer-facing config.
type GuardType string

const (
	GuardTypeVisitCount          GuardType = "visit_count"
	GuardTypeMatchesFields       GuardType = "matches_fields"
	GuardTypeAllChildrenComplete GuardType = "all_children_complete"
	GuardTypeAnyChildFailed      GuardType = "any_child_failed"
	GuardTypeSameName            GuardType = "same_name"
)

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
