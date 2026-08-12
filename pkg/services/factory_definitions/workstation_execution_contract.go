package factorydefinitions

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// WorkstationExecutionPolicyService resolves execution policy authored on a
// Workstation definition. Consumers receive this contract from composition.
type WorkstationExecutionPolicyService interface {
	ExecutionTimeout(*FactoryWorkstationConfig) (time.Duration, error)
}

// InvocationDefinitionContext contains the already-normalized invocation
// values needed to resolve authored interpolation. It contains no runtime,
// session, provider, model, or filesystem object; ReadFile is the one explicit
// boundary for FILE_CONTENTS arguments.
type InvocationDefinitionContext struct {
	Arguments *work.InvocationArguments
	ReadFile  FileReader
}

// ExecutionCatalogReferenceCatalog is an optional detached identity catalog
// supplied by composition. Definitions uses it only for pure membership
// validation and never calls a provider, model, runner, or process service.
type ExecutionCatalogReferenceCatalog struct {
	Runners   map[string]struct{}
	Providers map[string]struct{}
	Models    map[string]struct{}
}

// ResolveExecutionCatalogRequest selects one effective authored definition
// and invocation context for immutable execution-policy resolution.
type ResolveExecutionCatalogRequest struct {
	EffectiveDefinition *FactoryConfig
	Invocation          InvocationDefinitionContext
	References          ExecutionCatalogReferenceCatalog
}

// ExecutionCatalogDiagnosticCode classifies a deterministic Definition
// resolution finding. Messages contain identity and path facts only.
type ExecutionCatalogDiagnosticCode string

const (
	ExecutionCatalogDiagnosticInvalidDefinition    ExecutionCatalogDiagnosticCode = "invalid-definition"
	ExecutionCatalogDiagnosticInvalidInterpolation ExecutionCatalogDiagnosticCode = "invalid-interpolation"
	ExecutionCatalogDiagnosticInvalidRunner        ExecutionCatalogDiagnosticCode = "invalid-runner"
	ExecutionCatalogDiagnosticUnknownRunner        ExecutionCatalogDiagnosticCode = "unknown-runner"
	ExecutionCatalogDiagnosticInvalidProvider      ExecutionCatalogDiagnosticCode = "invalid-provider"
	ExecutionCatalogDiagnosticUnknownProvider      ExecutionCatalogDiagnosticCode = "unknown-provider"
	ExecutionCatalogDiagnosticInvalidModel         ExecutionCatalogDiagnosticCode = "invalid-model"
	ExecutionCatalogDiagnosticUnknownModel         ExecutionCatalogDiagnosticCode = "unknown-model"
	ExecutionCatalogDiagnosticUnknownWorker        ExecutionCatalogDiagnosticCode = "unknown-worker"
	ExecutionCatalogDiagnosticUnknownWorkstation   ExecutionCatalogDiagnosticCode = "unknown-workstation"
	ExecutionCatalogDiagnosticDuplicateIdentity    ExecutionCatalogDiagnosticCode = "duplicate-identity"
	ExecutionCatalogDiagnosticInvalidTimeout       ExecutionCatalogDiagnosticCode = "invalid-timeout"
)

// ExecutionCatalogDiagnostic is a detached, sensitive-safe resolution
// finding.
type ExecutionCatalogDiagnostic struct {
	Code      ExecutionCatalogDiagnosticCode `json:"code"`
	Path      string                         `json:"path"`
	Reference string                         `json:"reference,omitempty"`
	Message   string                         `json:"message"`
}

// ExecutionCatalogError reports one or more actionable Definition
// resolution findings. The result still carries detached diagnostics so a
// caller can present every invalid reference in one validation pass.
type ExecutionCatalogError struct {
	Diagnostics []ExecutionCatalogDiagnostic
}

func (e *ExecutionCatalogError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "execution catalog resolution failed"
	}
	return fmt.Sprintf("execution catalog resolution failed: %s", e.Diagnostics[0].Message)
}

// ResolvedModelOperationSlot is a detached model-operation capability value.
type ResolvedModelOperationSlot struct {
	Name         string   `json:"name"`
	ContentTypes []string `json:"contentTypes,omitempty"`
	Required     bool     `json:"required,omitempty"`
}

