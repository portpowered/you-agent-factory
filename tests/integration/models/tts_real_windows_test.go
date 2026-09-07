//go:build windows

package models_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	localAIRealBinaryEnv     = "INFINITE_YOU_INTEGRATION_BINARY"
	localAIRealEnableEnv     = "INFINITE_YOU_LOCALAI_REAL_ENABLE"
	localAIRealReportEnv     = "INFINITE_YOU_LOCALAI_EVIDENCE_OUTPUT"
	localAIRealLedgerEnv     = "INFINITE_YOU_LOCALAI_BUDGET_LEDGER"
	localAIRealCacheEnv      = "INFINITE_YOU_LOCALAI_CACHE_ROOT"
	localAIRealRunID         = "localai-real-run"
	localAIRealPrompt        = "Local AI works on this machine"
	localAIRealDownloadLimit = int64(4 << 30)
)

type localAIRealSelectorSpec struct {
	selector           string
	kind               string
	offline            bool
	inputPath          string
	expectedInputSHA   string
	expectedTranscript string
	reservationAmount  int64
	mode               string
}

func TestLocalAIRealHarnessFirstUseTTS(t *testing.T) {
	runLocalAIRealSelector(t, localAIRealSelectorSpec{
		selector: "first-use-tts", kind: localAIJourneyTTS, mode: "pass", reservationAmount: 1,
	})
}

func TestLocalAIRealHarnessOfflineCacheReuseTTS(t *testing.T) {
	runLocalAIRealSelector(t, localAIRealSelectorSpec{
		selector: "offline-cache-reuse-tts", kind: localAIJourneyTTS, offline: true, mode: "pass", reservationAmount: 1,
	})
}

func TestLocalAIRealHarnessKnownFixtureASR(t *testing.T) {
	fixturePath := localAIKnownFixturePath(t)
	runLocalAIRealSelector(t, localAIRealSelectorSpec{
		selector: "known-fixture-asr", kind: localAIJourneyASR, inputPath: fixturePath,
		expectedInputSHA: knownASRFixtureSHA256, expectedTranscript: knownASRFixtureTranscript,
		mode: "asr-pass", reservationAmount: 1,
	})
}

func TestLocalAIRealHarnessTTSASRIntegration(t *testing.T) {
	runLocalAIRealSelector(t, localAIRealSelectorSpec{
		selector: "tts-to-asr-integration", kind: localAIJourneyTTSASR,
		expectedTranscript: localAIRealPrompt, mode: "integration", reservationAmount: 2,
	})
}

// The short lane discovers every real entry point but admits no external
// artifact, command, model, backend, or ledger reservation.
func TestLocalAIRealHarnessSelectorsShortMode(t *testing.T) {
	if !testing.Short() {
		t.Skip("short-mode selector discovery is only exercised by the short lane")
	}
	ledgerPath := filepath.Join(t.TempDir(), "localai-budget.json")
	t.Setenv(localAIRealLedgerEnv, ledgerPath)
	for _, spec := range []localAIRealSelectorSpec{
		{selector: "first-use-tts", kind: localAIJourneyTTS, mode: "pass", reservationAmount: 1},
		{selector: "offline-cache-reuse-tts", kind: localAIJourneyTTS, offline: true, mode: "pass", reservationAmount: 1},
		{selector: "known-fixture-asr", kind: localAIJourneyASR, mode: "asr-pass", reservationAmount: 1},
		{selector: "tts-to-asr-integration", kind: localAIJourneyTTSASR, mode: "integration", reservationAmount: 2},
	} {
		runLocalAIRealSelector(t, spec)
	}
	if _, err := os.Stat(ledgerPath); !os.IsNotExist(err) {
		t.Fatalf("short selector discovery touched the durable ledger: stat=%v", err)
	}
}

func TestLocalAIRealHarnessSelectorControlledCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		selector      string
		kind          string
		mode          string
		offline       bool
		expected      string
		wantStatus    string
		wantOwner     string
		wantArtifacts int
	}{
		{name: "IC-02-offline-cache-reuse-tts", selector: "offline-cache-reuse-tts", kind: localAIJourneyTTS, mode: "pass", offline: true, wantStatus: "PASS", wantArtifacts: 1},
		{name: "IC-03-known-fixture-asr", selector: "known-fixture-asr", kind: localAIJourneyASR, mode: "asr-pass", expected: knownASRFixtureTranscript, wantStatus: "PASS", wantArtifacts: 3},
		{name: "IC-04-tts-to-asr-integration", selector: "tts-to-asr-integration", kind: localAIJourneyTTSASR, mode: "integration", expected: localAIRealPrompt, wantStatus: "PASS", wantArtifacts: 4},
		{name: "IC-06-asr-invocation-failure", selector: "known-fixture-asr", kind: localAIJourneyASR, mode: "asr-failure", expected: knownASRFixtureTranscript, wantStatus: "FAIL", wantOwner: "product"},
		{name: "IC-07-asr-semantic-mismatch", selector: "known-fixture-asr", kind: localAIJourneyASR, mode: "asr-mismatch", expected: knownASRFixtureTranscript, wantStatus: "FAIL", wantOwner: "product"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			request := localAIControlledSelectorRequest(t, t.TempDir(), testCase.selector, testCase.kind, testCase.mode, testCase.offline, testCase.expected)
			report, err := mustLocalAIRealRunner(t).Run(t.Context(), request)
			if err != nil {
				t.Fatalf("runner returned infrastructure error: %v", err)
			}
			assertLocalAIJourneyResult(t, request, report, testCase.wantStatus, testCase.wantOwner)
			if testCase.wantArtifacts > 0 && len(report.Journeys[0].Artifacts) != testCase.wantArtifacts {
				t.Fatalf("artifacts = %d, want %d", len(report.Journeys[0].Artifacts), testCase.wantArtifacts)
			}
			if testCase.offline && !report.Journeys[0].Offline {
				t.Fatal("offline selector did not retain offline evidence")
			}
		})
	}
}

func localAIControlledSelectorRequest(t testing.TB, root, selector, kind, mode string, offline bool, expected string) localAIRealRunRequest {
	t.Helper()
	request := localAIControlledRequest(t, root, mode)
	request.Selector = selector
	request.Kind = kind
	request.Offline = offline
	request.ExpectedTranscript = expected
	request.ReservationAmount = 1
	request.Command.Environment[0] = localAIRealHelperModeEnv + "=" + mode
	if kind == localAIJourneyASR {
		request.InputPath = localAIKnownFixturePath(t)
		request.ExpectedInputSHA256 = knownASRFixtureSHA256
		request.Command.OutputName = localAIRealSegmentsFile
	}
	if kind == localAIJourneyTTSASR {
		request.ReservationAmount = 2
		request.Limits.ModelCalls = 2
		request.Command.Environment[0] = localAIRealHelperModeEnv + "=integration-tts"
		request.Command.OutputName = "tts.wav"
		followUp := request.Command
		followUp.Environment = []string{localAIRealHelperModeEnv + "=integration-asr", localAIRealHelperOutputEnv + "={output}"}
		followUp.OutputName = localAIRealSegmentsFile
		request.FollowUp = &followUp
	}
	return request
}

func runLocalAIRealSelector(t *testing.T, spec localAIRealSelectorSpec) {
	t.Helper()
	if testing.Short() {
		t.Logf("LOCALAI-SELECTOR selector=%s status=INCONCLUSIVE reason=short-mode-no-external-process-or-ledger", spec.selector)
		return
	}
	if strings.TrimSpace(os.Getenv(localAIRealEnableEnv)) != "1" {
		t.Logf("LOCALAI-SELECTOR selector=%s status=INCONCLUSIVE reason=real-selector-not-enabled", spec.selector)
		return
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Fatalf("real selector %q requires Windows amd64", spec.selector)
	}
	request, err := newLocalAIRealSelectorRequest(t, spec)
	if err != nil {
		t.Fatal(err)
	}
	runner := mustLocalAIRealRunner(t)
	report, err := runner.Run(t.Context(), request)
	if err != nil {
		t.Fatalf("selector %q report: %v", spec.selector, err)
	}
	if report.Status != "PASS" || len(report.Journeys) != 1 || report.Journeys[0].Selector != spec.selector {
		t.Fatalf("selector %q report = %#v, want one PASS journey", spec.selector, report)
	}
	t.Logf("LOCALAI-SELECTOR selector=%s status=PASS report=%s ledger=%s", spec.selector, filepath.Base(request.ReportPath), filepath.Base(request.LedgerPath))
}

