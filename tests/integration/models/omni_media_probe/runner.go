package omni_media_probe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/filesystem"
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

	if runner.Executor == nil {
		report.Status = "READY"
	} else if mode == probeRunPrebuiltCLI {
		outcome := runner.executePrebuiltCLI(runContext, roots)
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
		executionErr := runner.executeJourneys(runContext, &report, roots)
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

func ReadProbeInput(path string) (ProbeInput, error) {
	path, err := normalizeProbePath(path, "input path")
	if err != nil {
		return ProbeInput{}, err
	}
	data, err := readProbeJSON(path, probeInputMaxBytes)
	if err != nil {
		return ProbeInput{}, fmt.Errorf("read OMNI probe input: %w", err)
	}
	var input ProbeInput
	if err := decodeStrictJSON(data, &input); err != nil {
		var typed *ValidationError
		if errors.As(err, &typed) {
			return ProbeInput{}, err
		}
		if strings.Contains(err.Error(), "json: unknown field") {
			return ProbeInput{}, validationError(CodeUnknownField, "$", "known probe input fields", "unknown field", err)
		}
		return ProbeInput{}, validationError(CodeProbeInvalidJSON, "$", "one strict probe input JSON value", "invalid", err)
	}
	if err := input.Validate(); err != nil {
		return ProbeInput{}, err
	}
	return input, nil
}

func WriteProbeInputAtomic(path string, input ProbeInput) error {
	if err := input.Validate(); err != nil {
		return fmt.Errorf("validate OMNI probe input: %w", err)
	}
	body, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return fmt.Errorf("encode OMNI probe input: %w", err)
	}
	body = append(body, '\n')
	return writeProbeJSONAtomic(path, body)
}

func WriteProbeInput(path string, input ProbeInput) error {
	return WriteProbeInputAtomic(path, input)
}

func (input ProbeInput) Validate() error { return input.validateShape() }

func (input ProbeInput) validateShape() error {
	if input.SchemaVersion != ProbeInputSchemaV1 {
		return validationError(CodeProbeInvalidInput, "schemaVersion", ProbeInputSchemaV1, input.SchemaVersion, nil)
	}
	if err := validateProbeLabel(input.RunID, "runId"); err != nil {
		return err
	}
	if err := validateBuildShape(input.Build); err != nil {
		return err
	}
	for name, identity := range map[string]ProbeFileIdentity{
		"dependencies.model":     input.Dependencies.Model,
		"dependencies.projector": input.Dependencies.Projector,
		"dependencies.backend":   input.Dependencies.Backend,
		"fixtureManifest":        input.FixtureManifest,
	} {
		if err := validateFileIdentityShape(identity, name); err != nil {
			return err
		}
	}
	if err := validateAbsolutePathShape(input.ProbeRoot, "probeRoot"); err != nil {
		return err
	}
	if len(input.Journeys) != 2 || input.Journeys[0] != probeJourneyImage || input.Journeys[1] != probeJourneyVideo {
		return validationError(CodeProbeInvalidInput, "journeys", "[image, video]", fmt.Sprint(input.Journeys), nil)
	}
	return validateProbeLimits(input.Limits)
}

func ReadReport(path string) (Report, error) {
	path, err := normalizeProbePath(path, "report path")
	if err != nil {
		return Report{}, err
	}
	data, err := readProbeJSON(path, probeInputMaxBytes)
	if err != nil {
		return Report{}, fmt.Errorf("read OMNI probe report: %w", err)
	}
	var report Report
	if err := decodeStrictJSON(data, &report); err != nil {
		var typed *ValidationError
		if errors.As(err, &typed) {
			return Report{}, err
		}
		if strings.Contains(err.Error(), "json: unknown field") {
			return Report{}, validationError(CodeUnknownField, "$", "known probe report fields", "unknown field", err)
		}
		return Report{}, validationError(CodeProbeInvalidJSON, "$", "one strict probe report JSON value", "invalid", err)
	}
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report, nil
}

func WriteReportAtomic(path string, report Report) error {
	if err := report.Validate(); err != nil {
		return fmt.Errorf("validate OMNI probe report: %w", err)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode OMNI probe report: %w", err)
	}
	body = append(body, '\n')
	return writeProbeJSONAtomic(path, body)
}

func WriteReport(path string, report Report) error { return WriteReportAtomic(path, report) }