// ResolvedModelOperation is a detached worker model-operation value.
type ResolvedModelOperation struct {
	Name    string                       `json:"name"`
	Inputs  []ResolvedModelOperationSlot `json:"inputs,omitempty"`
	Outputs []ResolvedModelOperationSlot `json:"outputs,omitempty"`
}

// ResolvedWorkerDefinition contains only immutable authored policy facts.
type ResolvedWorkerDefinition struct {
	ID               string                   `json:"id,omitempty"`
	Name             string                   `json:"name"`
	Type             string                   `json:"type"`
	Provider         string                   `json:"provider,omitempty"`
	Model            string                   `json:"model,omitempty"`
	ModelProvider    string                   `json:"modelProvider,omitempty"`
	ReasoningEffort  string                   `json:"reasoningEffort,omitempty"`
	ModelLocality    string                   `json:"modelLocality,omitempty"`
	ExecutorProvider string                   `json:"executorProvider,omitempty"`
	Command          string                   `json:"command,omitempty"`
	Args             []string                 `json:"args,omitempty"`
	Body             string                   `json:"body,omitempty"`
	PromptSourcePath string                   `json:"promptSourcePath,omitempty"`
	StopToken        string                   `json:"stopToken,omitempty"`
	Timeout          time.Duration            `json:"timeout,omitempty"`
	SkipPermissions  bool                     `json:"skipPermissions,omitempty"`
	AgentToolPolicy  string                   `json:"agentToolPolicy,omitempty"`
	Operations       []ResolvedModelOperation `json:"operations,omitempty"`
	Resources        []ResolvedResource       `json:"resources,omitempty"`
}

// ResolvedInputGuard is a detached workstation input-guard value.
type ResolvedInputGuard struct {
	Type        GuardType `json:"type"`
	MatchInput  string    `json:"matchInput,omitempty"`
	ParentInput string    `json:"parentInput,omitempty"`
	SpawnedBy   string    `json:"spawnedBy,omitempty"`
}

// ResolvedWorkstationIO is a detached workstation routing value.
type ResolvedWorkstationIO struct {
	WorkTypeName string              `json:"workType"`
	StateName    string              `json:"state"`
	Guard        *ResolvedInputGuard `json:"guard,omitempty"`
}

// ResolvedClassificationRoute is a detached classification routing value.
type ResolvedClassificationRoute struct {
	Label   string                  `json:"label"`
	Outputs []ResolvedWorkstationIO `json:"outputs,omitempty"`
}

// ResolvedWorkstationLimits contains detached scheduling and output bounds.
type ResolvedWorkstationLimits struct {
	MaxRetries                    int    `json:"maxRetries,omitempty"`
	MaxExecutionTime              string `json:"maxExecutionTime,omitempty"`
	MaxGeneratedWorkItems         int    `json:"maxGeneratedWorkItems,omitempty"`
	MaxGeneratedWorkItemsArgument string `json:"maxGeneratedWorkItemsArgument,omitempty"`
	MaxGeneratedWorkItemsOffset   int    `json:"maxGeneratedWorkItemsOffset,omitempty"`
}

// ResolvedResource is a detached resource-capacity value.
type ResolvedResource struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Capacity   int    `json:"capacity"`
	Model      string `json:"model,omitempty"`
	Backend    string `json:"backend,omitempty"`
	LoadPolicy string `json:"loadPolicy,omitempty"`
	Provider   string `json:"provider,omitempty"`
}

// ResolvedModelOperationBinding is a detached operation-input policy value.
type ResolvedModelOperationBinding struct {
	Slot           string                         `json:"slot"`
	Selector       *ModelOperationBindingSelector `json:"selector,omitempty"`
	Config         []work.WorkContentPart         `json:"config,omitempty"`
	DefaultContent []work.WorkContentPart         `json:"defaultContent,omitempty"`
}

