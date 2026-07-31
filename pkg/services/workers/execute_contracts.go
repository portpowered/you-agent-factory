package workers

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// ErrInvalidExecuteRequest reports that Execute rejected a request before an
// attempt started.
var ErrInvalidExecuteRequest = errors.New("invalid Workers execute request")

// ErrExecuteUnavailable reports that the inert Workers root was constructed
// without a request-scoped Execute capability.
var ErrExecuteUnavailable = errors.New("Workers execute capability unavailable")

// ExecutionOutcome is the normalized business/execution result of one attempt.
type ExecutionOutcome string

const (
	ExecutionOutcomeAccepted ExecutionOutcome = "ACCEPTED"
	ExecutionOutcomeContinue ExecutionOutcome = "CONTINUE"
	ExecutionOutcomeRejected ExecutionOutcome = "REJECTED"
	ExecutionOutcomeFailed   ExecutionOutcome = "FAILED"
	ExecutionOutcomeCanceled ExecutionOutcome = "CANCELED"
)

// ExecuteRequest is the complete, detached input for one Workers attempt.
// Callers must not require Workers to back-query Factory Sessions, Runtime, or
// Definitions for missing context.
type ExecuteRequest struct {
	Correlation ExecutionCorrelation
	Target      ExecutionTarget
	Input       ExecutionInput
	Attempt     AttemptContext
}

// ExecutionCorrelation identifies one attempt for logging, observations, and
// result attribution without Runtime token or place vocabulary.
type ExecutionCorrelation struct {
	FactorySessionID string
	RuntimeID        string
	DispatchID       string
	AttemptID        string
	RequestID        string
	TraceID          string
}

// ExecutionTarget carries already-resolved execution policy for one attempt.
type ExecutionTarget struct {
	WorkerName      string
	WorkstationName string
	RunnerID        string

	Provider ProviderReference
	Model    ModelReference

	Prompt      PromptPolicy
	Tools       ToolPolicy
	Output      OutputPolicy
	Environment EnvironmentPolicy
	Workspace   WorkspacePolicy
	Permissions PermissionPolicy
	Timeout     time.Duration
}

// ProviderReference is a detached provider identity reference.
type ProviderReference struct {
	ID    string
	Alias string
}

// ModelReference is a detached model identity reference.
type ModelReference struct {
	Name     string
	Provider string
	Locality string
}

// PromptPolicy carries resolved prompt material for one attempt.
type PromptPolicy struct {
	SystemPrompt string
	UserMessage  string
	OutputSchema string
}

// ToolPolicy carries resolved tool-execution policy for one attempt.
type ToolPolicy struct {
	ExecutionMode                RunnerToolExecutionMode
	RequiredOptionalCapabilities []RunnerOptionalCapability
}

// OutputPolicy carries resolved output-contract facts for one attempt.
type OutputPolicy struct {
	Contract string
}

// EnvironmentPolicy carries resolved environment additions for one attempt.
// Values are request-scoped execution inputs and must not be persisted by
// Workers diagnostics or observations.
type EnvironmentPolicy struct {
	Vars                 map[string]string
	ProcessEnvironment   []string
	WorkingDirectory     string
	WorkingDirectorySet  bool
	SkipProcessInheritance bool
}

// WorkspacePolicy carries resolved workspace and Worktree facts for one attempt.
type WorkspacePolicy struct {
	Worktree           string
	WorkingDirectory   string
	PrepareWorktree    bool
	FactoryDirectory   string
	CheckoutIdentifier string
}

// PermissionPolicy carries resolved permission facts for one attempt.
type PermissionPolicy struct {
	SkipPermissions bool
}

// ExecutionInput carries Work projections and prior-attempt facts for one call.
type ExecutionInput struct {
	Work             []WorkInput
	Invocation       work.InvocationArguments
	ModelBindings    []ResolvedModelOperationBinding
	ModelOperation   string
	PreviousAttempts []AttemptSummary
	Resume           *ProviderContinuationRef
}