func (report ProbeReport) Validate() error {
	if report.SchemaVersion != ProbeReportSchemaV1 {
		return validationError(CodeProbeInvalidReport, "schemaVersion", ProbeReportSchemaV1, report.SchemaVersion, nil)
	}
	if err := validateProbeLabel(report.RunID, "runId"); err != nil {
		return err
	}
	if report.Status != "READY" && report.Status != "PASS" && report.Status != "FAIL" && report.Status != "INCONCLUSIVE" {
		return validationError(CodeProbeInvalidReport, "status", "READY, PASS, FAIL, or INCONCLUSIVE", report.Status, nil)
	}
	for name, identity := range map[string]RecordedIdentity{
		"build":                  report.Build,
		"dependencies.model":     report.Dependencies.Model,
		"dependencies.projector": report.Dependencies.Projector,
		"dependencies.backend":   report.Dependencies.Backend,
	} {
		if err := validateRecordedIdentity(identity, name, true); err != nil {
			return err
		}
	}
	if len(report.Fixtures) != 2 {
		return validationError(CodeProbeInvalidReport, "fixtures", "exactly two fixture identities", fmt.Sprint(len(report.Fixtures)), nil)
	}
	for index, fixture := range report.Fixtures {
		if err := validateRecordedIdentity(fixture, fmt.Sprintf("fixtures[%d]", index), true); err != nil {
			return err
		}
	}
	if err := validateProbePolicy(report.Policy); err != nil {
		return err
	}
	if len(report.Journeys) != 2 || report.Journeys[0].Name != probeJourneyImage || report.Journeys[1].Name != probeJourneyVideo {
		return validationError(CodeProbeInvalidReport, "journeys", "ordered image and video journeys", fmt.Sprint(report.Journeys), nil)
	}
	for index := range report.Journeys {
		if err := validateJourneyReport(report.Journeys[index], index); err != nil {
			return err
		}
	}
	for index, output := range report.Outputs {
		if err := validateRecordedIdentity(output, fmt.Sprintf("outputs[%d]", index), false); err != nil {
			return err
		}
	}
	for index, process := range report.Processes {
		if err := validateProcessEvidence(process, index); err != nil {
			return err
		}
	}
	if !report.Cleanup.Checked || report.Cleanup.OwnedProcessSurvivors != 0 || report.Cleanup.OwnedListenerSurvivors != 0 || report.Cleanup.PartialOutputs != 0 {
		return validationError(CodeProbeInvalidReport, "cleanup", "checked cleanup with zero survivors and partial outputs", fmt.Sprintf("%+v", report.Cleanup), nil)
	}
	if report.Failure != nil {
		if err := validateReportFailure(*report.Failure, "failure"); err != nil {
			return err
		}
	}
	if err := validateReportState(report); err != nil {
		return err
	}
	if err := validateReportRedaction(report); err != nil {
		return err
	}
	return nil
}

type admittedProbeInput struct {
	input        ProbeInput
	reportPath   string
	build        RecordedIdentity
	dependencies ProbeReportDependencies
	manifest     Manifest
}

func admitProbeInput(ctx context.Context, input ProbeInput, reportPath string) (admittedProbeInput, error) {
	if err := input.Validate(); err != nil {
		return admittedProbeInput{}, err
	}
	reportPath, err := normalizeProbePath(reportPath, "report path")
	if err != nil {
		return admittedProbeInput{}, validationError(CodeProbeInvalidReport, "reportPath", "absolute clean report path", reportPath, err)
	}
	if err := validateReportDestination(reportPath, input.ProbeRoot); err != nil {
		return admittedProbeInput{}, err
	}
	if err := ctx.Err(); err != nil {
		return admittedProbeInput{}, probeContextError(err)
	}

	build, err := admitBuildIdentity(input.Build)
	if err != nil {
		return admittedProbeInput{}, err
	}
	dependencies := ProbeReportDependencies{}
	for name, declared := range map[string]ProbeFileIdentity{
		"model":     input.Dependencies.Model,
		"projector": input.Dependencies.Projector,
		"backend":   input.Dependencies.Backend,
	} {
		observed, observeErr := admitFileIdentity(declared, "dependencies."+name)
		if observeErr != nil {
			return admittedProbeInput{}, observeErr
		}
		switch name {
		case "model":
			dependencies.Model = observed
		case "projector":
			dependencies.Projector = observed
		case "backend":
			dependencies.Backend = observed
		}
	}
	manifestIdentity, err := admitFileIdentity(input.FixtureManifest, "fixtureManifest")
	if err != nil {
		return admittedProbeInput{}, err
	}
	manifest, err := LoadManifest(input.FixtureManifest.Path)
	if err != nil {
		return admittedProbeInput{}, fmt.Errorf("admit fixture manifest: %w", err)
	}
	if filepath.Clean(manifest.ManifestPath) != filepath.Clean(input.FixtureManifest.Path) || manifestIdentity.Bytes <= 0 {
		return admittedProbeInput{}, validationError(CodeProbeIdentityMismatch, "fixtureManifest.path", input.FixtureManifest.Path, manifest.ManifestPath, nil)
	}
	if err := validateProbeRoot(input.ProbeRoot); err != nil {
		return admittedProbeInput{}, err
	}
	if err := ensureDistinctDeclaredPaths(input); err != nil {
		return admittedProbeInput{}, err
	}
	return admittedProbeInput{input: input, reportPath: reportPath, build: build, dependencies: dependencies, manifest: manifest}, nil
}

