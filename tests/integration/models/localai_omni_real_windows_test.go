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
	"github.com/portpowered/infinite-you/pkg/platform/locking"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	localAIOMNIInputSchema     = "localai.windows-omni-diagnostic-input.v1"
	localAIOMNIEvidenceSchema  = "localai.windows-omni-diagnostic-evidence.v1"
	localAIOMNIRealEnableEnv   = "INFINITE_YOU_LOCALAI_OMNI_REAL_ENABLE"
	localAIOMNIRealManifestEnv = "INFINITE_YOU_LOCALAI_OMNI_REAL_MANIFEST"
	localAIOMNIControlled      = "controlled"
	localAIOMNIReal            = "real"
	localAIOMNIText            = "text"
	localAIOMNIImage           = "image"
	localAIOMNIVideo           = "video"
	localAIOMNIModelCall       = "modelCall"
	localAIOMNINetworkPolicy   = "loopback-only-external-denied"
	localAIOMNIMaxManifest     = 128 << 10
	localAIOMNIMaxExcerpt      = 512
	localAIOMNIMaxStream       = 64 << 10
	localAIOMNIMaxFailure      = 192
	localAIOMNIMaxArguments    = 64
	localAIOMNIMaxFacts        = 8
	localAIOMNIMaxIdentity     = 256
)

