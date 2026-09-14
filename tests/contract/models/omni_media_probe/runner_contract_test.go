package omni_media_probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProbeRunnerPreflightRecordsReadyEvidence(t *testing.T) {
	input, inputPath, reportPath := validProbeInput(t, "ready")
	if err := WriteProbeInputAtomic(inputPath, input); err != nil {
		t.Fatalf("write probe input: %v", err)
	}

	report, err := NewRunner(nil).Run(context.Background(), inputPath, reportPath)
	if err != nil {
		t.Fatalf("run preparation preflight: %v", err)
	}
	if report.Status != "READY" || report.Failure != nil {
		t.Fatalf("preflight report = %#v, want READY without failure", report)
	}
	if report.Journeys[0].Name != probeJourneyImage || report.Journeys[1].Name != probeJourneyVideo || report.Journeys[0].Status != JourneyNotRun || report.Journeys[1].Status != JourneyNotRun {
		t.Fatalf("preflight journeys = %#v, want ordered NOT_RUN image/video", report.Journeys)
	}
	if report.Journeys[0].RequestInputs[1].SHA256 != wantImageSHA256 || report.Journeys[1].RequestInputs[1].SHA256 != wantVideoSHA256 {
		t.Fatalf("preflight fixture request identities = %#v, want exact promoted hashes", report.Journeys)
	}
	if report.Policy.Port == ProbeForbiddenPort || len(report.Policy.RootIdentities) < 6 || !uniqueStrings(report.Policy.RootIdentities) {
		t.Fatalf("preflight policy = %#v, want unique isolated roots and non-7437 port", report.Policy)
	}
	if report.Policy.MaxCompilerTestProcesses != ProbeMaxCompilerTestProcesses || report.Policy.MaxDiskBytes != ProbeMaxDiskBytes {
		t.Fatalf("preflight declared limits = compiler=%d disk=%d, want compiler=%d disk=%d", report.Policy.MaxCompilerTestProcesses, report.Policy.MaxDiskBytes, ProbeMaxCompilerTestProcesses, ProbeMaxDiskBytes)
	}
	if len(report.Journeys[0].Command) == 0 || len(report.Journeys[1].Command) == 0 || len(report.Journeys[0].SemanticRubric) == 0 || len(report.Journeys[1].SemanticRubric) == 0 {
		t.Fatalf("preflight did not record commands/rubrics: %#v", report.Journeys)
	}
	if _, err := ReadReport(reportPath); err != nil {
		t.Fatalf("strictly re-read atomic report: %v", err)
	}
	if _, err := os.Stat(input.ProbeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned probe root = %v, want removed after preflight", err)
	}
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read preflight report: %v", err)
	}
	for _, leaked := range []string{input.ProbeRoot, input.Build.Path, input.Dependencies.Model.Path, input.Dependencies.Projector.Path, input.Dependencies.Backend.Path} {
		if strings.Contains(string(body), leaked) {
			t.Fatalf("report leaked owned path %q: %s", leaked, body)
		}
	}
	if report.Cleanup != (CleanupEvidence{Checked: true}) || len(report.Processes) != 0 || len(report.Outputs) != 0 {
		t.Fatalf("preflight cleanup/process/output evidence = %#v/%#v/%#v", report.Cleanup, report.Processes, report.Outputs)
	}
}

func TestProbeRunnerRunsImageBeforeVideoWithControlledExecutor(t *testing.T) {
	input, _, reportPath := validProbeInput(t, "ordered")
	executor := &recordingExecutor{}
	report, err := NewRunner(executor).RunInput(context.Background(), input, reportPath)
	if err != nil {
		t.Fatalf("run controlled ordered probe: %v", err)
	}
	if report.Status != "PASS" || report.Journeys[0].Status != JourneyPass || report.Journeys[1].Status != JourneyPass {
		t.Fatalf("ordered report = %#v, want PASS/PASS/PASS", report)
	}
	executor.mu.Lock()
	requests := append([]ExecutionRequest(nil), executor.requests...)
	executor.mu.Unlock()
	if len(requests) != 2 || requests[0].Journey != probeJourneyImage || requests[1].Journey != probeJourneyVideo {
		t.Fatalf("controlled requests = %#v, want image then video", requests)
	}
	if requests[0].Port == ProbeForbiddenPort || requests[0].Port != requests[1].Port {
		t.Fatalf("controlled ports = %d/%d, want one non-7437 port", requests[0].Port, requests[1].Port)
	}
	if requests[0].Command[len(requests[0].Command)-1] != "image=@fixture:omni-image-v1" || requests[1].Command[len(requests[1].Command)-1] != "video=@fixture:omni-video-v1" {
		t.Fatalf("controlled commands = %#v, want redacted exact fixture selectors", requests)
	}
	executor.mu.Lock()
	rootsObserved := executor.rootsObserved
	executor.mu.Unlock()
	if !rootsObserved {
		t.Fatal("controlled executor did not observe fresh isolated roots")
	}
}

