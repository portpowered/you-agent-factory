//go:build windows

package models_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/locking"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func (executor localAIProcessExecutor) Execute(ctx context.Context, spec localAICommandSpec, roots localAIRealRoots) localAICommandObservation {
	result := localAICommandObservation{ExitCode: -1}
	if strings.TrimSpace(spec.BinaryPath) == "" || !filepath.IsAbs(spec.BinaryPath) {
		return result
	}
	command := exec.Command(spec.BinaryPath, spec.Arguments...)
	command.Dir = roots.Work
	command.Env = spec.Environment
	var stdout, stderr localAIBoundedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	platformprocess.ConfigureSubprocessTree(command)
	if err := command.Start(); err != nil {
		return result
	}
	result.Started = true
	tree, attachErr := platformprocess.AttachSubprocessTree(command)
	if attachErr != nil {
		_ = platformprocess.TerminateSubprocessTree(command, tree)
	} else {
		result.ProcessTreeAttached = true
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- command.Wait() }()
	select {
	case waitErr := <-waitCh:
		result = localAICommandResultFromWait(result, command, waitErr)
	case <-ctx.Done():
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		_ = platformprocess.TerminateSubprocessTree(command, tree)
		result = localAICommandResultFromWait(result, command, <-waitCh)
	}
	platformprocess.CloseSubprocessTree(command, tree)
	result.ProcessTreeClosed = result.ProcessTreeAttached
	result.Stdout = append([]byte(nil), stdout.Bytes()...)
	result.Stderr = append([]byte(nil), stderr.Bytes()...)
	result.StdoutTruncated = stdout.Truncated()
	result.StderrTruncated = stderr.Truncated()
	return result
}

func localAICommandResultFromWait(result localAICommandObservation, command *exec.Cmd, waitErr error) localAICommandObservation {
	result.ProcessExited = command.ProcessState != nil
	if waitErr == nil {
		result.ExitCode = 0
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	return result
}

type localAIBoundedBuffer struct {
	bytes.Buffer
	truncated bool
}

func (buffer *localAIBoundedBuffer) Write(value []byte) (int, error) {
	remaining := localAIRealMaxStreamBytes - buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = buffer.Buffer.Write(value[:remaining])
		buffer.truncated = true
		return len(value), nil
	}
	return buffer.Buffer.Write(value)
}

func (buffer *localAIBoundedBuffer) Truncated() bool { return buffer.truncated }

func localAIStreamViolation(stdout, stderr []byte) string {
	return localAIStreamViolationForJourney(stdout, stderr, false)
}

func localAIStreamViolationForJourney(stdout, stderr []byte, allowExpectedTranscript bool) string {
	value := strings.ToLower(string(append(append([]byte(nil), stdout...), stderr...)))
	for _, marker := range []string{
		"hf_token=", "authorization:", "bearer ", "password=", "api_key=", "access_token=",
		"x-amz-signature=", "signed_url=", "127.0.0.1", "localhost:", "grpc://", "tcp://",
		"raw audio", "riff",
	} {
		if strings.Contains(value, marker) {
			return "forbidden stream marker"
		}
	}
	if !allowExpectedTranscript && strings.Contains(value, "local ai works on this machine") {
		return "forbidden stream marker"
	}
	return ""
}

func TestLocalAIRealHarnessControlledHelper(t *testing.T) {
	mode := strings.TrimSpace(os.Getenv(localAIRealHelperModeEnv))
	if mode == "" {
		return
	}
	switch mode {
	case "pass":
		outputPath := os.Getenv(localAIRealHelperOutputEnv)
		if err := writeLocalAIControlledWAV(outputPath); err != nil {
			os.Exit(21)
		}
		_, _ = os.Stdout.Write([]byte(`{"outputs":[{"name":"audio","modality":"AUDIO","mediaType":"audio/wav","content":"controlled"}]}`))
		os.Exit(0)
	case "offline-pass":
		if os.Getenv("HF_HUB_OFFLINE") != "1" || os.Getenv("LOCALAI_OFFLINE") != "1" || !strings.Contains(os.Getenv("HTTP_PROXY"), "127.0.0.1:9") {
			os.Exit(24)
		}
		outputPath := os.Getenv(localAIRealHelperOutputEnv)
		if err := writeLocalAIControlledWAV(outputPath); err != nil {
			os.Exit(21)
		}
		_, _ = os.Stdout.Write([]byte(`{"outputs":[{"name":"audio","modality":"AUDIO","mediaType":"audio/wav","content":"controlled"}]}`))
		os.Exit(0)
	case "malformed":
		_, _ = os.Stdout.Write([]byte(`{"outputs":[`))
		os.Exit(0)
	case "secret":
		_, _ = os.Stdout.Write([]byte(`HF_TOKEN=controlled-secret`))
		os.Exit(0)
	case "asr-pass":
		writeLocalAIControlledASR(t, "zero", 600)
		os.Exit(0)
	case "asr-mismatch":
		writeLocalAIControlledASR(t, "one", 600)
		os.Exit(0)
	case "asr-failure":
		os.Exit(23)
	case "integration-tts":
		outputPath := os.Getenv(localAIRealHelperOutputEnv)
		if err := writeLocalAIControlledWAV(outputPath); err != nil {
			os.Exit(21)
		}
		_, _ = os.Stdout.Write([]byte(`{"outputs":[{"name":"audio","modality":"AUDIO","mediaType":"audio/wav","content":"controlled"}]}`))
		os.Exit(0)
	case "integration-asr":
		writeLocalAIControlledASR(t, "local ai works on this machine", 8)
		os.Exit(0)
	case "ledger-reserve":
		writeLocalAIReservationHelperResult(t)
		os.Exit(0)
	default:
		os.Exit(22)
	}
}

