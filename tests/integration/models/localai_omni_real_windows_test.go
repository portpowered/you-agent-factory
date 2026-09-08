//go:build windows

package models_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/locking"
)

const (
	localAIOMNIInputSchema       = "localai.windows-omni-diagnostic-input.v1"
	localAIOMNIEvidenceSchema    = "localai.windows-omni-diagnostic-evidence.v1"
	localAIOMNIRealEnableEnv     = "INFINITE_YOU_LOCALAI_OMNI_REAL_ENABLE"
	localAIOMNIRealManifestEnv   = "INFINITE_YOU_LOCALAI_OMNI_REAL_MANIFEST"
	localAIOMNIEvidenceKind      = "controlled"
	localAIOMNIRealEvidenceKind  = "real"
	localAIOMNISelectorText      = "text"
	localAIOMNISelectorImage     = "image"
	localAIOMNISelectorVideo     = "video"
	localAIOMNIModelCallKind     = "modelCall"
	localAIOMNINetworkPolicy     = "loopback-only-external-denied"
	localAIOMNIMaxManifestBytes  = 128 << 10
	localAIOMNIMaxExcerptBytes   = 512
	localAIOMNIMaxStreamBytes    = 64 << 10
	localAIOMNIMaxFailureBytes   = 192
	localAIOMNIMaxArguments      = 64
	localAIOMNIMaxFacts          = 8
	localAIOMNIMaxIdentityLength = 256
)

var (
	errLocalAIOMNIRealDisabled      = errors.New("localai omni real selector is disabled")
	errLocalAIOMNIReportInterrupted = errors.New("localai omni report interruption")
	localAIOMNITransitionPattern    = regexp.MustCompile(`(?i)(?:transition|change|switch|midpoint|at)[^0-9]{0,32}([0-9]{1,6})\s*(?:ms|milliseconds)\b`)
	localAIOMNIPathPattern          = regexp.MustCompile(`(?i)(?:[a-z]:\\|\\\\)[^\s"<>]+`)
)

type localAIOMNIManifest struct {
	Schema    string                   `json:"schema"`
	Selector  string                   `json:"selector"`
	CLI       localAIOMNICLIIdentity   `json:"cli"`
	Model     localAIOMNIModelIdentity `json:"model"`
	Backend   localAIOMNIBackend       `json:"backend"`
	Text      localAIOMNITextFixture   `json:"text"`
	Image     localAIOMNIImageFixture  `json:"image"`
	Video     localAIOMNIVideoFixture  `json:"video"`
	Isolation localAIOMNIIsolation     `json:"isolation"`
	Limits    localAIOMNILimits        `json:"limits"`
	Evidence  localAIOMNIEvidencePaths `json:"evidence"`
}

type localAIOMNICLIIdentity struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type localAIOMNIModelIdentity struct {
	Name     string `json:"name"`
	Identity string `json:"identity"`
}

type localAIOMNIBackend struct {
	Identity string `json:"identity"`
}

type localAIOMNITextFixture struct {
	Token string `json:"token"`
}

type localAIOMNIImageFixture struct {
	Path          string   `json:"path"`
	SHA256        string   `json:"sha256"`
	RequiredFacts []string `json:"requiredFacts"`
}

type localAIOMNIVideoFixture struct {
	Path                        string `json:"path"`
	SHA256                      string `json:"sha256"`
	Phase1                      string `json:"phase1"`
	Phase1Color                 string `json:"phase1Color"`
	Phase2                      string `json:"phase2"`
	Phase2Color                 string `json:"phase2Color"`
	TransitionStartMilliseconds int64  `json:"transitionStartMilliseconds"`
	TransitionEndMilliseconds   int64  `json:"transitionEndMilliseconds"`
}

type localAIOMNIIsolation struct {
	WorkRoot      string `json:"workRoot"`
	StateRoot     string `json:"stateRoot"`
	CacheRoot     string `json:"cacheRoot"`
	TempRoot      string `json:"tempRoot"`
	OutputRoot    string `json:"outputRoot"`
	StreamsRoot   string `json:"streamsRoot"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	NetworkPolicy string `json:"networkPolicy"`
}

type localAIOMNILimits struct {
	TimeoutSeconds int   `json:"timeoutSeconds"`
	Processes      int   `json:"processes"`
	DownloadBytes  int64 `json:"downloadBytes"`
	ModelCalls     int64 `json:"modelCalls"`
}

type localAIOMNIEvidencePaths struct {
	ReportPath string `json:"reportPath"`
	LedgerPath string `json:"ledgerPath"`
}

type localAIOMNIInvocation struct {
	Manifest       localAIOMNIManifest
	ManifestSHA256 string
	RunID          string
	EvidenceKind   string
	Command        localAIOMNICommand
}

type localAIOMNICommand struct {
	BinaryPath  string
	Arguments   []string
	Environment []string
}

type localAIOMNIExecutionRoots struct {
	Work    string
	State   string
	Cache   string
	Temp    string
	Output  string
	Streams string
}

type localAIOMNIObservation struct {
	Started           bool
	ProcessExited     bool
	ExitCode          int
	TimedOut          bool
	Cancelled         bool
	ProcessTreeClosed bool
	OwnedProcesses    int
	OwnedListeners    int
	OwnedLeases       int
	Stdout            []byte
	Stderr            []byte
	BackendLogs       []localAIOMNIRawLog
	RuntimeLogs       []localAIOMNIRawLog
	Artifacts         []localAIOMNIRawArtifact
}

type localAIOMNIRawLog struct {
	Path string
	Body []byte
}

type localAIOMNIRawArtifact struct {
	Kind      string
	Path      string
	MediaType string
	Body      []byte
}

type localAIOMNIExecutor interface {
	Execute(context.Context, localAIOMNICommand, localAIOMNIExecutionRoots) localAIOMNIObservation
}

type localAIOMNIRunner struct {
	executor    localAIOMNIExecutor
	locks       locking.Service
	writeReport func(string, localAIOMNIReport) error
}

type localAIOMNIReport struct {
	Schema         string                          `json:"schema"`
	EvidenceKind   string                          `json:"evidenceKind"`
	RunID          string                          `json:"runId"`
	Selector       string                          `json:"selector"`
	Status         string                          `json:"status"`
	Platform       string                          `json:"platform"`
	Architecture   string                          `json:"architecture"`
	Redacted       bool                            `json:"redacted"`
	ManifestSHA256 string                          `json:"manifestSha256"`
	InputIdentity  localAIOMNIInputIdentity        `json:"inputIdentity"`
	Policy         localAIOMNIPolicy               `json:"policy"`
	Command        localAIOMNICommandEvidence      `json:"command"`
	Logs           localAIOMNILogs                 `json:"logs"`
	Artifacts      []localAIOMNIArtifactEvidence   `json:"artifacts"`
	Cache          localAIOMNICacheEvidence        `json:"cache"`
	Semantic       localAIOMNISemanticEvidence     `json:"semantic"`
	Reservation    *localAIOMNIReservationEvidence `json:"reservation,omitempty"`
	Release        localAIOMNIReleaseEvidence      `json:"release"`
	Failure        *localAIOMNIFailure             `json:"failure,omitempty"`
	Activity       localAIOMNIActivity             `json:"activity"`
}

type localAIOMNIInputIdentity struct {
	CLISHA256          string `json:"cliSha256"`
	ModelIdentity      string `json:"modelIdentity"`
	BackendIdentity    string `json:"backendIdentity"`
	FixtureSHA256      string `json:"fixtureSha256"`
	ImageFixtureSHA256 string `json:"imageFixtureSha256"`
	VideoFixtureSHA256 string `json:"videoFixtureSha256"`
	RubricSHA256       string `json:"rubricSha256"`
}

type localAIOMNIPolicy struct {
	RootIdentities    []string `json:"rootIdentities"`
	Host              string   `json:"host"`
	Port              int      `json:"port"`
	TimeoutSeconds    int      `json:"timeoutSeconds"`
	ProcessLimit      int      `json:"processLimit"`
	DownloadByteLimit int64    `json:"downloadByteLimit"`
	ModelCallLimit    int64    `json:"modelCallLimit"`
	NetworkPolicy     string   `json:"networkPolicy"`
}

type localAIOMNICommandEvidence struct {
	Arguments       []string                  `json:"arguments"`
	SecretsRedacted bool                      `json:"secretsRedacted"`
	Controlled      bool                      `json:"controlled"`
	Stdout          localAIOMNIStreamEvidence `json:"stdout"`
	Stderr          localAIOMNIStreamEvidence `json:"stderr"`
}

type localAIOMNIStreamEvidence struct {
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
	Excerpt   string `json:"excerpt"`
	Truncated bool   `json:"truncated"`
}

type localAIOMNILogs struct {
	Backend []localAIOMNILogEvidence `json:"backend"`
	Runtime []localAIOMNILogEvidence `json:"runtime"`
}

type localAIOMNILogEvidence struct {
	PathIdentity string `json:"pathIdentity"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	Excerpt      string `json:"excerpt"`
	Truncated    bool   `json:"truncated"`
}

type localAIOMNIArtifactEvidence struct {
	Kind         string `json:"kind"`
	MediaType    string `json:"mediaType"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	PathIdentity string `json:"pathIdentity"`
	Truncated    bool   `json:"truncated"`
}

type localAIOMNICacheEvidence struct {
	BeforeSHA256          string `json:"beforeSha256"`
	AfterSHA256           string `json:"afterSha256"`
	BeforeEntries         int    `json:"beforeEntries"`
	AfterEntries          int    `json:"afterEntries"`
	BeforeFiles           int    `json:"beforeFiles"`
	AfterFiles            int    `json:"afterFiles"`
	BeforeBytes           int64  `json:"beforeBytes"`
	AfterBytes            int64  `json:"afterBytes"`
	PartialArtifacts      int    `json:"partialArtifacts"`
	AfterInspectionFailed bool   `json:"afterInspectionFailed"`
}

type localAIOMNISemanticEvidence struct {
	Assertion                     string   `json:"assertion"`
	Expected                      string   `json:"expected"`
	Observed                      string   `json:"observed"`
	Passed                        bool     `json:"passed"`
	GroundedFacts                 []string `json:"groundedFacts,omitempty"`
	NumericTransitionMilliseconds *int64   `json:"numericTransitionMilliseconds,omitempty"`
	PromptEcho                    bool     `json:"promptEcho"`
}

type localAIOMNIReservationEvidence struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Amount int64  `json:"amount"`
	State  string `json:"state"`
}

type localAIOMNIReleaseEvidence struct {
	Checked           bool `json:"checked"`
	InspectionFailed  bool `json:"inspectionFailed"`
	ProcessTreeClosed bool `json:"processTreeClosed"`
	OwnedProcesses    int  `json:"ownedProcesses"`
	OwnedListeners    int  `json:"ownedListeners"`
	OwnedLeases       int  `json:"ownedLeases"`
	PartialArtifacts  int  `json:"partialArtifacts"`
}

type localAIOMNIFailure struct {
	Owner     string `json:"owner"`
	Assertion string `json:"assertion"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
}