func validateBuildShape(build ProbeBuildIdentity) error {
	if err := validateAbsolutePathShape(build.Path, "build.path"); err != nil {
		return err
	}
	if err := validateProbeLabel(build.Identity, "build.identity"); err != nil {
		return err
	}
	return validateSHA256(build.SHA256, "build.sha256")
}

func validateFileIdentityShape(identity ProbeFileIdentity, field string) error {
	if err := validateAbsolutePathShape(identity.Path, field+".path"); err != nil {
		return err
	}
	if err := validateProbeLabel(identity.Identity, field+".identity"); err != nil {
		return err
	}
	if identity.Bytes <= 0 {
		return validationError(CodeProbeInvalidIdentity, field+".bytes", "positive byte count", fmt.Sprint(identity.Bytes), nil)
	}
	return validateSHA256(identity.SHA256, field+".sha256")
}

func validateAbsolutePathShape(path, field string) error {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return validationError(CodeProbeInvalidIdentity, field, "absolute clean path", path, nil)
	}
	if filepath.IsAbs(filepath.VolumeName(path)) && filepath.Clean(path) == filepath.VolumeName(path) {
		return validationError(CodeProbeInvalidIdentity, field, "non-root absolute path", path, nil)
	}
	return nil
}

func validateProbeLabel(value, field string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, 0) || len(value) > 256 {
		return validationError(CodeProbeInvalidInput, field, "non-empty bounded label", value, nil)
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return validationError(CodeProbeInvalidInput, field, "printable label", "control character", nil)
		}
	}
	return nil
}

func validateSHA256(value, field string) error {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return validationError(CodeProbeInvalidIdentity, field, "lowercase SHA-256", value, nil)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return validationError(CodeProbeInvalidIdentity, field, "lowercase SHA-256", value, err)
	}
	return nil
}

func validateProbeLimits(limits ProbeLimits) error {
	if limits.TimeoutSeconds < 1 || limits.TimeoutSeconds > ProbeMaxTimeoutSeconds {
		return validationError(CodeProbeInvalidInput, "limits.timeoutSeconds", fmt.Sprintf("1..%d", ProbeMaxTimeoutSeconds), fmt.Sprint(limits.TimeoutSeconds), nil)
	}
	if limits.MaxHeavyProcesses != ProbeMaxHeavyProcesses {
		return validationError(CodeProbeInvalidInput, "limits.maxHeavyProcesses", "1", fmt.Sprint(limits.MaxHeavyProcesses), nil)
	}
	if limits.MaxCompilerTestProcesses != ProbeMaxCompilerTestProcesses {
		return validationError(CodeProbeInvalidInput, "limits.maxCompilerTestProcesses", "4", fmt.Sprint(limits.MaxCompilerTestProcesses), nil)
	}
	if limits.MaxDiskBytes <= 0 || limits.MaxDiskBytes > ProbeMaxDiskBytes {
		return validationError(CodeProbeInvalidInput, "limits.maxDiskBytes", fmt.Sprintf("1..%d", ProbeMaxDiskBytes), fmt.Sprint(limits.MaxDiskBytes), nil)
	}
	if limits.MaxDownloadBytes != 0 {
		return validationError(CodeProbeInvalidInput, "limits.maxDownloadBytes", "0", fmt.Sprint(limits.MaxDownloadBytes), nil)
	}
	if limits.MaxPaidUSD != 0 {
		return validationError(CodeProbeInvalidInput, "limits.maxPaidUsd", "0", fmt.Sprint(limits.MaxPaidUSD), nil)
	}
	if limits.ForbiddenPort != ProbeForbiddenPort {
		return validationError(CodeProbeInvalidInput, "limits.forbiddenPort", fmt.Sprint(ProbeForbiddenPort), fmt.Sprint(limits.ForbiddenPort), nil)
	}
	if limits.NetworkPolicy != ProbeNetworkPolicy {
		return validationError(CodeProbeInvalidInput, "limits.networkPolicy", ProbeNetworkPolicy, limits.NetworkPolicy, nil)
	}
	return nil
}

func validateProbeRoot(path string) error {
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return validationError(CodeProbeInvalidRoot, "probeRoot", "fresh path without symlink components", path, err)
	}
	info, err := os.Lstat(path)
	if err == nil {
		return validationError(CodeProbeRootNotFresh, "probeRoot", "non-existent fresh directory", info.Mode().String(), nil)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return validationError(CodeProbeInvalidRoot, "probeRoot", "inspectable fresh path", path, err)
	}
	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil || !parentInfo.IsDir() {
		return validationError(CodeProbeInvalidRoot, "probeRoot", "existing parent directory", filepath.Dir(path), err)
	}
	return nil
}