func TestLocalAIRealHarnessOutputBounds(t *testing.T) {
	t.Parallel()
	audio := localAIControlledWAVBytes()
	if _, ok := localAIWAVMetadataWithLimits(audio, localAIRealMaxAudioBytes, localAIRealMaxAudioDuration); !ok {
		t.Fatal("valid controlled WAV was rejected by the production bounds")
	}
	if _, ok := localAIWAVMetadataWithLimits(audio, int64(len(audio))-1, localAIRealMaxAudioDuration); ok {
		t.Fatal("WAV over the byte limit was accepted")
	}
	if _, ok := localAIWAVMetadataWithLimits(audio, localAIRealMaxAudioBytes, time.Millisecond); ok {
		t.Fatal("WAV over the duration limit was accepted")
	}
}

func TestLocalAIRealHarnessInconclusiveReport(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	request := localAIControlledRequest(t, root, "pass")
	report := newLocalAIRealReport(request)
	setLocalAIInconclusive(&report, "unresolved", "controlled dependency availability", "authorized dependency is available", "dependency availability was not established")
	if _, err := mustLocalAIRealRunner(t).finish(request, report); err != nil {
		t.Fatalf("write inconclusive report: %v", err)
	}
	if report.Status != "INCONCLUSIVE" || report.Journeys[0].Failure == nil {
		t.Fatalf("inconclusive report = %#v, want bounded unresolved failure", report)
	}
}