var (
	errLocalAIOMNIRealDisabled      = errors.New("localai omni real selector is disabled")
	errLocalAIOMNIReportInterrupted = errors.New("localai omni report interruption")
	localAIOMNITransition           = regexp.MustCompile(`(?i)(?:transition|change|switch|midpoint|at)[^0-9]{0,32}([0-9]{1,6})\s*(?:ms|milliseconds)\b`)
	localAIOMNIPathPattern          = regexp.MustCompile(`(?i)(?:[a-z]:\\|\\\\)[^\s"<>]+`)
	localAIOMNISecretPattern        = regexp.MustCompile(`(?i)(hf_token=|password=|api_key=|access_token=|x-amz-signature=|signed_url=|authorization:\s*|bearer\s+)[^\s,;]+`)
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
		localAIRealSemantic
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
	localAIOMNISnapshot = localAIRealCacheTree
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
func (r localAIOMNIRunner) RunRealFromEnvironment(ctx context.Context) (localAIOMNIReport, error) {
	in, err := localAIOMNIInvocationValues(os.Getenv(localAIOMNIRealEnableEnv), os.Getenv(localAIOMNIRealManifestEnv))
	if err != nil {
		return localAIOMNIReport{}, err
	}
	if r.executor == nil {
		r.executor = localAIOMNIProcessExecutor{}
	}
	return r.Run(ctx, in)
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
			return writeLocalAIOMNIReportAtomicHook(path, report, nil)
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
	return errors.Join(localAIOMNIRequire(m.Schema == localAIOMNIInputSchema && localAIOMNISelector(m.Selector), "manifest schema or selector is invalid"), localAIOMNIFile(m.CLI.Path, m.CLI.SHA256, "CLI"), localAIOMNIIdentities(m.Model.Name, m.Model.Identity, m.Backend.Identity), localAIOMNIRequire(strings.TrimSpace(m.Text.Token) != "" && len(m.Text.Token) <= localAIOMNIMaxFailure && !strings.ContainsAny(m.Text.Token, "\r\n"), "text token is invalid"), localAIOMNIFile(m.Image.Path, m.Image.SHA256, "image fixture"), localAIOMNIFacts(m.Image.RequiredFacts), localAIOMNIFile(m.Video.Path, m.Video.SHA256, "video fixture"), localAIOMNIIdentities(m.Video.Phase1, m.Video.Phase1Color, m.Video.Phase2, m.Video.Phase2Color), localAIOMNIRequire(m.Video.TransitionStartMilliseconds >= 0 && m.Video.TransitionEndMilliseconds >= m.Video.TransitionStartMilliseconds && m.Video.TransitionEndMilliseconds <= 24*60*60*1000, "video timing window is invalid"), localAIOMNIRootSet(m.Isolation), localAIOMNIRequire(m.Isolation.Host == "127.0.0.1" && m.Isolation.Port >= 1 && m.Isolation.Port <= 65535 && m.Isolation.NetworkPolicy == localAIOMNINetworkPolicy, "isolation listener or network policy is invalid"), localAIOMNIRequire(m.Limits.TimeoutSeconds > 0 && m.Limits.TimeoutSeconds <= 24*60*60 && m.Limits.Processes > 0 && m.Limits.Processes <= 128 && m.Limits.DownloadBytes >= 0 && m.Limits.ModelCalls > 0 && m.Limits.ModelCalls <= 1000, "execution limits are invalid"), localAIOMNIAbs(m.Evidence.ReportPath), localAIOMNIRequire(localAIOMNIAbs(m.Evidence.LedgerPath) == nil && !strings.EqualFold(filepath.Clean(m.Evidence.ReportPath), filepath.Clean(m.Evidence.LedgerPath)), "evidence paths are invalid"))
}
func localAIOMNIRequire(ok bool, msg string) error {
	if ok {
		return nil
	}
	return errors.New(msg)
}
func localAIOMNIIdentities(values ...string) error {
	for _, value := range values {
		if err := localAIOMNIIdentity(value, "manifest identity"); err != nil {
			return err
		}
	}
	return nil
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
func localAIOMNIFile(path, expected, label string) error {
	if err := localAIOMNIAbs(path); err != nil || !isLocalAISHA256(expected) {
		return fmt.Errorf("%s identity is invalid", label)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("%s is not a nonempty regular file", label)
	}
	identity, ok := localAIReadFileIdentity(path)
	return localAIOMNIRequire(ok && strings.EqualFold(identity.SHA256, expected), fmt.Sprintf("%s hash mismatch", label))
}
func localAIOMNISelector(v string) bool {
	return v == localAIOMNIText || v == localAIOMNIImage || v == localAIOMNIVideo
}
func localAIOMNIRoots(m localAIOMNIManifest) localAIRealRoots {
	return localAIRealRoots{Root: m.Isolation.WorkRoot, Work: m.Isolation.WorkRoot, Profile: filepath.Clean(m.Isolation.StateRoot), Cache: filepath.Clean(m.Isolation.CacheRoot), HFHome: filepath.Join(filepath.Dir(filepath.Clean(m.Isolation.StateRoot)), "."+filepath.Base(filepath.Clean(m.Isolation.StateRoot))+"-hf-home"), HFCache: filepath.Join(filepath.Dir(filepath.Clean(m.Isolation.CacheRoot)), "."+filepath.Base(filepath.Clean(m.Isolation.CacheRoot))+"-hf-cache"), Temp: m.Isolation.TempRoot, Output: m.Isolation.OutputRoot, Streams: m.Isolation.StreamsRoot}
}
func localAIOMNIInvocationValues(enable, path string) (localAIOMNIInvocation, error) {
	if strings.TrimSpace(enable) != "1" {
		return localAIOMNIInvocation{}, errLocalAIOMNIRealDisabled
	}
	if err := localAIOMNIAbs(path); err != nil {
		return localAIOMNIInvocation{}, errors.New("real selector requires an absolute manifest path")
	}
	m, hash, err := localAIOMNIReadManifest(path)
	if err != nil {
		return localAIOMNIInvocation{}, err
	}
	return localAIOMNIInvocation{Manifest: m, ManifestSHA256: hash, RunID: "real-" + m.Selector + "-" + hash[:12], EvidenceKind: localAIOMNIReal, Command: localAIOMNICommand(m)}, nil
}
func localAIOMNIReadManifest(path string) (localAIOMNIManifest, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return localAIOMNIManifest{}, "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > localAIOMNIMaxManifest {
		return localAIOMNIManifest{}, "", errors.New("manifest is not bounded")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return localAIOMNIManifest{}, "", err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	var m localAIOMNIManifest
	if err := d.Decode(&m); err != nil {
		return localAIOMNIManifest{}, "", err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return localAIOMNIManifest{}, "", errors.New("manifest contains trailing JSON")
	}
	if err := localAIOMNIValidateManifest(m); err != nil {
		return localAIOMNIManifest{}, "", err
	}
	return m, sha256Hex(body), nil
}
func localAIOMNICommand(m localAIOMNIManifest) localAICommandSpec {
	prompt, media := "Return this exact token: "+m.Text.Token, ""
	if m.Selector == localAIOMNIImage {
		prompt, media = "Name every required fact visible in the image: "+strings.Join(m.Image.RequiredFacts, ", "), "image=@"+m.Image.Path
	}
	if m.Selector == localAIOMNIVideo {
		prompt, media = fmt.Sprintf("Report %s/%s and %s/%s, and the numeric transition time in milliseconds.", m.Video.Phase1, m.Video.Phase1Color, m.Video.Phase2, m.Video.Phase2Color), "video=@"+m.Video.Path
	}
	args := []string{"--json", "models", "invoke", m.Model.Name, "--operation", "OMNI", "--input", "prompt=" + prompt}
	if media != "" {
		args = append(args, "--input", media)
	}
	return localAICommandSpec{BinaryPath: m.CLI.Path, Arguments: args, Environment: []string{"HTTP_PROXY=127.0.0.1:9", "HTTPS_PROXY=127.0.0.1:9", "NO_PROXY=127.0.0.1"}}
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
	s := localAIOMNISemantic{localAIRealSemantic: localAIRealSemantic{Observed: localAIOMNIBoundedRedacted(text)}}
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
func localAIOMNISnapshotRoots(r localAIRealRoots) (localAIOMNISnapshot, error) {
	h := sha256.New()
	result := localAIOMNISnapshot{}
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
func writeLocalAIOMNIReportAtomicHook(path string, r localAIOMNIReport, hook func() error) error {
	if err := localAIOMNIValidateReport(r); err != nil {
		return err
	}
	return writeLocalAIJSONAtomic(path, r, hook)
}
func localAIOMNIValidateReport(r localAIOMNIReport) error {
	if r.Schema != localAIOMNIEvidenceSchema || (r.EvidenceKind != localAIOMNIControlled && r.EvidenceKind != localAIOMNIReal) || r.RunID == "" || r.Platform == "" || r.Architecture == "" || !r.Redacted || !localAIStatus(r.Status) {
		return errors.New("report identity or status is invalid")
	}
	admission := r.Failure != nil && r.Failure.Assertion == "manifest admission"
	if err := errors.Join(localAIOMNIRequire(localAIOMNISelector(r.Selector) || admission, "report selector or manifest identity is invalid"), localAIOMNIRequire(r.ManifestSHA256 == "" || isLocalAISHA256(r.ManifestSHA256), "report selector or manifest identity is invalid"), localAIOMNIRequire(len(r.Command.Arguments) <= localAIOMNIMaxArguments && r.Command.SecretsRedacted, "command evidence is invalid")); err != nil {
		return err
	}
	if r.EvidenceKind == localAIOMNIControlled && r.Activity != (localAIOMNIActivity{}) {
		return errors.New("controlled evidence recorded real activity")
	}
	if r.Status == "PASS" {
		if r.Failure != nil || !r.Semantic.Passed || r.Reservation == nil || r.Reservation.State != "COMMITTED" || len(r.Artifacts) == 0 || len(r.Logs.Backend) == 0 || len(r.Logs.Runtime) == 0 || !r.Release.Checked || r.Release.InspectionFailed || !r.Release.ProcessTreeClosed || r.Release.OwnedProcesses != 0 || r.Release.OwnedListeners != 0 || r.Release.OwnedLeases != 0 || r.Release.PartialArtifacts != 0 || r.Cache.AfterInspectionFailed {
			return errors.New("pass report omitted semantic or release proof")
		}
	} else if r.Failure == nil {
		return errors.New("non-pass report omitted failure")
	}
	if !r.Release.Checked && r.Failure != nil && r.Failure.Assertion != "manifest admission" && r.Failure.Assertion != "final cache and release inspection" {
		return errors.New("release inspection was not established")
	}
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return localAIOMNIRequire(!localAIOMNISecret(body) && !localAIOMNIPathPattern.Match(body), "report contains unredacted sensitive evidence")
}

type localAIOMNIControlledExecutor struct {
	Observation localAIOMNIObservation
	Calls       atomic.Int32
	RemoveRoot  bool
}

func (e *localAIOMNIControlledExecutor) Execute(_ context.Context, _ localAICommandSpec, roots localAIRealRoots) localAIOMNIObservation {
	e.Calls.Add(1)
	if e.RemoveRoot {
		_ = os.WriteFile(filepath.Join(roots.Work, ".omni-inspection-failure"), nil, 0o600)
	}
	return e.Observation
}

type localAIOMNICancellationExecutor struct {
	Started chan struct{}
	once    sync.Once
}

func (e *localAIOMNICancellationExecutor) Execute(ctx context.Context, _ localAICommandSpec, _ localAIRealRoots) localAIOMNIObservation {
	e.once.Do(func() { close(e.Started) })
	<-ctx.Done()
	return localAIOMNIObservation{localAICommandObservation: localAICommandObservation{Started: true, ProcessExited: true, ExitCode: 1, ProcessTreeClosed: true}, Cancelled: true, BackendLogs: []localAIOMNIRawLog{{Path: "backend.log", Body: []byte("cancelled")}}, RuntimeLogs: []localAIOMNIRawLog{{Path: "runtime.log", Body: []byte("cancelled")}}}
}

type localAIOMNIProcessExecutor struct{}

func (buffer *localAIBoundedBuffer) ReadFrom(reader io.Reader) (int64, error) {
	body, err := io.ReadAll(io.LimitReader(reader, localAIRealMaxStreamBytes+1))
	_, _ = buffer.Write(body)
	return int64(len(body)), err
}
func (localAIOMNIProcessExecutor) Execute(ctx context.Context, c localAICommandSpec, r localAIRealRoots) localAIOMNIObservation {
	c.Arguments, c.Environment = localAIExpandArguments(c.Arguments, r), localAIProcessEnvironment(r, c.Environment)
	o := localAIProcessExecutor{}.Execute(ctx, c, r)
	return localAIOMNIObservation{localAICommandObservation: o, BackendLogs: localAIOMNIReadLogs(r.Streams, "backend.log"), RuntimeLogs: localAIOMNIReadLogs(r.Streams, "runtime.log"), Artifacts: []localAIOMNIRawArtifact{{Kind: "text", Path: "stdout", MediaType: "text/plain", Body: o.Stdout}}}
}
func localAIOMNIReadLogs(root string, names ...string) []localAIOMNIRawLog {
	logs := make([]localAIOMNIRawLog, 0, len(names))
	for _, name := range names {
		if body, err := localAIReadBoundedFile(filepath.Join(root, name), localAIOMNIMaxStream); err == nil {
			logs = append(logs, localAIOMNIRawLog{Path: filepath.Join(root, name), Body: body})
		}
	}
	return logs
}
func localAIOMNIFailIf(t testing.TB, bad bool, format string, args ...any) {
	if bad {
		t.Fatalf(format, args...)
	}
}
func mustLocalAIOMNIRunner(t testing.TB, e localAIOMNIExecutor) localAIOMNIRunner {
	locks, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		t.Fatalf("new omni locks: %v", err)
	}
	return localAIOMNIRunner{executor: e, locks: locks, writeReport: func(path string, report localAIOMNIReport) error {
		return writeLocalAIOMNIReportAtomicHook(path, report, nil)
	}}
}
func localAIOMNIInvocationForTest(t testing.TB, m localAIOMNIManifest, id, kind string) localAIOMNIInvocation {
	body, _ := json.Marshal(m)
	return localAIOMNIInvocation{Manifest: m, ManifestSHA256: sha256Hex(body), RunID: id, EvidenceKind: kind, Command: localAIOMNICommand(m)}
}
func localAIOMNIManifestForTest(t testing.TB, root string) localAIOMNIManifest {
	cli, image, video := filepath.Join(root, "bin", "you.exe"), filepath.Join(root, "fixtures", "infinite-you.png"), filepath.Join(root, "fixtures", "groundtruth-fixture.mp4")
	for path, body := range map[string][]byte{cli: []byte("controlled CLI identity"), image: []byte("controlled image fixture: infinity symbol / INFINITE YOU"), video: []byte("controlled video fixture: PHASE 1 red then PHASE 2 blue")} {
		localAIOMNIFailIf(t, os.MkdirAll(filepath.Dir(path), 0o700) != nil, "fixture directory creation failed")
		localAIOMNIFailIf(t, os.WriteFile(path, body, 0o600) != nil, "fixture write failed")
	}
	hash := func(path string) string {
		identity, ok := localAIReadFileIdentity(path)
		localAIOMNIFailIf(t, !ok, "hash fixture %s", path)
		return identity.SHA256
	}
	return localAIOMNIManifest{Schema: localAIOMNIInputSchema, Selector: localAIOMNIText, CLI: localAIOMNICLI{Path: cli, SHA256: hash(cli)}, Model: localAIOMNIModel{Name: "llm", Identity: "model-revision-controlled"}, Backend: localAIOMNIBackend{Identity: "backend-revision-controlled"}, Text: localAIOMNITextFixture{Token: "COBALT-17"}, Image: localAIOMNIImageFixture{Path: image, SHA256: hash(image), RequiredFacts: []string{"infinity symbol", "INFINITE YOU"}}, Video: localAIOMNIVideoFixture{Path: video, SHA256: hash(video), Phase1: "PHASE 1", Phase1Color: "red", Phase2: "PHASE 2", Phase2Color: "blue", TransitionStartMilliseconds: 1500, TransitionEndMilliseconds: 2500}, Isolation: localAIOMNIIsolation{WorkRoot: filepath.Join(root, "work"), StateRoot: filepath.Join(root, "state"), CacheRoot: filepath.Join(root, "cache"), TempRoot: filepath.Join(root, "temp"), OutputRoot: filepath.Join(root, "output"), StreamsRoot: filepath.Join(root, "streams"), Host: "127.0.0.1", Port: 54321, NetworkPolicy: localAIOMNINetworkPolicy}, Limits: localAIOMNILimits{TimeoutSeconds: 1200, Processes: 4, ModelCalls: 1}, Evidence: localAIOMNIEvidencePaths{ReportPath: filepath.Join(root, "evidence", "report.json"), LedgerPath: filepath.Join(root, "evidence", "ledger.json")}}
}
func localAIOMNIPass(root, output string) localAIOMNIObservation {
	body := []byte(output)
	return localAIOMNIObservation{localAICommandObservation: localAICommandObservation{ProcessExited: true, ExitCode: 0, ProcessTreeClosed: true, Stdout: body}, BackendLogs: []localAIOMNIRawLog{{Path: filepath.Join(root, "backend.log"), Body: []byte("backend completed")}}, RuntimeLogs: []localAIOMNIRawLog{{Path: filepath.Join(root, "runtime.log"), Body: []byte("runtime completed")}}, Artifacts: []localAIOMNIRawArtifact{{Kind: "text", Path: filepath.Join(root, "output.txt"), MediaType: "text/plain", Body: body}}}
}
func localAIOMNIReportMust(t testing.TB, m localAIOMNIManifest, id string, o localAIOMNIObservation, e localAIOMNIExecutor) localAIOMNIReport {
	r, err := mustLocalAIOMNIRunner(t, e).Run(t.Context(), localAIOMNIInvocationForTest(t, m, id, localAIOMNIControlled))
	localAIOMNIFailIf(t, err != nil, "runner error: %v", err)
	return r
}
func localAIOMNIStatusFor(t testing.TB, m localAIOMNIManifest, id, output, want string) localAIOMNIReport {
	r := localAIOMNIReportMust(t, m, id, localAIOMNIPass(filepath.Dir(m.Evidence.ReportPath), output), &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(filepath.Dir(m.Evidence.ReportPath), output)})
	localAIOMNIFailIf(t, r.Status != want, "status=%s want=%s", r.Status, want)
	return r
}
func TestLocalAIOMNIRealDiagnosticRunnerControlled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		run  func(*testing.T)
	}{{"I01-controlled-default-zero-real-activity", localAIOMNII01}, {"I02-incomplete-manifest-rejects-before-launch", localAIOMNII02}, {"I03-durable-budget-and-race-safe-reservation", localAIOMNII03}, {"I04-fixture-hash-rejection-before-launch", localAIOMNII04}, {"I05-text-exact-token-pass", localAIOMNISemanticCase}, {"I06-text-token-absence-fail", localAIOMNISemanticCase}, {"I07-image-grounded-facts-pass", localAIOMNISemanticCase}, {"I08-video-grounded-numeric-window-pass", localAIOMNISemanticCase}, {"I09-video-vague-midpoint-inconclusive", localAIOMNISemanticCase}, {"I10-video-prompt-echo-inconclusive", localAIOMNISemanticCase}, {"I11-video-numeric-window-fail", localAIOMNISemanticCase}, {"I12-bounded-redacted-evidence", localAIOMNII12}, {"I13-atomic-report-interruption", localAIOMNII13}, {"I14-cancellation-and-timeout-cleanup", localAIOMNII14}, {"I15-owned-resource-leak-never-passes", localAIOMNII15}}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { t.Parallel(); c.run(t) })
	}
}
func localAIOMNII01(t *testing.T) {
	_, err := localAIOMNIInvocationValues("", "")
	localAIOMNIFailIf(t, !errors.Is(err, errLocalAIOMNIRealDisabled), "disabled admission=%v", err)
	_, err = localAIOMNIInvocationValues("1", "relative.json")
	localAIOMNIFailIf(t, err == nil, "relative real manifest accepted")
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	path := filepath.Join(root, "manifest.json")
	body, _ := json.Marshal(m)
	localAIOMNIFailIf(t, os.WriteFile(path, body, 0o600) != nil, "manifest write failed")
	in, err := localAIOMNIInvocationValues("1", path)
	localAIOMNIFailIf(t, err != nil || in.EvidenceKind != localAIOMNIReal, "real admission=%#v err=%v", in, err)
	r := localAIOMNIStatusFor(t, m, "I01", m.Text.Token, "PASS")
	localAIOMNIFailIf(t, r.Activity != (localAIOMNIActivity{}) || r.Reservation == nil || r.Reservation.State != "COMMITTED", "report=%#v", r)
	data, _ := os.ReadFile(m.Evidence.ReportPath)
	localAIOMNIFailIf(t, bytes.Contains(data, []byte(m.Isolation.WorkRoot)) || bytes.Contains(data, []byte(m.CLI.Path)), "report leaked an owned path")
	shell, err := exec.LookPath("cmd.exe")
	localAIOMNIFailIf(t, err != nil, "cmd.exe lookup failed: %v", err)
	identity, ok := localAIReadFileIdentity(shell)
	localAIOMNIFailIf(t, !ok, "hash process-boundary executable %s", shell)
	boundaryRoot := t.TempDir()
	bm := localAIOMNIManifestForTest(t, boundaryRoot)
	bm.CLI = localAIOMNICLI{Path: shell, SHA256: identity.SHA256}
	roots := localAIOMNIRoots(bm)
	localAIOMNIFailIf(t, prepareLocalAIRoots(roots) != nil, "root preparation failed")
	localAIOMNIFailIf(t, os.WriteFile(filepath.Join(roots.Streams, "backend.log"), []byte("backend.log completed"), 0o600) != nil || os.WriteFile(filepath.Join(roots.Streams, "runtime.log"), []byte("runtime.log completed"), 0o600) != nil, "log write failed")
	script := fmt.Sprintf("echo %s & echo HOME=%%HOME%%", bm.Text.Token)
	in = localAIOMNIInvocationForTest(t, bm, "I01-boundary", localAIOMNIControlled)
	in.Command = localAICommandSpec{BinaryPath: shell, Arguments: []string{"/D", "/C", script}}
	boundaryReport, err := mustLocalAIOMNIRunner(t, &localAIOMNIProcessExecutor{}).Run(t.Context(), in)
	localAIOMNIFailIf(t, err != nil || boundaryReport.Status != "PASS" || boundaryReport.Activity != (localAIOMNIActivity{}) || len(boundaryReport.Logs.Backend) != 1 || len(boundaryReport.Logs.Runtime) != 1, "process-boundary report=%#v err=%v", boundaryReport, err)
	localAIOMNIFailIf(t, !strings.Contains(boundaryReport.Semantic.Observed, pathIdentityHash(roots.Profile)), "process-boundary observed=%q, want isolated HOME identity", boundaryReport.Semantic.Observed)
}
func localAIOMNII02(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	valid, _ := json.Marshal(m)
	m.Text.Token = ""
	localAIOMNIAdmissionFailure(t, m, "I02", true)
	cases := map[string][]byte{"malformed": []byte("{"), "unknown": append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"unknown":1}`)...), "trailing": append(append([]byte(nil), valid...), []byte(" {}")...)}
	for name, body := range cases {
		path := filepath.Join(root, name+".json")
		localAIOMNIFailIf(t, os.WriteFile(path, body, 0o600) != nil, "manifest fixture write failed")
		_, _, err := localAIOMNIReadManifest(path)
		localAIOMNIFailIf(t, err == nil, "%s manifest was accepted", name)
	}
}
func localAIOMNIAdmissionFailure(t *testing.T, m localAIOMNIManifest, id string, ledgerAbsent bool) {
	e := &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(m.Isolation.WorkRoot, "unused")}
	r := localAIOMNIReportMust(t, m, id, localAIOMNIPass(m.Isolation.WorkRoot, "unused"), e)
	localAIOMNIFailIf(t, r.Status != "FAIL" || r.Failure == nil || r.Failure.Assertion != "manifest admission" || e.Calls.Load() != 0, "report=%#v calls=%d", r, e.Calls.Load())
	if ledgerAbsent {
		_, err := os.Stat(m.Evidence.LedgerPath)
		localAIOMNIFailIf(t, !errors.Is(err, os.ErrNotExist), "ledger=%v", err)
	}
}
func localAIOMNII03(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	_ = localAIOMNIStatusFor(t, m, "I03", m.Text.Token, "PASS")
	m.Evidence.ReportPath = filepath.Join(root, "evidence", "second.json")
	e := &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(root, m.Text.Token)}
	r := localAIOMNIReportMust(t, m, "I03", localAIOMNIPass(root, m.Text.Token), e)
	localAIOMNIFailIf(t, r.Status != "FAIL" || r.Failure == nil || r.Failure.Assertion != "durable model-call budget" || e.Calls.Load() != 0, "exhausted report=%#v calls=%d", r, e.Calls.Load())
	corruptRoot := t.TempDir()
	cm := localAIOMNIManifestForTest(t, corruptRoot)
	localAIOMNIFailIf(t, os.MkdirAll(filepath.Dir(cm.Evidence.LedgerPath), 0o700) != nil, "ledger directory creation failed")
	localAIOMNIFailIf(t, os.WriteFile(cm.Evidence.LedgerPath, []byte(`{"schema":"corrupt"}`), 0o600) != nil, "corrupt ledger write failed")
	ce := &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(corruptRoot, cm.Text.Token)}
	cr := localAIOMNIReportMust(t, cm, "I03-corrupt", localAIOMNIPass(corruptRoot, cm.Text.Token), ce)
	localAIOMNIFailIf(t, cr.Status != "FAIL" || cr.Failure == nil || cr.Failure.Assertion != "durable model-call ledger" || ce.Calls.Load() != 0, "corrupt ledger report=%#v calls=%d", cr, ce.Calls.Load())
	raceRoot := t.TempDir()
	ledger := filepath.Join(raceRoot, "ledger.json")
	const attempts = 9
	results := make(chan localAIOMNIReport, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			cell := filepath.Join(raceRoot, fmt.Sprintf("cell-%d", i))
			cm := localAIOMNIManifestForTest(t, cell)
			cm.Limits.ModelCalls = 3
			cm.Evidence.LedgerPath = ledger
			cm.Evidence.ReportPath = filepath.Join(cell, "report.json")
			rr, e := mustLocalAIOMNIRunner(t, &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(cell, cm.Text.Token)}).Run(t.Context(), localAIOMNIInvocationForTest(t, cm, "I03-race", localAIOMNIControlled))
			localAIOMNIFailIf(t, e != nil, "race report: %v", e)
			results <- rr
		}()
	}
	wg.Wait()
	close(results)
	pass, exhausted := 0, 0
	for r := range results {
		if r.Status == "PASS" {
			pass++
		} else if r.Status == "FAIL" && r.Failure != nil && r.Failure.Assertion == "durable model-call budget" {
			exhausted++
		} else {
			t.Fatalf("race report=%#v", r)
		}
	}
	localAIOMNIFailIf(t, pass != 3 || exhausted != attempts-3, "race pass=%d exhausted=%d", pass, exhausted)
}
func localAIOMNII04(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	m.Selector = localAIOMNIImage
	m.Image.SHA256 = strings.Repeat("0", 64)
	localAIOMNIAdmissionFailure(t, m, "I04", false)
	localAIOMNIFailIf(t, os.Remove(m.Image.Path) != nil, "fixture removal failed")
	localAIOMNIAdmissionFailure(t, m, "I04-missing-fixture", false)
}

var localAIOMNISemantics = map[string]struct{ selector, output, want string }{
	"I05": {localAIOMNIText, "COBALT-17", "PASS"}, "I06": {localAIOMNIText, "COBALT-18", "FAIL"},
	"I07": {localAIOMNIImage, "The image shows an infinity symbol and the words INFINITE YOU.", "PASS"},
	"I08": {localAIOMNIVideo, "PHASE 1 is red, then PHASE 2 is blue; the transition occurred at 2000 ms.", "PASS"},
	"I09": {localAIOMNIVideo, "PHASE 1 is red, then PHASE 2 is blue around the midpoint.", "INCONCLUSIVE"},
	"I10": {localAIOMNIVideo, "", "INCONCLUSIVE"}, "I11": {localAIOMNIVideo, "PHASE 1 red; PHASE 2 blue; transition at 3000 ms.", "FAIL"},
}

func localAIOMNISemanticCase(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	name := t.Name()
	key := strings.SplitN(name, "/", 2)[1][:3]
	spec := localAIOMNISemantics[key]
	selector, output, want := spec.selector, spec.output, spec.want
	if key == "I10" {
		selector = localAIOMNIVideo
		m.Selector = selector
		cmd := localAIOMNICommand(m)
		for i, a := range cmd.Arguments {
			if a == "--input" && i+1 < len(cmd.Arguments) && strings.HasPrefix(cmd.Arguments[i+1], "prompt=") {
				output = strings.TrimPrefix(cmd.Arguments[i+1], "prompt=")
			}
		}
		want = "INCONCLUSIVE"
	}
	m.Selector = selector
	r := localAIOMNIStatusFor(t, m, name, output, want)
	localAIOMNIFailIf(t, key == "I08" && (r.Semantic.NumericTransitionMilliseconds == nil || *r.Semantic.NumericTransitionMilliseconds != 2000), "video semantic=%#v", r.Semantic)
	localAIOMNIFailIf(t, key == "I10" && !r.Semantic.PromptEcho, "prompt echo was not classified inconclusive")
}
func localAIOMNII12(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	body := append([]byte("HF_TOKEN=controlled-secret "), bytes.Repeat([]byte("bounded-output "), localAIOMNIMaxStream)...)
	o := localAIOMNIPass(root, m.Text.Token)
	o.Stderr, o.BackendLogs[0].Body, o.RuntimeLogs[0].Body = body, body, body
	r := localAIOMNIReportMust(t, m, "I12", o, &localAIOMNIControlledExecutor{Observation: o})
	localAIOMNIFailIf(t, r.Status != "FAIL" || r.Failure == nil || r.Failure.Owner != "harness" || !r.Command.Stderr.Truncated || !r.Logs.Backend[0].Truncated || !r.Logs.Runtime[0].Truncated, "evidence report=%#v", r)
	data, _ := os.ReadFile(m.Evidence.ReportPath)
	localAIOMNIFailIf(t, bytes.Contains(data, []byte("controlled-secret")), "secret leaked")
	processRoot := t.TempDir()
	pm := localAIOMNIManifestForTest(t, processRoot)
	roots := localAIOMNIRoots(pm)
	localAIOMNIFailIf(t, os.MkdirAll(roots.Work, 0o700) != nil, "process work root creation failed")
	shell, err := exec.LookPath("cmd.exe")
	localAIOMNIFailIf(t, err != nil, "cmd.exe lookup failed: %v", err)
	actual := localAIOMNIProcessExecutor{}.Execute(t.Context(), localAICommandSpec{BinaryPath: shell, Arguments: []string{"/D", "/C", "for /L %i in (1,1,70000) do @echo X"}}, roots)
	stderr := localAIOMNIProcessExecutor{}.Execute(t.Context(), localAICommandSpec{BinaryPath: shell, Arguments: []string{"/D", "/C", "for /L %i in (1,1,70000) do @echo X 1^>^&2"}}, roots)
	actual.Stderr, actual.StderrTruncated = stderr.Stderr, stderr.StderrTruncated
	localAIOMNIFailIf(t, !actual.StdoutTruncated || !actual.StderrTruncated, "actual process observation bytes=%d truncated=%t stderrBytes=%d stderrTruncated=%t", len(actual.Stdout), actual.StdoutTruncated, len(actual.Stderr), actual.StderrTruncated)
	truncated := localAIOMNIReportMust(t, pm, "I12-process", actual, &localAIOMNIControlledExecutor{Observation: actual})
	localAIOMNIFailIf(t, truncated.Status != "FAIL" || truncated.Failure == nil || truncated.Failure.Assertion != "bounded command streams", "actual process report=%#v", truncated)
}
func localAIOMNII13(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	m.Limits.ModelCalls = 2
	_ = localAIOMNIStatusFor(t, m, "I13", m.Text.Token, "PASS")
	old, _ := os.ReadFile(m.Evidence.ReportPath)
	runner := mustLocalAIOMNIRunner(t, &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(root, m.Text.Token)})
	runner.writeReport = func(path string, r localAIOMNIReport) error {
		return writeLocalAIOMNIReportAtomicHook(path, r, func() error { return errLocalAIOMNIReportInterrupted })
	}
	r, err := runner.Run(t.Context(), localAIOMNIInvocationForTest(t, m, "I13", localAIOMNIControlled))
	localAIOMNIFailIf(t, !errors.Is(err, errLocalAIOMNIReportInterrupted) || r.Status != "PASS", "interrupted report=%#v err=%v", r, err)
	now, _ := os.ReadFile(m.Evidence.ReportPath)
	localAIOMNIFailIf(t, !bytes.Equal(old, now), "atomic interruption replaced canonical report")
}
func localAIOMNII14(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	e := &localAIOMNICancellationExecutor{Started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan localAIOMNIReport, 1)
	go func() {
		r, err := mustLocalAIOMNIRunner(t, e).Run(ctx, localAIOMNIInvocationForTest(t, m, "I14", localAIOMNIControlled))
		localAIOMNIFailIf(t, err != nil, "cancellation report: %v", err)
		done <- r
	}()
	<-e.Started
	cancel()
	out := <-done
	localAIOMNIFailIf(t, out.Status != "FAIL" || out.Failure == nil || out.Failure.Owner != "harness" || !out.Release.Checked || !out.Release.ProcessTreeClosed, "cancellation report=%#v", out)
}
func localAIOMNII15(t *testing.T) {
	root := t.TempDir()
	m := localAIOMNIManifestForTest(t, root)
	_ = os.MkdirAll(m.Isolation.OutputRoot, 0o700)
	_ = os.WriteFile(filepath.Join(m.Isolation.OutputRoot, "response.partial"), []byte("partial"), 0o600)
	o := localAIOMNIPass(root, m.Text.Token)
	o.Started, o.ProcessExited, o.ProcessTreeClosed, o.OwnedProcesses = true, false, false, 1
	r := localAIOMNIReportMust(t, m, "I15", o, &localAIOMNIControlledExecutor{Observation: o})
	localAIOMNIFailIf(t, r.Status != "FAIL" || r.Failure == nil || r.Failure.Owner != "harness" || !r.Release.Checked || r.Release.PartialArtifacts == 0 || r.Release.ProcessTreeClosed, "cleanup report=%#v", r)
	inspectionRoot := t.TempDir()
	im := localAIOMNIManifestForTest(t, inspectionRoot)
	runner := mustLocalAIOMNIRunner(t, &localAIOMNIControlledExecutor{Observation: localAIOMNIPass(inspectionRoot, im.Text.Token), RemoveRoot: true})
	final, err := runner.Run(t.Context(), localAIOMNIInvocationForTest(t, im, "I15-inspection", localAIOMNIControlled))
	localAIOMNIFailIf(t, err != nil || final.Status != "INCONCLUSIVE" || final.Failure == nil || final.Failure.Assertion != "final cache and release inspection" || final.Release.Checked || !final.Release.InspectionFailed, "final inspection report=%#v err=%v", final, err)
}