func validateReportDestination(path, probeRoot string) error {
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return validationError(CodeProbeInvalidReport, "reportPath", "path without symlink components", path, err)
	}
	if filepath.Clean(path) == filepath.Clean(probeRoot) {
		return validationError(CodeProbeInvalidReport, "reportPath", "report file distinct from probe root", path, nil)
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return validationError(CodeProbeInvalidReport, "reportPath", "absent or regular report file", info.Mode().String(), nil)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return validationError(CodeProbeInvalidReport, "reportPath", "inspectable report destination", path, err)
	}
	for _, name := range []string{"work", "profile", "cache", "model", "projector", "backend", "output", "streams"} {
		if pathWithin(filepath.Join(probeRoot, name), path) {
			return validationError(CodeProbeInvalidReport, "reportPath", "report outside owned resource roots", path, nil)
		}
	}
	return nil
}

func ensureDistinctDeclaredPaths(input ProbeInput) error {
	seen := map[string]string{}
	add := func(field, path string) error {
		clean := filepath.Clean(path)
		if previous, exists := seen[clean]; exists {
			return validationError(CodeProbeInvalidIdentity, field+".path", "unique declared path", previous, nil)
		}
		seen[clean] = field
		return nil
	}
	if err := add("build", input.Build.Path); err != nil {
		return err
	}
	for field, identity := range map[string]ProbeFileIdentity{
		"dependencies.model":     input.Dependencies.Model,
		"dependencies.projector": input.Dependencies.Projector,
		"dependencies.backend":   input.Dependencies.Backend,
		"fixtureManifest":        input.FixtureManifest,
	} {
		if err := add(field, identity.Path); err != nil {
			return err
		}
	}
	return nil
}

func admitBuildIdentity(build ProbeBuildIdentity) (RecordedIdentity, error) {
	info, digest, err := inspectDeclaredFile(build.Path, "build")
	if err != nil {
		return RecordedIdentity{}, err
	}
	if err := validateExecutable(build.Path, info); err != nil {
		return RecordedIdentity{}, err
	}
	if digest != build.SHA256 {
		return RecordedIdentity{}, validationError(CodeProbeIdentityMismatch, "build.sha256", build.SHA256, digest, nil)
	}
	return RecordedIdentity{Identity: build.Identity, PathIdentity: pathIdentity(build.Path), Bytes: info.Size(), SHA256: digest}, nil
}

func admitFileIdentity(declared ProbeFileIdentity, field string) (RecordedIdentity, error) {
	info, digest, err := inspectDeclaredFile(declared.Path, field)
	if err != nil {
		return RecordedIdentity{}, err
	}
	if info.Size() != declared.Bytes {
		return RecordedIdentity{}, validationError(CodeProbeIdentityMismatch, field+".bytes", fmt.Sprint(declared.Bytes), fmt.Sprint(info.Size()), nil)
	}
	if digest != declared.SHA256 {
		return RecordedIdentity{}, validationError(CodeProbeIdentityMismatch, field+".sha256", declared.SHA256, digest, nil)
	}
	return RecordedIdentity{Identity: declared.Identity, PathIdentity: pathIdentity(declared.Path), Bytes: info.Size(), SHA256: digest}, nil
}

func inspectDeclaredFile(path, field string) (os.FileInfo, string, error) {
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return nil, "", validationError(CodeProbeInvalidIdentity, field+".path", "non-aliased regular file", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		code := CodeProbeMissingIdentity
		if !errors.Is(err, os.ErrNotExist) {
			code = CodeProbeInvalidIdentity
		}
		return nil, "", validationError(code, field+".path", "existing regular file", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, "", validationError(CodeProbeInvalidIdentity, field+".path", "nonempty regular file", info.Mode().String(), nil)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", validationError(CodeProbeInvalidIdentity, field+".path", "readable regular file", path, err)
	}
	digest := sha256.New()
	_, copyErr := io.Copy(digest, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return nil, "", validationError(CodeProbeInvalidIdentity, field+".path", "readable stable file", path, errors.Join(copyErr, closeErr))
	}
	finalInfo, err := os.Lstat(path)
	if err != nil || finalInfo.Size() != info.Size() {
		return nil, "", validationError(CodeProbeIdentityMismatch, field+".path", "stable file size", fmt.Sprint(info.Size()), err)
	}
	return info, hex.EncodeToString(digest.Sum(nil)), nil
}

func validateExecutable(path string, info os.FileInfo) error {
	if runtime.GOOS == "windows" {
		if strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}
		return validationError(CodeProbeInvalidIdentity, "build.path", "Windows executable with .exe suffix", filepath.Ext(path), nil)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return validationError(CodeProbeInvalidIdentity, "build.path", "executable file mode", info.Mode().String(), nil)
	}
	return nil
}

func reserveProbeHeavyOwner(ctx context.Context) (func(), error) {
	select {
	case probeHeavyOwner <- struct{}{}:
		return func() { <-probeHeavyOwner }, nil
	case <-ctx.Done():
		return nil, probeContextError(ctx.Err())
	default:
		return nil, validationError(CodeProbeHeavyOwnerBusy, "limits.maxHeavyProcesses", "one available heavy owner", "another OMNI probe owns it", nil)
	}
}