// ResolvedWorkstationDefinition contains scheduling-neutral execution policy
// and routing facts for one authored Workstation.
type ResolvedWorkstationDefinition struct {
	ID                          string                          `json:"id,omitempty"`
	Name                        string                          `json:"name"`
	Kind                        WorkstationKind                 `json:"kind,omitempty"`
	Type                        string                          `json:"type,omitempty"`
	Operation                   string                          `json:"operation,omitempty"`
	WorkerName                  string                          `json:"worker,omitempty"`
	Runner                      string                          `json:"runner,omitempty"`
	RunnerSelectionSource       string                          `json:"runnerSelectionSource,omitempty"`
	PromptFile                  string                          `json:"promptFile,omitempty"`
	OutputSchema                string                          `json:"outputSchema,omitempty"`
	OutputContract              string                          `json:"outputContract,omitempty"`
	Body                        string                          `json:"body,omitempty"`
	PromptTemplate              string                          `json:"promptTemplate,omitempty"`
	WorkingDirectory            string                          `json:"workingDirectory,omitempty"`
	Worktree                    string                          `json:"worktree,omitempty"`
	Environment                 map[string]string               `json:"environment,omitempty"`
	Timeout                     time.Duration                   `json:"timeout,omitempty"`
	OutputFormat                string                          `json:"outputFormat,omitempty"`
	DecisionEnvelope            bool                            `json:"decisionEnvelope,omitempty"`
	GoalRoutingDecisionEnvelope bool                            `json:"goalRoutingDecisionEnvelope,omitempty"`
	FormatInvocationSummary     bool                            `json:"formatInvocationSummary,omitempty"`
	FormatInvocationResponse    bool                            `json:"formatInvocationResponse,omitempty"`
	FormatTTSMetadata           bool                            `json:"formatTTSMetadata,omitempty"`
	WorkPropagation             WorkPropagationMode             `json:"workPropagation,omitempty"`
	Limits                      ResolvedWorkstationLimits       `json:"limits,omitempty"`
	StopWords                   []string                        `json:"stopWords,omitempty"`
	RuntimeStopWords            []string                        `json:"runtimeStopWords,omitempty"`
	OperationBindings           []ResolvedModelOperationBinding `json:"operationBindings,omitempty"`
	Inputs                      []ResolvedWorkstationIO         `json:"inputs,omitempty"`
	Outputs                     []ResolvedWorkstationIO         `json:"outputs,omitempty"`
	OnContinue                  []ResolvedWorkstationIO         `json:"onContinue,omitempty"`
	OnRejection                 []ResolvedWorkstationIO         `json:"onRejection,omitempty"`
	OnFailure                   []ResolvedWorkstationIO         `json:"onFailure,omitempty"`
	ClassificationRoutes        []ResolvedClassificationRoute   `json:"classificationRoutes,omitempty"`
	ExpectedArtifacts           []ExpectedArtifactConfig        `json:"expectedArtifacts,omitempty"`
	Resources                   []ResolvedResource              `json:"resources,omitempty"`
	Guards                      []GuardConfig                   `json:"guards,omitempty"`
}

// ResolvedExecutionCatalog is the detached, deterministic policy snapshot
// produced before any execution machinery is selected or invoked.
type ResolvedExecutionCatalog struct {
	DefinitionVersion string                                   `json:"definitionVersion,omitempty"`
	Workers           map[string]ResolvedWorkerDefinition      `json:"workers"`
	Workstations      map[string]ResolvedWorkstationDefinition `json:"workstations"`
}

// ResolveExecutionCatalogResult carries the detached catalog and all safe
// diagnostics produced while resolving it.
type ResolveExecutionCatalogResult struct {
	ResolvedExecutionCatalog
	Diagnostics []ExecutionCatalogDiagnostic `json:"diagnostics,omitempty"`
}

// Clone returns a detached copy of a resolved catalog result. The resolver
// itself already returns a detached value, but this helper makes ownership
// explicit for callers that retain or cache a snapshot.
func (r ResolveExecutionCatalogResult) Clone() ResolveExecutionCatalogResult {
	data, err := json.Marshal(r)
	if err != nil {
		return ResolveExecutionCatalogResult{}
	}
	var cloned ResolveExecutionCatalogResult
	if err := json.Unmarshal(data, &cloned); err != nil {
		return ResolveExecutionCatalogResult{}
	}
	return cloned
}