type localAIOMNIActivity struct {
	BackendProcesses int   `json:"backendProcesses"`
	ExternalRequests int   `json:"externalRequests"`
	Downloads        int64 `json:"downloads"`
	ModelCalls       int64 `json:"modelCalls"`
}

type localAIOMNICacheSnapshot struct {
	IdentitySHA256 string
	Entries        int
	Files          int
	Bytes          int64
	Partial        int
}

func (runner localAIOMNIRunner) Run(ctx context.Context, invocation localAIOMNIInvocation) (localAIOMNIReport, error) {
	report := newLocalAIOMNIReport(invocation)
	roots := localAIOMNIExecutionRootsForManifest(invocation.Manifest)
	prepared := false
	if err := validateLocalAIOMNIInvocation(invocation); err != nil {
		localAIOMNISetFailure(&report, "harness", "manifest admission", "complete immutable manifest", err.Error())
		return runner.finish(invocation, roots, report, prepared)
	}
	if err := prepareLocalAIOMNIRoots(roots); err != nil {
		localAIOMNISetFailure(&report, "environment", "isolated roots", "all declared roots are creatable", err.Error())
		return runner.finish(invocation, roots, report, prepared)
	}
	prepared = true
	before, err := localAIOMNISnapshotRoots(roots)
	if err != nil {
		localAIOMNISetFailure(&report, "harness", "cache admission inspection", "cache identity is observable", err.Error())
		return runner.finish(invocation, roots, report, prepared)
	}
	localAIOMNISetCacheBefore(&report.Cache, before)
	reservation, _, err := reserveLocalAIBudget(
		ctx,
		runner.locks,
		invocation.Manifest.Evidence.LedgerPath,
		invocation.RunID,
		invocation.Manifest.Selector,
		localAIOMNIModelCallKind,
		1,
		localAIBudgetLimits{ModelCalls: invocation.Manifest.Limits.ModelCalls, DownloadBytes: invocation.Manifest.Limits.DownloadBytes},
	)
	if err != nil {
		localAIOMNISetBudgetFailure(&report, err)
		return runner.finish(invocation, roots, report, prepared)
	}
	report.Reservation = &localAIOMNIReservationEvidence{ID: reservation.ID, Kind: reservation.Kind, Amount: reservation.Amount, State: reservation.State}
	if runner.executor == nil {
		localAIOMNISetFailure(&report, "harness", "controlled executor", "an executor is configured", "executor is nil")
	} else {
		executionContext, cancel := context.WithTimeout(ctx, time.Duration(invocation.Manifest.Limits.TimeoutSeconds)*time.Second)
		observation := runner.executor.Execute(executionContext, invocation.Command, roots)
		contextErr := executionContext.Err()
		cancel()
		if contextErr != nil {
			observation.Cancelled = observation.Cancelled || errors.Is(contextErr, context.Canceled)
			observation.TimedOut = observation.TimedOut || errors.Is(contextErr, context.DeadlineExceeded)
		}
		localAIOMNIRecordObservation(&report, invocation, observation)
		localAIOMNIClassifyObservation(&report, invocation, observation)
		if err := commitLocalAIBudget(ctx, runner.locks, invocation.Manifest.Evidence.LedgerPath, invocation.RunID, reservation.ID); err != nil {
			localAIOMNISetFailure(&report, "harness", "durable reservation commit", "executed allowance is committed", err.Error())
		} else if report.Reservation != nil {
			report.Reservation.State = "COMMITTED"
		}
	}
	return runner.finish(invocation, roots, report, prepared)
}

func (runner localAIOMNIRunner) RunRealFromEnvironment(ctx context.Context) (localAIOMNIReport, error) {
	invocation, err := localAIOMNIInvocationFromEnvironment()
	if err != nil {
		return localAIOMNIReport{}, err
	}
	if runner.executor == nil {
		runner.executor = localAIOMNIProcessExecutor{}
	}
	return runner.Run(ctx, invocation)
}

func (runner localAIOMNIRunner) finish(invocation localAIOMNIInvocation, roots localAIOMNIExecutionRoots, report localAIOMNIReport, prepared bool) (localAIOMNIReport, error) {
	if prepared {
		after, err := localAIOMNISnapshotRoots(roots)
		if err != nil {
			report.Cache.AfterInspectionFailed = true
			report.Release.Checked = false
			report.Release.InspectionFailed = true
			localAIOMNISetInconclusive(&report, "harness", "final cache and release inspection", "owned resources and cache state are observable", err.Error())
		} else {
			localAIOMNISetCacheAfter(&report.Cache, after)
			report.Release.Checked = true
			report.Release.PartialArtifacts = after.Partial
			if report.Release.ProcessTreeClosed == false && report.Command.Stdout.SHA256 == "" && report.Command.Stderr.SHA256 == "" {
				report.Release.ProcessTreeClosed = true
			}
			if report.Status == "PASS" && (!report.Release.ProcessTreeClosed || report.Release.OwnedProcesses != 0 || report.Release.OwnedListeners != 0 || report.Release.OwnedLeases != 0 || report.Release.PartialArtifacts != 0) {
				localAIOMNISetFailure(&report, "harness", "owned-resource cleanup", "processes, listeners, leases and partials are zero", fmt.Sprintf("processes=%d listeners=%d leases=%d partials=%d treeClosed=%t", report.Release.OwnedProcesses, report.Release.OwnedListeners, report.Release.OwnedLeases, report.Release.PartialArtifacts, report.Release.ProcessTreeClosed))
			}
		}
	} else {
		report.Release.Checked = false
		report.Release.InspectionFailed = false
	}
	if runner.writeReport == nil {
		runner.writeReport = writeLocalAIOMNIReportAtomic
	}
	if !filepath.IsAbs(invocation.Manifest.Evidence.ReportPath) {
		return report, errors.New("diagnostic report path is not absolute")
	}
	if err := runner.writeReport(invocation.Manifest.Evidence.ReportPath, report); err != nil {
		return report, err
	}
	return report, nil
}

func newLocalAIOMNIReport(invocation localAIOMNIInvocation) localAIOMNIReport {
	manifest := invocation.Manifest
	return localAIOMNIReport{
		Schema:         localAIOMNIEvidenceSchema,
		EvidenceKind:   invocation.EvidenceKind,
		RunID:          invocation.RunID,
		Selector:       manifest.Selector,
		Status:         "INCONCLUSIVE",
		Platform:       runtime.GOOS,
		Architecture:   runtime.GOARCH,
		Redacted:       true,
		ManifestSHA256: invocation.ManifestSHA256,
		InputIdentity: localAIOMNIInputIdentity{
			CLISHA256:          manifest.CLI.SHA256,
			ModelIdentity:      manifest.Model.Identity,
			BackendIdentity:    manifest.Backend.Identity,
			FixtureSHA256:      localAIOMNIFixtureSHA(manifest),
			ImageFixtureSHA256: manifest.Image.SHA256,
			VideoFixtureSHA256: manifest.Video.SHA256,
			RubricSHA256:       localAIOMNIRubricSHA(manifest),
		},
		Policy: localAIOMNIPolicy{
			RootIdentities:    localAIOMNIPathIdentities(manifest.Isolation),
			Host:              manifest.Isolation.Host,
			Port:              manifest.Isolation.Port,
			TimeoutSeconds:    manifest.Limits.TimeoutSeconds,
			ProcessLimit:      manifest.Limits.Processes,
			DownloadByteLimit: manifest.Limits.DownloadBytes,
			ModelCallLimit:    manifest.Limits.ModelCalls,
			NetworkPolicy:     manifest.Isolation.NetworkPolicy,
		},
		Command: localAIOMNICommandEvidence{
			Arguments:       localAIOMNIRedactArguments(invocation.Command.Arguments),
			SecretsRedacted: true,
			Controlled:      invocation.EvidenceKind == localAIOMNIEvidenceKind,
		},
		Logs:      localAIOMNILogs{Backend: []localAIOMNILogEvidence{}, Runtime: []localAIOMNILogEvidence{}},
		Artifacts: []localAIOMNIArtifactEvidence{},
	}
}

func localAIOMNIRecordObservation(report *localAIOMNIReport, invocation localAIOMNIInvocation, observation localAIOMNIObservation) {
	report.Command.Stdout = localAIOMNICaptureStream(observation.Stdout)
	report.Command.Stderr = localAIOMNICaptureStream(observation.Stderr)
	report.Logs.Backend = localAIOMNIRecordLogs(observation.BackendLogs, "controlled-backend.log")
	report.Logs.Runtime = localAIOMNIRecordLogs(observation.RuntimeLogs, "controlled-runtime.log")
	for _, artifact := range observation.Artifacts {
		body := artifact.Body
		truncated := false
		if len(body) > localAIOMNIMaxStreamBytes {
			body = body[:localAIOMNIMaxStreamBytes]
			truncated = true
		}
		report.Artifacts = append(report.Artifacts, localAIOMNIArtifactEvidence{
			Kind:         artifact.Kind,
			MediaType:    artifact.MediaType,
			Bytes:        int64(len(body)),
			SHA256:       sha256Hex(body),
			PathIdentity: localAIOMNIPathIdentity(artifact.Path),
			Truncated:    truncated,
		})
	}
	if len(report.Artifacts) == 0 && len(observation.Stdout) > 0 {
		body := observation.Stdout
		truncated := false
		if len(body) > localAIOMNIMaxStreamBytes {
			body = body[:localAIOMNIMaxStreamBytes]
			truncated = true
		}
		report.Artifacts = append(report.Artifacts, localAIOMNIArtifactEvidence{
			Kind: "text", MediaType: "text/plain", Bytes: int64(len(body)),
			SHA256: sha256Hex(body), PathIdentity: localAIOMNIPathIdentity("controlled-stdout"), Truncated: truncated,
		})
	}
	report.Release = localAIOMNIReleaseEvidence{
		ProcessTreeClosed: observation.ProcessTreeClosed || !observation.Started,
		OwnedProcesses:    observation.OwnedProcesses,
		OwnedListeners:    observation.OwnedListeners,
		OwnedLeases:       observation.OwnedLeases,
	}
	if report.EvidenceKind == localAIOMNIRealEvidenceKind && (observation.Started || observation.ProcessExited) {
		report.Activity.BackendProcesses = 1
		report.Activity.ModelCalls = 1
	}
}