func TestProbeRunnerStopsVideoAfterImageFailureAndPublishesAtomicReport(t *testing.T) {
	input, _, reportPath := validProbeInput(t, "image-failure")
	executor := &recordingExecutor{failure: &ReportFailure{
		Owner: "controlled", Code: "MODEL_BACKEND_FAILURE", Expected: "image command succeeds", Observed: "controlled image failure", NextAction: "inspect the image gate",
	}}
	report, err := NewRunner(executor).RunInput(context.Background(), input, reportPath)
	if err != nil {
		t.Fatalf("run controlled image failure: %v", err)
	}
	if report.Status != "FAIL" || report.Failure == nil || report.Journeys[0].Status != JourneyFail || report.Journeys[1].Status != JourneyNotRun {
		t.Fatalf("image failure report = %#v, want atomic FAIL with video NOT_RUN", report)
	}
	executor.mu.Lock()
	requests := append([]ExecutionRequest(nil), executor.requests...)
	executor.mu.Unlock()
	if len(requests) != 1 || requests[0].Journey != probeJourneyImage {
		t.Fatalf("image failure requests = %#v, want image only", requests)
	}
	if got, readErr := ReadReport(reportPath); readErr != nil || got.Status != "FAIL" || got.Journeys[1].Status != JourneyNotRun {
		t.Fatalf("persisted image failure report = %#v, err=%v", got, readErr)
	}
	if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed-run owned root = %v, want removed", statErr)
	}
}

func TestProbeRunnerCancellationPublishesInconclusiveReportAfterCleanup(t *testing.T) {
	input, _, reportPath := validProbeInput(t, "cancelled")
	ctx, cancel := context.WithCancel(context.Background())
	executor := &cancellingExecutor{cancel: cancel}
	report, err := NewRunner(executor).RunInput(ctx, input, reportPath)
	if err != nil {
		t.Fatalf("run cancelled controlled probe: %v", err)
	}
	if report.Status != "INCONCLUSIVE" || report.Failure == nil || report.Failure.Code != string(CodeProbeCancelled) || report.Journeys[1].Status != JourneyNotRun {
		t.Fatalf("cancelled report = %#v, want inconclusive image-only evidence", report)
	}
	if _, err := ReadReport(reportPath); err != nil {
		t.Fatalf("read cancelled report: %v", err)
	}
	if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled-run owned root = %v, want removed", statErr)
	}
}

func TestProbeRunnerRefusesToPublishWhenExecutorReportsSurvivors(t *testing.T) {
	input, _, reportPath := validProbeInput(t, "survivor")
	executor := &survivorExecutor{}
	_, err := NewRunner(executor).RunInput(context.Background(), input, reportPath)
	var validation *ValidationError
	if err == nil || !errors.As(err, &validation) || validation.Code != CodeProbeCleanupFailure {
		t.Fatalf("survivor run error = %v, want fail-closed cleanup error", err)
	}
	if _, statErr := os.Stat(reportPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("survivor run published report: %v", statErr)
	}
	if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("survivor-run owned root = %v, want removed", statErr)
	}
}