func newLocalAIRealSelectorRequest(t testing.TB, spec localAIRealSelectorSpec) (localAIRealRunRequest, error) {
	t.Helper()
	binaryPath := strings.TrimSpace(os.Getenv(localAIRealBinaryEnv))
	reportPath := strings.TrimSpace(os.Getenv(localAIRealReportEnv))
	ledgerPath := strings.TrimSpace(os.Getenv(localAIRealLedgerEnv))
	cacheRoot := strings.TrimSpace(os.Getenv(localAIRealCacheEnv))
	for name, value := range map[string]string{
		localAIRealBinaryEnv: binaryPath, localAIRealReportEnv: reportPath,
		localAIRealLedgerEnv: ledgerPath, localAIRealCacheEnv: cacheRoot,
	} {
		if value == "" {
			return localAIRealRunRequest{}, fmt.Errorf("%s is required for an enabled real selector", name)
		}
		if !filepath.IsAbs(value) {
			return localAIRealRunRequest{}, fmt.Errorf("%s must be an absolute path", name)
		}
	}
	if !strings.EqualFold(filepath.Ext(binaryPath), ".exe") {
		return localAIRealRunRequest{}, errors.New("integration binary must be a Windows executable")
	}
	identity, ok := localAIReadFileIdentity(binaryPath)
	if !ok {
		return localAIRealRunRequest{}, errors.New("integration binary is not a readable regular file")
	}
	commit, tree, ok := readLocalAIRealBuildIdentity(t)
	if !ok {
		return localAIRealRunRequest{}, errors.New("source commit/tree identity could not be recorded")
	}
	root := filepath.Join(cacheRoot, "runner")
	request := localAIRealRunRequest{
		Selector: spec.selector, Kind: spec.kind, RunID: localAIRealRunID,
		Root: root, ReportPath: reportPath, LedgerPath: ledgerPath,
		Limits:          localAIBudgetLimits{ModelCalls: 5, DownloadBytes: localAIRealDownloadLimit},
		ReservationKind: "modelCall", ReservationAmount: spec.reservationAmount,
		Build:   localAIRealBuildIdentity{PathIdentity: filepath.Base(binaryPath), Bytes: identity.Bytes, SHA256: identity.SHA256, Commit: commit, Tree: tree},
		Offline: spec.offline, CacheIdentitySHA256: sha256Hex([]byte("cache:" + filepath.Clean(cacheRoot))),
		ExpectedInputSHA256: spec.expectedInputSHA, ExpectedTranscript: spec.expectedTranscript,
		Unproven: []string{"real model/backend semantic fidelity is separately authorized by the real gate", "public clean installation and discovery"},
		Timeout:  90 * time.Minute,
	}
	request.Command = localAIRealSelectorCommand(binaryPath, spec, false)
	if spec.inputPath != "" {
		request.InputPath = spec.inputPath
	}
	if spec.kind == localAIJourneyTTSASR {
		followUp := localAIRealSelectorCommand(binaryPath, spec, true)
		request.FollowUp = &followUp
	}
	return request, nil
}

func localAIRealSelectorCommand(binaryPath string, spec localAIRealSelectorSpec, followUp bool) localAICommandSpec {
	if followUp {
		return localAICommandSpec{
			BinaryPath:  binaryPath,
			Arguments:   []string{"--json", "models", "invoke", "asr", "--operation", "ASR", "--input", "audio=@" + localAIRealInputToken, "--output", "transcript=" + localAIRealTranscriptToken, "--output", "segments=" + localAIRealSegmentsToken},
			Environment: []string{"HF_HUB_OFFLINE=0", "LOCALAI_OFFLINE=0"}, OutputName: localAIRealSegmentsFile,
		}
	}
	command := localAICommandSpec{
		BinaryPath:  binaryPath,
		Environment: []string{"HF_HUB_OFFLINE=0", "LOCALAI_OFFLINE=0"}, OutputName: "tts.wav",
		Arguments: []string{"--json", "models", "invoke", "tts", "--operation", "TTS", "--input", "text=" + localAIRealPrompt, "--output", "audio=" + localAIRealOutputToken},
	}
	if spec.offline {
		command.Environment = []string{"HF_HUB_OFFLINE=1", "LOCALAI_OFFLINE=1", "HTTP_PROXY=http://127.0.0.1:9", "HTTPS_PROXY=http://127.0.0.1:9"}
	}
	if spec.kind == localAIJourneyASR {
		command.Arguments = []string{"--json", "models", "invoke", "asr", "--operation", "ASR", "--input", "audio=@" + spec.inputPath, "--output", "transcript=" + localAIRealTranscriptToken, "--output", "segments=" + localAIRealSegmentsToken}
		command.OutputName = localAIRealSegmentsFile
	}
	return command
}

func localAIKnownFixturePath(t testing.TB) string {
	t.Helper()
	return filepath.Clean(filepath.Join(testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join("tests", "integration", "models", "testdata", knownASRFixtureFile)))))
}

func readLocalAIRealBuildIdentity(t testing.TB) (string, string, bool) {
	t.Helper()
	root := testutil.MustRepoRoot(t)
	read := func(args ...string) (string, bool) {
		command := exec.CommandContext(t.Context(), "git", args...)
		command.Dir = root
		output, err := command.Output()
		return strings.TrimSpace(string(output)), err == nil
	}
	commit, commitOK := read("rev-parse", "--verify", "HEAD")
	tree, treeOK := read("rev-parse", "--verify", "HEAD^{tree}")
	return commit, tree, commitOK && treeOK && commit != "" && tree != ""
}