type probeRoots struct {
	Root  string
	Paths RootPaths
	Owned []string
	Port  int
}

func createProbeRoots(root, reportPath string) (probeRoots, error) {
	paths := RootPaths{
		Work: filepath.Join(root, "work"), Profile: filepath.Join(root, "profile"),
		Cache: filepath.Join(root, "cache"), Model: filepath.Join(root, "model"),
		Projector: filepath.Join(root, "projector"), Backend: filepath.Join(root, "backend"),
		Output: filepath.Join(root, "output"), Streams: filepath.Join(root, "streams"),
	}
	owned := []string{paths.Work, paths.Profile, paths.Cache, paths.Model, paths.Projector, paths.Backend, paths.Output, paths.Streams}
	if err := os.Mkdir(root, 0o700); err != nil {
		return probeRoots{}, fmt.Errorf("create fresh OMNI probe root: %w", err)
	}
	created := probeRoots{Root: root, Paths: paths, Owned: owned}
	for _, path := range owned {
		if err := os.Mkdir(path, 0o700); err != nil {
			_ = cleanupProbeRoots(created, reportPath)
			return probeRoots{}, fmt.Errorf("create isolated OMNI probe root %q: %w", filepath.Base(path), err)
		}
	}
	return created, nil
}

func cleanupProbeRoots(roots probeRoots, reportPath string) error {
	var cleanupErr error
	for _, path := range roots.Owned {
		cleanupErr = errors.Join(cleanupErr, os.RemoveAll(path))
	}
	if !pathWithin(roots.Root, reportPath) {
		cleanupErr = errors.Join(cleanupErr, os.RemoveAll(roots.Root))
	}
	return cleanupErr
}

func reserveProbePort(forbidden int) (net.Listener, int, error) {
	for attempt := 0; attempt < 8; attempt++ {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, 0, err
		}
		address, ok := listener.Addr().(*net.TCPAddr)
		if !ok || address.Port <= 0 {
			_ = listener.Close()
			return nil, 0, errors.New("loopback listener did not expose a port")
		}
		if address.Port != forbidden {
			return listener, address.Port, nil
		}
		_ = listener.Close()
	}
	return nil, 0, fmt.Errorf("ephemeral port repeatedly selected forbidden port %d", forbidden)
}

func newProbeReport(prepared admittedProbeInput, roots probeRoots, port int) (ProbeReport, error) {
	image, video, err := probeJourneyReports(prepared.manifest)
	if err != nil {
		return ProbeReport{}, err
	}
	return ProbeReport{
		SchemaVersion: ProbeReportSchemaV1,
		RunID:         prepared.input.RunID,
		Status:        "INCONCLUSIVE",
		Build:         prepared.build,
		Dependencies:  prepared.dependencies,
		Fixtures: []RecordedIdentity{
			recordedArtifact(prepared.manifest.Artifacts[0]), recordedArtifact(prepared.manifest.Artifacts[1]),
		},
		Policy: ProbePolicy{
			RootIdentities: rootIdentities(roots), Port: port,
			TimeoutSeconds:    prepared.input.Limits.TimeoutSeconds,
			NetworkPolicy:     prepared.input.Limits.NetworkPolicy,
			DownloadBytes:     prepared.input.Limits.MaxDownloadBytes,
			PaidUSD:           prepared.input.Limits.MaxPaidUSD,
			MaxHeavyProcesses: prepared.input.Limits.MaxHeavyProcesses,
		},
		Journeys: []JourneyReport{image, video},
		Outputs:  []RecordedIdentity{}, Processes: []ProcessEvidence{},
	}, nil
}

func probeJourneyReports(manifest Manifest) (JourneyReport, JourneyReport, error) {
	imageRubric, err := reportRubric(manifest.Artifacts[0])
	if err != nil {
		return JourneyReport{}, JourneyReport{}, err
	}
	videoRubric, err := reportRubric(manifest.Artifacts[1])
	if err != nil {
		return JourneyReport{}, JourneyReport{}, err
	}
	imagePrompt := "Identify the infinity symbol, text, and accent colors in this image."
	videoPrompt := "Describe the ordered phases and hard-cut transition in this video."
	image := makeJourneyReport(probeJourneyImage, imagePrompt, manifest.Artifacts[0], imageRubric)
	video := makeJourneyReport(probeJourneyVideo, videoPrompt, manifest.Artifacts[1], videoRubric)
	return image, video, nil
}

