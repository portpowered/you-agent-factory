//go:build windows

package omni_diag

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/locking"
	"golang.org/x/sys/windows"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	localAIOMNIInputSchema    = "localai.windows-omni-diagnostic-input.v1"
	localAIOMNIEvidenceSchema = "localai.windows-omni-diagnostic-evidence.v1"
	localAIOMNIControlled     = "controlled"
	localAIOMNIReal           = "real"
	localAIOMNIText           = "text"
	localAIOMNIImage          = "image"
	localAIOMNIVideo          = "video"
	localAIOMNIModelCall      = "modelCall"
	localAIOMNINetworkPolicy  = "loopback-only-external-denied"
	localAIOMNIMaxManifest    = 128 << 10
	localAIOMNIMaxExcerpt     = 512
	localAIOMNIMaxStream      = 64 << 10
	localAIOMNIMaxFailure     = 192
	localAIOMNIMaxArguments   = 64
	localAIOMNIMaxFacts       = 8
	localAIOMNIMaxIdentity    = 256
)

var (
	localAIOMNITransition    = regexp.MustCompile(`(?i)(?:transition|change|switch|midpoint|at)[^0-9]{0,32}([0-9]{1,6})\s*(?:ms|milliseconds)\b`)
	localAIOMNIPathPattern   = regexp.MustCompile(`(?i)(?:[a-z]:\\|\\\\)[^\s"<>]+`)
	localAIOMNISecretPattern = regexp.MustCompile(`(?i)(hf_token=|password=|api_key=|access_token=|x-amz-signature=|signed_url=|authorization:\s*|bearer\s+)[^\s,;]+`)
)

