//go:build windows

package models_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	requireFreshCache  bool
	requireCacheReuse  bool
	inputPath          string
	expectedInputSHA   string
	expectedTranscript string
	reservationAmount  int64
	mode               string
}

func TestLocalAIRealHarnessFirstUseTTS(t *testing.T) {
	runLocalAIRealSelector(t, localAIRealSelectorSpec{
		selector: "first-use-tts", kind: localAIJourneyTTS, mode: "pass", reservationAmount: 1, requireFreshCache: true,
	})
}

func TestLocalAIRealHarnessOfflineCacheReuseTTS(t *testing.T) {
	runLocalAIRealSelector(t, localAIRealSelectorSpec{
		selector: "offline-cache-reuse-tts", kind: localAIJourneyTTS, offline: true, mode: "pass", reservationAmount: 1, requireCacheReuse: true,
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
		stub          *localAIStubExecutor
	}{
		{name: "IC-02-offline-cache-reuse-tts", selector: "offline-cache-reuse-tts", kind: localAIJourneyTTS, mode: "offline-pass", offline: true, wantStatus: "PASS", wantArtifacts: 1},
		{name: "IC-03-known-fixture-asr", selector: "known-fixture-asr", kind: localAIJourneyASR, mode: "asr-pass", expected: knownASRFixtureTranscript, wantStatus: "PASS", wantArtifacts: 3},
		{name: "IC-04-tts-to-asr-integration", selector: "tts-to-asr-integration", kind: localAIJourneyTTSASR, mode: "integration", expected: localAIRealPrompt, wantStatus: "PASS", wantArtifacts: 4},
		{name: "IC-06-asr-invocation-failure", selector: "known-fixture-asr", kind: localAIJourneyASR, mode: "asr-failure", expected: knownASRFixtureTranscript, wantStatus: "FAIL", wantOwner: "product"},
		{name: "IC-07-asr-semantic-mismatch", selector: "known-fixture-asr", kind: localAIJourneyASR, mode: "asr-mismatch", expected: knownASRFixtureTranscript, wantStatus: "FAIL", wantOwner: "product"},
		{name: "IC-12-offline-cache-miss", selector: "offline-cache-reuse-tts", kind: localAIJourneyTTS, mode: "pass", offline: true, wantStatus: "FAIL", wantOwner: "harness", stub: &localAIStubExecutor{}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			request := localAIControlledSelectorRequest(t, t.TempDir(), testCase.selector, testCase.kind, testCase.mode, testCase.offline, testCase.expected)
			if testCase.name == "IC-12-offline-cache-miss" {
				request.RequireCacheReuse = true
			} else if testCase.offline {
				request.RequireCacheReuse = true
				writeLocalAIControlledCache(t, request.Root)
			}
			runner := mustLocalAIRealRunner(t)
			if testCase.stub != nil {
				runner.executor = testCase.stub
			}
			report, err := runner.Run(t.Context(), request)
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
			if testCase.name == "IC-02-offline-cache-reuse-tts" && (!report.Journeys[0].Cache.Reused || report.Journeys[0].Cache.BeforeEntries == 0 || report.Journeys[0].Cache.PartialArtifacts != 0) {
				t.Fatalf("offline cache evidence = %#v, want content-backed reuse", report.Journeys[0].Cache)
			}
			if testCase.name == "IC-12-offline-cache-miss" && testCase.stub.calls.Load() != 0 {
				t.Fatal("offline cache miss launched a command")
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
	request.NetworkPolicy = "controlled-no-network"
	request.Command.Environment[0] = localAIRealHelperModeEnv + "=" + mode
	if kind == localAIJourneyASR {
		request.InputPath = localAIKnownFixturePath(t)
		request.ExpectedInputSHA256 = knownASRFixtureSHA256
		request.Command.OutputName = localAIRealSegmentsFile
	}
	if offline {
		request.Command.Environment = append(request.Command.Environment,
			"HF_HUB_OFFLINE=1", "LOCALAI_OFFLINE=1", "HTTP_PROXY=http://127.0.0.1:9", "HTTPS_PROXY=http://127.0.0.1:9",
		)
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

func writeLocalAIControlledCache(t testing.TB, root string) {
	t.Helper()
	roots := localAIRealRootsFor(root)
	if err := prepareLocalAIRoots(roots); err != nil {
		t.Fatalf("prepare controlled cache roots: %v", err)
	}
	modelManifest := filepath.Join(roots.Cache, "controlled-model", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(modelManifest), 0o700); err != nil {
		t.Fatalf("create controlled model cache: %v", err)
	}
	if err := os.WriteFile(modelManifest, []byte(`{"model":"controlled","revision":"fixture"}`), 0o600); err != nil {
		t.Fatalf("write controlled model cache: %v", err)
	}
	hfIndex := filepath.Join(roots.HFCache, "controlled-index")
	if err := os.WriteFile(hfIndex, []byte("controlled-cache-index"), 0o600); err != nil {
		t.Fatalf("write controlled HF cache: %v", err)
	}
}

func TestLocalAIRealHarnessCacheIdentityUsesContents(t *testing.T) {
	t.Parallel()
	roots := localAIRealRootsFor(t.TempDir())
	if err := prepareLocalAIRoots(roots); err != nil {
		t.Fatal(err)
	}
	before, err := localAIRealCacheSnapshotForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	writeLocalAIControlledCache(t, roots.Root)
	after, err := localAIRealCacheSnapshotForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	if before.IdentitySHA256 == after.IdentitySHA256 || before.Entries == after.Entries {
		t.Fatalf("cache identity did not reflect contents: before=%#v after=%#v", before, after)
	}
}

func TestLocalAIRealHarnessFirstUseRejectsWarmCache(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	request := localAIControlledSelectorRequest(t, root, "first-use-tts", localAIJourneyTTS, "pass", false, "")
	request.RequireFreshCache = true
	writeLocalAIControlledCache(t, root)
	stub := &localAIStubExecutor{}
	runner := mustLocalAIRealRunner(t)
	runner.executor = stub
	report, err := runner.Run(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertLocalAIJourneyResult(t, request, report, "FAIL", "harness")
	if stub.calls.Load() != 0 {
		t.Fatal("warm first-use cache launched a command")
	}
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
	cleanCacheRoot := filepath.Clean(cacheRoot)
	root := filepath.Join(filepath.Dir(cleanCacheRoot), "."+filepath.Base(cleanCacheRoot)+"-"+spec.selector+"-runner")
	request := localAIRealRunRequest{
		Selector: spec.selector, Kind: spec.kind, RunID: localAIRealRunID,
		Root: root, ReportPath: reportPath, LedgerPath: ledgerPath,
		CacheRoot:       cacheRoot,
		Limits:          localAIBudgetLimits{ModelCalls: 5, DownloadBytes: localAIRealDownloadLimit},
		ReservationKind: "modelCall", ReservationAmount: spec.reservationAmount,
		Build:   localAIRealBuildIdentity{PathIdentity: filepath.Base(binaryPath), Bytes: identity.Bytes, SHA256: identity.SHA256, Commit: commit, Tree: tree},
		Offline: spec.offline, RequireFreshCache: spec.requireFreshCache, RequireCacheReuse: spec.requireCacheReuse,
		NetworkPolicy: func() string {
			if spec.offline {
				return "offline-no-network"
			}
			return "real-gate-network-allowed"
		}(),
		ChildProcessLimit: 2, SemanticRetries: 0,
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
func localAICommandFailure(observation localAICommandObservation) *localAIObservationFailure {
	return localAICommandFailureForJourney(observation, false)
}

func localAICommandFailureForJourney(observation localAICommandObservation, allowExpectedTranscript bool) *localAIObservationFailure {
	if observation.StdoutTruncated || observation.StderrTruncated {
		return &localAIObservationFailure{Owner: "harness", Assertion: "bounded command streams", Expected: "streams fit the redaction bound", Observed: "stream limit exceeded"}
	}
	if violation := localAIStreamViolationForJourney(observation.Stdout, observation.Stderr, allowExpectedTranscript); violation != "" {
		return &localAIObservationFailure{Owner: "product", Assertion: "redacted command streams", Expected: "no secret, prompt, address, or raw media", Observed: violation}
	}
	if !observation.Started {
		return &localAIObservationFailure{Owner: "environment", Assertion: "selected command start", Expected: "controlled command starts", Observed: "process did not start"}
	}
	if observation.TimedOut {
		return &localAIObservationFailure{Owner: "environment", Assertion: "selected command timeout", Expected: "command exits within declared timeout", Observed: "command timed out"}
	}
	if !observation.ProcessExited {
		return &localAIObservationFailure{Owner: "harness", Assertion: "process lifecycle", Expected: "process exits", Observed: "process did not exit"}
	}
	if !observation.ProcessTreeAttached || !observation.ProcessTreeClosed {
		return &localAIObservationFailure{Owner: "harness", Assertion: "process-tree cleanup", Expected: "owned process tree is attached and closed", Observed: "cleanup was not proven"}
	}
	if observation.ExitCode != 0 {
		return &localAIObservationFailure{Owner: "product", Assertion: "selected command exit status", Expected: "exitCode=0", Observed: fmt.Sprintf("exitCode=%d", observation.ExitCode)}
	}
	return nil
}

func observeLocalAI(
	request localAIRealRunRequest,
	roots localAIRealRoots,
	observations []localAICommandObservation,
) (localAIRealSemantic, []localAIRealArtifact, *localAIObservationFailure) {
	switch request.Kind {
	case localAIJourneyTTS:
		if len(observations) != 1 {
			return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "harness", Assertion: "selected journey command count", Expected: "one command", Observed: "unexpected command count"}
		}
		semantic, artifact, failure := observeLocalAITTS(request, roots, observations[0])
		return semantic, []localAIRealArtifact{artifact}, failure
	case localAIJourneyASR:
		if len(observations) != 1 {
			return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "harness", Assertion: "selected journey command count", Expected: "one command", Observed: "unexpected command count"}
		}
		return observeLocalAIASR(request, roots, observations[0])
	case localAIJourneyTTSASR:
		if len(observations) != 2 {
			return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "harness", Assertion: "selected journey command count", Expected: "TTS followed by ASR", Observed: "unexpected command count"}
		}
		ttsSemantic, ttsArtifact, failure := observeLocalAITTS(request, roots, observations[0])
		if failure != nil {
			return localAIRealSemantic{}, nil, failure
		}
		asrRequest := request
		asrRequest.Kind = localAIJourneyASR
		asrRequest.InputPath = filepath.Join(roots.Output, request.Command.OutputName)
		asrSemantic, asrArtifacts, failure := observeLocalAIASR(asrRequest, roots, observations[1])
		if failure != nil {
			return localAIRealSemantic{}, append([]localAIRealArtifact{ttsArtifact}, asrArtifacts...), failure
		}
		return localAIRealSemantic{
			Assertion: "TTS output is consumed by ASR with bounded semantic output",
			Expected:  "audio output and normalized ASR transcript with bounded segments",
			Observed:  fmt.Sprintf("tts=%s;asr=%s", ttsSemantic.Observed, asrSemantic.Observed),
			Passed:    true,
		}, append([]localAIRealArtifact{ttsArtifact}, asrArtifacts...), nil
	default:
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "harness", Assertion: "selected journey kind", Expected: "bounded TTS, ASR, or TTS-to-ASR selector", Observed: "unknown journey"}
	}
}

func observeLocalAITTS(request localAIRealRunRequest, roots localAIRealRoots, observation localAICommandObservation) (localAIRealSemantic, localAIRealArtifact, *localAIObservationFailure) {
	response, err := decodeLocalAIInvocationResponse(observation.Stdout)
	if err != nil {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "strict TTS response JSON", Expected: "one bounded audio output", Observed: "malformed response"}
	}
	if response.Failure != nil {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS invocation response", Expected: "successful audio output", Observed: "bounded invocation failure"}
	}
	if len(response.Outputs) != 1 || response.Outputs[0].Name != "audio" || response.Outputs[0].Modality != "AUDIO" {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output slot", Expected: "one AUDIO output named audio", Observed: "output shape mismatch"}
	}
	output := response.Outputs[0]
	mediaType := strings.ToLower(strings.TrimSpace(output.MediaType))
	if mediaType != "audio/wav" && mediaType != "audio/wave" {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output media type", Expected: "audio/wav", Observed: "unsupported media type"}
	}
	if strings.TrimSpace(output.Content) == "" {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output materialization", Expected: "non-empty content marker", Observed: "empty content"}
	}
	audio, err := localAIReadBoundedFile(filepath.Join(roots.Output, request.Command.OutputName), localAIRealMaxAudioBytes)
	if err != nil {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output artifact", Expected: "readable WAV artifact", Observed: "audio artifact missing"}
	}
	metadata, ok := localAIWAVMetadata(audio)
	if !ok {
		return localAIRealSemantic{}, localAIRealArtifact{}, &localAIObservationFailure{Owner: "product", Assertion: "TTS output decode", Expected: "bounded PCM WAV", Observed: "WAV invariant mismatch"}
	}
	artifact := localAIRealArtifact{Kind: "output-audio", Path: filepath.Base(request.Command.OutputName), MediaType: mediaType, Bytes: int64(len(audio)), SHA256: sha256Hex(audio)}
	semantic := localAIRealSemantic{
		Assertion: "decodable PCM WAV output",
		Expected:  "audio/wav with PCM16 frames",
		Observed:  fmt.Sprintf("bytes=%d;sampleRateHz=%d;channels=%d;bits=%d;durationMillis=%d", len(audio), metadata.sampleRate, metadata.channels, metadata.bits, metadata.durationMillis),
		Passed:    true,
	}
	return semantic, artifact, nil
}

type localAIASRSegment struct {
	ID    int64  `json:"id"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Text  string `json:"text"`
}

func observeLocalAIASR(
	request localAIRealRunRequest,
	roots localAIRealRoots,
	observation localAICommandObservation,
) (localAIRealSemantic, []localAIRealArtifact, *localAIObservationFailure) {
	response, err := decodeLocalAIInvocationResponse(observation.Stdout)
	if err != nil {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "strict ASR response JSON", Expected: "transcript and segments outputs", Observed: "malformed response"}
	}
	if response.Failure != nil {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR invocation response", Expected: "successful transcript and segments outputs", Observed: "bounded invocation failure"}
	}
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "transcript" || response.Outputs[1].Name != "segments" {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR output slots", Expected: "ordered transcript and segments outputs", Observed: "output shape mismatch"}
	}
	transcriptOutput, segmentsOutput := response.Outputs[0], response.Outputs[1]
	if strings.ToLower(strings.TrimSpace(transcriptOutput.MediaType)) != "text/plain" || strings.ToLower(strings.TrimSpace(segmentsOutput.MediaType)) != "application/json" {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR output media types", Expected: "text/plain and application/json", Observed: "unsupported media type"}
	}
	if strings.TrimSpace(transcriptOutput.Content) == "" || strings.TrimSpace(segmentsOutput.Content) == "" {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR output materialization", Expected: "non-empty transcript and segments content", Observed: "empty content"}
	}
	transcriptPath := filepath.Join(roots.Output, localAIRealTranscriptFile)
	segmentsPath := filepath.Join(roots.Output, localAIRealSegmentsFile)
	transcript, err := localAIReadBoundedFile(transcriptPath, localAIRealMaxStreamBytes)
	if err != nil || string(transcript) != transcriptOutput.Content {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR transcript materialization", Expected: "response content matches bounded transcript file", Observed: "transcript artifact mismatch"}
	}
	segmentsBody, err := localAIReadBoundedFile(segmentsPath, localAIRealMaxStreamBytes)
	if err != nil || string(segmentsBody) != segmentsOutput.Content {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR segments materialization", Expected: "response content matches bounded segments file", Observed: "segments artifact mismatch"}
	}
	var segments []localAIASRSegment
	if err := decodeLocalAIJSONStrict(segmentsBody, &segments); err != nil || len(segments) == 0 {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR segment structure", Expected: "non-empty strict JSON segment array", Observed: "invalid segments"}
	}
	inputPath := request.InputPath
	if inputPath == "" {
		inputPath = filepath.Join(roots.Output, request.Command.OutputName)
	}
	input, err := localAIReadBoundedFile(inputPath, localAIRealMaxAudioBytes)
	if err != nil {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR input artifact", Expected: "readable bounded WAV input", Observed: "input audio missing"}
	}
	metadata, ok := localAIWAVMetadata(input)
	if !ok {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR input audio", Expected: "bounded PCM WAV", Observed: "input audio is not a valid WAV"}
	}
	inputSHA := sha256Hex(input)
	if request.ExpectedInputSHA256 != "" && !strings.EqualFold(request.ExpectedInputSHA256, inputSHA) {
		return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR input identity", Expected: "pinned input SHA-256", Observed: "input digest mismatch"}
	}
	previousID, previousStart, previousEnd := int64(-1), int64(0), int64(0)
	var segmentText strings.Builder
	for index, segment := range segments {
		if segment.ID < 0 || segment.Start < 0 || segment.End <= segment.Start || segment.End > metadata.durationMillis || strings.TrimSpace(segment.Text) == "" || (index > 0 && (segment.ID <= previousID || segment.Start < previousStart || segment.End < previousEnd)) {
			return localAIRealSemantic{}, nil, &localAIObservationFailure{Owner: "product", Assertion: "ASR segment bounds", Expected: "finite monotonic segments bounded by input duration", Observed: "segment invariant mismatch"}
		}
		if segmentText.Len() > 0 {
			segmentText.WriteByte(' ')
		}
		segmentText.WriteString(segment.Text)
		previousID, previousStart, previousEnd = segment.ID, segment.Start, segment.End
	}
	wantTranscript := request.ExpectedTranscript
	if wantTranscript == "" {
		wantTranscript = "zero"
	}
	inputArtifact := localAIRealArtifact{Kind: "input-audio", Path: filepath.Base(inputPath), MediaType: "audio/wav", Bytes: int64(len(input)), SHA256: inputSHA}
	transcriptArtifact := localAIRealArtifact{Kind: "transcript", Path: localAIRealTranscriptFile, MediaType: "text/plain", Bytes: int64(len(transcript)), SHA256: sha256Hex(transcript)}
	segmentsArtifact := localAIRealArtifact{Kind: "segments", Path: localAIRealSegmentsFile, MediaType: "application/json", Bytes: int64(len(segmentsBody)), SHA256: sha256Hex(segmentsBody)}
	artifacts := []localAIRealArtifact{inputArtifact, transcriptArtifact, segmentsArtifact}
	if normalizeLocalAITranscript(string(transcript)) != normalizeLocalAITranscript(wantTranscript) || normalizeLocalAITranscript(segmentText.String()) != normalizeLocalAITranscript(wantTranscript) {
		return localAIRealSemantic{}, artifacts, &localAIObservationFailure{Owner: "product", Assertion: "ASR semantic transcript", Expected: "normalized transcript and segments agree with the selected input", Observed: "semantic mismatch"}
	}
	observedTranscript := strings.TrimSpace(string(transcript))
	if strings.Contains(strings.ToLower(observedTranscript), "local ai works on this machine") {
		observedTranscript = "sha256=" + sha256Hex(transcript)
	}
	semantic := localAIRealSemantic{
		Assertion: "normalized ASR transcript and bounded segments",
		Expected:  "selected transcript with monotonic duration-bounded segments",
		Observed:  fmt.Sprintf("transcript=%s;segments=%d;inputBytes=%d;inputSha256=%s", observedTranscript, len(segments), len(input), inputSHA),
		Passed:    true,
	}
	return semantic, artifacts, nil
}

func decodeLocalAIJSONStrict(body []byte, destination any) error {
	if len(body) == 0 || len(body) > localAIRealMaxStreamBytes {
		return errors.New("JSON value outside bound")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("JSON value contained trailing data")
	}
	return nil
}

func normalizeLocalAITranscript(value string) string {
	var normalized strings.Builder
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			normalized.WriteRune(character)
			continue
		}
		normalized.WriteByte(' ')
	}
	return strings.Join(strings.Fields(normalized.String()), " ")
}

type localAIInvocationResponse struct {
	Outputs []localAIInvocationOutput `json:"outputs"`
	Failure any                       `json:"failure"`
}

type localAIInvocationOutput struct {
	Name      string `json:"name"`
	Modality  string `json:"modality"`
	MediaType string `json:"mediaType"`
	Content   string `json:"content"`
}

func decodeLocalAIInvocationResponse(body []byte) (localAIInvocationResponse, error) {
	if len(body) == 0 || len(body) > localAIRealMaxStreamBytes {
		return localAIInvocationResponse{}, errors.New("response outside bound")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var response localAIInvocationResponse
	if err := decoder.Decode(&response); err != nil {
		return localAIInvocationResponse{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return localAIInvocationResponse{}, errors.New("response contained trailing JSON")
	}
	return response, nil
}

func prepareLocalAIRoots(roots localAIRealRoots) error {
	for _, path := range []string{roots.Root, roots.Work, roots.Profile, roots.Cache, roots.HFHome, roots.HFCache, roots.Temp, roots.Output, roots.Streams} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func localAIRealRootsFor(root string) localAIRealRoots {
	return localAIRealRoots{
		Root: root, Work: filepath.Join(root, "work"), Profile: filepath.Join(root, "profile"),
		Cache: filepath.Join(root, "cache"), HFHome: filepath.Join(root, "hf-home"),
		HFCache: filepath.Join(root, "hf-cache"), Temp: filepath.Join(root, "temp"),
		Output: filepath.Join(root, "output"), Streams: filepath.Join(root, "streams"),
	}
}

func localAIRealRootsForRequest(request localAIRealRunRequest) localAIRealRoots {
	roots := localAIRealRootsFor(request.Root)
	if request.CacheRoot != "" {
		roots.Cache = request.CacheRoot
		roots.HFHome = filepath.Join(request.Root, "hf-home")
		cleanCacheRoot := filepath.Clean(request.CacheRoot)
		roots.HFCache = filepath.Join(filepath.Dir(cleanCacheRoot), "."+filepath.Base(cleanCacheRoot)+"-hf-cache")
	}
	return roots
}

func localAIExpandArguments(arguments []string, roots localAIRealRoots) []string {
	values := map[string]string{
		localAIRealOutputToken:     filepath.Join(roots.Output, "tts.wav"),
		localAIRealInputToken:      filepath.Join(roots.Output, "tts.wav"),
		localAIRealTranscriptToken: filepath.Join(roots.Output, localAIRealTranscriptFile),
		localAIRealSegmentsToken:   filepath.Join(roots.Output, localAIRealSegmentsFile),
		localAIRealRootToken:       roots.Root,
		localAIRealWorkToken:       roots.Work,
	}
	expanded := make([]string, len(arguments))
	for index, argument := range arguments {
		expanded[index] = argument
		for token, value := range values {
			expanded[index] = strings.ReplaceAll(expanded[index], token, value)
		}
	}
	return expanded
}

func localAIProcessEnvironment(roots localAIRealRoots, overrides []string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && !localAIForbiddenEnvironmentKey(key) {
			values[key] = value
		}
	}
	values["TEMP"] = roots.Temp
	values["TMP"] = roots.Temp
	values["USERPROFILE"] = roots.Profile
	values["HOME"] = roots.Profile
	values["LOCALAPPDATA"] = roots.Profile
	values["APPDATA"] = roots.Profile
	values["XDG_CACHE_HOME"] = filepath.Join(roots.Profile, "cache")
	values["XDG_CONFIG_HOME"] = filepath.Join(roots.Profile, "config")
	values[localAIRealModelCacheEnv] = roots.Cache
	values["HF_HOME"] = roots.HFHome
	values["HUGGINGFACE_HUB_CACHE"] = roots.HFCache
	values["HF_HUB_DISABLE_TELEMETRY"] = "1"
	values["LOCALAI_REAL_OUTPUT_ROOT"] = roots.Output
	values["LOCALAI_REAL_INPUT_ROOT"] = roots.Output
	for _, entry := range overrides {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || localAIForbiddenEnvironmentOverrideKey(key) {
			continue
		}
		value = strings.ReplaceAll(value, localAIRealOutputToken, filepath.Join(roots.Output, "tts.wav"))
		value = strings.ReplaceAll(value, localAIRealInputToken, filepath.Join(roots.Output, "tts.wav"))
		value = strings.ReplaceAll(value, localAIRealTranscriptToken, filepath.Join(roots.Output, localAIRealTranscriptFile))
		value = strings.ReplaceAll(value, localAIRealSegmentsToken, filepath.Join(roots.Output, localAIRealSegmentsFile))
		value = strings.ReplaceAll(value, localAIRealRootToken, roots.Root)
		value = strings.ReplaceAll(value, localAIRealWorkToken, roots.Work)
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for index := range keys {
		for other := index + 1; other < len(keys); other++ {
			if strings.ToUpper(keys[other]) < strings.ToUpper(keys[index]) {
				keys[index], keys[other] = keys[other], keys[index]
			}
		}
	}
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}

func localAIForbiddenEnvironmentKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if strings.HasPrefix(upper, "INFINITE_YOU_") || strings.HasPrefix(upper, "LOCALAI_REAL_") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") {
		return true
	}
	switch upper {
	case "HF_ENDPOINT", "HUGGINGFACE_HUB_CACHE", "HF_HOME", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "HF_HUB_OFFLINE", "LOCALAI_OFFLINE", localAIRealModelCacheEnv:
		return true
	default:
		return false
	}
}

func localAIForbiddenEnvironmentOverrideKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "SECRET") {
		return true
	}
	switch upper {
	case "HF_ENDPOINT", "HUGGINGFACE_HUB_CACHE", "HF_HOME", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", localAIRealModelCacheEnv:
		return true
	default:
		return false
	}
}

type localAIFileIdentity struct {
	Bytes  int64
	SHA256 string
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
	if _, err := io.Copy(hasher, file); err != nil {
		return localAIFileIdentity{}, false
	}
	return localAIFileIdentity{Bytes: info.Size(), SHA256: hex.EncodeToString(hasher.Sum(nil))}, true
}

type localAIWAVDetails struct {
	durationMillis int64
	channels       uint16
	sampleRate     uint32
	bits           uint16
}

func localAIWAVMetadata(audio []byte) (localAIWAVDetails, bool) {
	return localAIWAVMetadataWithLimits(audio, localAIRealMaxAudioBytes, localAIRealMaxAudioDuration)
}

func localAIWAVMetadataWithLimits(audio []byte, maxBytes int64, maxDuration time.Duration) (localAIWAVDetails, bool) {
	if maxBytes <= 0 || maxDuration <= 0 || int64(len(audio)) > maxBytes || len(audio) < 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" || string(audio[12:16]) != "fmt " || string(audio[36:40]) != "data" {
		return localAIWAVDetails{}, false
	}
	dataSize := binary.LittleEndian.Uint32(audio[40:44])
	if uint64(dataSize)+44 != uint64(len(audio)) || binary.LittleEndian.Uint32(audio[16:20]) != 16 || binary.LittleEndian.Uint16(audio[20:22]) != 1 {
		return localAIWAVDetails{}, false
	}
	channels := binary.LittleEndian.Uint16(audio[22:24])
	sampleRate := binary.LittleEndian.Uint32(audio[24:28])
	blockAlign := binary.LittleEndian.Uint16(audio[32:34])
	bits := binary.LittleEndian.Uint16(audio[34:36])
	if channels == 0 || sampleRate == 0 || blockAlign == 0 || bits != 16 || blockAlign != channels*bits/8 || binary.LittleEndian.Uint32(audio[28:32]) != sampleRate*uint32(blockAlign) || dataSize == 0 || dataSize%uint32(blockAlign) != 0 {
		return localAIWAVDetails{}, false
	}
	durationMillis := int64(dataSize/uint32(blockAlign)) * 1000 / int64(sampleRate)
	duration := time.Duration(dataSize/uint32(blockAlign)) * time.Second / time.Duration(sampleRate)
	if duration <= 0 || duration > maxDuration {
		return localAIWAVDetails{}, false
	}
	return localAIWAVDetails{durationMillis: durationMillis, channels: channels, sampleRate: sampleRate, bits: bits}, true
}

func writeLocalAIControlledWAV(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("controlled output path is empty")
	}
	audio := localAIControlledWAVBytes()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, audio, 0o600)
}

func localAIControlledWAVBytes() []byte {
	const (
		sampleRate = 8000
		channels   = 1
		bits       = 16
		frames     = 80
	)
	dataBytes := frames * channels * bits / 8
	audio := make([]byte, 44+dataBytes)
	copy(audio[:4], "RIFF")
	binary.LittleEndian.PutUint32(audio[4:8], uint32(len(audio)-8))
	copy(audio[8:12], "WAVE")
	copy(audio[12:16], "fmt ")
	binary.LittleEndian.PutUint32(audio[16:20], 16)
	binary.LittleEndian.PutUint16(audio[20:22], 1)
	binary.LittleEndian.PutUint16(audio[22:24], channels)
	binary.LittleEndian.PutUint32(audio[24:28], sampleRate)
	binary.LittleEndian.PutUint32(audio[28:32], sampleRate*channels*bits/8)
	binary.LittleEndian.PutUint16(audio[32:34], channels*bits/8)
	binary.LittleEndian.PutUint16(audio[34:36], bits)
	copy(audio[36:40], "data")
	binary.LittleEndian.PutUint32(audio[40:44], uint32(dataBytes))
	return audio
}

func writeLocalAIControlledASR(t *testing.T, transcript string, endMillis int64) {
	t.Helper()
	root := os.Getenv("LOCALAI_REAL_OUTPUT_ROOT")
	if root == "" {
		os.Exit(21)
	}
	transcriptBody := []byte(transcript)
	segmentsBody, err := json.Marshal([]localAIASRSegment{{ID: 0, Start: 0, End: endMillis, Text: transcript}})
	if err != nil {
		os.Exit(21)
	}
	if err := os.WriteFile(filepath.Join(root, localAIRealTranscriptFile), transcriptBody, 0o600); err != nil {
		os.Exit(21)
	}
	if err := os.WriteFile(filepath.Join(root, localAIRealSegmentsFile), segmentsBody, 0o600); err != nil {
		os.Exit(21)
	}
	body, err := json.Marshal(localAIInvocationResponse{Outputs: []localAIInvocationOutput{
		{Name: "transcript", Modality: "TEXT", MediaType: "text/plain", Content: string(transcriptBody)},
		{Name: "segments", Modality: "JSON", MediaType: "application/json", Content: string(segmentsBody)},
	}})
	if err != nil {
		os.Exit(21)
	}
	_, _ = os.Stdout.Write(body)
}

func localAIControlledRequest(t testing.TB, root, mode string) localAIRealRunRequest {
	t.Helper()
	binaryPath, err := os.Executable()
	if err != nil {
		t.Fatalf("test executable: %v", err)
	}
	identity, ok := localAIReadFileIdentity(binaryPath)
	if !ok {
		t.Fatalf("read test executable identity")
	}
	return localAIRealRunRequest{
		Selector:          "controlled-tts",
		Kind:              localAIJourneyTTS,
		RunID:             "controlled-run",
		Root:              root,
		ReportPath:        filepath.Join(root, "evidence.json"),
		LedgerPath:        filepath.Join(root, "localai-budget.json"),
		Limits:            localAIBudgetLimits{ModelCalls: 1},
		ReservationKind:   "modelCall",
		ReservationAmount: 1,
		Command: localAICommandSpec{
			BinaryPath: binaryPath,
			Arguments:  []string{"-test.run=TestLocalAIRealHarnessControlledHelper", "--", mode},
			Environment: []string{
				localAIRealHelperModeEnv + "=" + mode,
				localAIRealHelperOutputEnv + "=" + localAIRealOutputToken,
			},
			OutputName: "tts.wav",
		},
		Build:               localAIRealBuildIdentity{PathIdentity: "controlled-test-binary", Bytes: identity.Bytes, SHA256: identity.SHA256, Commit: "controlled", Tree: "controlled"},
		CacheIdentitySHA256: sha256Hex([]byte("controlled-cache")),
		Unproven:            []string{"real LocalAI model/backend", "public installation and discovery"},
	}
}