func makeJourneyReport(name JourneyName, prompt string, artifact Artifact, rubric json.RawMessage) JourneyReport {
	promptBytes := []byte(prompt)
	fixtureName := "image"
	modality := "IMAGE"
	mediaType := "image/png"
	if name == probeJourneyVideo {
		fixtureName, modality, mediaType = "video", "VIDEO", "video/mp4"
	}
	return JourneyReport{
		Name: name, Status: JourneyNotRun,
		Command: []string{"models", "invoke", "llm", "--operation", "OMNI", "--input", "prompt=" + prompt, "--input", fixtureName + "=@fixture:" + artifact.ID},
		RequestInputs: []RequestInput{
			{Order: 0, Name: "prompt", Modality: "TEXT", MediaType: "text/plain", Bytes: int64(len(promptBytes)), SHA256: hashBytes(promptBytes)},
			{Order: 1, Name: fixtureName, Modality: modality, MediaType: mediaType, Bytes: artifact.Bytes, SHA256: artifact.SHA256},
		},
		SemanticRubric: rubric,
	}
}

func reportRubric(artifact Artifact) (json.RawMessage, error) {
	if artifact.SemanticRubric.Image != nil {
		predicates := make([]struct {
			Kind  string          `json:"kind"`
			Exact json.RawMessage `json:"exact"`
		}, 0, len(artifact.SemanticRubric.Image.AllOf))
		for _, predicate := range artifact.SemanticRubric.Image.AllOf {
			predicates = append(predicates, struct {
				Kind  string          `json:"kind"`
				Exact json.RawMessage `json:"exact"`
			}{Kind: predicate.Kind, Exact: predicate.Exact})
		}
		return json.Marshal(struct {
			AllOf any `json:"allOf"`
		}{AllOf: predicates})
	}
	if artifact.SemanticRubric.Video != nil {
		return json.Marshal(struct {
			OrderedPhases []VideoPhase     `json:"orderedPhases"`
			Transition    *VideoTransition `json:"transition"`
		}{OrderedPhases: artifact.SemanticRubric.Video.OrderedPhases, Transition: artifact.SemanticRubric.Video.Transition})
	}
	return nil, validationError(CodeProbeInvalidReport, "semanticRubric", "one media rubric", "missing", nil)
}