type (
	localAIOMNIManifest struct {
		Schema    string                   `json:"schema"`
		Selector  string                   `json:"selector"`
		CLI       localAIOMNICLI           `json:"cli"`
		Model     localAIOMNIModel         `json:"model"`
		Backend   localAIOMNIBackend       `json:"backend"`
		Text      localAIOMNITextFixture   `json:"text"`
		Image     localAIOMNIImageFixture  `json:"image"`
		Video     localAIOMNIVideoFixture  `json:"video"`
		Isolation localAIOMNIIsolation     `json:"isolation"`
		Limits    localAIOMNILimits        `json:"limits"`
		Evidence  localAIOMNIEvidencePaths `json:"evidence"`
	}
	localAIOMNICLI struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	localAIOMNIModel struct {
		Name     string `json:"name"`
		Identity string `json:"identity"`
	}
	localAIOMNIBackend struct {
		Identity string `json:"identity"`
	}
	localAIOMNITextFixture struct {
		Token string `json:"token"`
	}
	localAIOMNIImageFixture struct {
		Path          string   `json:"path"`
		SHA256        string   `json:"sha256"`
		RequiredFacts []string `json:"requiredFacts"`
	}
	localAIOMNIVideoFixture struct {
		Path                        string `json:"path"`
		SHA256                      string `json:"sha256"`
		Phase1                      string `json:"phase1"`
		Phase1Color                 string `json:"phase1Color"`
		Phase2                      string `json:"phase2"`
		Phase2Color                 string `json:"phase2Color"`
		TransitionStartMilliseconds int64  `json:"transitionStartMilliseconds"`
		TransitionEndMilliseconds   int64  `json:"transitionEndMilliseconds"`
	}
	localAIOMNIIsolation struct {
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
	localAIOMNILimits struct {
		TimeoutSeconds int   `json:"timeoutSeconds"`
		Processes      int   `json:"processes"`
		DownloadBytes  int64 `json:"downloadBytes"`
		ModelCalls     int64 `json:"modelCalls"`
	}
	localAIOMNIEvidencePaths struct {
		ReportPath string `json:"reportPath"`
		LedgerPath string `json:"ledgerPath"`
	}
	localAIOMNIInvocation struct {
		Manifest                            localAIOMNIManifest
		ManifestSHA256, RunID, EvidenceKind string
		Command                             localAICommandSpec
	}
	localAIOMNIObservation struct {
		localAICommandObservation
		Cancelled                bool
		BackendLogs, RuntimeLogs []localAIOMNIRawLog
		Artifacts                []localAIOMNIRawArtifact
	}
	localAIOMNIRawLog struct {
		Path string
		Body []byte
	}
	localAIOMNIRawArtifact struct {
		Kind, Path, MediaType string
		Body                  []byte
	}
	localAIOMNIExecutor interface {
		Execute(context.Context, localAICommandSpec, localAIRealRoots) localAIOMNIObservation
	}
	localAIOMNIRunner struct {
		executor    localAIOMNIExecutor
		locks       locking.Service
		writeReport func(string, localAIOMNIReport) error
	}
	localAIOMNIReport struct {
		Schema         string                     `json:"schema"`
		EvidenceKind   string                     `json:"evidenceKind"`
		RunID          string                     `json:"runId"`
		Selector       string                     `json:"selector"`
		Status         string                     `json:"status"`
		Platform       string                     `json:"platform"`
		Architecture   string                     `json:"architecture"`
		Redacted       bool                       `json:"redacted"`
		ManifestSHA256 string                     `json:"manifestSha256"`
		Input          localAIOMNIInputIdentity   `json:"inputIdentity"`
		Policy         localAIOMNIPolicy          `json:"policy"`
		Command        localAIOMNICommandEvidence `json:"command"`
		Logs           localAIOMNILogs            `json:"logs"`
		Artifacts      []localAIRealArtifact      `json:"artifacts"`
		Cache          localAIOMNICache           `json:"cache"`
		Semantic       localAIOMNISemantic        `json:"semantic"`
		Reservation    *localAIOMNIReservation    `json:"reservation,omitempty"`
		Release        localAIRealRelease         `json:"release"`
		Failure        *localAIRealFailure        `json:"failure,omitempty"`
		Activity       localAIOMNIActivity        `json:"activity"`
	}
	localAIOMNIInputIdentity struct {
		CLISHA256          string `json:"cliSha256"`
		ModelIdentity      string `json:"modelIdentity"`
		BackendIdentity    string `json:"backendIdentity"`
		FixtureSHA256      string `json:"fixtureSha256"`
		ImageFixtureSHA256 string `json:"imageFixtureSha256"`
		VideoFixtureSHA256 string `json:"videoFixtureSha256"`
		RubricSHA256       string `json:"rubricSha256"`
	}
	localAIOMNIPolicy struct {
		RootIdentities    []string `json:"rootIdentities"`
		Host              string   `json:"host"`
		Port              int      `json:"port"`
		TimeoutSeconds    int      `json:"timeoutSeconds"`
		ProcessLimit      int      `json:"processLimit"`
		DownloadByteLimit int64    `json:"downloadByteLimit"`
		ModelCallLimit    int64    `json:"modelCallLimit"`
		NetworkPolicy     string   `json:"networkPolicy"`
	}
	localAIOMNICommandEvidence struct {
		Arguments       []string          `json:"arguments"`
		SecretsRedacted bool              `json:"secretsRedacted"`
		Controlled      bool              `json:"controlled"`
		Stdout          localAIOMNIStream `json:"stdout"`
		Stderr          localAIOMNIStream `json:"stderr"`
	}
	localAIOMNIStream struct {
		Bytes     int64  `json:"bytes"`
		SHA256    string `json:"sha256"`
		Excerpt   string `json:"excerpt"`
		Truncated bool   `json:"truncated"`
	}
	localAIOMNILogs struct {
		Backend []localAIOMNILog `json:"backend"`
		Runtime []localAIOMNILog `json:"runtime"`
	}
	localAIOMNILog struct {
		localAIOMNIStream
		PathIdentity string `json:"pathIdentity"`
	}
	localAIOMNICache struct {
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
	localAIOMNISemantic struct {
		Assertion                     string   `json:"assertion"`
		Expected                      string   `json:"expected"`
		Observed                      string   `json:"observed"`
		Passed                        bool     `json:"passed"`
		GroundedFacts                 []string `json:"groundedFacts,omitempty"`
		NumericTransitionMilliseconds *int64   `json:"numericTransitionMilliseconds,omitempty"`
		PromptEcho                    bool     `json:"promptEcho"`
	}
	localAIOMNIReservation = localAIBudgetReservation
	localAIOMNIActivity    struct {
		BackendProcesses int   `json:"backendProcesses"`
		ExternalRequests int   `json:"externalRequests"`
		Downloads        int64 `json:"downloads"`
		ModelCalls       int64 `json:"modelCalls"`
	}
	localAIOMNIPathMetadataFunc func(string) (os.FileInfo, uint32, error)
	localAIOMNIReparsePathError struct {
		Label          string
		ComponentIndex int
		Final          bool
	}
	localAIRealRelease struct {
		Checked           bool `json:"checked"`
		InspectionFailed  bool `json:"inspectionFailed"`
		ProcessTreeClosed bool `json:"processTreeClosed"`
		OwnedProcesses    int  `json:"ownedProcesses"`
		OwnedListeners    int  `json:"ownedListeners"`
		OwnedLeases       int  `json:"ownedLeases"`
		PartialArtifacts  int  `json:"partialArtifacts"`
	}
	localAIRealFailure struct {
		Owner     string `json:"owner"`
		Assertion string `json:"assertion"`
		Expected  string `json:"expected"`
		Observed  string `json:"observed"`
	}
	localAIRealArtifact struct {
		Kind      string `json:"kind"`
		Path      string `json:"pathIdentity"`
		MediaType string `json:"mediaType"`
		Bytes     int64  `json:"bytes"`
		SHA256    string `json:"sha256"`
	}
	localAICommandSpec struct {
		BinaryPath string
		Arguments  []string
	}
	localAIRealRoots struct {
		Work, Profile, Cache, HFHome, HFCache, Temp, Output, Streams string
	}
	localAICommandObservation struct {
		Started, ProcessExited, TimedOut, ProcessTreeClosed bool
		ExitCode                                            int
		Stdout, Stderr                                      []byte
		StdoutTruncated, StderrTruncated                    bool
		OwnedProcesses, OwnedListeners, OwnedLeases         int
	}
	localAIBudgetLimits struct {
		ModelCalls    int64 `json:"modelCalls"`
		DownloadBytes int64 `json:"downloadBytes"`
	}
	localAIBudgetConsumed struct {
		ModelCalls    int64 `json:"modelCalls"`
		DownloadBytes int64 `json:"downloadBytes"`
	}
	localAIBudgetReservation struct {
		ID      string `json:"id"`
		Journey string `json:"journey"`
		Kind    string `json:"kind"`
		Amount  int64  `json:"amount"`
		State   string `json:"state"`
	}
	localAIBudgetLedger struct {
		Schema       string                     `json:"schema"`
		RunID        string                     `json:"runId"`
		Limits       localAIBudgetLimits        `json:"limits"`
		Consumed     localAIBudgetConsumed      `json:"consumed"`
		Reservations []localAIBudgetReservation `json:"reservations"`
	}
	localAIBudgetError struct {
		Code     string
		Consumed int64
		Limit    int64
	}
	localAIFileIdentity struct {
		Bytes  int64
		SHA256 string
	}
	localAIRealCacheTree struct {
		IdentitySHA256   string
		Entries          int
		Files            int
		Bytes            int64
		PartialArtifacts int
	}
)

func (r localAIOMNIRunner) Run(ctx context.Context, in localAIOMNIInvocation) (localAIOMNIReport, error) {
	report, roots := localAIOMNIReportFor(in), localAIOMNIRoots(in.Manifest)
	if err := localAIOMNIAdmit(in); err != nil {
		localAIOMNISet(&report, "FAIL", "harness", "manifest admission", "complete immutable manifest", err.Error())
		return r.finish(in, roots, report, false)
	}
	if err := prepareLocalAIRoots(roots); err != nil {
		localAIOMNISet(&report, "FAIL", "environment", "isolated roots", "all declared roots are creatable", err.Error())
		return r.finish(in, roots, report, false)
	}
	before, err := localAIOMNISnapshotRoots(roots)
	if err != nil {
		localAIOMNISet(&report, "FAIL", "harness", "cache admission inspection", "cache identity is observable", err.Error())
		return r.finish(in, roots, report, true)
	}
	report.Cache = localAIOMNICache{BeforeSHA256: before.IdentitySHA256, BeforeEntries: before.Entries, BeforeFiles: before.Files, BeforeBytes: before.Bytes, PartialArtifacts: before.PartialArtifacts}
	reservation, _, err := reserveLocalAIBudget(ctx, r.locks, in.Manifest.Evidence.LedgerPath, in.RunID, in.Manifest.Selector, localAIOMNIModelCall, 1, localAIBudgetLimits{ModelCalls: in.Manifest.Limits.ModelCalls, DownloadBytes: in.Manifest.Limits.DownloadBytes})
	if err != nil {
		localAIOMNISetBudgetFailure(&report, err)
		return r.finish(in, roots, report, true)
	}
	report.Reservation = &localAIOMNIReservation{ID: reservation.ID, Journey: in.Manifest.Selector, Kind: reservation.Kind, Amount: reservation.Amount, State: reservation.State}
	if r.executor == nil {
		localAIOMNISet(&report, "FAIL", "harness", "controlled executor", "an executor is configured", "executor is nil")
	} else {
		execCtx, cancel := context.WithTimeout(ctx, time.Duration(in.Manifest.Limits.TimeoutSeconds)*time.Second)
		observation := r.executor.Execute(execCtx, in.Command, roots)
		ctxErr := execCtx.Err()
		cancel()
		observation.Cancelled = observation.Cancelled || errors.Is(ctxErr, context.Canceled)
		observation.TimedOut = observation.TimedOut || errors.Is(ctxErr, context.DeadlineExceeded)
		localAIOMNIRecord(&report, in, observation)
		localAIOMNIClassify(&report, in, observation)
	}
	if err := commitLocalAIBudget(ctx, r.locks, in.Manifest.Evidence.LedgerPath, in.RunID, reservation.ID); err != nil {
		localAIOMNISet(&report, "FAIL", "harness", "durable reservation commit", "executed allowance is committed", err.Error())
	} else {
		report.Reservation.State = "COMMITTED"
	}
	return r.finish(in, roots, report, true)
}
func (r localAIOMNIRunner) finish(in localAIOMNIInvocation, roots localAIRealRoots, report localAIOMNIReport, prepared bool) (localAIOMNIReport, error) {
	if prepared {
		after, err := localAIOMNISnapshotRoots(roots)
		if err != nil {
			report.Cache.AfterInspectionFailed = true
			report.Release.InspectionFailed = true
			localAIOMNISet(&report, "INCONCLUSIVE", "harness", "final cache and release inspection", "owned resources and cache state are observable", err.Error())
		} else {
			report.Cache.AfterSHA256, report.Cache.AfterEntries, report.Cache.AfterFiles, report.Cache.AfterBytes, report.Cache.PartialArtifacts = after.IdentitySHA256, after.Entries, after.Files, after.Bytes, after.PartialArtifacts
			report.Release.PartialArtifacts = after.PartialArtifacts
			report.Release.Checked = true
			if report.Status == "PASS" && (!report.Release.ProcessTreeClosed || report.Release.OwnedProcesses != 0 || report.Release.OwnedListeners != 0 || report.Release.OwnedLeases != 0 || report.Release.PartialArtifacts != 0) {
				localAIOMNISet(&report, "FAIL", "harness", "owned-resource cleanup", "processes, listeners, leases and partials are zero", fmt.Sprintf("processes=%d listeners=%d leases=%d partials=%d treeClosed=%t", report.Release.OwnedProcesses, report.Release.OwnedListeners, report.Release.OwnedLeases, report.Release.PartialArtifacts, report.Release.ProcessTreeClosed))
			}
		}
	}
	if r.writeReport == nil {
		r.writeReport = func(path string, report localAIOMNIReport) error {
			return writeLocalAIJSONAtomic(path, report, nil)
		}
	}
	if !filepath.IsAbs(in.Manifest.Evidence.ReportPath) {
		return report, errors.New("diagnostic report path is not absolute")
	}
	if err := r.writeReport(in.Manifest.Evidence.ReportPath, report); err != nil {
		return report, err
	}
	return report, nil
}
func localAIOMNIReportFor(in localAIOMNIInvocation) localAIOMNIReport {
	m, roots := in.Manifest, localAIOMNIRoots(in.Manifest)
	return localAIOMNIReport{Schema: localAIOMNIEvidenceSchema, EvidenceKind: in.EvidenceKind, RunID: in.RunID, Selector: m.Selector, Status: "INCONCLUSIVE", Platform: runtime.GOOS, Architecture: runtime.GOARCH, Redacted: true, ManifestSHA256: in.ManifestSHA256, Input: localAIOMNIInputIdentity{CLISHA256: m.CLI.SHA256, ModelIdentity: m.Model.Identity, BackendIdentity: m.Backend.Identity, FixtureSHA256: localAIOMNIFixtureSHA(m), ImageFixtureSHA256: m.Image.SHA256, VideoFixtureSHA256: m.Video.SHA256, RubricSHA256: localAIOMNIRubricSHA(m)}, Policy: localAIOMNIPolicy{RootIdentities: []string{pathIdentityHash(roots.Work), pathIdentityHash(roots.Profile), pathIdentityHash(roots.Cache), pathIdentityHash(roots.HFHome), pathIdentityHash(roots.HFCache), pathIdentityHash(roots.Temp), pathIdentityHash(roots.Output), pathIdentityHash(roots.Streams)}, Host: m.Isolation.Host, Port: m.Isolation.Port, TimeoutSeconds: m.Limits.TimeoutSeconds, ProcessLimit: m.Limits.Processes, DownloadByteLimit: m.Limits.DownloadBytes, ModelCallLimit: m.Limits.ModelCalls, NetworkPolicy: m.Isolation.NetworkPolicy}, Command: localAIOMNICommandEvidence{Arguments: localAIOMNIRedactArguments(in.Command.Arguments), SecretsRedacted: true, Controlled: in.EvidenceKind == localAIOMNIControlled}, Logs: localAIOMNILogs{Backend: []localAIOMNILog{}, Runtime: []localAIOMNILog{}}, Artifacts: []localAIRealArtifact{}}
}
func localAIOMNIRecord(report *localAIOMNIReport, in localAIOMNIInvocation, o localAIOMNIObservation) {
	report.Command.Stdout, report.Command.Stderr = localAIOMNICapture(o.Stdout, o.StdoutTruncated), localAIOMNICapture(o.Stderr, o.StderrTruncated)
	report.Logs.Backend, report.Logs.Runtime = localAIOMNILogsFor(o.BackendLogs, "backend.log"), localAIOMNILogsFor(o.RuntimeLogs, "runtime.log")
	for _, a := range o.Artifacts {
		s := localAIOMNICapture(a.Body)
		report.Artifacts = append(report.Artifacts, localAIRealArtifact{Kind: a.Kind, Path: pathIdentityHash(a.Path), MediaType: a.MediaType, Bytes: s.Bytes, SHA256: s.SHA256})
	}
	report.Release = localAIRealRelease{ProcessTreeClosed: o.ProcessTreeClosed || !o.Started, OwnedProcesses: o.OwnedProcesses, OwnedListeners: o.OwnedListeners, OwnedLeases: o.OwnedLeases}
	if in.EvidenceKind == localAIOMNIReal && (o.Started || o.ProcessExited) {
		report.Activity.BackendProcesses, report.Activity.ModelCalls = 1, 1
	}
}
func localAIOMNILogsFor(logs []localAIOMNIRawLog, fallback string) []localAIOMNILog {
	out := make([]localAIOMNILog, 0, len(logs))
	for _, l := range logs {
		if l.Path == "" {
			l.Path = fallback
		}
		s := localAIOMNICapture(l.Body)
		out = append(out, localAIOMNILog{localAIOMNIStream: s, PathIdentity: pathIdentityHash(l.Path)})
	}
	return out
}
func localAIOMNIClassify(report *localAIOMNIReport, in localAIOMNIInvocation, o localAIOMNIObservation) {
	checks := []struct {
		bad                                  bool
		owner, assertion, expected, observed string
	}{
		{o.TimedOut || o.Cancelled || (o.Started && !o.ProcessTreeClosed), "harness", "timeout and cancellation cleanup", "cancellation is acknowledged and owned resources are released", "controlled execution did not finish cleanly"},
		{localAIOMNISecret(o.Stdout) || localAIOMNISecret(o.Stderr), "harness", "bounded redacted evidence", "sensitive output is absent from evidence", "sensitive output was observed and redacted"},
		{o.StdoutTruncated || o.StderrTruncated, "harness", "bounded command streams", "streams fit the bounded capture", "a command stream was truncated"},
		{slices.ContainsFunc(report.Logs.Backend, func(log localAIOMNILog) bool { return log.Truncated }) || slices.ContainsFunc(report.Logs.Runtime, func(log localAIOMNILog) bool { return log.Truncated }), "harness", "bounded backend/runtime logs", "logs fit the bounded capture", "a backend or runtime log was truncated"},
		{o.Started && !o.ProcessExited, "product", "backend process completion", "the selected command exits", "the selected command did not exit"},
		{o.OwnedProcesses > in.Manifest.Limits.Processes, "harness", "owned process limit", fmt.Sprintf("at most %d owned processes", in.Manifest.Limits.Processes), fmt.Sprintf("observed %d owned processes", o.OwnedProcesses)},
		{o.ExitCode != 0, "product", "OMNI command result", "the selected command exits successfully", fmt.Sprintf("exit code %d", o.ExitCode)},
	}
	for _, check := range checks {
		if check.bad {
			localAIOMNISet(report, "FAIL", check.owner, check.assertion, check.expected, check.observed)
			return
		}
	}
	s, v, f := localAIOMNIClassifySemantics(report.Selector, in.Manifest, in.Command, o.Stdout)
	report.Semantic = s
	if v == "PASS" {
		report.Status, report.Semantic.Passed = "PASS", true
	} else if v == "INCONCLUSIVE" {
		localAIOMNISet(report, "INCONCLUSIVE", f.Owner, f.Assertion, f.Expected, f.Observed)
	} else {
		localAIOMNISet(report, "FAIL", f.Owner, f.Assertion, f.Expected, f.Observed)
	}
}
func localAIOMNISet(r *localAIOMNIReport, status, owner, assertion, expected, observed string) {
	r.Status, r.Semantic.Passed, r.Failure = status, false, &localAIRealFailure{Owner: owner, Assertion: assertion, Expected: expected, Observed: localAIOMNIBoundedRedacted(observed)}
}
func localAIOMNISetBudgetFailure(r *localAIOMNIReport, err error) {
	var b *localAIBudgetError
	if errors.As(err, &b) && b.Code == "budget_exhausted" {
		localAIOMNISet(r, "FAIL", "harness", "durable model-call budget", "consumed allowance remains below limit", fmt.Sprintf("consumed=%d limit=%d", b.Consumed, b.Limit))
		return
	}
	if b != nil {
		err = errors.New(b.Code)
	}
	localAIOMNISet(r, "FAIL", "harness", "durable model-call ledger", "valid locked atomic ledger", err.Error())
}
func localAIOMNIAdmit(in localAIOMNIInvocation) error {
	return errors.Join(localAIOMNIRequire(in.EvidenceKind == localAIOMNIControlled || in.EvidenceKind == localAIOMNIReal, "evidence kind is invalid"), localAIOMNIRequire(strings.TrimSpace(in.RunID) != "" && len(in.RunID) <= localAIOMNIMaxIdentity && isLocalAISHA256(in.ManifestSHA256), "run or manifest identity is invalid"), localAIOMNIValidateManifest(in.Manifest), localAIOMNIRequire(in.Command.BinaryPath == in.Manifest.CLI.Path && filepath.IsAbs(in.Command.BinaryPath) && len(in.Command.Arguments) > 0 && len(in.Command.Arguments) <= localAIOMNIMaxArguments, "command does not match immutable admission"))
}
func localAIOMNIValidateManifest(m localAIOMNIManifest) error {
	return errors.Join(localAIOMNIRequire(m.Schema == localAIOMNIInputSchema && (m.Selector == localAIOMNIText || m.Selector == localAIOMNIImage || m.Selector == localAIOMNIVideo), "manifest schema or selector is invalid"), localAIOMNIFile(m.CLI.Path, m.CLI.SHA256, "CLI"), localAIOMNIIdentities(m.Model.Name, m.Model.Identity, m.Backend.Identity), localAIOMNIRequire(strings.TrimSpace(m.Text.Token) != "" && len(m.Text.Token) <= localAIOMNIMaxFailure && !strings.ContainsAny(m.Text.Token, "\r\n"), "text token is invalid"), localAIOMNIFile(m.Image.Path, m.Image.SHA256, "image fixture"), localAIOMNIFacts(m.Image.RequiredFacts), localAIOMNIFile(m.Video.Path, m.Video.SHA256, "video fixture"), localAIOMNIIdentities(m.Video.Phase1, m.Video.Phase1Color, m.Video.Phase2, m.Video.Phase2Color), localAIOMNIRequire(m.Video.TransitionStartMilliseconds >= 0 && m.Video.TransitionEndMilliseconds >= m.Video.TransitionStartMilliseconds && m.Video.TransitionEndMilliseconds <= 24*60*60*1000, "video timing window is invalid"), localAIOMNIRootSet(m.Isolation), localAIOMNIRequire(m.Isolation.Host == "127.0.0.1" && m.Isolation.Port >= 1 && m.Isolation.Port <= 65535 && m.Isolation.NetworkPolicy == localAIOMNINetworkPolicy, "isolation listener or network policy is invalid"), localAIOMNIRequire(m.Limits.TimeoutSeconds > 0 && m.Limits.TimeoutSeconds <= 24*60*60 && m.Limits.Processes > 0 && m.Limits.Processes <= 128 && m.Limits.DownloadBytes >= 0 && m.Limits.ModelCalls > 0 && m.Limits.ModelCalls <= 1000, "execution limits are invalid"), localAIOMNIAbs(m.Evidence.ReportPath), localAIOMNIRequire(localAIOMNIAbs(m.Evidence.LedgerPath) == nil && !strings.EqualFold(filepath.Clean(m.Evidence.ReportPath), filepath.Clean(m.Evidence.LedgerPath)), "evidence paths are invalid"))
}
func localAIOMNIRequire(ok bool, msg string) error {
	if ok {
		return nil
	}
	return errors.New(msg)
}
func localAIOMNIIdentities(values ...string) error {
	return localAIOMNIRequire(!slices.ContainsFunc(values, func(value string) bool { return localAIOMNIIdentity(value, "manifest identity") != nil }), "manifest identity is invalid")
}
func localAIOMNIFacts(values []string) error {
	if len(values) == 0 || len(values) > localAIOMNIMaxFacts {
		return errors.New("image rubric is invalid")
	}
	seen := map[string]bool{}
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || len(key) > localAIOMNIMaxFailure || seen[key] {
			return errors.New("image rubric fact is invalid")
		}
		seen[key] = true
	}
	return nil
}
func localAIOMNIRootSet(i localAIOMNIIsolation) error {
	seen := map[string]bool{}
	for _, path := range []string{i.WorkRoot, i.StateRoot, i.CacheRoot, i.TempRoot, i.OutputRoot, i.StreamsRoot} {
		if err := localAIOMNIAbs(path); err != nil {
			return err
		}
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] {
			return errors.New("isolated roots are not unique")
		}
		seen[key] = true
	}
	return nil
}
func localAIOMNIIdentity(value, label string) error {
	return localAIOMNIRequire(strings.TrimSpace(value) != "" && len(value) <= localAIOMNIMaxIdentity && !strings.ContainsAny(value, "\r\n"), fmt.Sprintf("%s is invalid", label))
}
func localAIOMNIAbs(path string) error {
	return localAIOMNIRequire(strings.TrimSpace(path) != "" && filepath.IsAbs(path) && !strings.ContainsAny(path, "\r\n*?"), "path is not absolute and bounded")
}
func (err *localAIOMNIReparsePathError) Error() string {
	return fmt.Sprintf("%s path component %d is a Windows reparse point", err.Label, err.ComponentIndex)
}
func localAIOMNIWindowsPathMetadata(path string) (os.FileInfo, uint32, error) {
	name, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return nil, 0, err
	}
	attributes, attributesErr := windows.GetFileAttributes(name)
	info, infoErr := os.Lstat(path)
	return info, attributes, errors.Join(infoErr, attributesErr)
}
func localAIOMNIRejectReparsePath(path, label string, metadata localAIOMNIPathMetadataFunc) error {
	if err := localAIOMNIAbs(path); err != nil {
		return err
	}
	if metadata == nil {
		return errors.New("path metadata reader is required")
	}
	current := filepath.Clean(path)
	for index := 0; ; index++ {
		_, attributes, err := metadata(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return &localAIOMNIReparsePathError{Label: label, ComponentIndex: index, Final: index == 0}
		}
		if filepath.Dir(current) == current {
			return nil
		}
		current = filepath.Dir(current)
	}
}
func localAIOMNIFile(path, expected, label string) error {
	return localAIOMNIFileWith(path, expected, label, localAIOMNIWindowsPathMetadata, localAIReadFileIdentity)
}
func localAIOMNIFileWith(path, expected, label string, metadata localAIOMNIPathMetadataFunc, identity func(string) (localAIFileIdentity, bool)) error {
	if err := localAIOMNIAbs(path); err != nil || !isLocalAISHA256(expected) || identity == nil {
		return fmt.Errorf("%s identity is invalid", label)
	}
	if err := localAIOMNIRejectReparsePath(path, label, metadata); err != nil {
		return err
	}
	info, _, err := metadata(path)
	if err != nil || info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("%s is not a nonempty regular file", label)
	}
	fileIdentity, ok := identity(path)
	return localAIOMNIRequire(ok && strings.EqualFold(fileIdentity.SHA256, expected), fmt.Sprintf("%s hash mismatch", label))
}
func localAIOMNIRoots(m localAIOMNIManifest) localAIRealRoots {
	return localAIRealRoots{Work: m.Isolation.WorkRoot, Profile: filepath.Clean(m.Isolation.StateRoot), Cache: filepath.Clean(m.Isolation.CacheRoot), HFHome: filepath.Join(filepath.Dir(filepath.Clean(m.Isolation.StateRoot)), "."+filepath.Base(filepath.Clean(m.Isolation.StateRoot))+"-hf-home"), HFCache: filepath.Join(filepath.Dir(filepath.Clean(m.Isolation.CacheRoot)), "."+filepath.Base(filepath.Clean(m.Isolation.CacheRoot))+"-hf-cache"), Temp: m.Isolation.TempRoot, Output: m.Isolation.OutputRoot, Streams: m.Isolation.StreamsRoot}
}
func localAIOMNIFixtureSHA(m localAIOMNIManifest) string {
	if m.Selector == localAIOMNIImage {
		return m.Image.SHA256
	}
	if m.Selector == localAIOMNIVideo {
		return m.Video.SHA256
	}
	return sha256Hex([]byte(m.Text.Token))
}
func localAIOMNIRubricSHA(m localAIOMNIManifest) string {
	body, _ := json.Marshal(struct {
		Selector, Text string
		Image          localAIOMNIImageFixture
		Video          localAIOMNIVideoFixture
	}{m.Selector, m.Text.Token, m.Image, m.Video})
	return sha256Hex(body)
}
func localAIOMNIClassifySemantics(selector string, m localAIOMNIManifest, command localAICommandSpec, body []byte) (localAIOMNISemantic, string, localAIRealFailure) {
	text := strings.TrimSpace(string(body))
	s := localAIOMNISemantic{Observed: localAIOMNIBoundedRedacted(text)}
	if text == "" {
		return s, "FAIL", localAIRealFailure{Owner: "product", Assertion: "OMNI text output", Expected: "a nonempty semantic response", Observed: "empty response"}
	}
	if selector == localAIOMNIText {
		s.Assertion, s.Expected = "exact requested text token", m.Text.Token
		if strings.Contains(text, m.Text.Token) {
			s.GroundedFacts = []string{m.Text.Token}
			return s, "PASS", localAIRealFailure{}
		}
		return s, "FAIL", localAIRealFailure{Owner: "product", Assertion: s.Assertion, Expected: s.Expected, Observed: "requested token is absent"}
	}
	if localAIOMNIPromptEcho(command, text) {
		s.PromptEcho = true
		s.Assertion = "fixture-grounded image/video facts"
		return s, "INCONCLUSIVE", localAIRealFailure{Owner: "product", Assertion: s.Assertion, Expected: "grounded media facts", Observed: "prompt echo did not establish grounding"}
	}
	if selector == localAIOMNIImage {
		s.Assertion, s.Expected = "fixture-grounded image facts", strings.Join(m.Image.RequiredFacts, ", ")
		var missing []string
		s.GroundedFacts, missing = localAIOMNIFactMatch(text, m.Image.RequiredFacts)
		if len(missing) == 0 {
			return s, "PASS", localAIRealFailure{}
		}
		return s, "FAIL", localAIRealFailure{Owner: "product", Assertion: s.Assertion, Expected: s.Expected, Observed: "missing image facts: " + strings.Join(missing, ", ")}
	}
	s.Assertion = "grounded video phases, colors and numeric transition"
	s.Expected = fmt.Sprintf("%s/%s then %s/%s with transition in %d..%d ms", m.Video.Phase1, m.Video.Phase1Color, m.Video.Phase2, m.Video.Phase2Color, m.Video.TransitionStartMilliseconds, m.Video.TransitionEndMilliseconds)
	var missing []string
	s.GroundedFacts, missing = localAIOMNIFactMatch(text, []string{m.Video.Phase1, m.Video.Phase1Color, m.Video.Phase2, m.Video.Phase2Color})
	if len(missing) > 0 {
		return s, "FAIL", localAIRealFailure{Owner: "product", Assertion: s.Assertion, Expected: s.Expected, Observed: "missing video facts: " + strings.Join(missing, ", ")}
	}
	match := localAIOMNITransition.FindStringSubmatch(text)
	if len(match) != 2 {
		return s, "INCONCLUSIVE", localAIRealFailure{Owner: "product", Assertion: "numeric video transition", Expected: fmt.Sprintf("transition in %d..%d ms", m.Video.TransitionStartMilliseconds, m.Video.TransitionEndMilliseconds), Observed: "grounded phases and colors were present, but no numeric transition was observed"}
	}
	value, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return s, "INCONCLUSIVE", localAIRealFailure{Owner: "product", Assertion: "numeric video transition", Expected: s.Expected, Observed: "numeric transition could not be parsed"}
	}
	s.NumericTransitionMilliseconds = &value
	if value < m.Video.TransitionStartMilliseconds || value > m.Video.TransitionEndMilliseconds {
		return s, "FAIL", localAIRealFailure{Owner: "product", Assertion: "numeric video transition", Expected: s.Expected, Observed: fmt.Sprintf("transition=%d ms", value)}
	}
	return s, "PASS", localAIRealFailure{}
}
func localAIOMNIFactMatch(text string, facts []string) (matched, missing []string) {
	lower := strings.ToLower(text)
	for _, fact := range facts {
		if strings.Contains(lower, strings.ToLower(fact)) {
			matched = append(matched, fact)
		} else {
			missing = append(missing, fact)
		}
	}
	return
}
func localAIOMNIPromptEcho(c localAICommandSpec, text string) bool {
	for i, a := range c.Arguments {
		if a == "--input" && i+1 < len(c.Arguments) && strings.HasPrefix(c.Arguments[i+1], "prompt=") {
			p := strings.TrimPrefix(c.Arguments[i+1], "prompt=")
			return strings.EqualFold(strings.TrimSpace(text), p) || strings.Contains(strings.ToLower(text), strings.ToLower(p))
		}
	}
	return false
}
func localAIOMNICapture(body []byte, alreadyTruncated ...bool) localAIOMNIStream {
	cut := len(body) > localAIOMNIMaxStream || (len(alreadyTruncated) > 0 && alreadyTruncated[0])
	if cut {
		body = body[:localAIOMNIMaxStream]
	}
	return localAIOMNIStream{Bytes: int64(len(body)), SHA256: sha256Hex(body), Excerpt: localAIOMNIBoundedText(localAIOMNIRedact(string(body)), localAIOMNIMaxExcerpt), Truncated: cut}
}
func localAIOMNIBoundedText(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return "sha256=" + sha256Hex([]byte(value))
}
func localAIOMNIBoundedRedacted(value string) string {
	return localAIOMNIBoundedText(localAIOMNIRedact(value), localAIOMNIMaxFailure)
}
func localAIOMNIRedact(value string) string {
	return localAIOMNISecretPattern.ReplaceAllString(localAIOMNIPathPattern.ReplaceAllStringFunc(value, pathIdentityHash), `${1}<redacted>`)
}
func localAIOMNIRedactArguments(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = localAIOMNIRedact(arg)
	}
	return out
}
func localAIOMNISecret(body []byte) bool {
	value := string(body)
	return localAIOMNISecretPattern.ReplaceAllString(value, `${1}<redacted>`) != value
}
func localAIOMNISnapshotRoots(r localAIRealRoots) (localAIRealCacheTree, error) {
	h := sha256.New()
	result := localAIRealCacheTree{}
	if _, err := os.Stat(filepath.Join(r.Work, ".omni-inspection-failure")); err == nil {
		return result, errors.New("controlled final inspection failure")
	}
	for _, root := range []struct{ name, path string }{{"work", r.Work}, {"state", r.Profile}, {"cache", r.Cache}, {"hf-home", r.HFHome}, {"hf-cache", r.HFCache}, {"temp", r.Temp}, {"output", r.Output}, {"streams", r.Streams}} {
		tree, err := readLocalAIRealCacheTree(root.path)
		if err != nil {
			return result, err
		}
		result.Entries, result.Files, result.Bytes, result.PartialArtifacts = result.Entries+tree.Entries, result.Files+tree.Files, result.Bytes+tree.Bytes, result.PartialArtifacts+tree.PartialArtifacts
		_, _ = fmt.Fprintf(h, "%s\x00%s\n", root.name, tree.IdentitySHA256)
	}
	result.IdentitySHA256 = hex.EncodeToString(h.Sum(nil))
	return result, nil
}
func (err *localAIBudgetError) Error() string {
	return fmt.Sprintf("budget ledger %s (%d/%d)", err.Code, err.Consumed, err.Limit)
}