func localAIOMNIRecordLogs(logs []localAIOMNIRawLog, fallback string) []localAIOMNILogEvidence {
	result := make([]localAIOMNILogEvidence, 0, len(logs))
	for _, log := range logs {
		path := log.Path
		if path == "" {
			path = fallback
		}
		stream := localAIOMNICaptureStream(log.Body)
		result = append(result, localAIOMNILogEvidence{
			PathIdentity: localAIOMNIPathIdentity(path),
			Bytes:        stream.Bytes,
			SHA256:       stream.SHA256,
			Excerpt:      stream.Excerpt,
			Truncated:    stream.Truncated,
		})
	}
	return result
}

func localAIOMNIClassifyObservation(report *localAIOMNIReport, invocation localAIOMNIInvocation, observation localAIOMNIObservation) {
	if observation.TimedOut || observation.Cancelled || (invocation.EvidenceKind == localAIOMNIEvidenceKind && observation.Started && !observation.ProcessTreeClosed) {
		localAIOMNISetFailure(report, "harness", "timeout and cancellation cleanup", "cancellation is acknowledged and owned resources are released", "controlled execution did not finish cleanly")
		return
	}
	if localAIOMNIContainsSensitive(observation.Stdout) || localAIOMNIContainsSensitive(observation.Stderr) {
		localAIOMNISetFailure(report, "harness", "bounded redacted evidence", "sensitive output is absent from evidence", "sensitive output was observed and redacted")
		return
	}
	if len(observation.Stdout) > localAIOMNIMaxStreamBytes {
		localAIOMNISetFailure(report, "harness", "bounded stdout capture", "semantic output fits the bounded capture", "stdout exceeded the bounded capture")
		return
	}
	if observation.Started && !observation.ProcessExited {
		localAIOMNISetFailure(report, "product", "backend process completion", "the selected command exits", "the selected command did not exit")
		return
	}
	if observation.OwnedProcesses > invocation.Manifest.Limits.Processes {
		localAIOMNISetFailure(report, "harness", "owned process limit", fmt.Sprintf("at most %d owned processes", invocation.Manifest.Limits.Processes), fmt.Sprintf("observed %d owned processes", observation.OwnedProcesses))
		return
	}
	if observation.ExitCode != 0 {
		localAIOMNISetFailure(report, "product", "OMNI command result", "the selected command exits successfully", fmt.Sprintf("exit code %d", observation.ExitCode))
		return
	}
	semantic, verdict, failure := localAIOMNIClassifyText(report.Selector, invocation.Manifest, invocation.Command, observation.Stdout)
	report.Semantic = semantic
	switch verdict {
	case "PASS":
		report.Status = "PASS"
		report.Semantic.Passed = true
	case "FAIL":
		localAIOMNISetFailure(report, failure.Owner, failure.Assertion, failure.Expected, failure.Observed)
	case "INCONCLUSIVE":
		localAIOMNISetInconclusive(report, failure.Owner, failure.Assertion, failure.Expected, failure.Observed)
	default:
		localAIOMNISetFailure(report, "harness", "semantic classifier", "known semantic verdict", "classifier returned an unknown verdict")
	}
}

func localAIOMNISetFailure(report *localAIOMNIReport, owner, assertion, expected, observed string) {
	report.Status = "FAIL"
	report.Failure = &localAIOMNIFailure{Owner: owner, Assertion: assertion, Expected: expected, Observed: localAIOMNIBoundedRedacted(observed)}
	report.Semantic.Passed = false
}

func localAIOMNISetInconclusive(report *localAIOMNIReport, owner, assertion, expected, observed string) {
	report.Status = "INCONCLUSIVE"
	report.Failure = &localAIOMNIFailure{Owner: owner, Assertion: assertion, Expected: expected, Observed: localAIOMNIBoundedRedacted(observed)}
	report.Semantic.Passed = false
}

func localAIOMNISetBudgetFailure(report *localAIOMNIReport, err error) {
	var budgetErr *localAIBudgetError
	if errors.As(err, &budgetErr) {
		if budgetErr.Code == "budget_exhausted" {
			localAIOMNISetFailure(report, "harness", "durable model-call budget", "consumed allowance remains below limit", fmt.Sprintf("consumed=%d limit=%d", budgetErr.Consumed, budgetErr.Limit))
			return
		}
		localAIOMNISetFailure(report, "harness", "durable model-call ledger", "valid locked atomic ledger", budgetErr.Code)
		return
	}
	localAIOMNISetFailure(report, "harness", "durable model-call ledger", "valid locked atomic ledger", err.Error())
}

func validateLocalAIOMNIInvocation(invocation localAIOMNIInvocation) error {
	if invocation.EvidenceKind != localAIOMNIEvidenceKind && invocation.EvidenceKind != localAIOMNIRealEvidenceKind {
		return errors.New("evidence kind is not bounded")
	}
	if strings.TrimSpace(invocation.RunID) == "" || len(invocation.RunID) > localAIOMNIMaxIdentityLength {
		return errors.New("run identity is not bounded")
	}
	if err := validateLocalAIOMNIManifest(invocation.Manifest); err != nil {
		return err
	}
	if !localAIOMNIHash(invocation.ManifestSHA256) {
		return errors.New("manifest identity is not a SHA-256")
	}
	if !filepath.IsAbs(invocation.Command.BinaryPath) || invocation.Command.BinaryPath != invocation.Manifest.CLI.Path {
		return errors.New("command binary does not match the immutable CLI identity")
	}
	if len(invocation.Command.Arguments) == 0 || len(invocation.Command.Arguments) > localAIOMNIMaxArguments {
		return errors.New("command arguments are not bounded")
	}
	return nil
}

func validateLocalAIOMNIManifest(manifest localAIOMNIManifest) error {
	if manifest.Schema != localAIOMNIInputSchema {
		return errors.New("manifest schema is invalid")
	}
	switch manifest.Selector {
	case localAIOMNISelectorText, localAIOMNISelectorImage, localAIOMNISelectorVideo:
	default:
		return errors.New("manifest selector is invalid")
	}
	if err := validateLocalAIOMNIImmutableFile(manifest.CLI.Path, manifest.CLI.SHA256, "CLI"); err != nil {
		return err
	}
	if err := validateLocalAIOMNIIdentity(manifest.Model.Name, "model name"); err != nil {
		return err
	}
	if err := validateLocalAIOMNIIdentity(manifest.Model.Identity, "model identity"); err != nil {
		return err
	}
	if err := validateLocalAIOMNIIdentity(manifest.Backend.Identity, "backend identity"); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.Text.Token) == "" || len(manifest.Text.Token) > localAIOMNIMaxFailureBytes || strings.ContainsAny(manifest.Text.Token, "\r\n") {
		return errors.New("text token is invalid")
	}
	if err := validateLocalAIOMNIImage(manifest.Image); err != nil {
		return err
	}
	if err := validateLocalAIOMNIVideo(manifest.Video); err != nil {
		return err
	}
	if err := validateLocalAIOMNIIsolation(manifest.Isolation); err != nil {
		return err
	}
	if err := validateLocalAIOMNILimits(manifest.Limits); err != nil {
		return err
	}
	if err := validateLocalAIOMNIEvidencePaths(manifest.Evidence); err != nil {
		return err
	}
	return nil
}

func validateLocalAIOMNIIdentity(value, label string) error {
	if strings.TrimSpace(value) == "" || len(value) > localAIOMNIMaxIdentityLength || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s is invalid", label)
	}
	return nil
}