// WorkInput is the Worker-facing projection of canonical Work. It excludes
// Runtime token, place, marking, and Petri topology vocabulary.
type WorkInput struct {
	WorkID       string
	WorkTypeID   string
	RequestID    string
	Content      []work.WorkContentPart
	Tags         map[string]string
	Relations    []work.Relation
	Lineage      WorkLineage
	AttemptFacts AttemptFacts
}

// WorkLineage carries orchestration-neutral lineage references.
type WorkLineage struct {
	ParentWorkID string
	TraceID      string
	OriginRef    string
}

// AttemptFacts carries prior-failure or attempt facts projected by Runtime.
type AttemptFacts struct {
	AttemptNumber int
	LastOutcome   string
	LastFailure   string
}

// AttemptContext identifies how many times Runtime has already attempted the
// same logical dispatch. Attempt numbering remains Runtime-owned.
type AttemptContext struct {
	Number int
}

// AttemptSummary summarizes one prior attempt without Runtime mutation state.
type AttemptSummary struct {
	AttemptID string
	Outcome   ExecutionOutcome
	Failure   *ExecutionFailure
	Finished  time.Time
}

// ProviderContinuationRef is an opaque provider-conversation reference.
// Workers and Runtime may correlate and forward it; they must not inspect or
// retain underlying provider session objects.
type ProviderContinuationRef struct {
	Provider          string
	ProviderSessionID string
	ExternalRef       string
}

// ExecuteResult is the detached terminal result of one started Workers attempt.
type ExecuteResult struct {
	Correlation  ExecutionCorrelation
	Outcome      ExecutionOutcome
	Output       ProposedOutput
	Failure      *ExecutionFailure
	Diagnostics  *SafeDiagnostics
	Metrics      ExecutionMetrics
	Continuation *ProviderContinuationRef
}

// ProposedOutput carries Worker-proposed content. Canonical Work materialization
// remains Work-owned.
type ProposedOutput struct {
	Primary        []work.WorkContentPart
	Feedback       string
	Classification string
	ProposedWork   []ProposedWork
	ArtifactRefs   []ArtifactRef
}

// ProposedWork is a non-canonical follow-up Work proposal.
type ProposedWork struct {
	WorkTypeID string
	Name       string
	State      string
	Content    []work.WorkContentPart
	Tags       map[string]string
	Relations  []work.Relation
}

// ArtifactRef is a detached artifact identity Workers observed or produced.
type ArtifactRef struct {
	ArtifactID string
	Label      string
	URI        string
}

// ExecutionFailure carries normalized failure facts for a started attempt.
type ExecutionFailure struct {
	Type      WorkFailureType
	Family    WorkFailureFamily
	Message   string
	RetryHint bool
	Detail    *FailureDetail
}

// ExecutionMetrics carries timing and cost facts for one attempt.
type ExecutionMetrics struct {
	Duration   time.Duration
	Cost       float64
	RetryCount int
}

// SafeDiagnostics carries persistence-safe diagnostic facts. Raw prompts,
// environment values, credentials, and command stdin are excluded.
type SafeDiagnostics struct {
	RenderedPrompt *SafeRenderedPromptDiagnostic
	Provider       *SafeProviderDiagnostic
	AgentRun       *SafeAgentRunDiagnostic
	Invocation     *InvocationDiagnostic
	Command        *SafeCommandDiagnostic
	Panic          *PanicDiagnostic
	Metadata       map[string]string
}

// SafeCommandDiagnostic records command identity and exit facts without stdin
// or environment values.
type SafeCommandDiagnostic struct {
	Command    string
	Args       []string
	Stdout     string
	Stderr     string
	ExitCode   int
	TimedOut   bool
	Duration   time.Duration
	WorkingDir string
}