func TestProbeRunnerDoesNotTrustCleanupWhenExecutorErrors(t *testing.T) {
	cases := []struct {
		name string
		set  func(*ExecutionObservation)
	}{
		{name: "process survivor", set: func(observation *ExecutionObservation) { observation.OwnedProcessSurvivors = 1 }},
		{name: "listener survivor", set: func(observation *ExecutionObservation) { observation.OwnedListenerSurvivors = 1 }},
		{name: "partial output", set: func(observation *ExecutionObservation) { observation.PartialOutputs = 1 }},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			input, _, reportPath := validProbeInput(t, testCase.name)
			executor := &errorWithCleanupEvidenceExecutor{set: testCase.set}
			_, err := NewRunner(executor).RunInput(context.Background(), input, reportPath)
			var validation *ValidationError
			if err == nil || !errors.As(err, &validation) || validation.Code != CodeProbeCleanupFailure {
				t.Fatalf("error with %s = %v, want fail-closed cleanup error", testCase.name, err)
			}
			if _, statErr := os.Stat(reportPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("error with %s published report: %v", testCase.name, statErr)
			}
			if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("error with %s left owned root: %v", testCase.name, statErr)
			}
		})
	}
}

func TestProbeRunnerReleasesReservedPortBeforeExecutorBind(t *testing.T) {
	input, _, reportPath := validProbeInput(t, "bind-port")
	report, err := NewRunner(&bindingExecutor{}).RunInput(context.Background(), input, reportPath)
	if err != nil {
		t.Fatalf("run port-binding executor: %v", err)
	}
	if report.Status != "PASS" {
		t.Fatalf("port-binding report status = %q, want PASS", report.Status)
	}
}

func TestProbeRunnerRejectsInvalidAdmissionBeforeOwnedEffects(t *testing.T) {
	cases := []struct {
		name   string
		code   ValidationCode
		mutate func(*ProbeInput)
	}{
		{name: "relative build path", code: CodeProbeInvalidIdentity, mutate: func(input *ProbeInput) { input.Build.Path = "you" }},
		{name: "build digest mismatch", code: CodeProbeIdentityMismatch, mutate: func(input *ProbeInput) { input.Build.SHA256 = strings.Repeat("0", sha256.Size*2) }},
		{name: "existing probe root", code: CodeProbeRootNotFresh, mutate: func(input *ProbeInput) { _ = os.Mkdir(input.ProbeRoot, 0o700) }},
		{name: "wrong journey order", code: CodeProbeInvalidInput, mutate: func(input *ProbeInput) { input.Journeys = []JourneyName{probeJourneyVideo, probeJourneyImage} }},
		{name: "two heavy processes", code: CodeProbeInvalidInput, mutate: func(input *ProbeInput) { input.Limits.MaxHeavyProcesses = 2 }},
		{name: "fixture byte mismatch", code: CodeProbeIdentityMismatch, mutate: func(input *ProbeInput) { input.FixtureManifest.Bytes++ }},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			input, _, reportPath := validProbeInput(t, testCase.name)
			testCase.mutate(&input)
			_, err := NewRunner(nil).RunInput(context.Background(), input, reportPath)
			if err == nil {
				t.Fatal("invalid probe input was accepted")
			}
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Code != testCase.code {
				t.Fatalf("admission error = %v, want typed code %q", err, testCase.code)
			}
			if testCase.name != "existing probe root" {
				if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("invalid admission created probe root: %v", statErr)
				}
			}
			if _, statErr := os.Stat(reportPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("invalid admission created report: %v", statErr)
			}
		})
	}
}

func TestProbeRunnerSerializesConcurrentHeavyAdmissions(t *testing.T) {
	first, _, firstReportPath := validProbeInput(t, "heavy-first")
	second, _, secondReportPath := validProbeInput(t, "heavy-second")
	firstExecutor := &blockingExecutor{started: make(chan struct{}), release: make(chan struct{})}
	firstResult := make(chan error, 1)
	go func() {
		_, err := NewRunner(firstExecutor).RunInput(context.Background(), first, firstReportPath)
		firstResult <- err
	}()
	select {
	case <-firstExecutor.started:
	case err := <-firstResult:
		t.Fatalf("first heavy admission exited before acquiring owner: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("first controlled image journey did not start")
	}

	_, err := NewRunner(nil).RunInput(context.Background(), second, secondReportPath)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Code != CodeProbeHeavyOwnerBusy {
		t.Fatalf("concurrent heavy admission error = %v, want %q", err, CodeProbeHeavyOwnerBusy)
	}
	if _, statErr := os.Stat(second.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("heavy-owner loser created roots: %v", statErr)
	}

	close(firstExecutor.release)
	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("first heavy admission: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first heavy admission did not release")
	}
}