func validateLocalAIOMNIImage(image localAIOMNIImageFixture) error {
	if err := validateLocalAIOMNIImmutableFile(image.Path, image.SHA256, "image fixture"); err != nil {
		return err
	}
	if len(image.RequiredFacts) == 0 || len(image.RequiredFacts) > localAIOMNIMaxFacts {
		return errors.New("image rubric is empty or too large")
	}
	seen := make(map[string]struct{}, len(image.RequiredFacts))
	for _, fact := range image.RequiredFacts {
		if strings.TrimSpace(fact) == "" || len(fact) > localAIOMNIMaxFailureBytes || strings.ContainsAny(fact, "\r\n") {
			return errors.New("image rubric fact is invalid")
		}
		key := strings.ToLower(strings.TrimSpace(fact))
		if _, exists := seen[key]; exists {
			return errors.New("image rubric fact is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateLocalAIOMNIVideo(video localAIOMNIVideoFixture) error {
	if err := validateLocalAIOMNIImmutableFile(video.Path, video.SHA256, "video fixture"); err != nil {
		return err
	}
	for value, label := range map[string]string{
		video.Phase1:      "video phase 1",
		video.Phase1Color: "video phase 1 color",
		video.Phase2:      "video phase 2",
		video.Phase2Color: "video phase 2 color",
	} {
		if err := validateLocalAIOMNIIdentity(value, label); err != nil {
			return err
		}
	}
	if video.TransitionStartMilliseconds < 0 || video.TransitionEndMilliseconds < video.TransitionStartMilliseconds || video.TransitionEndMilliseconds > 24*60*60*1000 {
		return errors.New("video timing window is invalid")
	}
	return nil
}

func validateLocalAIOMNIIsolation(isolation localAIOMNIIsolation) error {
	paths := []string{isolation.WorkRoot, isolation.StateRoot, isolation.CacheRoot, isolation.TempRoot, isolation.OutputRoot, isolation.StreamsRoot}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if err := validateLocalAIOMNIAbsolutePath(path); err != nil {
			return err
		}
		key := strings.ToLower(filepath.Clean(path))
		if _, exists := seen[key]; exists {
			return errors.New("isolated roots are not unique")
		}
		seen[key] = struct{}{}
	}
	if isolation.Host != "127.0.0.1" || isolation.Port < 1 || isolation.Port > 65535 {
		return errors.New("isolation listener is not loopback and bounded")
	}
	if isolation.NetworkPolicy != localAIOMNINetworkPolicy {
		return errors.New("network policy is not fail-closed")
	}
	return nil
}

func validateLocalAIOMNILimits(limits localAIOMNILimits) error {
	if limits.TimeoutSeconds <= 0 || limits.TimeoutSeconds > 24*60*60 || limits.Processes <= 0 || limits.Processes > 128 || limits.DownloadBytes < 0 || limits.ModelCalls <= 0 || limits.ModelCalls > 1000 {
		return errors.New("execution limits are invalid")
	}
	return nil
}

func validateLocalAIOMNIEvidencePaths(evidence localAIOMNIEvidencePaths) error {
	if err := validateLocalAIOMNIAbsolutePath(evidence.ReportPath); err != nil {
		return errors.New("evidence report path is invalid")
	}
	if err := validateLocalAIOMNIAbsolutePath(evidence.LedgerPath); err != nil {
		return errors.New("evidence ledger path is invalid")
	}
	if strings.EqualFold(filepath.Clean(evidence.ReportPath), filepath.Clean(evidence.LedgerPath)) {
		return errors.New("evidence report and ledger paths collide")
	}
	return nil
}

func validateLocalAIOMNIAbsolutePath(path string) error {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n*?") {
		return errors.New("path is not absolute and bounded")
	}
	return nil
}

func validateLocalAIOMNIImmutableFile(path, expectedSHA, label string) error {
	if err := validateLocalAIOMNIAbsolutePath(path); err != nil {
		return fmt.Errorf("%s path is invalid", label)
	}
	if !localAIOMNIHash(expectedSHA) {
		return fmt.Errorf("%s hash is invalid", label)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s is unavailable: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("%s is not a nonempty regular file", label)
	}
	actual, err := localAIOMNIFileSHA256(path)
	if err != nil {
		return fmt.Errorf("%s could not be hashed: %w", label, err)
	}
	if !strings.EqualFold(actual, expectedSHA) {
		return fmt.Errorf("%s hash mismatch", label)
	}
	return nil
}

func localAIOMNIFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func localAIOMNIHash(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func localAIOMNIExecutionRootsForManifest(manifest localAIOMNIManifest) localAIOMNIExecutionRoots {
	return localAIOMNIExecutionRoots{
		Work: manifest.Isolation.WorkRoot, State: manifest.Isolation.StateRoot, Cache: manifest.Isolation.CacheRoot,
		Temp: manifest.Isolation.TempRoot, Output: manifest.Isolation.OutputRoot, Streams: manifest.Isolation.StreamsRoot,
	}
}

func prepareLocalAIOMNIRoots(roots localAIOMNIExecutionRoots) error {
	for _, root := range []string{roots.Work, roots.State, roots.Cache, roots.Temp, roots.Output, roots.Streams} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func localAIOMNIInvocationFromEnvironment() (localAIOMNIInvocation, error) {
	return localAIOMNIInvocationFromValues(os.Getenv(localAIOMNIRealEnableEnv), os.Getenv(localAIOMNIRealManifestEnv))
}

func localAIOMNIInvocationFromValues(enable, manifestPath string) (localAIOMNIInvocation, error) {
	if strings.TrimSpace(enable) != "1" {
		return localAIOMNIInvocation{}, errLocalAIOMNIRealDisabled
	}
	if err := validateLocalAIOMNIAbsolutePath(manifestPath); err != nil {
		return localAIOMNIInvocation{}, errors.New("real selector requires an absolute manifest path")
	}
	manifest, manifestSHA, err := readLocalAIOMNIManifest(manifestPath)
	if err != nil {
		return localAIOMNIInvocation{}, err
	}
	return localAIOMNIInvocation{
		Manifest:       manifest,
		ManifestSHA256: manifestSHA,
		RunID:          "real-" + manifest.Selector + "-" + manifestSHA[:12],
		EvidenceKind:   localAIOMNIRealEvidenceKind,
		Command:        localAIOMNICommandForManifest(manifest),
	}, nil
}

func readLocalAIOMNIManifest(path string) (localAIOMNIManifest, string, error) {
	if err := validateLocalAIOMNIAbsolutePath(path); err != nil {
		return localAIOMNIManifest{}, "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return localAIOMNIManifest{}, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > localAIOMNIMaxManifestBytes {
		return localAIOMNIManifest{}, "", errors.New("manifest is not a bounded regular file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return localAIOMNIManifest{}, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest localAIOMNIManifest
	if err := decoder.Decode(&manifest); err != nil {
		return localAIOMNIManifest{}, "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return localAIOMNIManifest{}, "", errors.New("manifest contains trailing JSON")
	}
	if err := validateLocalAIOMNIManifest(manifest); err != nil {
		return localAIOMNIManifest{}, "", err
	}
	return manifest, sha256Hex(body), nil
}

func localAIOMNICommandForManifest(manifest localAIOMNIManifest) localAIOMNICommand {
	prompt := "Return the exact requested text token."
	var media string
	switch manifest.Selector {
	case localAIOMNISelectorText:
		prompt = "Return this exact token: " + manifest.Text.Token
	case localAIOMNISelectorImage:
		prompt = "Name every required fact visible in the image: " + strings.Join(manifest.Image.RequiredFacts, ", ")
		media = "image=@" + manifest.Image.Path
	case localAIOMNISelectorVideo:
		prompt = fmt.Sprintf("Report %s/%s and %s/%s, and the numeric transition time in milliseconds.", manifest.Video.Phase1, manifest.Video.Phase1Color, manifest.Video.Phase2, manifest.Video.Phase2Color)
		media = "video=@" + manifest.Video.Path
	}
	arguments := []string{"--json", "models", "invoke", manifest.Model.Name, "--operation", "OMNI", "--input", "prompt=" + prompt}
	if media != "" {
		arguments = append(arguments, "--input", media)
	}
	return localAIOMNICommand{
		BinaryPath: manifest.CLI.Path,
		Arguments:  arguments,
		Environment: []string{
			"HTTP_PROXY=127.0.0.1:9",
			"HTTPS_PROXY=127.0.0.1:9",
			"NO_PROXY=127.0.0.1",
		},
	}
}

func localAIOMNIFixtureSHA(manifest localAIOMNIManifest) string {
	switch manifest.Selector {
	case localAIOMNISelectorImage:
		return manifest.Image.SHA256
	case localAIOMNISelectorVideo:
		return manifest.Video.SHA256
	default:
		return sha256Hex([]byte(manifest.Text.Token))
	}
}

func localAIOMNIRubricSHA(manifest localAIOMNIManifest) string {
	rubric := struct {
		Selector string                  `json:"selector"`
		Text     string                  `json:"text"`
		Image    localAIOMNIImageFixture `json:"image"`
		Video    localAIOMNIVideoFixture `json:"video"`
	}{Selector: manifest.Selector, Text: manifest.Text.Token, Image: manifest.Image, Video: manifest.Video}
	body, _ := json.Marshal(rubric)
	return sha256Hex(body)
}

func localAIOMNIPathIdentities(isolation localAIOMNIIsolation) []string {
	paths := []string{isolation.WorkRoot, isolation.StateRoot, isolation.CacheRoot, isolation.TempRoot, isolation.OutputRoot, isolation.StreamsRoot}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		result = append(result, localAIOMNIPathIdentity(path))
	}
	return result
}

func localAIOMNIPathIdentity(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return "sha256=" + sha256Hex([]byte(filepath.Clean(path)))
}

func localAIOMNIReadBounded(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxBytes {
		return nil, errors.New("file exceeds bounded read limit")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) != info.Size() || int64(len(body)) > maxBytes {
		return nil, errors.New("file changed during bounded read")
	}
	return body, nil
}

func localAIOMNIRedactArguments(arguments []string) []string {
	result := make([]string, 0, minInt(len(arguments), localAIOMNIMaxArguments))
	for _, argument := range arguments {
		if len(result) >= localAIOMNIMaxArguments {
			break
		}
		result = append(result, localAIOMNIRedactText(argument))
	}
	return result
}

func localAIOMNIRedactText(value string) string {
	value = localAIOMNIPathPattern.ReplaceAllStringFunc(value, func(path string) string {
		return localAIOMNIPathIdentity(path)
	})
	for _, marker := range []string{"hf_token=", "password=", "api_key=", "access_token=", "x-amz-signature=", "signed_url="} {
		lower := strings.ToLower(value)
		index := strings.Index(lower, marker)
		if index < 0 {
			continue
		}
		end := index + len(marker)
		for end < len(value) && !strings.ContainsRune(" \t\r\n,;", rune(value[end])) {
			end++
		}
		value = value[:index] + value[index:index+len(marker)] + "<redacted>" + value[end:]
	}
	if strings.Contains(strings.ToLower(value), "bearer ") {
		lower := strings.ToLower(value)
		index := strings.Index(lower, "bearer ")
		end := index + len("bearer ")
		for end < len(value) && !strings.ContainsRune(" \t\r\n,;", rune(value[end])) {
			end++
		}
		value = value[:index] + value[index:index+len("bearer ")] + "<redacted>" + value[end:]
	}
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func localAIOMNICaptureStream(body []byte) localAIOMNIStreamEvidence {
	captured := body
	truncated := false
	if len(captured) > localAIOMNIMaxStreamBytes {
		captured = captured[:localAIOMNIMaxStreamBytes]
		truncated = true
	}
	return localAIOMNIStreamEvidence{
		Bytes:     int64(len(captured)),
		SHA256:    sha256Hex(captured),
		Excerpt:   localAIOMNIBoundedText(localAIOMNIRedactText(string(captured)), localAIOMNIMaxExcerptBytes),
		Truncated: truncated,
	}
}

func localAIOMNIBoundedText(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	return "sha256=" + sha256Hex([]byte(value))
}

func localAIOMNIBoundedRedacted(value string) string {
	return localAIOMNIBoundedText(localAIOMNIRedactText(value), localAIOMNIMaxFailureBytes)
}

func localAIOMNIContainsSensitive(body []byte) bool {
	lower := strings.ToLower(string(body))
	for _, marker := range []string{
		"hf_token=", "authorization:", "bearer ", "password=", "api_key=", "access_token=", "x-amz-signature=", "signed_url=",
	} {
		index := strings.Index(lower, marker)
		if index < 0 {
			continue
		}
		value := strings.TrimSpace(lower[index+len(marker):])
		if value != "" && !strings.HasPrefix(value, "<redacted>") {
			return true
		}
	}
	return false
}

func localAIOMNIClassifyText(selector string, manifest localAIOMNIManifest, command localAIOMNICommand, body []byte) (localAIOMNISemanticEvidence, string, localAIOMNIFailure) {
	observed := strings.TrimSpace(string(body))
	semantic := localAIOMNISemanticEvidence{Observed: localAIOMNIBoundedRedacted(observed)}
	if observed == "" {
		return semantic, "FAIL", localAIOMNIFailure{Owner: "product", Assertion: "OMNI text output", Expected: "a nonempty semantic response", Observed: "empty response"}
	}
	switch selector {
	case localAIOMNISelectorText:
		semantic.Assertion = "exact requested text token"
		semantic.Expected = manifest.Text.Token
		if strings.Contains(observed, manifest.Text.Token) {
			semantic.GroundedFacts = []string{manifest.Text.Token}
			return semantic, "PASS", localAIOMNIFailure{}
		}
		return semantic, "FAIL", localAIOMNIFailure{Owner: "product", Assertion: semantic.Assertion, Expected: manifest.Text.Token, Observed: "requested token is absent"}
	case localAIOMNISelectorImage:
		semantic.Assertion = "fixture-grounded image facts"
		semantic.Expected = strings.Join(manifest.Image.RequiredFacts, ", ")
		if localAIOMNIIsPromptEcho(command, observed) {
			semantic.PromptEcho = true
			return semantic, "INCONCLUSIVE", localAIOMNIFailure{Owner: "product", Assertion: semantic.Assertion, Expected: semantic.Expected, Observed: "prompt echo did not establish image grounding"}
		}
		missing := make([]string, 0, len(manifest.Image.RequiredFacts))
		for _, fact := range manifest.Image.RequiredFacts {
			if strings.Contains(strings.ToLower(observed), strings.ToLower(fact)) {
				semantic.GroundedFacts = append(semantic.GroundedFacts, fact)
				continue
			}
			missing = append(missing, fact)
		}
		if len(missing) == 0 {
			return semantic, "PASS", localAIOMNIFailure{}
		}
		return semantic, "FAIL", localAIOMNIFailure{Owner: "product", Assertion: semantic.Assertion, Expected: semantic.Expected, Observed: "missing image facts: " + strings.Join(missing, ", ")}
	case localAIOMNISelectorVideo:
		semantic.Assertion = "grounded video phases, colors and numeric transition"
		semantic.Expected = fmt.Sprintf("%s/%s then %s/%s with transition in %d..%d ms", manifest.Video.Phase1, manifest.Video.Phase1Color, manifest.Video.Phase2, manifest.Video.Phase2Color, manifest.Video.TransitionStartMilliseconds, manifest.Video.TransitionEndMilliseconds)
		if localAIOMNIIsPromptEcho(command, observed) {
			semantic.PromptEcho = true
			return semantic, "INCONCLUSIVE", localAIOMNIFailure{Owner: "product", Assertion: semantic.Assertion, Expected: semantic.Expected, Observed: "prompt echo did not establish video grounding"}
		}
		missing := localAIOMNIMissingVideoFacts(manifest.Video, observed, &semantic)
		if len(missing) > 0 {
			return semantic, "FAIL", localAIOMNIFailure{Owner: "product", Assertion: semantic.Assertion, Expected: semantic.Expected, Observed: "missing video facts: " + strings.Join(missing, ", ")}
		}
		transition, ok := localAIOMNITransitionMilliseconds(observed)
		if !ok {
			return semantic, "INCONCLUSIVE", localAIOMNIFailure{Owner: "product", Assertion: "numeric video transition", Expected: fmt.Sprintf("transition in %d..%d ms", manifest.Video.TransitionStartMilliseconds, manifest.Video.TransitionEndMilliseconds), Observed: "grounded phases and colors were present, but no numeric transition was observed"}
		}
		semantic.NumericTransitionMilliseconds = &transition
		if transition < manifest.Video.TransitionStartMilliseconds || transition > manifest.Video.TransitionEndMilliseconds {
			return semantic, "FAIL", localAIOMNIFailure{Owner: "product", Assertion: "numeric video transition", Expected: fmt.Sprintf("transition in %d..%d ms", manifest.Video.TransitionStartMilliseconds, manifest.Video.TransitionEndMilliseconds), Observed: fmt.Sprintf("transition=%d ms", transition)}
		}
		return semantic, "PASS", localAIOMNIFailure{}
	default:
		return semantic, "FAIL", localAIOMNIFailure{Owner: "harness", Assertion: "semantic selector", Expected: "text, image or video", Observed: selector}
	}
}

func localAIOMNIIsPromptEcho(command localAIOMNICommand, observed string) bool {
	for index, argument := range command.Arguments {
		if argument != "--input" || index+1 >= len(command.Arguments) {
			continue
		}
		input := command.Arguments[index+1]
		if !strings.HasPrefix(input, "prompt=") {
			continue
		}
		prompt := strings.TrimSpace(strings.TrimPrefix(input, "prompt="))
		return strings.EqualFold(strings.TrimSpace(observed), prompt) || strings.Contains(strings.ToLower(observed), strings.ToLower(prompt))
	}
	return false
}

func localAIOMNIMissingVideoFacts(video localAIOMNIVideoFixture, observed string, semantic *localAIOMNISemanticEvidence) []string {
	missing := make([]string, 0, 4)
	for _, fact := range []string{video.Phase1, video.Phase1Color, video.Phase2, video.Phase2Color} {
		if strings.Contains(strings.ToLower(observed), strings.ToLower(fact)) {
			semantic.GroundedFacts = append(semantic.GroundedFacts, fact)
			continue
		}
		missing = append(missing, fact)
	}
	return missing
}

func localAIOMNITransitionMilliseconds(observed string) (int64, bool) {
	matches := localAIOMNITransitionPattern.FindStringSubmatch(observed)
	if len(matches) != 2 {
		return 0, false
	}
	value, err := strconv.ParseInt(matches[1], 10, 64)
	return value, err == nil
}

func localAIOMNISetCacheBefore(cache *localAIOMNICacheEvidence, snapshot localAIOMNICacheSnapshot) {
	cache.BeforeSHA256 = snapshot.IdentitySHA256
	cache.BeforeEntries = snapshot.Entries
	cache.BeforeFiles = snapshot.Files
	cache.BeforeBytes = snapshot.Bytes
	cache.PartialArtifacts = snapshot.Partial
}

func localAIOMNISetCacheAfter(cache *localAIOMNICacheEvidence, snapshot localAIOMNICacheSnapshot) {
	cache.AfterSHA256 = snapshot.IdentitySHA256
	cache.AfterEntries = snapshot.Entries
	cache.AfterFiles = snapshot.Files
	cache.AfterBytes = snapshot.Bytes
	cache.PartialArtifacts = snapshot.Partial
}

func localAIOMNISnapshotRoots(roots localAIOMNIExecutionRoots) (localAIOMNICacheSnapshot, error) {
	hasher := sha256.New()
	var entries, files, partial int
	var bytesCount int64
	orderedRoots := []struct {
		name string
		root string
	}{
		{name: "work", root: roots.Work}, {name: "state", root: roots.State}, {name: "cache", root: roots.Cache},
		{name: "temp", root: roots.Temp}, {name: "output", root: roots.Output}, {name: "streams", root: roots.Streams},
	}
	for _, ownedRoot := range orderedRoots {
		if err := localAIOMNIHashRoot(hasher, ownedRoot.name, ownedRoot.root, &entries, &files, &bytesCount, &partial); err != nil {
			return localAIOMNICacheSnapshot{}, err
		}
	}
	return localAIOMNICacheSnapshot{IdentitySHA256: hex.EncodeToString(hasher.Sum(nil)), Entries: entries, Files: files, Bytes: bytesCount, Partial: partial}, nil
}

func localAIOMNIHashRoot(hasher io.Writer, name, root string, entries, files *int, bytesCount *int64, partial *int) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("cache root is empty")
	}
	_, _ = io.WriteString(hasher, name+"\x00")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("owned root contains a symlink")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		*entries++
		if strings.HasSuffix(strings.ToLower(relative), ".partial") || strings.Contains(strings.ToLower(relative), "/.partial/") {
			*partial++
		}
		if entry.IsDir() {
			_, _ = io.WriteString(hasher, relative+"\x00dir\n")
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("owned root contains a non-regular entry")
		}
		identity, err := localAIOMNIFileSHA256(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		*files++
		*bytesCount += info.Size()
		_, _ = io.WriteString(hasher, fmt.Sprintf("%s\x00file:%d:%s\n", relative, info.Size(), identity))
		return nil
	})
	return err
}

func writeLocalAIOMNIReportAtomic(path string, report localAIOMNIReport) error {
	return writeLocalAIOMNIReportAtomicWithHook(path, report, nil)
}

func writeLocalAIOMNIReportAtomicWithHook(path string, report localAIOMNIReport, beforeReplace func() error) error {
	if err := validateLocalAIOMNIReport(report); err != nil {
		return err
	}
	return writeLocalAIJSONAtomic(path, report, beforeReplace)
}

func validateLocalAIOMNIReport(report localAIOMNIReport) error {
	if report.Schema != localAIOMNIEvidenceSchema || report.EvidenceKind != localAIOMNIEvidenceKind && report.EvidenceKind != localAIOMNIRealEvidenceKind || report.RunID == "" || report.Platform == "" || report.Architecture == "" || !report.Redacted || !localAIOMNIStatus(report.Status) {
		return errors.New("report identity or status is invalid")
	}
	if report.ManifestSHA256 != "" && !localAIOMNIHash(report.ManifestSHA256) {
		return errors.New("report manifest identity is invalid")
	}
	if !localAIOMNISelector(report.Selector) {
		if report.Failure == nil || report.Failure.Assertion != "manifest admission" {
			return errors.New("report selector is invalid")
		}
	}
	if err := validateLocalAIOMNIInputIdentity(report.InputIdentity); err != nil && !localAIOMNIAdmissionFailure(report) {
		return err
	}
	if err := validateLocalAIOMNIPolicy(report.Policy); err != nil && !localAIOMNIAdmissionFailure(report) {
		return err
	}
	if len(report.Command.Arguments) > localAIOMNIMaxArguments || !report.Command.SecretsRedacted {
		return errors.New("command evidence is not bounded or redacted")
	}
	if err := validateLocalAIOMNIStream(report.Command.Stdout); err != nil && !localAIOMNIAdmissionFailure(report) && !localAIOMNIExecutionEvidenceAbsent(report) {
		return err
	}
	if err := validateLocalAIOMNIStream(report.Command.Stderr); err != nil && !localAIOMNIAdmissionFailure(report) && !localAIOMNIExecutionEvidenceAbsent(report) {
		return err
	}
	if err := validateLocalAIOMNILogs(report.Logs); err != nil {
		return err
	}
	for _, artifact := range report.Artifacts {
		if artifact.Kind == "" || artifact.MediaType == "" || artifact.Bytes <= 0 || artifact.Bytes > localAIOMNIMaxStreamBytes || !localAIOMNIHash(artifact.SHA256) || !localAIOMNIPathIdentityValid(artifact.PathIdentity) {
			return errors.New("artifact evidence is invalid")
		}
	}
	if err := validateLocalAIOMNICache(report.Cache); err != nil && !localAIOMNIAdmissionFailure(report) {
		return err
	}
	if report.Failure != nil {
		if !localAIOMNIFailureOwner(report.Failure.Owner) || report.Failure.Assertion == "" || report.Failure.Expected == "" || report.Failure.Observed == "" || len(report.Failure.Observed) > localAIOMNIMaxFailureBytes {
			return errors.New("failure evidence is invalid")
		}
	}
	if report.Reservation != nil && (report.Reservation.ID == "" || report.Reservation.Kind != localAIOMNIModelCallKind || report.Reservation.Amount <= 0 || report.Reservation.State != "RESERVED" && report.Reservation.State != "COMMITTED") {
		return errors.New("reservation evidence is invalid")
	}
	if report.Activity.BackendProcesses < 0 || report.Activity.ExternalRequests < 0 || report.Activity.Downloads < 0 || report.Activity.ModelCalls < 0 {
		return errors.New("activity evidence is invalid")
	}
	if report.Release.OwnedProcesses < 0 || report.Release.OwnedListeners < 0 || report.Release.OwnedLeases < 0 || report.Release.PartialArtifacts < 0 {
		return errors.New("release evidence is invalid")
	}
	if report.EvidenceKind == localAIOMNIEvidenceKind && (report.Activity.BackendProcesses != 0 || report.Activity.ExternalRequests != 0 || report.Activity.Downloads != 0 || report.Activity.ModelCalls != 0) {
		return errors.New("controlled evidence recorded real dependency activity")
	}
	if report.Status == "PASS" {
		if report.Failure != nil || !report.Semantic.Passed || report.Reservation == nil || report.Reservation.State != "COMMITTED" || len(report.Artifacts) == 0 || len(report.Logs.Backend) == 0 || len(report.Logs.Runtime) == 0 || !report.Release.Checked || report.Release.InspectionFailed || !report.Release.ProcessTreeClosed || report.Release.OwnedProcesses != 0 || report.Release.OwnedListeners != 0 || report.Release.OwnedLeases != 0 || report.Release.PartialArtifacts != 0 || report.Cache.AfterInspectionFailed {
			return errors.New("pass report omitted semantic, reservation or release proof")
		}
	} else if report.Failure == nil {
		return errors.New("non-pass report omitted bounded failure")
	}
	if !report.Release.Checked && report.Failure != nil && report.Failure.Assertion != "manifest admission" && report.Failure.Assertion != "final cache and release inspection" {
		return errors.New("release inspection was not established")
	}
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if localAIOMNIContainsSensitive(body) || localAIOMNIPathPattern.Match(body) {
		return errors.New("report contains unredacted sensitive evidence")
	}
	return nil
}

func validateLocalAIOMNIInputIdentity(identity localAIOMNIInputIdentity) error {
	for _, value := range []string{identity.CLISHA256, identity.FixtureSHA256, identity.ImageFixtureSHA256, identity.VideoFixtureSHA256, identity.RubricSHA256} {
		if !localAIOMNIHash(value) {
			return errors.New("input identity is invalid")
		}
	}
	if strings.TrimSpace(identity.ModelIdentity) == "" || strings.TrimSpace(identity.BackendIdentity) == "" {
		return errors.New("model or backend identity is missing")
	}
	return nil
}

func validateLocalAIOMNIPolicy(policy localAIOMNIPolicy) error {
	if len(policy.RootIdentities) != 6 || policy.Host != "127.0.0.1" || policy.Port < 1 || policy.Port > 65535 || policy.TimeoutSeconds <= 0 || policy.ProcessLimit <= 0 || policy.DownloadByteLimit < 0 || policy.ModelCallLimit <= 0 || policy.NetworkPolicy != localAIOMNINetworkPolicy {
		return errors.New("policy evidence is invalid")
	}
	for _, identity := range policy.RootIdentities {
		if !localAIOMNIPathIdentityValid(identity) {
			return errors.New("root identity is invalid")
		}
	}
	return nil
}

func validateLocalAIOMNIStream(stream localAIOMNIStreamEvidence) error {
	if stream.Bytes < 0 || stream.Bytes > localAIOMNIMaxStreamBytes || !localAIOMNIHash(stream.SHA256) || len(stream.Excerpt) > localAIOMNIMaxExcerptBytes || localAIOMNIContainsSensitive([]byte(stream.Excerpt)) {
		return errors.New("stream evidence is invalid")
	}
	return nil
}

func validateLocalAIOMNILogs(logs localAIOMNILogs) error {
	for _, entries := range [][]localAIOMNILogEvidence{logs.Backend, logs.Runtime} {
		for _, entry := range entries {
			if !localAIOMNIPathIdentityValid(entry.PathIdentity) || entry.Bytes < 0 || entry.Bytes > localAIOMNIMaxStreamBytes || !localAIOMNIHash(entry.SHA256) || len(entry.Excerpt) > localAIOMNIMaxExcerptBytes || localAIOMNIContainsSensitive([]byte(entry.Excerpt)) {
				return errors.New("log evidence is invalid")
			}
		}
	}
	return nil
}

func validateLocalAIOMNICache(cache localAIOMNICacheEvidence) error {
	if !localAIOMNIHash(cache.BeforeSHA256) || !localAIOMNIHash(cache.AfterSHA256) || cache.BeforeEntries < 0 || cache.AfterEntries < 0 || cache.BeforeFiles < 0 || cache.AfterFiles < 0 || cache.BeforeBytes < 0 || cache.AfterBytes < 0 || cache.PartialArtifacts < 0 {
		return errors.New("cache evidence is invalid")
	}
	return nil
}

func localAIOMNIAdmissionFailure(report localAIOMNIReport) bool {
	return report.Failure != nil && report.Failure.Assertion == "manifest admission"
}

func localAIOMNIExecutionEvidenceAbsent(report localAIOMNIReport) bool {
	return report.Reservation == nil && report.Command.Stdout.SHA256 == "" && report.Command.Stderr.SHA256 == ""
}

func localAIOMNIStatus(value string) bool {
	return value == "PASS" || value == "FAIL" || value == "INCONCLUSIVE"
}

func localAIOMNISelector(value string) bool {
	return value == localAIOMNISelectorText || value == localAIOMNISelectorImage || value == localAIOMNISelectorVideo
}

func localAIOMNIFailureOwner(value string) bool {
	switch value {
	case "product", "harness", "environment", "unresolved":
		return true
	default:
		return false
	}
}

func localAIOMNIPathIdentityValid(value string) bool {
	return strings.HasPrefix(value, "sha256=") && localAIOMNIHash(strings.TrimPrefix(value, "sha256="))
}

type localAIOMNIControlledExecutor struct {
	Observation localAIOMNIObservation
	Calls       atomic.Int32
}

func (executor *localAIOMNIControlledExecutor) Execute(_ context.Context, _ localAIOMNICommand, _ localAIOMNIExecutionRoots) localAIOMNIObservation {
	executor.Calls.Add(1)
	return executor.Observation
}

type localAIOMNICancellationExecutor struct {
	Started chan struct{}
	once    sync.Once
}

func (executor *localAIOMNICancellationExecutor) Execute(ctx context.Context, _ localAIOMNICommand, _ localAIOMNIExecutionRoots) localAIOMNIObservation {
	executor.once.Do(func() { close(executor.Started) })
	<-ctx.Done()
	return localAIOMNIObservation{
		Started: true, ProcessExited: true, ExitCode: 1, TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded), Cancelled: true, ProcessTreeClosed: true,
		BackendLogs: []localAIOMNIRawLog{{Path: "backend.log", Body: []byte("cancelled")}}, RuntimeLogs: []localAIOMNIRawLog{{Path: "runtime.log", Body: []byte("cancelled")}},
	}
}

type localAIOMNIProcessExecutor struct{}

func (localAIOMNIProcessExecutor) Execute(ctx context.Context, command localAIOMNICommand, roots localAIOMNIExecutionRoots) localAIOMNIObservation {
	observation := localAIProcessExecutor{}.Execute(ctx, localAICommandSpec{BinaryPath: command.BinaryPath, Arguments: command.Arguments, Environment: command.Environment, OutputName: "omni-output.txt"}, localAIRealRoots{Work: roots.Work})
	return localAIOMNIObservation{
		Started: observation.Started, ProcessExited: observation.ProcessExited, ExitCode: observation.ExitCode,
		TimedOut: observation.TimedOut, ProcessTreeClosed: observation.ProcessTreeClosed,
		OwnedProcesses: observation.OwnedProcesses, OwnedListeners: observation.OwnedListeners, OwnedLeases: observation.OwnedLeases,
		Stdout: observation.Stdout, Stderr: observation.Stderr,
		Artifacts: []localAIOMNIRawArtifact{{Kind: "text", Path: "stdout", MediaType: "text/plain", Body: observation.Stdout}},
	}
}

func mustLocalAIOMNIRunner(t testing.TB, executor localAIOMNIExecutor) localAIOMNIRunner {
	t.Helper()
	locks, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		t.Fatalf("new omni lock service: %v", err)
	}
	return localAIOMNIRunner{executor: executor, locks: locks, writeReport: writeLocalAIOMNIReportAtomic}
}