// Validate reports typed pre-start validation failures.
func (request ExecuteRequest) Validate() error {
	if strings.TrimSpace(request.Correlation.DispatchID) == "" {
		return fmt.Errorf("%w: dispatch id is required", ErrInvalidExecuteRequest)
	}
	if strings.TrimSpace(request.Correlation.AttemptID) == "" {
		return fmt.Errorf("%w: attempt id is required", ErrInvalidExecuteRequest)
	}
	if strings.TrimSpace(request.Target.RunnerID) == "" &&
		strings.TrimSpace(request.Target.Provider.ID) == "" &&
		strings.TrimSpace(request.Target.Provider.Alias) == "" &&
		strings.TrimSpace(request.Target.Model.Name) == "" {
		return fmt.Errorf("%w: runner, provider, or model target is required", ErrInvalidExecuteRequest)
	}
	if request.Target.Timeout < 0 {
		return fmt.Errorf("%w: timeout must not be negative", ErrInvalidExecuteRequest)
	}
	if request.Input.Resume != nil {
		if strings.TrimSpace(request.Input.Resume.Provider) == "" &&
			strings.TrimSpace(request.Input.Resume.ProviderSessionID) == "" &&
			strings.TrimSpace(request.Input.Resume.ExternalRef) == "" {
			return fmt.Errorf("%w: resume continuation is empty", ErrInvalidExecuteRequest)
		}
	}
	return nil
}

// Clone returns a detached ExecuteRequest copy.
func (request ExecuteRequest) Clone() ExecuteRequest {
	clone := request
	clone.Target = request.Target.Clone()
	clone.Input = request.Input.Clone()
	return clone
}

// Clone returns a detached ExecutionTarget copy.
func (target ExecutionTarget) Clone() ExecutionTarget {
	clone := target
	clone.Tools.RequiredOptionalCapabilities = append(
		[]RunnerOptionalCapability(nil),
		target.Tools.RequiredOptionalCapabilities...,
	)
	clone.Environment.Vars = cloneStringMap(target.Environment.Vars)
	clone.Environment.ProcessEnvironment = append(
		[]string(nil),
		target.Environment.ProcessEnvironment...,
	)
	return clone
}

// Clone returns a detached ExecutionInput copy.
func (input ExecutionInput) Clone() ExecutionInput {
	clone := input
	if args := work.CloneInvocationArguments(&input.Invocation); args != nil {
		clone.Invocation = *args
	}
	clone.ModelBindings = CloneResolvedModelOperationBindings(input.ModelBindings)
	if len(input.Work) > 0 {
		clone.Work = make([]WorkInput, len(input.Work))
		for i, item := range input.Work {
			clone.Work[i] = item.Clone()
		}
	}
	if len(input.PreviousAttempts) > 0 {
		clone.PreviousAttempts = make([]AttemptSummary, len(input.PreviousAttempts))
		for i, summary := range input.PreviousAttempts {
			clone.PreviousAttempts[i] = summary.Clone()
		}
	}
	if input.Resume != nil {
		resume := *input.Resume
		clone.Resume = &resume
	}
	return clone
}

// Clone returns a detached WorkInput copy.
func (input WorkInput) Clone() WorkInput {
	clone := input
	clone.Content = work.CloneWorkContentParts(input.Content)
	clone.Tags = cloneStringMap(input.Tags)
	if len(input.Relations) > 0 {
		clone.Relations = append([]work.Relation(nil), input.Relations...)
	}
	return clone
}

// Clone returns a detached AttemptSummary copy.
func (summary AttemptSummary) Clone() AttemptSummary {
	clone := summary
	if summary.Failure != nil {
		failure := summary.Failure.Clone()
		clone.Failure = &failure
	}
	return clone
}

// Clone returns a detached ExecutionFailure copy.
func (failure ExecutionFailure) Clone() ExecutionFailure {
	clone := failure
	clone.Detail = CloneFailureDetail(failure.Detail)
	return clone
}