func TestProbeInputAndReportRejectUnknownAndDuplicateJSONKeys(t *testing.T) {
	input, inputPath, reportPath := validProbeInput(t, "strict")
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode strict input: %v", err)
	}
	body = append(bytesReplaceOnce(body, []byte("\"limits\":"), []byte("\"unexpected\":true,\"limits\":")), '\n')
	if err := os.WriteFile(inputPath, body, 0o600); err != nil {
		t.Fatalf("write unknown-field input: %v", err)
	}
	if _, err := ReadProbeInput(inputPath); !hasValidationCode(err, CodeUnknownField) {
		t.Fatalf("unknown input field error = %v, want %q", err, CodeUnknownField)
	}

	if err := WriteProbeInputAtomic(inputPath, input); err != nil {
		t.Fatalf("restore strict input: %v", err)
	}
	body, err = os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read strict input: %v", err)
	}
	body = append(bytesReplaceOnce(body, []byte("\"runId\":"), []byte("\"runId\":\"strict\",\"runId\":")), '\n')
	if err := os.WriteFile(inputPath, body, 0o600); err != nil {
		t.Fatalf("write duplicate-key input: %v", err)
	}
	if _, err := ReadProbeInput(inputPath); !hasValidationCode(err, CodeDuplicateJSONKey) {
		t.Fatalf("duplicate input key error = %v, want %q", err, CodeDuplicateJSONKey)
	}

	if _, err := Preflight(context.Background(), inputPath, reportPath); err == nil {
		t.Fatal("strict invalid input unexpectedly reached preflight")
	}
}

type recordingExecutor struct {
	mu            sync.Mutex
	requests      []ExecutionRequest
	failure       *ReportFailure
	rootsObserved bool
}

func (executor *recordingExecutor) Execute(_ context.Context, request ExecutionRequest) (ExecutionObservation, error) {
	executor.mu.Lock()
	executor.requests = append(executor.requests, request)
	failure := executor.failure
	if _, err := os.Stat(request.Roots.Work); err == nil {
		executor.rootsObserved = true
	}
	executor.mu.Unlock()
	observation := ExecutionObservation{Process: ProcessEvidence{
		Identity: string(request.Journey) + "-controlled-process", Kind: "controlled", PID: 0, Owner: "controlled", Started: true, Exited: true,
	}}
	if failure != nil {
		observation.Process.ExitCode = 1
		observation.Failure = normalizeReportFailure(*failure, "controlled")
	}
	return observation, nil
}

type blockingExecutor struct {
	mu      sync.Mutex
	started chan struct{}
	release chan struct{}
	blocked bool
}

func (executor *blockingExecutor) Execute(ctx context.Context, request ExecutionRequest) (ExecutionObservation, error) {
	executor.mu.Lock()
	first := !executor.blocked
	if first {
		executor.blocked = true
	}
	executor.mu.Unlock()
	if first {
		close(executor.started)
		select {
		case <-executor.release:
		case <-ctx.Done():
			return ExecutionObservation{Process: ProcessEvidence{Identity: "cancelled", Kind: "controlled", Owner: "controlled", Started: true, Exited: true}, Cancelled: true}, nil
		}
	}
	return ExecutionObservation{Process: ProcessEvidence{Identity: string(request.Journey) + "-controlled-process", Kind: "controlled", Owner: "controlled", Started: true, Exited: true}}, nil
}

type cancellingExecutor struct {
	cancel context.CancelFunc
}

func (executor *cancellingExecutor) Execute(_ context.Context, request ExecutionRequest) (ExecutionObservation, error) {
	executor.cancel()
	return ExecutionObservation{Process: ProcessEvidence{Identity: string(request.Journey) + "-cancelled", Kind: "controlled", Owner: "controlled", Started: true, Exited: true}}, nil
}

type survivorExecutor struct{}

func (*survivorExecutor) Execute(_ context.Context, request ExecutionRequest) (ExecutionObservation, error) {
	return ExecutionObservation{
		Process:               ProcessEvidence{Identity: string(request.Journey) + "-survivor", Kind: "controlled", Owner: "controlled", Started: true, Exited: true},
		OwnedProcessSurvivors: 1,
	}, nil
}