func localAIOMNIInvocationForTest(t testing.TB, manifest localAIOMNIManifest, runID string, evidenceKind string) localAIOMNIInvocation {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal omni manifest: %v", err)
	}
	return localAIOMNIInvocation{
		Manifest:       manifest,
		ManifestSHA256: sha256Hex(body),
		RunID:          runID,
		EvidenceKind:   evidenceKind,
		Command:        localAIOMNICommandForManifest(manifest),
	}
}

func localAIOMNIManifestForTest(t testing.TB, root string) localAIOMNIManifest {
	t.Helper()
	cliPath := filepath.Join(root, "bin", "you.exe")
	imagePath := filepath.Join(root, "fixtures", "infinite-you.png")
	videoPath := filepath.Join(root, "fixtures", "groundtruth-fixture.mp4")
	for path, body := range map[string][]byte{
		cliPath:   []byte("controlled CLI identity"),
		imagePath: []byte("controlled image fixture: infinity symbol / INFINITE YOU"),
		videoPath: []byte("controlled video fixture: PHASE 1 red then PHASE 2 blue"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("prepare fixture directory: %v", err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
	manifest := localAIOMNIManifest{
		Schema:   localAIOMNIInputSchema,
		Selector: localAIOMNISelectorText,
		CLI:      localAIOMNICLIIdentity{Path: cliPath, SHA256: localAIOMNIFileHashForTest(t, cliPath)},
		Model:    localAIOMNIModelIdentity{Name: "llm", Identity: "model-revision-controlled"},
		Backend:  localAIOMNIBackend{Identity: "backend-revision-controlled"},
		Text:     localAIOMNITextFixture{Token: "COBALT-17"},
		Image: localAIOMNIImageFixture{
			Path: imagePath, SHA256: localAIOMNIFileHashForTest(t, imagePath), RequiredFacts: []string{"infinity symbol", "INFINITE YOU"},
		},
		Video: localAIOMNIVideoFixture{
			Path: videoPath, SHA256: localAIOMNIFileHashForTest(t, videoPath), Phase1: "PHASE 1", Phase1Color: "red", Phase2: "PHASE 2", Phase2Color: "blue", TransitionStartMilliseconds: 1500, TransitionEndMilliseconds: 2500,
		},
		Isolation: localAIOMNIIsolation{
			WorkRoot: filepath.Join(root, "work"), StateRoot: filepath.Join(root, "state"), CacheRoot: filepath.Join(root, "cache"), TempRoot: filepath.Join(root, "temp"), OutputRoot: filepath.Join(root, "output"), StreamsRoot: filepath.Join(root, "streams"), Host: "127.0.0.1", Port: 54321, NetworkPolicy: localAIOMNINetworkPolicy,
		},
		Limits:   localAIOMNILimits{TimeoutSeconds: 1200, Processes: 4, DownloadBytes: 0, ModelCalls: 1},
		Evidence: localAIOMNIEvidencePaths{ReportPath: filepath.Join(root, "evidence", "omni-report.json"), LedgerPath: filepath.Join(root, "evidence", "omni-ledger.json")},
	}
	return manifest
}

func localAIOMNIFileHashForTest(t testing.TB, path string) string {
	t.Helper()
	hash, err := localAIOMNIFileSHA256(path)
	if err != nil {
		t.Fatalf("hash fixture %s: %v", path, err)
	}
	return hash
}

func localAIOMNIPassObservation(root string, output string) localAIOMNIObservation {
	body := []byte(output)
	return localAIOMNIObservation{
		ProcessExited: true, ExitCode: 0, ProcessTreeClosed: true, Stdout: body,
		BackendLogs: []localAIOMNIRawLog{{Path: filepath.Join(root, "backend.log"), Body: []byte("backend completed")}},
		RuntimeLogs: []localAIOMNIRawLog{{Path: filepath.Join(root, "runtime.log"), Body: []byte("runtime completed")}},
		Artifacts:   []localAIOMNIRawArtifact{{Kind: "text", Path: filepath.Join(root, "output.txt"), MediaType: "text/plain", Body: body}},
	}
}

func localAIOMNIReadReport(t testing.TB, path string) localAIOMNIReport {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read omni report: %v", err)
	}
	var report localAIOMNIReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatalf("decode omni report: %v", err)
	}
	return report
}

func TestLocalAIOMNIRealDiagnosticRunnerControlled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		test func(*testing.T)
	}{
		{name: "I01-controlled-default-zero-real-activity", test: localAIOMNII01Controlled},
		{name: "I02-incomplete-manifest-rejects-before-launch", test: localAIOMNII02Admission},
		{name: "I03-durable-budget-and-race-safe-reservation", test: localAIOMNII03Budget},
		{name: "I04-fixture-hash-rejection-before-launch", test: localAIOMNII04FixtureAdmission},
		{name: "I05-text-exact-token-pass", test: localAIOMNII05TextPass},
		{name: "I06-text-token-absence-fail", test: localAIOMNII06TextFail},
		{name: "I07-image-grounded-facts-pass", test: localAIOMNII07ImagePass},
		{name: "I08-video-grounded-numeric-window-pass", test: localAIOMNII08VideoPass},
		{name: "I09-video-vague-midpoint-inconclusive", test: localAIOMNII09VideoVague},
		{name: "I10-video-prompt-echo-inconclusive", test: localAIOMNII10VideoEcho},
		{name: "I11-video-numeric-window-fail", test: localAIOMNII11VideoOutsideWindow},
		{name: "I12-bounded-redacted-evidence", test: localAIOMNII12Evidence},
		{name: "I13-atomic-report-interruption", test: localAIOMNII13AtomicReport},
		{name: "I14-cancellation-and-timeout-cleanup", test: localAIOMNII14Cancellation},
		{name: "I15-owned-resource-leak-never-passes", test: localAIOMNII15Cleanup},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			testCase.test(t)
		})
	}
}

