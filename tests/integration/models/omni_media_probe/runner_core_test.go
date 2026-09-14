package omni_media_probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	ProbeInputSchemaV1  = "you.localai.omni-media-probe-input.v1"
	ProbeReportSchemaV1 = "you.localai.omni-media-probe-report.v1"

	ProbeNetworkPolicy = "declared-local-assets-and-loopback-only"
	ProbeForbiddenPort = 7437

	ProbeMaxHeavyProcesses        int64 = 1
	ProbeMaxCompilerTestProcesses int64 = 4
	ProbeMaxDiskBytes             int64 = 1 << 30
	ProbeMaxTimeoutSeconds        int64 = 30 * 60
	ProbeMaxCalls                 int64 = 2
	ProbeMaxRetries               int64 = 0

	probeInputMaxBytes = 1 << 20
	probeJourneyImage  = JourneyName("image")
	probeJourneyVideo  = JourneyName("video")
	probeJourneyCLI    = JourneyName("preflight")
)

const (
	CodeProbeInvalidJSON      ValidationCode = "probe_invalid_json"
	CodeProbeInvalidInput     ValidationCode = "probe_invalid_input"
	CodeProbeInvalidIdentity  ValidationCode = "probe_invalid_identity"
	CodeProbeMissingIdentity  ValidationCode = "probe_missing_identity"
	CodeProbeIdentityMismatch ValidationCode = "probe_identity_mismatch"
	CodeProbeInvalidRoot      ValidationCode = "probe_invalid_root"
	CodeProbeRootNotFresh     ValidationCode = "probe_root_not_fresh"
	CodeProbeInvalidReport    ValidationCode = "probe_invalid_report_path"
	CodeProbeHeavyOwnerBusy   ValidationCode = "probe_heavy_owner_busy"
	CodeProbeCancelled        ValidationCode = "probe_cancelled"
	CodeProbeTimedOut         ValidationCode = "probe_timed_out"
	CodeProbeExecutionFailure ValidationCode = "probe_execution_failure"
	CodeProbeCleanupFailure   ValidationCode = "probe_cleanup_failure"
	CodeProbeOutputFailure    ValidationCode = "probe_output_failure"
)

// JourneyName is the stable report and executor name for one OMNI modality.
type JourneyName string

// JourneyStatus records whether a controlled or real journey ran. NOT_RUN is
// intentional for video after an image admission or execution failure.
type JourneyStatus string

const (
	JourneyNotRun       JourneyStatus = "NOT_RUN"
	JourneyPass         JourneyStatus = "PASS"
	JourneyFail         JourneyStatus = "FAIL"
	JourneyInconclusive JourneyStatus = "INCONCLUSIVE"
)

// ProbeFileIdentity is an absolute, content-addressed input declared by the
// later real validator. Identity is a caller-facing label; pathIdentity is
// derived and is the only path form that may be persisted in a report.
type ProbeFileIdentity struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
}

// ProbeBuildIdentity omits Bytes in the input contract so a prebuilt artifact
// can be admitted by its path and digest, then have its observed size recorded
// in the report.
type ProbeBuildIdentity struct {
	Path     string `json:"path"`
	Identity string `json:"identity"`
	SHA256   string `json:"sha256"`
}

type ProbeDependencies struct {
	Model     ProbeFileIdentity `json:"model"`
	Projector ProbeFileIdentity `json:"projector"`
	Backend   ProbeFileIdentity `json:"backend"`
}

type ProbeLimits struct {
	TimeoutSeconds           int64   `json:"timeoutSeconds"`
	MaxHeavyProcesses        int64   `json:"maxHeavyProcesses"`
	MaxCompilerTestProcesses int64   `json:"maxCompilerTestProcesses"`
	MaxDiskBytes             int64   `json:"maxDiskBytes"`
	MaxDownloadBytes         int64   `json:"maxDownloadBytes"`
	MaxPaidUSD               float64 `json:"maxPaidUsd"`
	MaxCalls                 int64   `json:"maxCalls"`
	MaxRetries               int64   `json:"maxRetries"`
	ForbiddenPort            int     `json:"forbiddenPort"`
	NetworkPolicy            string  `json:"networkPolicy"`
}

// ProbeInput is the strict, artifact-parameterized admission declaration.
type ProbeInput struct {
	SchemaVersion   string             `json:"schemaVersion"`
	RunID           string             `json:"runId"`
	Build           ProbeBuildIdentity `json:"build"`
	Dependencies    ProbeDependencies  `json:"dependencies"`
	FixtureManifest ProbeFileIdentity  `json:"fixtureManifest"`
	ProbeRoot       string             `json:"probeRoot"`
	Journeys        []JourneyName      `json:"journeys"`
	Limits          ProbeLimits        `json:"limits"`
}