type errorWithCleanupEvidenceExecutor struct {
	set func(*ExecutionObservation)
}

func (executor *errorWithCleanupEvidenceExecutor) Execute(_ context.Context, request ExecutionRequest) (ExecutionObservation, error) {
	observation := ExecutionObservation{Process: ProcessEvidence{Identity: string(request.Journey) + "-error", Kind: "controlled", Owner: "controlled", Started: true, Exited: true}}
	executor.set(&observation)
	return observation, errors.New("controlled executor failure")
}

type bindingExecutor struct{}

func (*bindingExecutor) Execute(_ context.Context, request ExecutionRequest) (ExecutionObservation, error) {
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", request.Port))
	if err != nil {
		return ExecutionObservation{}, err
	}
	if err := listener.Close(); err != nil {
		return ExecutionObservation{}, err
	}
	return ExecutionObservation{Process: ProcessEvidence{
		Identity: string(request.Journey) + "-bound", Kind: "controlled", Owner: "controlled", Started: true, Exited: true,
	}}, nil
}

func validProbeInput(t *testing.T, runID string) (ProbeInput, string, string) {
	t.Helper()
	root := t.TempDir()
	inputPath := filepath.Join(root, "probe-input.json")
	reportPath := filepath.Join(root, "probe-report.json")
	probeRoot := filepath.Join(root, "probe-root")
	buildName := "controlled-you"
	if runtime.GOOS == "windows" {
		buildName += ".exe"
	}
	buildPath := filepath.Join(root, buildName)
	writeExecutableTestFile(t, buildPath, []byte("controlled prebuilt you artifact\n"))
	modelPath := writeProbeDependency(t, root, "model.bin", "controlled model identity\n")
	projectorPath := writeProbeDependency(t, root, "projector.bin", "controlled projector identity\n")
	backendPath := writeProbeDependency(t, root, "backend.bin", "controlled backend identity\n")
	manifestPath := checkedInManifestPath(t)
	return ProbeInput{
		SchemaVersion: ProbeInputSchemaV1,
		RunID:         runID,
		Build:         ProbeBuildIdentity{Path: buildPath, Identity: "controlled-you@ff194dc", SHA256: fileSHA256(t, buildPath)},
		Dependencies: ProbeDependencies{
			Model:     writeFileIdentity(t, modelPath, "model@controlled"),
			Projector: writeFileIdentity(t, projectorPath, "projector@controlled"),
			Backend:   writeFileIdentity(t, backendPath, "backend@controlled"),
		},
		FixtureManifest: writeFileIdentity(t, manifestPath, "omni-fixture-manifest@v1"),
		ProbeRoot:       probeRoot,
		Journeys:        []JourneyName{probeJourneyImage, probeJourneyVideo},
		Limits: ProbeLimits{
			TimeoutSeconds: 5, MaxHeavyProcesses: ProbeMaxHeavyProcesses, MaxCompilerTestProcesses: ProbeMaxCompilerTestProcesses,
			MaxDiskBytes: ProbeMaxDiskBytes, MaxDownloadBytes: 0, MaxPaidUSD: 0, ForbiddenPort: ProbeForbiddenPort, NetworkPolicy: ProbeNetworkPolicy,
		},
	}, inputPath, reportPath
}

func writeProbeDependency(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write controlled dependency %s: %v", name, err)
	}
	return path
}

func writeExecutableTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o700); err != nil {
		t.Fatalf("write controlled build: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatalf("chmod controlled build: %v", err)
		}
	}
}

func writeFileIdentity(t *testing.T, path, identity string) ProbeFileIdentity {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat identity file %s: %v", path, err)
	}
	return ProbeFileIdentity{Path: path, Identity: identity, Bytes: info.Size(), SHA256: fileSHA256(t, path)}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read identity file %s: %v", path, err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func hasValidationCode(err error, want ValidationCode) bool {
	var validation *ValidationError
	return errors.As(err, &validation) && validation.Code == want
}

func bytesReplaceOnce(body, old, replacement []byte) []byte {
	if !strings.Contains(string(body), string(old)) {
		panic(fmt.Sprintf("test replacement target %q absent", old))
	}
	return []byte(strings.Replace(string(body), string(old), string(replacement), 1))
}