func localAIOMNII01Controlled(t *testing.T) {
	if _, err := localAIOMNIInvocationFromValues("", ""); !errors.Is(err, errLocalAIOMNIRealDisabled) {
		t.Fatalf("real selector admission error=%v, want disabled before manifest lookup", err)
	}
	if _, err := localAIOMNIInvocationFromValues("1", "relative-manifest.json"); err == nil {
		t.Fatal("real selector accepted a non-absolute manifest path")
	}
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifestPath := filepath.Join(root, "manifest.json")
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBody, 0o600); err != nil {
		t.Fatal(err)
	}
	admitted, err := localAIOMNIInvocationFromValues("1", manifestPath)
	if err != nil || admitted.EvidenceKind != localAIOMNIRealEvidenceKind {
		t.Fatalf("immutable real-selector admission=%#v err=%v, want validated opt-in contract", admitted, err)
	}
	executor := &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(root, manifest.Text.Token)}
	report, err := mustLocalAIOMNIRunner(t, executor).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I01", localAIOMNIEvidenceKind))
	if err != nil {
		t.Fatalf("controlled run: %v", err)
	}
	if report.Status != "PASS" || report.EvidenceKind != localAIOMNIEvidenceKind || executor.Calls.Load() != 1 {
		t.Fatalf("report=%#v calls=%d, want controlled PASS with one in-process helper call", report, executor.Calls.Load())
	}
	if report.Activity != (localAIOMNIActivity{}) || report.Reservation == nil || report.Reservation.State != "COMMITTED" {
		t.Fatalf("activity/reservation=%#v/%#v, want zero real activity and committed controlled budget", report.Activity, report.Reservation)
	}
	if !report.Release.Checked || report.Release.OwnedProcesses != 0 || report.Release.OwnedListeners != 0 || report.Release.OwnedLeases != 0 || report.Release.PartialArtifacts != 0 {
		t.Fatalf("release=%#v, want clean release", report.Release)
	}
	body, err := os.ReadFile(manifest.Evidence.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(manifest.Isolation.WorkRoot)) || bytes.Contains(body, []byte(manifest.CLI.Path)) {
		t.Fatalf("report leaked an owned path: %s", body)
	}
	if got := localAIOMNIReadReport(t, manifest.Evidence.ReportPath); got.Status != "PASS" {
		t.Fatalf("persisted report status=%q, want PASS", got.Status)
	}
}