// Input is a short compatibility alias for callers that prefer the contract's
// noun without the Probe prefix.
type Input = ProbeInput

type RecordedIdentity struct {
	Identity     string `json:"identity"`
	PathIdentity string `json:"pathIdentity"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
}

type ProbeReportDependencies struct {
	Model     RecordedIdentity `json:"model"`
	Projector RecordedIdentity `json:"projector"`
	Backend   RecordedIdentity `json:"backend"`
}

type ProbePolicy struct {
	RootIdentities    []string `json:"rootIdentities"`
	Port              int      `json:"port"`
	TimeoutSeconds    int64    `json:"timeoutSeconds"`
	NetworkPolicy     string   `json:"networkPolicy"`
	DownloadBytes     int64    `json:"downloadBytes"`
	PaidUSD           float64  `json:"paidUsd"`
	MaxHeavyProcesses int64    `json:"maxHeavyProcesses"`
	MaxCalls          int64    `json:"maxCalls"`
	MaxRetries        int64    `json:"maxRetries"`
}

type RequestInput struct {
	Order     int    `json:"order"`
	Name      string `json:"name"`
	Modality  string `json:"modality"`
	MediaType string `json:"mediaType"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

type JourneyReport struct {
	Name           JourneyName     `json:"name"`
	Status         JourneyStatus   `json:"status"`
	Command        []string        `json:"command"`
	RequestInputs  []RequestInput  `json:"requestInputs"`
	SemanticRubric json.RawMessage `json:"semanticRubric"`
	Failure        *ReportFailure  `json:"failure"`
}

type ProcessEvidence struct {
	Identity string `json:"identity"`
	Kind     string `json:"kind"`
	PID      int    `json:"pid"`
	Owner    string `json:"owner"`
	Started  bool   `json:"started"`
	Exited   bool   `json:"exited"`
	ExitCode int    `json:"exitCode"`
	TimedOut bool   `json:"timedOut"`
}

type CleanupEvidence struct {
	Checked                bool `json:"checked"`
	OwnedProcessSurvivors  int  `json:"ownedProcessSurvivors"`
	OwnedListenerSurvivors int  `json:"ownedListenerSurvivors"`
	PartialOutputs         int  `json:"partialOutputs"`
}

type ReportFailure struct {
	Owner      string `json:"owner"`
	Code       string `json:"code"`
	Expected   string `json:"expected"`
	Observed   string `json:"observed"`
	NextAction string `json:"nextAction"`
}

type ProbeReport struct {
	SchemaVersion string                  `json:"schemaVersion"`
	RunID         string                  `json:"runId"`
	Status        string                  `json:"status"`
	Build         RecordedIdentity        `json:"build"`
	Dependencies  ProbeReportDependencies `json:"dependencies"`
	Fixtures      []RecordedIdentity      `json:"fixtures"`
	Policy        ProbePolicy             `json:"policy"`
	Journeys      []JourneyReport         `json:"journeys"`
	Outputs       []RecordedIdentity      `json:"outputs"`
	Processes     []ProcessEvidence       `json:"processes"`
	Cleanup       CleanupEvidence         `json:"cleanup"`
	Failure       *ReportFailure          `json:"failure"`
}

// Report is a short compatibility alias for the persisted evidence contract.
type Report = ProbeReport

// RootPaths are fresh, caller-owned directories supplied to an Executor. The
// paths never enter the persisted report.
type RootPaths struct {
	Work      string
	Profile   string
	Cache     string
	Model     string
	Projector string
	Backend   string
	Output    string
	Streams   string
}

type ExecutionRequest struct {
	Journey JourneyName
	Command []string
	Inputs  []RequestInput
	Roots   RootPaths
	Port    int
}

type ExecutionObservation struct {
	Process                ProcessEvidence
	Outputs                []RecordedIdentity
	Failure                *ReportFailure
	TimedOut               bool
	Cancelled              bool
	OwnedProcessSurvivors  int
	OwnedListenerSurvivors int
	PartialOutputs         int
}

// Executor is the only boundary capable of starting a future prebuilt CLI or
// real dependency. A nil Executor performs preparation-only admission and
// never starts a process, model, projector, backend, or download.
type Executor interface {
	Execute(context.Context, ExecutionRequest) (ExecutionObservation, error)
}

// Runner owns only test-only OMNI preparation state. Heavy ownership is
// process-wide so independent Runner values cannot overlap real admissions.
type Runner struct {
	Executor Executor
}

type probeCallBudget struct {
	maxCalls int64
	calls    int64
}

func (budget *probeCallBudget) reserve() error {
	budget.calls++
	if budget.calls > budget.maxCalls {
		return validationError(CodeProbeExecutionFailure, "limits.maxCalls", fmt.Sprintf("at most %d executor calls", budget.maxCalls), fmt.Sprintf("executor call %d", budget.calls), nil)
	}
	return nil
}

func (runner Runner) execute(ctx context.Context, request ExecutionRequest, budget *probeCallBudget) (ExecutionObservation, error) {
	if err := budget.reserve(); err != nil {
		return ExecutionObservation{}, err
	}
	return runner.Executor.Execute(ctx, request)
}

var probeHeavyOwner = make(chan struct{}, 1)

func NewRunner(executor Executor) Runner { return Runner{Executor: executor} }

// Preflight is the no-effect convenience entry point used by the preparation
// lane and later by the one-build integration handoff.
func Preflight(ctx context.Context, inputPath, reportPath string) (Report, error) {
	return NewRunner(nil).Run(ctx, inputPath, reportPath)
}

func Run(ctx context.Context, inputPath, reportPath string, executor Executor) (Report, error) {
	return NewRunner(executor).Run(ctx, inputPath, reportPath)
}

func (runner Runner) Run(ctx context.Context, inputPath, reportPath string) (Report, error) {
	input, err := ReadProbeInput(inputPath)
	if err != nil {
		return Report{}, err
	}
	return runner.RunInput(ctx, input, reportPath)
}

func (runner Runner) RunInput(ctx context.Context, input ProbeInput, reportPath string) (Report, error) {
	return runner.runInput(ctx, input, reportPath, probeRunJourneys)
}

// RunPreflight executes one supplied prebuilt-process check after the same
// strict admission and isolated-root setup as the full probe. The executor
// receives a --version-shaped request, while the report intentionally remains
// READY with both semantic journeys NOT_RUN: this boundary proves executable
// selection and process cleanup, not OMNI inference.
func (runner Runner) RunPreflight(ctx context.Context, inputPath, reportPath string) (Report, error) {
	input, err := ReadProbeInput(inputPath)
	if err != nil {
		return Report{}, err
	}
	return runner.RunPreflightInput(ctx, input, reportPath)
}

func (runner Runner) RunPreflightInput(ctx context.Context, input ProbeInput, reportPath string) (Report, error) {
	return runner.runInput(ctx, input, reportPath, probeRunPrebuiltCLI)
}

type probeRunMode uint8

const (
	probeRunJourneys probeRunMode = iota
	probeRunPrebuiltCLI
)

func (runner Runner) runInput(ctx context.Context, input ProbeInput, reportPath string, mode probeRunMode) (Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if mode == probeRunPrebuiltCLI && runner.Executor == nil {
		return Report{}, validationError(CodeProbeExecutionFailure, "executor", "prebuilt CLI executor", "nil", nil)
	}
	prepared, err := admitProbeInput(ctx, input, reportPath)
	if err != nil {
		return Report{}, err
	}
	if err := ctx.Err(); err != nil {
		return Report{}, probeContextError(err)
	}

	release, err := reserveProbeHeavyOwner(ctx)
	if err != nil {
		return Report{}, err
	}
	defer release()

	runContext, cancel := context.WithTimeout(ctx, time.Duration(input.Limits.TimeoutSeconds)*time.Second)
	defer cancel()
	roots, err := createProbeRoots(input.ProbeRoot, prepared.reportPath)
	if err != nil {
		return Report{}, err
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = cleanupProbeRoots(roots, prepared.reportPath)
		}
	}()

	listener, port, err := reserveProbePort(input.Limits.ForbiddenPort)
	if err != nil {
		return Report{}, fmt.Errorf("reserve OMNI probe port: %w", err)
	}
	roots.Port = port
	report, err := newProbeReport(prepared, roots, port)
	if err != nil {
		_ = listener.Close()
		return Report{}, err
	}
	budget := &probeCallBudget{maxCalls: input.Limits.MaxCalls}

	if runner.Executor == nil {
		report.Status = "READY"
	} else if mode == probeRunPrebuiltCLI {
		outcome := runner.executePrebuiltCLI(runContext, roots, budget)
		report.Processes = append(report.Processes, outcome.process)
		report.Outputs = append(report.Outputs, outcome.outputs...)
		if outcome.failure != nil {
			report.Failure = outcome.failure
			if outcome.failure.Code == string(CodeProbeCancelled) || outcome.failure.Code == string(CodeProbeTimedOut) {
				report.Status = "INCONCLUSIVE"
			} else {
				report.Status = "FAIL"
			}
		} else {
			report.Status = "READY"
		}
		if outcome.cleanupFailure != nil {
			listenerErr := listener.Close()
			cleanupErr := cleanupProbeRoots(roots, prepared.reportPath)
			cleaned = true
			return report, errors.Join(
				validationError(CodeProbeCleanupFailure, "cleanup", "zero owned survivors", outcome.cleanupFailure.Observed, nil),
				listenerErr,
				cleanupErr,
			)
		}
	} else {
		executionErr := runner.executeJourneys(runContext, &report, roots, budget)
		if executionErr != nil {
			listenerErr := listener.Close()
			cleanupErr := cleanupProbeRoots(roots, prepared.reportPath)
			cleaned = true
			return report, errors.Join(executionErr, listenerErr, cleanupErr)
		}
	}

	listenerErr := listener.Close()
	cleanupErr := cleanupProbeRoots(roots, prepared.reportPath)
	cleaned = true
	if listenerErr != nil || cleanupErr != nil {
		return report, errors.Join(
			wrapProbeCleanupError("close loopback listener", listenerErr),
			wrapProbeCleanupError("remove owned probe roots", cleanupErr),
		)
	}
	report.Cleanup = CleanupEvidence{Checked: true}
	if err := report.Validate(); err != nil {
		return report, fmt.Errorf("validate OMNI probe report: %w", err)
	}
	if err := WriteReportAtomic(prepared.reportPath, report); err != nil {
		return report, fmt.Errorf("persist OMNI probe report: %w", err)
	}
	return report, nil
}