const (
	localAIBudgetSchema = "localai.windows-real-budget.v1"
)

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
func isLocalAISHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func pathIdentityHash(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return "sha256=" + sha256Hex([]byte(filepath.Clean(path)))
}
func localAIReadFileIdentity(path string) (localAIFileIdentity, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return localAIFileIdentity{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return localAIFileIdentity{}, false
	}
	defer file.Close()
	hasher := sha256.New()
	_, err = io.Copy(hasher, file)
	return localAIFileIdentity{Bytes: info.Size(), SHA256: hex.EncodeToString(hasher.Sum(nil))}, err == nil
}
func prepareLocalAIRoots(roots localAIRealRoots) error {
	for _, path := range []string{roots.Work, roots.Profile, roots.Cache, roots.HFHome, roots.HFCache, roots.Temp, roots.Output, roots.Streams} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}
func writeLocalAIJSONAtomic(path string, value any, beforeReplace func() error) (err error) {
	if strings.TrimSpace(path) == "" {
		return errors.New("atomic path is empty")
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.partial")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if err = f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if n, writeErr := f.Write(body); writeErr != nil {
		_ = f.Close()
		return writeErr
	} else if n != len(body) {
		_ = f.Close()
		return io.ErrShortWrite
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if beforeReplace != nil {
		if err = beforeReplace(); err != nil {
			return err
		}
	}
	from, err := windows.UTF16PtrFromString(tmp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func localAIWithBudget(ctx context.Context, locks locking.Service, path, runID string, limits *localAIBudgetLimits, mutate func(*localAIBudgetLedger) error) (localAIBudgetLedger, error) {
	if locks == nil {
		return localAIBudgetLedger{}, errors.New("budget lock service is required")
	}
	lock, err := locks.Lock(ctx, path+".lock")
	if err != nil {
		return localAIBudgetLedger{}, err
	}
	defer lock.Close()
	declared := localAIBudgetLimits{}
	if limits != nil {
		declared = *limits
	}
	ledger, err := readOrCreateLocalAIBudget(path, runID, declared)
	if err != nil {
		return ledger, err
	}
	if ledger.RunID != runID {
		return ledger, &localAIBudgetError{Code: "run_id_mismatch"}
	}
	if limits != nil && ledger.Limits != *limits {
		return ledger, &localAIBudgetError{Code: "limits_mismatch"}
	}
	if err := mutate(&ledger); err != nil {
		return ledger, err
	}
	return ledger, writeLocalAIBudgetAtomic(path, ledger)
}
func reserveLocalAIBudget(ctx context.Context, locks locking.Service, path, runID, journey, kind string, amount int64, limits localAIBudgetLimits) (localAIBudgetReservation, localAIBudgetLedger, error) {
	var reservation localAIBudgetReservation
	ledger, err := localAIWithBudget(ctx, locks, path, runID, &limits, func(ledger *localAIBudgetLedger) error {
		limit, consumed := localAIBudgetValues(*ledger, kind)
		if amount <= 0 || limit < 0 || consumed < 0 || consumed+amount > limit {
			return &localAIBudgetError{Code: "budget_exhausted", Consumed: consumed, Limit: limit}
		}
		id, err := newLocalAIReservationID()
		if err != nil {
			return err
		}
		reservation = localAIBudgetReservation{ID: id, Journey: journey, Kind: kind, Amount: amount, State: "RESERVED"}
		localAIBudgetAddConsumed(ledger, kind, amount)
		ledger.Reservations = append(ledger.Reservations, reservation)
		return nil
	})
	return reservation, ledger, err
}
func commitLocalAIBudget(ctx context.Context, locks locking.Service, path, runID, reservationID string) error {
	_, err := localAIWithBudget(ctx, locks, path, runID, nil, func(ledger *localAIBudgetLedger) error {
		index := slices.IndexFunc(ledger.Reservations, func(r localAIBudgetReservation) bool { return r.ID == reservationID })
		if index < 0 {
			return &localAIBudgetError{Code: "ledger_corrupt"}
		}
		r := &ledger.Reservations[index]
		switch r.State {
		case "COMMITTED":
			return nil
		case "RESERVED":
			r.State = "COMMITTED"
			return nil
		default:
			return &localAIBudgetError{Code: "ledger_corrupt"}
		}
	})
	return err
}
func readOrCreateLocalAIBudget(path, runID string, limits localAIBudgetLimits) (localAIBudgetLedger, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		ledger := localAIBudgetLedger{Schema: localAIBudgetSchema, RunID: runID, Limits: limits, Reservations: []localAIBudgetReservation{}}
		return ledger, writeLocalAIBudgetAtomic(path, ledger)
	}
	if err != nil {
		return localAIBudgetLedger{}, err
	}
	ledger, err := decodeLocalAIBudget(body)
	if err != nil {
		return localAIBudgetLedger{}, &localAIBudgetError{Code: "ledger_corrupt"}
	}
	return ledger, nil
}
func decodeLocalAIBudget(body []byte) (localAIBudgetLedger, error) {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	var ledger localAIBudgetLedger
	if err := d.Decode(&ledger); err != nil {
		return localAIBudgetLedger{}, err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return localAIBudgetLedger{}, errors.New("ledger contained trailing JSON")
	}
	if err := validateLocalAIBudgetLedger(ledger); err != nil {
		return localAIBudgetLedger{}, err
	}
	return ledger, nil
}
func validateLocalAIBudgetLedger(ledger localAIBudgetLedger) error {
	if ledger.Schema != localAIBudgetSchema || ledger.RunID == "" || ledger.Limits.ModelCalls < 0 || ledger.Limits.DownloadBytes < 0 || ledger.Consumed.ModelCalls < 0 || ledger.Consumed.DownloadBytes < 0 {
		return errors.New("ledger identity or counters are invalid")
	}
	seen := map[string]bool{}
	var modelCalls, downloadBytes int64
	for _, reservation := range ledger.Reservations {
		if reservation.ID == "" || reservation.Journey == "" || reservation.Amount <= 0 || (reservation.State != "RESERVED" && reservation.State != "COMMITTED") {
			return errors.New("ledger reservation is invalid")
		}
		if seen[reservation.ID] {
			return errors.New("ledger reservation is duplicated")
		}
		seen[reservation.ID] = true
		switch reservation.Kind {
		case "modelCall":
			modelCalls += reservation.Amount
		case "downloadBytes":
			downloadBytes += reservation.Amount
		default:
			return errors.New("ledger reservation kind is invalid")
		}
	}
	if ledger.Consumed.ModelCalls != modelCalls || ledger.Consumed.DownloadBytes != downloadBytes || modelCalls > ledger.Limits.ModelCalls || downloadBytes > ledger.Limits.DownloadBytes {
		return errors.New("ledger counters do not match reservations")
	}
	return nil
}
func localAIBudgetValues(ledger localAIBudgetLedger, kind string) (int64, int64) {
	if kind == "downloadBytes" {
		return ledger.Limits.DownloadBytes, ledger.Consumed.DownloadBytes
	}
	return ledger.Limits.ModelCalls, ledger.Consumed.ModelCalls
}
func localAIBudgetAddConsumed(ledger *localAIBudgetLedger, kind string, amount int64) {
	if kind == "downloadBytes" {
		ledger.Consumed.DownloadBytes += amount
	} else {
		ledger.Consumed.ModelCalls += amount
	}
}
func newLocalAIReservationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
func writeLocalAIBudgetAtomic(path string, ledger localAIBudgetLedger) error {
	if err := validateLocalAIBudgetLedger(ledger); err != nil {
		return err
	}
	return writeLocalAIJSONAtomic(path, ledger, nil)
}
func readLocalAIRealCacheTree(root string) (localAIRealCacheTree, error) {
	entries := map[string]string{}
	var totalBytes int64
	files, partials := 0, 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return fs.SkipDir
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("cache symlink is not allowed")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		lower := strings.ToLower(relative)
		if strings.HasSuffix(lower, ".partial") || strings.Contains(lower, "/.partial/") {
			partials++
		}
		if entry.IsDir() {
			entries[relative] = "dir"
			return nil
		}
		identity, ok := localAIReadFileIdentity(path)
		if !ok {
			return errors.New("cache file identity could not be read")
		}
		entries[relative] = fmt.Sprintf("file:%d:%s", identity.Bytes, identity.SHA256)
		files++
		totalBytes += identity.Bytes
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.SkipDir) {
		return localAIRealCacheTree{}, err
	}
	paths := make([]string, 0, len(entries))
	for path := range entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		_, _ = fmt.Fprintf(h, "%s\x00%s\n", path, entries[path])
	}
	return localAIRealCacheTree{IdentitySHA256: hex.EncodeToString(h.Sum(nil)), Entries: len(entries), Files: files, Bytes: totalBytes, PartialArtifacts: partials}, nil
}