func localAIOMNII02Admission(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Text.Token = ""
	executor := &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(root, "unused")}
	report, err := mustLocalAIOMNIRunner(t, executor).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I02", localAIOMNIEvidenceKind))
	if err != nil {
		t.Fatalf("admission run: %v", err)
	}
	if report.Status != "FAIL" || report.Failure == nil || report.Failure.Assertion != "manifest admission" || executor.Calls.Load() != 0 {
		t.Fatalf("report=%#v calls=%d, want pre-launch manifest FAIL", report, executor.Calls.Load())
	}
	if _, err := os.Stat(manifest.Evidence.LedgerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ledger stat=%v, want no reservation ledger", err)
	}
}

func localAIOMNII03Budget(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	firstExecutor := &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(root, manifest.Text.Token)}
	first, err := mustLocalAIOMNIRunner(t, firstExecutor).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I03", localAIOMNIEvidenceKind))
	if err != nil || first.Status != "PASS" {
		t.Fatalf("first budget run report=%#v err=%v, want PASS", first, err)
	}
	manifest.Evidence.ReportPath = filepath.Join(root, "evidence", "second-report.json")
	secondExecutor := &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(root, manifest.Text.Token)}
	second, err := mustLocalAIOMNIRunner(t, secondExecutor).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I03", localAIOMNIEvidenceKind))
	if err != nil || second.Status != "FAIL" || second.Failure == nil || second.Failure.Assertion != "durable model-call budget" || secondExecutor.Calls.Load() != 0 {
		t.Fatalf("second budget report=%#v calls=%d err=%v, want durable exhaustion before launch", second, secondExecutor.Calls.Load(), err)
	}

	raceRoot := t.TempDir()
	sharedLedger := filepath.Join(raceRoot, "ledger.json")
	const attempts = 9
	results := make(chan localAIOMNIReport, attempts)
	errorsSeen := make(chan error, attempts)
	var group sync.WaitGroup
	for index := 0; index < attempts; index++ {
		index := index
		group.Add(1)
		go func() {
			defer group.Done()
			cellRoot := filepath.Join(raceRoot, fmt.Sprintf("cell-%d", index))
			cellManifest := localAIOMNIManifestForTest(t, cellRoot)
			cellManifest.Limits.ModelCalls = 3
			cellManifest.Evidence.LedgerPath = sharedLedger
			cellManifest.Evidence.ReportPath = filepath.Join(cellRoot, "report.json")
			executor := &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(cellRoot, cellManifest.Text.Token)}
			report, runErr := mustLocalAIOMNIRunner(t, executor).Run(context.Background(), localAIOMNIInvocationForTest(t, cellManifest, "I03-race", localAIOMNIEvidenceKind))
			results <- report
			errorsSeen <- runErr
		}()
	}
	group.Wait()
	close(results)
	close(errorsSeen)
	passCount, exhaustedCount := 0, 0
	for report := range results {
		if report.Status == "PASS" {
			passCount++
			continue
		}
		if report.Status == "FAIL" && report.Failure != nil && report.Failure.Assertion == "durable model-call budget" {
			exhaustedCount++
			continue
		}
		t.Fatalf("unexpected race report=%#v", report)
	}
	for runErr := range errorsSeen {
		if runErr != nil {
			t.Fatalf("race run error: %v", runErr)
		}
	}
	if passCount != 3 || exhaustedCount != attempts-3 {
		t.Fatalf("race admissions=%d exhausted=%d, want 3 and %d", passCount, exhaustedCount, attempts-3)
	}
}