func (runner Runner) executeJourneys(ctx context.Context, report *ProbeReport, roots probeRoots, budget *probeCallBudget) error {
	for index := range report.Journeys {
		if index == 1 && report.Journeys[0].Status != JourneyPass {
			continue
		}
		outcome := runner.executeJourney(ctx, &report.Journeys[index], roots, budget)
		report.Processes = append(report.Processes, outcome.process)
		report.Outputs = append(report.Outputs, outcome.outputs...)
		if outcome.failure != nil && report.Failure == nil {
			report.Failure = outcome.failure
		}
		if outcome.cleanupFailure != nil && report.Failure == nil {
			report.Failure = outcome.cleanupFailure
		}
		if outcome.cleanupFailure != nil {
			return fmt.Errorf("journey %s cleanup was not proven: %w", report.Journeys[index].Name, validationError(CodeProbeCleanupFailure, "cleanup", "zero owned survivors", outcome.cleanupFailure.Observed, nil))
		}
		if report.Journeys[index].Status != JourneyPass {
			if report.Journeys[index].Status == JourneyInconclusive {
				report.Status = "INCONCLUSIVE"
			} else {
				report.Status = "FAIL"
			}
			return nil
		}
	}
	report.Status = "PASS"
	return nil
}

type journeyOutcome struct {
	process        ProcessEvidence
	outputs        []RecordedIdentity
	failure        *ReportFailure
	cleanupFailure *ReportFailure
}