// Clone returns a detached ExecuteResult copy.
func (result ExecuteResult) Clone() ExecuteResult {
	clone := result
	clone.Output = result.Output.Clone()
	if result.Failure != nil {
		failure := result.Failure.Clone()
		clone.Failure = &failure
	}
	clone.Diagnostics = CloneSafeDiagnostics(result.Diagnostics)
	if result.Continuation != nil {
		continuation := *result.Continuation
		clone.Continuation = &continuation
	}
	return clone
}

// Clone returns a detached ProposedOutput copy.
func (output ProposedOutput) Clone() ProposedOutput {
	clone := output
	clone.Primary = work.CloneWorkContentParts(output.Primary)
	if len(output.ProposedWork) > 0 {
		clone.ProposedWork = make([]ProposedWork, len(output.ProposedWork))
		for i, item := range output.ProposedWork {
			clone.ProposedWork[i] = item.Clone()
		}
	}
	if len(output.ArtifactRefs) > 0 {
		clone.ArtifactRefs = append([]ArtifactRef(nil), output.ArtifactRefs...)
	}
	return clone
}

// Clone returns a detached ProposedWork copy.
func (item ProposedWork) Clone() ProposedWork {
	clone := item
	clone.Content = work.CloneWorkContentParts(item.Content)
	clone.Tags = cloneStringMap(item.Tags)
	if len(item.Relations) > 0 {
		clone.Relations = append([]work.Relation(nil), item.Relations...)
	}
	return clone
}

// CloneSafeDiagnostics returns a detached SafeDiagnostics copy.
func CloneSafeDiagnostics(diagnostics *SafeDiagnostics) *SafeDiagnostics {
	if diagnostics == nil {
		return nil
	}
	clone := &SafeDiagnostics{
		RenderedPrompt: cloneSafeRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneSafeProviderDiagnostic(diagnostics.Provider),
		AgentRun:       cloneSafeAgentRunDiagnostic(diagnostics.AgentRun),
		Invocation:     CloneInvocationDiagnostic(diagnostics.Invocation),
		Metadata:       cloneStringMap(diagnostics.Metadata),
	}
	if diagnostics.Command != nil {
		clone.Command = &SafeCommandDiagnostic{
			Command:    diagnostics.Command.Command,
			Args:       append([]string(nil), diagnostics.Command.Args...),
			Stdout:     diagnostics.Command.Stdout,
			Stderr:     diagnostics.Command.Stderr,
			ExitCode:   diagnostics.Command.ExitCode,
			TimedOut:   diagnostics.Command.TimedOut,
			Duration:   diagnostics.Command.Duration,
			WorkingDir: diagnostics.Command.WorkingDir,
		}
	}
	if diagnostics.Panic != nil {
		clone.Panic = &PanicDiagnostic{
			Message: diagnostics.Panic.Message,
			Stack:   diagnostics.Panic.Stack,
		}
	}
	return clone
}

func cloneSafeRenderedPromptDiagnostic(
	diagnostic *SafeRenderedPromptDiagnostic,
) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func cloneSafeProviderDiagnostic(diagnostic *SafeProviderDiagnostic) *SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func cloneSafeAgentRunDiagnostic(diagnostic *SafeAgentRunDiagnostic) *SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	clone := &SafeAgentRunDiagnostic{
		ExecutionBehavior: diagnostic.ExecutionBehavior,
		FailureClass:      diagnostic.FailureClass,
		RecoveryAction:    diagnostic.RecoveryAction,
		ToolPolicy:        diagnostic.ToolPolicy,
		ToolCallCount:     diagnostic.ToolCallCount,
	}
	if len(diagnostic.ToolDiagnostics) > 0 {
		clone.ToolDiagnostics = append([]AgentRunToolDiagnostic(nil), diagnostic.ToolDiagnostics...)
	}
	if len(diagnostic.Transcript) > 0 {
		clone.Transcript = append([]AgentRunTranscriptEntry(nil), diagnostic.Transcript...)
	}
	return clone
}