func localAIOMNII04FixtureAdmission(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Selector = localAIOMNISelectorImage
	manifest.Image.SHA256 = strings.Repeat("0", sha256.Size*2)
	executor := &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(root, "unused")}
	report, err := mustLocalAIOMNIRunner(t, executor).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I04", localAIOMNIEvidenceKind))
	if err != nil || report.Status != "FAIL" || report.Failure == nil || report.Failure.Assertion != "manifest admission" || executor.Calls.Load() != 0 {
		t.Fatalf("fixture admission report=%#v calls=%d err=%v, want pre-launch rejection", report, executor.Calls.Load(), err)
	}
}

func localAIOMNII05TextPass(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	report, err := localAIOMNIRunWithObservation(t, manifest, "I05", localAIOMNISelectorText, localAIOMNIPassObservation(root, manifest.Text.Token))
	if err != nil || report.Status != "PASS" || !report.Semantic.Passed || len(report.Semantic.GroundedFacts) != 1 {
		t.Fatalf("text pass report=%#v err=%v", report, err)
	}
}

func localAIOMNII06TextFail(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	response := "COBALT-18"
	report, err := localAIOMNIRunWithObservation(t, manifest, "I06", localAIOMNISelectorText, localAIOMNIPassObservation(root, response))
	if err != nil || report.Status != "FAIL" || report.Failure == nil || report.Failure.Owner != "product" || report.Semantic.Passed {
		t.Fatalf("text failure report=%#v err=%v", report, err)
	}
}

func localAIOMNII07ImagePass(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Selector = localAIOMNISelectorImage
	response := "The image shows an infinity symbol and the words INFINITE YOU."
	report, err := localAIOMNIRunWithObservation(t, manifest, "I07", localAIOMNISelectorImage, localAIOMNIPassObservation(root, response))
	if err != nil || report.Status != "PASS" || len(report.Semantic.GroundedFacts) != 2 {
		t.Fatalf("image pass report=%#v err=%v", report, err)
	}
}

func localAIOMNII08VideoPass(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Selector = localAIOMNISelectorVideo
	response := "PHASE 1 is red, then PHASE 2 is blue; the transition occurred at 2000 ms."
	report, err := localAIOMNIRunWithObservation(t, manifest, "I08", localAIOMNISelectorVideo, localAIOMNIPassObservation(root, response))
	if err != nil || report.Status != "PASS" || report.Semantic.NumericTransitionMilliseconds == nil || *report.Semantic.NumericTransitionMilliseconds != 2000 {
		t.Fatalf("video pass report=%#v err=%v", report, err)
	}
}

func localAIOMNII09VideoVague(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Selector = localAIOMNISelectorVideo
	response := "PHASE 1 is red, then PHASE 2 is blue around the midpoint."
	report, err := localAIOMNIRunWithObservation(t, manifest, "I09", localAIOMNISelectorVideo, localAIOMNIPassObservation(root, response))
	if err != nil || report.Status != "INCONCLUSIVE" || report.Failure == nil || report.Failure.Owner != "product" || report.Semantic.NumericTransitionMilliseconds != nil {
		t.Fatalf("video vague report=%#v err=%v", report, err)
	}
}

func localAIOMNII10VideoEcho(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Selector = localAIOMNISelectorVideo
	command := localAIOMNICommandForManifest(manifest)
	var prompt string
	for index, argument := range command.Arguments {
		if argument == "--input" && index+1 < len(command.Arguments) && strings.HasPrefix(command.Arguments[index+1], "prompt=") {
			prompt = strings.TrimPrefix(command.Arguments[index+1], "prompt=")
		}
	}
	invocation := localAIOMNIInvocationForTest(t, manifest, "I10", localAIOMNIEvidenceKind)
	report, err := mustLocalAIOMNIRunner(t, &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(root, prompt)}).Run(t.Context(), invocation)
	if err != nil || report.Status != "INCONCLUSIVE" || report.Failure == nil || !report.Semantic.PromptEcho {
		t.Fatalf("video echo report=%#v err=%v", report, err)
	}
}

func localAIOMNII11VideoOutsideWindow(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Selector = localAIOMNISelectorVideo
	response := "PHASE 1 red; PHASE 2 blue; transition at 3000 ms."
	report, err := localAIOMNIRunWithObservation(t, manifest, "I11", localAIOMNISelectorVideo, localAIOMNIPassObservation(root, response))
	if err != nil || report.Status != "FAIL" || report.Failure == nil || report.Failure.Owner != "product" || report.Semantic.NumericTransitionMilliseconds == nil {
		t.Fatalf("video outside-window report=%#v err=%v", report, err)
	}
}

func localAIOMNII12Evidence(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	secret := []byte("HF_TOKEN=controlled-secret ")
	secret = append(secret, bytes.Repeat([]byte("bounded-output "), localAIOMNIMaxStreamBytes)...)
	observation := localAIOMNIPassObservation(root, manifest.Text.Token)
	observation.Stderr = secret
	executor := &localAIOMNIControlledExecutor{Observation: observation}
	report, err := mustLocalAIOMNIRunner(t, executor).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I12", localAIOMNIEvidenceKind))
	if err != nil || report.Status != "FAIL" || report.Failure == nil || report.Failure.Owner != "harness" {
		t.Fatalf("evidence report=%#v err=%v, want bounded redaction failure", report, err)
	}
	if !report.Command.Stderr.Truncated || len(report.Command.Stderr.Excerpt) > localAIOMNIMaxExcerptBytes {
		t.Fatalf("stderr evidence=%#v, want bounded/truncated evidence", report.Command.Stderr)
	}
	body, err := os.ReadFile(manifest.Evidence.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("controlled-secret")) || bytes.Contains(body, []byte("HF_TOKEN=controlled-secret")) {
		t.Fatalf("report leaked secret: %s", body)
	}
}

func localAIOMNII13AtomicReport(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Limits.ModelCalls = 2
	first := &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(root, manifest.Text.Token)}
	if report, err := mustLocalAIOMNIRunner(t, first).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I13", localAIOMNIEvidenceKind)); err != nil || report.Status != "PASS" {
		t.Fatalf("seed report=%#v err=%v", report, err)
	}
	canonicalBefore, err := os.ReadFile(manifest.Evidence.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	second := &localAIOMNIControlledExecutor{Observation: localAIOMNIPassObservation(root, manifest.Text.Token)}
	runner := mustLocalAIOMNIRunner(t, second)
	runner.writeReport = func(path string, report localAIOMNIReport) error {
		return writeLocalAIOMNIReportAtomicWithHook(path, report, func() error { return errLocalAIOMNIReportInterrupted })
	}
	report, err := runner.Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I13", localAIOMNIEvidenceKind))
	if !errors.Is(err, errLocalAIOMNIReportInterrupted) || report.Status != "PASS" {
		t.Fatalf("interrupted report=%#v err=%v, want in-memory PASS and interruption", report, err)
	}
	canonicalAfter, err := os.ReadFile(manifest.Evidence.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalBefore, canonicalAfter) {
		t.Fatal("interrupted atomic report replaced the prior canonical report")
	}
}

func localAIOMNII14Cancellation(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	executor := &localAIOMNICancellationExecutor{Started: make(chan struct{})}
	runner := mustLocalAIOMNIRunner(t, executor)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	type result struct {
		report localAIOMNIReport
		err    error
	}
	done := make(chan result, 1)
	go func() {
		report, err := runner.Run(ctx, localAIOMNIInvocationForTest(t, manifest, "I14", localAIOMNIEvidenceKind))
		done <- result{report: report, err: err}
	}()
	<-executor.Started
	cancel()
	outcome := <-done
	if outcome.err != nil || outcome.report.Status != "FAIL" || outcome.report.Failure == nil || outcome.report.Failure.Owner != "harness" {
		t.Fatalf("cancellation report=%#v err=%v, want bounded harness FAIL", outcome.report, outcome.err)
	}
	if !outcome.report.Release.Checked || outcome.report.Release.OwnedProcesses != 0 || outcome.report.Release.OwnedListeners != 0 || outcome.report.Release.OwnedLeases != 0 || !outcome.report.Release.ProcessTreeClosed {
		t.Fatalf("cancellation release=%#v, want clean release inspection", outcome.report.Release)
	}
}

func localAIOMNII15Cleanup(t *testing.T) {
	root := t.TempDir()
	manifest := localAIOMNIManifestForTest(t, root)
	if err := os.MkdirAll(manifest.Isolation.OutputRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifest.Isolation.OutputRoot, "response.partial"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	observation := localAIOMNIPassObservation(root, manifest.Text.Token)
	observation.Started = true
	observation.ProcessExited = false
	observation.ProcessTreeClosed = false
	observation.OwnedProcesses = 1
	report, err := mustLocalAIOMNIRunner(t, &localAIOMNIControlledExecutor{Observation: observation}).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, "I15", localAIOMNIEvidenceKind))
	if err != nil || report.Status != "FAIL" || report.Failure == nil || report.Failure.Owner != "harness" {
		t.Fatalf("cleanup report=%#v err=%v, want harness FAIL", report, err)
	}
	if !report.Release.Checked || report.Release.PartialArtifacts == 0 || report.Release.ProcessTreeClosed || report.Release.OwnedProcesses != 1 {
		t.Fatalf("cleanup release=%#v, want checked leak evidence", report.Release)
	}
}

func localAIOMNIRunWithObservation(t *testing.T, manifest localAIOMNIManifest, runID, selector string, observation localAIOMNIObservation) (localAIOMNIReport, error) {
	t.Helper()
	manifest.Selector = selector
	executor := &localAIOMNIControlledExecutor{Observation: observation}
	return mustLocalAIOMNIRunner(t, executor).Run(t.Context(), localAIOMNIInvocationForTest(t, manifest, runID, localAIOMNIEvidenceKind))
}