func (runner Runner) executePrebuiltCLI(ctx context.Context, roots probeRoots, budget *probeCallBudget) journeyOutcome {
	request := ExecutionRequest{
		Journey: probeJourneyCLI, Command: []string{"--version"}, Roots: roots.Paths, Port: roots.Port,
	}
	observation, err := runner.execute(ctx, request, budget)
	process := observation.Process
	if process.Identity == "" {
		process.Identity = "not-started-preflight"
	}
	if process.Kind == "" {
		process.Kind = "prebuilt-cli"
	}
	if process.Owner == "" {
		process.Owner = "omni-media-probe"
	}
	if err != nil {
		failure := probeFailure("executor", string(CodeProbeExecutionFailure), "prebuilt CLI returns a bounded observation", "executor returned an error", "inspect the prebuilt CLI process evidence")
		return journeyOutcome{process: process, failure: failure}
	}
	if observation.OwnedProcessSurvivors != 0 || observation.OwnedListenerSurvivors != 0 || observation.PartialOutputs != 0 {
		failure := probeFailure("harness", string(CodeProbeCleanupFailure), "owned processes, listeners, and partial outputs are zero", "owned resource survivors were reported", "repair prebuilt CLI cleanup before retrying")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: failure, cleanupFailure: failure}
	}
	if observation.TimedOut || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		process.TimedOut = true
		failure := probeFailure("harness", string(CodeProbeTimedOut), "prebuilt CLI finishes within the declared timeout", "prebuilt CLI timed out", "inspect the bounded timeout evidence and retry the handoff")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: failure}
	}
	if observation.Cancelled || errors.Is(ctx.Err(), context.Canceled) {
		failure := probeFailure("harness", string(CodeProbeCancelled), "prebuilt CLI completes before cancellation", "prebuilt CLI was cancelled", "inspect cleanup evidence before retrying")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: failure}
	}
	if observation.Failure != nil {
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: normalizeReportFailure(*observation.Failure, "executor")}
	}
	if !process.Started || !process.Exited || process.ExitCode != 0 {
		failure := probeFailure("executor", string(CodeProbeExecutionFailure), "started prebuilt CLI exits with code zero", "prebuilt CLI process observation was not successful", "inspect process evidence before retrying")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: failure}
	}
	for index, output := range observation.Outputs {
		if err := validateRecordedIdentity(output, fmt.Sprintf("outputs[%d]", index), false); err != nil {
			failure := probeFailure("harness", string(CodeProbeOutputFailure), "output identity is complete and redacted", "output identity was invalid", "repair output recording before retrying")
			return journeyOutcome{process: process, failure: failure}
		}
	}
	return journeyOutcome{process: process, outputs: observation.Outputs}
}