func assertLocalAIJourneyResult(t testing.TB, request localAIRealRunRequest, report localAIRealReport, wantStatus, wantOwner string) {
	t.Helper()
	selector := "<missing>"
	if len(report.Journeys) == 1 {
		selector = report.Journeys[0].Selector
	}
	if report.Status != wantStatus || len(report.Journeys) != 1 || selector != request.Selector {
		failure := "<nil>"
		if report.Journeys != nil && len(report.Journeys) == 1 && report.Journeys[0].Failure != nil {
			failure = fmt.Sprintf("%+v", *report.Journeys[0].Failure)
		}
		t.Fatalf("report status=%s selector=%s failure=%s, want one %s %q journey", report.Status, selector, failure, wantStatus, request.Selector)
	}
	if wantOwner != "" {
		if report.Journeys[0].Failure == nil || report.Journeys[0].Failure.Owner != wantOwner {
			t.Fatalf("failure = %#v, want owner %q", report.Journeys[0].Failure, wantOwner)
		}
		if err := validateLocalAIFailure(*report.Journeys[0].Failure); err != nil {
			t.Fatalf("failure validation: %v", err)
		}
	}
	body, err := os.ReadFile(request.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if err := validateLocalAIRealReport(report); err != nil {
		t.Fatalf("report validation: %v", err)
	}
	if !bytes.Contains(body, []byte(`"redacted": true`)) {
		t.Fatalf("report did not record redaction: %s", body)
	}
	if bytes.Contains(body, []byte("controlled-secret")) || bytes.Contains(body, []byte("HF_TOKEN=")) {
		t.Fatalf("report leaked forbidden data: %s", body)
	}
}

func setupLocalAIReportInterruption(t testing.TB, _ string, request *localAIRealRunRequest) {
	t.Helper()
	old := newLocalAIRealReport(*request)
	old.Status = "PASS"
	old.Journeys[0].Status = "PASS"
	old.Journeys[0].Artifacts = []localAIRealArtifact{{Kind: "output-audio", Path: "tts.wav", MediaType: "audio/wav", Bytes: 44, SHA256: sha256Hex(bytes.Repeat([]byte{'a'}, 44))}}
	old.Journeys[0].Semantic = localAIRealSemantic{Assertion: "controlled", Expected: "valid", Observed: "valid", Passed: true}
	old.Journeys[0].Release = localAIRealRelease{Checked: true, ProcessTreeClosed: true}
	old.Journeys[0].Cache.AfterIdentitySHA256 = old.Journeys[0].Cache.BeforeIdentitySHA256
	old.Journeys[0].Cache.AfterEntries = old.Journeys[0].Cache.BeforeEntries
	old.Journeys[0].Cache.AfterBytes = old.Journeys[0].Cache.BeforeBytes
	old.BudgetLedger = localAIRealLedgerIdentity{PathIdentity: filepath.Base(request.LedgerPath), SHA256: sha256Hex([]byte("old-ledger"))}
	if err := writeLocalAIRealReportAtomic(request.ReportPath, old); err != nil {
		t.Fatalf("seed canonical report: %v", err)
	}
}

func assertLocalAIReportInterruption(t testing.TB, request localAIRealRunRequest, report localAIRealReport, err error) {
	t.Helper()
	if !errors.Is(err, errLocalAIReportInterrupted) {
		t.Fatalf("runner error = %v, want report interruption", err)
	}
	if report.Status != "PASS" {
		t.Fatalf("interrupted report result = %#v, want completed in-memory PASS", report)
	}
	body, readErr := os.ReadFile(request.ReportPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Contains(body, []byte(`"status": "PASS"`)) {
		t.Fatalf("previous canonical report was not preserved: %s", body)
	}
}

var errLocalAIReportInterrupted = errors.New("controlled report interruption")

func setupLocalAIBudgetExhaustion(t testing.TB, _ string, request *localAIRealRunRequest) {
	t.Helper()
	locks := mustLocalAILockService(t)
	if _, _, err := reserveLocalAIBudget(t.Context(), locks, request.LedgerPath, request.RunID, request.Selector, request.ReservationKind, request.ReservationAmount, request.Limits); err != nil {
		t.Fatalf("seed budget reservation: %v", err)
	}
}

func setupLocalAICorruptLedger(t testing.TB, _ string, request *localAIRealRunRequest) {
	t.Helper()
	if err := os.WriteFile(request.LedgerPath, []byte(`{"schema":"wrong"}`), 0o600); err != nil {
		t.Fatalf("write corrupt ledger: %v", err)
	}
}

func setupLocalAIPartialArtifactLeak(t testing.TB, root string, _ *localAIRealRunRequest) {
	t.Helper()
	roots := localAIRealRootsFor(root)
	if err := prepareLocalAIRoots(roots); err != nil {
		t.Fatalf("prepare leak roots: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roots.Output, "tts.wav.partial"), []byte("incomplete"), 0o600); err != nil {
		t.Fatalf("seed partial artifact: %v", err)
	}
}

func mustLocalAILockService(t testing.TB) locking.Service {
	t.Helper()
	locks, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		t.Fatalf("new lock service: %v", err)
	}
	return locks
}

func readLocalAIBudgetForTest(t testing.TB, path string) localAIBudgetLedger {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read budget: %v", err)
	}
	ledger, err := decodeLocalAIBudget(body)
	if err != nil {
		t.Fatalf("decode budget: %v", err)
	}
	return ledger
}

func runLocalAIReservationHelper(t testing.TB, root, ledgerPath, resultPath string) string {
	t.Helper()
	binaryPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	roots := localAIRealRootsFor(root)
	if err := prepareLocalAIRoots(roots); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), binaryPath, "-test.run=TestLocalAIRealHarnessControlledHelper", "--")
	command.Dir = roots.Work
	command.Env = localAIProcessEnvironment(roots, []string{
		localAIRealHelperModeEnv + "=ledger-reserve",
		"LOCALAI_REAL_HELPER_LEDGER=" + ledgerPath,
		"LOCALAI_REAL_HELPER_RUN_ID=run-persistence",
		localAIRealHelperResultEnv + "=" + resultPath,
	})
	if err := command.Run(); err != nil {
		t.Fatalf("reservation helper: %v", err)
	}
	body, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(body))
}

func writeLocalAIReservationHelperResult(t *testing.T) {
	ledgerPath := os.Getenv("LOCALAI_REAL_HELPER_LEDGER")
	resultPath := os.Getenv(localAIRealHelperResultEnv)
	locks, err := locking.New(locking.LocalFileSystem{})
	result := "ERROR"
	if err == nil {
		_, _, reserveErr := reserveLocalAIBudget(context.Background(), locks, ledgerPath, "run-persistence", "known-tts", "modelCall", 1, localAIBudgetLimits{ModelCalls: 1})
		var budgetErr *localAIBudgetError
		if errors.As(reserveErr, &budgetErr) && budgetErr.Code == "budget_exhausted" {
			result = "BUDGET_EXHAUSTED"
		} else if reserveErr == nil {
			result = "RESERVED"
		}
	}
	_ = os.WriteFile(resultPath, []byte(result), 0o600)
}

type localAIStubExecutor struct {
	observation localAICommandObservation
	calls       atomic.Int32
}

func (stub *localAIStubExecutor) Execute(context.Context, localAICommandSpec, localAIRealRoots) localAICommandObservation {
	stub.calls.Add(1)
	return stub.observation
}