func (runner Runner) executeJourneys(ctx context.Context, report *ProbeReport, roots probeRoots) error {
	for index := range report.Journeys {
		if index == 1 && report.Journeys[0].Status != JourneyPass {
			continue
		}
		outcome := runner.executeJourney(ctx, &report.Journeys[index], roots)
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

func (runner Runner) executePrebuiltCLI(ctx context.Context, roots probeRoots) journeyOutcome {
	request := ExecutionRequest{
		Journey: probeJourneyCLI, Command: []string{"--version"}, Roots: roots.Paths, Port: roots.Port,
	}
	observation, err := runner.Executor.Execute(ctx, request)
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

func (runner Runner) executeJourney(ctx context.Context, journey *JourneyReport, roots probeRoots) journeyOutcome {
	request := ExecutionRequest{
		Journey: journey.Name, Command: append([]string(nil), journey.Command...),
		Inputs: append([]RequestInput(nil), journey.RequestInputs...), Roots: roots.Paths,
		Port: roots.Port,
	}
	return runner.executeJourneyRequest(ctx, journey, request)
}

func (runner Runner) executeJourneyRequest(ctx context.Context, journey *JourneyReport, request ExecutionRequest) journeyOutcome {
	observation, err := runner.Executor.Execute(ctx, request)
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

func recordedArtifact(artifact Artifact) RecordedIdentity {
	return RecordedIdentity{Identity: artifact.ID, PathIdentity: pathIdentity(artifact.ResolvedPath), Bytes: artifact.Bytes, SHA256: artifact.SHA256}
}

func rootIdentities(roots probeRoots) []string {
	paths := []string{roots.Root, roots.Paths.Work, roots.Paths.Profile, roots.Paths.Cache, roots.Paths.Model, roots.Paths.Projector, roots.Paths.Backend, roots.Paths.Output, roots.Paths.Streams}
	identities := make([]string, 0, len(paths))
	for _, path := range paths {
		identities = append(identities, pathIdentity(path))
	}
	return identities
}

func pathIdentity(path string) string {
	digest := sha256.Sum256([]byte(filepath.Clean(path)))
	return "sha256=" + hex.EncodeToString(digest[:])
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (report ProbeReport) journeyFailureCount() int {
	count := 0
	for _, journey := range report.Journeys {
		if journey.Status == JourneyFail || journey.Status == JourneyInconclusive {
			count++
		}
	}
	return count
}

func validateProbePolicy(policy ProbePolicy) error {
	if len(policy.RootIdentities) < 6 || !uniqueStrings(policy.RootIdentities) {
		return validationError(CodeProbeInvalidReport, "policy.rootIdentities", "at least six unique redacted roots", fmt.Sprint(policy.RootIdentities), nil)
	}
	if policy.Port <= 0 || policy.Port > 65535 || policy.Port == ProbeForbiddenPort {
		return validationError(CodeProbeInvalidReport, "policy.port", "valid non-7437 port", fmt.Sprint(policy.Port), nil)
	}
	if policy.TimeoutSeconds < 1 || policy.NetworkPolicy != ProbeNetworkPolicy || policy.DownloadBytes != 0 || policy.PaidUSD != 0 || policy.MaxHeavyProcesses != ProbeMaxHeavyProcesses {
		return validationError(CodeProbeInvalidReport, "policy", "declared zero-download one-heavy local policy", fmt.Sprintf("%+v", policy), nil)
	}
	return nil
}

func validateJourneyReport(journey JourneyReport, index int) error {
	if journey.Status != JourneyNotRun && journey.Status != JourneyPass && journey.Status != JourneyFail && journey.Status != JourneyInconclusive {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].status", index), "known journey status", string(journey.Status), nil)
	}
	if len(journey.Command) == 0 {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].command", index), "non-empty redacted command", "empty", nil)
	}
	for _, argument := range journey.Command {
		if reportStringContainsAbsolutePath(argument) {
			return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].command", index), "redacted command without absolute paths", "absolute path", nil)
		}
	}
	if len(journey.RequestInputs) != 2 {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].requestInputs", index), "prompt and one ordered media input", fmt.Sprint(len(journey.RequestInputs)), nil)
	}
	for inputIndex, input := range journey.RequestInputs {
		if input.Order != inputIndex || input.Bytes < 0 || validateSHA256(input.SHA256, fmt.Sprintf("journeys[%d].requestInputs[%d].sha256", index, inputIndex)) != nil {
			return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].requestInputs[%d]", index, inputIndex), "ordered complete request input", fmt.Sprint(input), nil)
		}
	}
	wantMediaName, wantModality, wantMediaType := "image", "IMAGE", "image/png"
	if journey.Name == probeJourneyVideo {
		wantMediaName, wantModality, wantMediaType = "video", "VIDEO", "video/mp4"
	}
	if journey.RequestInputs[0].Name != "prompt" || journey.RequestInputs[0].Modality != "TEXT" || journey.RequestInputs[0].MediaType != "text/plain" || journey.RequestInputs[1].Name != wantMediaName || journey.RequestInputs[1].Modality != wantModality || journey.RequestInputs[1].MediaType != wantMediaType {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].requestInputs", index), "prompt followed by the named media input", fmt.Sprint(journey.RequestInputs), nil)
	}
	if len(bytes.TrimSpace(journey.SemanticRubric)) == 0 || journey.SemanticRubric[0] != '{' {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].semanticRubric", index), "non-empty JSON object rubric", "missing", nil)
	}
	var rubric map[string]json.RawMessage
	if err := json.Unmarshal(journey.SemanticRubric, &rubric); err != nil || len(rubric) == 0 {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].semanticRubric", index), "non-empty JSON object rubric", "invalid", err)
	}
	if journey.Status == JourneyNotRun && journey.Failure != nil {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].failure", index), "null for NOT_RUN", "failure present", nil)
	}
	if journey.Status == JourneyPass && journey.Failure != nil {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].failure", index), "null for PASS", "failure present", nil)
	}
	if (journey.Status == JourneyFail || journey.Status == JourneyInconclusive) && journey.Failure == nil {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("journeys[%d].failure", index), "typed failure for failed journey", "missing", nil)
	}
	if journey.Failure != nil {
		return validateReportFailure(*journey.Failure, fmt.Sprintf("journeys[%d].failure", index))
	}
	return nil
}

func validateRecordedIdentity(identity RecordedIdentity, field string, requireBytes bool) error {
	if err := validateProbeLabel(identity.Identity, field+".identity"); err != nil {
		return err
	}
	if !strings.HasPrefix(identity.PathIdentity, "sha256=") || len(identity.PathIdentity) != len("sha256=")+sha256.Size*2 {
		return validationError(CodeProbeInvalidReport, field+".pathIdentity", "redacted SHA-256 path identity", identity.PathIdentity, nil)
	}
	if err := validateSHA256(strings.TrimPrefix(identity.PathIdentity, "sha256="), field+".pathIdentity"); err != nil {
		return err
	}
	if identity.Bytes < 0 || (requireBytes && identity.Bytes == 0) {
		return validationError(CodeProbeInvalidReport, field+".bytes", "non-negative observed bytes", fmt.Sprint(identity.Bytes), nil)
	}
	return validateSHA256(identity.SHA256, field+".sha256")
}

func validateProcessEvidence(process ProcessEvidence, index int) error {
	if err := validateProbeLabel(process.Identity, fmt.Sprintf("processes[%d].identity", index)); err != nil {
		return err
	}
	if err := validateProbeLabel(process.Kind, fmt.Sprintf("processes[%d].kind", index)); err != nil {
		return err
	}
	if err := validateProbeLabel(process.Owner, fmt.Sprintf("processes[%d].owner", index)); err != nil {
		return err
	}
	if process.PID < 0 {
		return validationError(CodeProbeInvalidReport, fmt.Sprintf("processes[%d].pid", index), "non-negative process id", fmt.Sprint(process.PID), nil)
	}
	return nil
}