func (runner Runner) executeJourney(ctx context.Context, journey *JourneyReport, roots probeRoots, budget *probeCallBudget) journeyOutcome {
	request := ExecutionRequest{
		Journey: journey.Name, Command: append([]string(nil), journey.Command...),
		Inputs: append([]RequestInput(nil), journey.RequestInputs...), Roots: roots.Paths,
		Port: roots.Port,
	}
	return runner.executeJourneyRequest(ctx, journey, request, budget)
}

func (runner Runner) executeJourneyRequest(ctx context.Context, journey *JourneyReport, request ExecutionRequest, budget *probeCallBudget) journeyOutcome {
	observation, err := runner.execute(ctx, request, budget)
	process := observation.Process
	if process.Identity == "" {
		process.Identity = "not-started-" + string(journey.Name)
	}
	if process.Kind == "" {
		process.Kind = "controlled-executor"
	}
	if process.Owner == "" {
		process.Owner = "omni-media-probe"
	}
	if err != nil {
		journey.Status = JourneyFail
		journey.Failure = probeFailure("executor", string(CodeProbeExecutionFailure), "executor returns a bounded observation", "executor returned an error", "inspect the controlled or real executor evidence")
		return journeyOutcome{process: process, failure: journey.Failure}
	}
	if observation.OwnedProcessSurvivors != 0 || observation.OwnedListenerSurvivors != 0 || observation.PartialOutputs != 0 {
		journey.Status = JourneyFail
		failure := probeFailure("harness", string(CodeProbeCleanupFailure), "owned processes, listeners, and partial outputs are zero", "owned resource survivors were reported", "repair executor cleanup before retrying")
		journey.Failure = failure
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: failure, cleanupFailure: failure}
	}
	if observation.TimedOut || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		journey.Status = JourneyInconclusive
		journey.Failure = probeFailure("harness", string(CodeProbeTimedOut), "journey finishes within the declared timeout", "journey timed out", "inspect the bounded timeout evidence and retry the real gate")
		process.TimedOut = true
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	if observation.Cancelled || errors.Is(ctx.Err(), context.Canceled) {
		journey.Status = JourneyInconclusive
		journey.Failure = probeFailure("harness", string(CodeProbeCancelled), "journey completes before cancellation", "journey was cancelled", "inspect cleanup evidence before retrying")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	if observation.Failure != nil {
		journey.Status = JourneyFail
		journey.Failure = normalizeReportFailure(*observation.Failure, "executor")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	if !process.Started || !process.Exited || process.ExitCode != 0 {
		journey.Status = JourneyFail
		journey.Failure = probeFailure("executor", string(CodeProbeExecutionFailure), "started process exits with code zero", "process observation was not successful", "inspect process evidence before retrying")
		return journeyOutcome{process: process, outputs: observation.Outputs, failure: journey.Failure}
	}
	for index, output := range observation.Outputs {
		if err := validateRecordedIdentity(output, fmt.Sprintf("outputs[%d]", index), false); err != nil {
			journey.Status = JourneyFail
			journey.Failure = probeFailure("harness", string(CodeProbeOutputFailure), "output identity is complete and redacted", "output identity was invalid", "repair output recording before retrying")
			return journeyOutcome{process: process, failure: journey.Failure}
		}
	}
	journey.Status = JourneyPass
	return journeyOutcome{process: process, outputs: observation.Outputs}
}