func validateReportFailure(failure ReportFailure, field string) error {
	for name, value := range map[string]string{"owner": failure.Owner, "code": failure.Code, "expected": failure.Expected, "observed": failure.Observed, "nextAction": failure.NextAction} {
		if value == "" || reportStringContainsAbsolutePath(value) {
			return validationError(CodeProbeInvalidReport, field+"."+name, "non-empty redacted failure text", value, nil)
		}
	}
	return nil
}

func validateReportState(report ProbeReport) error {
	if report.Status == "READY" {
		if report.Failure != nil || report.journeyFailureCount() != 0 || report.Journeys[0].Status != JourneyNotRun || report.Journeys[1].Status != JourneyNotRun {
			return validationError(CodeProbeInvalidReport, "status", "READY with no executed journey or failure", "inconsistent", nil)
		}
	}
	if report.Status == "PASS" && (report.Failure != nil || report.Journeys[0].Status != JourneyPass || report.Journeys[1].Status != JourneyPass) {
		return validationError(CodeProbeInvalidReport, "status", "PASS with two passing journeys", "inconsistent", nil)
	}
	if (report.Status == "FAIL" || report.Status == "INCONCLUSIVE") && report.Failure == nil {
		return validationError(CodeProbeInvalidReport, "failure", "typed report failure", "missing", nil)
	}
	if report.Journeys[0].Status != JourneyPass && report.Journeys[1].Status != JourneyNotRun {
		return validationError(CodeProbeInvalidReport, "journeys[1].status", "video NOT_RUN after image failure", string(report.Journeys[1].Status), nil)
	}
	return nil
}

func validateReportRedaction(report ProbeReport) error {
	values := []string{report.Build.Identity, report.Dependencies.Model.Identity, report.Dependencies.Projector.Identity, report.Dependencies.Backend.Identity}
	for _, fixture := range report.Fixtures {
		values = append(values, fixture.Identity, fixture.PathIdentity)
	}
	for _, journey := range report.Journeys {
		values = append(values, journey.Command...)
	}
	for _, process := range report.Processes {
		values = append(values, process.Identity, process.Kind, process.Owner)
	}
	if report.Failure != nil {
		values = append(values, report.Failure.Owner, report.Failure.Code, report.Failure.Expected, report.Failure.Observed, report.Failure.NextAction)
	}
	for _, value := range values {
		if reportStringContainsAbsolutePath(value) {
			return validationError(CodeProbeInvalidReport, "report", "redacted path values", "absolute path", nil)
		}
	}
	return nil
}

func reportStringContainsAbsolutePath(value string) bool {
	if filepath.IsAbs(value) {
		return true
	}
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	return false
}

func normalizeReportFailure(failure ReportFailure, defaultOwner string) *ReportFailure {
	if failure.Owner == "" {
		failure.Owner = defaultOwner
	}
	if failure.Code == "" {
		failure.Code = string(CodeProbeExecutionFailure)
	}
	if failure.Expected == "" {
		failure.Expected = "bounded journey observation"
	}
	if failure.Observed == "" {
		failure.Observed = "executor reported failure"
	}
	if failure.NextAction == "" {
		failure.NextAction = "inspect the bounded evidence before retrying"
	}
	return &failure
}

func probeFailure(owner, code, expected, observed, nextAction string) *ReportFailure {
	return &ReportFailure{Owner: owner, Code: code, Expected: expected, Observed: observed, NextAction: nextAction}
}

func probeContextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return validationError(CodeProbeTimedOut, "context", "live bounded context", "deadline exceeded", err)
	}
	return validationError(CodeProbeCancelled, "context", "live bounded context", "cancelled", err)
}

func wrapProbeCleanupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, validationError(CodeProbeCleanupFailure, "cleanup", "zero owned survivors", "cleanup failed", err))
}

func readProbeJSON(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("JSON exceeds %d bytes", maximum)
	}
	return data, nil
}

func writeProbeJSONAtomic(path string, body []byte) error {
	path, err := normalizeProbePath(path, "atomic JSON path")
	if err != nil {
		return err
	}
	if err := rejectProbeSymlinkComponents(path); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("atomic JSON destination is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".omni-media-probe-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := (filesystem.Local{AllowRenameReplacement: true}).RenameReplacing(temporaryPath, path); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func normalizeProbePath(path, field string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return "", validationError(CodeProbeInvalidIdentity, field, "absolute path", path, nil)
	}
	clean := filepath.Clean(path)
	if clean != path {
		return "", validationError(CodeProbeInvalidIdentity, field, "clean absolute path", path, nil)
	}
	return clean, nil
}

func rejectProbeSymlinkComponents(path string) error {
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component is a symlink (%s)", pathIdentity(current))
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func uniqueStrings(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
