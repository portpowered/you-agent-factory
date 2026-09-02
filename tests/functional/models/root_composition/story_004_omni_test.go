package root_composition_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestModelsOmniVideoCapabilityAndCancellationThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	const timingResponse = "At 0:30, the scene changes from shadow to light."
	const followUpResponse = "The follow-up invocation completed."
	environment := buildCoordinatedOmniEnvironment(t, timingResponse, followUpResponse)
	runOmniVideoInvocation(t, environment, timingResponse)
	runUnsupportedOmniInvocation(t, environment)
	runCancelledOmniInvocation(t, environment)
	runOmniFollowUpInvocation(t, environment, followUpResponse)
	if environment.fixture.Calls() != 3 {
		t.Fatalf("protocol calls = %d, want video, cancelled, and follow-up generations only", environment.fixture.Calls())
	}
}

func TestModelsOmniCancellationReleasesHostAcrossRootProcesses(t *testing.T) {
	t.Parallel()

	const timingResponse = "At 0:30, the scene changes from shadow to light."
	const followUpResponse = "The follow-up invocation completed."
	launcher := &recordingModelHostLauncher{
		endpoint:  "unused-by-fixture",
		exclusive: true,
	}
	first := buildCoordinatedOmniEnvironmentWithLauncher(t, launcher, timingResponse, followUpResponse)
	runOmniVideoInvocation(t, first, timingResponse)
	runCancelledOmniInvocation(t, first)
	closeRootProcess(t, first.process, "close cancelled Omni process")

	secondProcess := functionalBuildProcess(t, first.edges)
	support.CleanupProcess(t, secondProcess)
	second := *first
	second.process = secondProcess
	runOmniFollowUpInvocation(t, &second, followUpResponse)
}

type coordinatedOmniEnvironment struct {
	process   support.Process
	home      string
	dir       string
	videoPath string
	edges     serviceedges.Edges
	fixture   *coordinatedOmniProtocolFixture
}

func buildCoordinatedOmniEnvironment(t *testing.T, responses ...string) *coordinatedOmniEnvironment {
	return buildCoordinatedOmniEnvironmentWithLauncher(t, &recordingModelHostLauncher{}, responses...)
}

func buildCoordinatedOmniEnvironmentWithLauncher(
	t *testing.T,
	hostLauncher *recordingModelHostLauncher,
	responses ...string,
) *coordinatedOmniEnvironment {
	t.Helper()
	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)
	home := functionalTempDir(t)
	writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))
	dir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	promptPath := filepath.Join(dir, "timing.txt")
	videoPath := filepath.Join(dir, "clip.mp4")
	inputFiles := map[string][]byte{promptPath: []byte("What happens at 0:30?"), videoPath: []byte("MP4-CLIP")}
	fixture := newCoordinatedOmniProtocolFixture(responses...)
	assetFiles := functionalModelAssetFileSystem{home: home}
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher.endpoint = modelServer.URL
	// Leave compatibility unset so this root-composition journey exercises the
	// same pinned artifact/platform default as the shipped CLI.
	edges := genericHTTPInvocationEdges(
		rejectingNetwork, assetFiles, hostLauncher,
		&joinedProtocolNegotiator{}, nil, modelServer,
	)
	edges.ModelCLIInputReadFile = func(_ context.Context, path string, _ int64) ([]byte, error) {
		data, ok := inputFiles[path]
		if !ok {
			return nil, fmt.Errorf("unexpected fixture input path %q", path)
		}
		return append([]byte(nil), data...), nil
	}
	edges.ModelResolveBackendArtifact = func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
		return selection, nil
	}
	edges.ModelInvocationProtocolClient = fixture
	process := functionalBuildProcess(t, edges)
	support.CleanupProcess(t, process)
	return &coordinatedOmniEnvironment{
		process: process, home: home, dir: dir, videoPath: videoPath, edges: edges, fixture: fixture,
	}
}

func runOmniVideoInvocation(t *testing.T, environment *coordinatedOmniEnvironment, response string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--input", "prompt=What happens at 0:30?",
		"--input", "video=@" + environment.videoPath,
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := environment.process.Execute(inputs.Input); err != nil {
		t.Fatalf("video timing invocation error = %v", err)
	}
	t.Logf("command: you models invoke llm --input prompt=\"What happens at 0:30?\" --input video=@%s", environment.videoPath)
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	if stdout.String() != response || stderr.Len() != 0 {
		t.Fatalf("video output = stdout %q, stderr %q, want response without diagnostics", stdout.String(), stderr.String())
	}
	request := environment.fixture.RequestAt(0)
	t.Logf("protocol request: operation=%q prompt=%q inputs=%#v", request.Operation, request.Prompt, request.Inputs)
	if request.Prompt != "What happens at 0:30?" || len(request.Inputs) != 2 || request.Inputs[0].Slot != "prompt" || request.Inputs[0].MediaType != "text/plain" || request.Inputs[0].Content != request.Prompt {
		t.Fatalf("video protocol request = %#v, want prompt plus text input", request)
	}
	media := request.Inputs[1]
	if media.Slot != "video" || media.Modality != models.ModalityVideo || media.MediaType != "video/mp4" || media.Content != "MP4-CLIP" {
		t.Fatalf("video media input = %#v, want ordered video media", media)
	}
}

func runUnsupportedOmniInvocation(t *testing.T, environment *coordinatedOmniEnvironment) {
	t.Helper()
	output := filepath.Join(environment.dir, "unsupported.txt")
	usageOutput := filepath.Join(environment.dir, "unsupported-usage.json")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--input", "prompt=Reject this modality",
		"--input", "audio=@" + environment.videoPath, "--output-map", "text=" + output,
		"--output-map", "usage=" + usageOutput,
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := environment.process.Execute(inputs.Input)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMediaCapability || failure.Slot != "audio" {
		t.Fatalf("unsupported audio/video error = %v, failure = %#v, want typed audio capability failure", err, failure)
	}
	t.Logf("command: you models invoke llm --input prompt=\"Reject this modality\" --input audio=@%s --output-map text=%s --output-map usage=%s", environment.videoPath, output, usageOutput)
	t.Logf("error: %v", err)
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	if stdout.Len() != 0 || !errors.Is(statAbsent(output), os.ErrNotExist) || !errors.Is(statAbsent(usageOutput), os.ErrNotExist) {
		t.Fatalf("unsupported modality left stdout or output artifacts: stdout=%q output=%v usage=%v", stdout.String(), statAbsent(output), statAbsent(usageOutput))
	}
	if environment.fixture.Calls() != 1 {
		t.Fatalf("unsupported modality protocol calls = %d, want no generation call", environment.fixture.Calls())
	}
}

func runCancelledOmniInvocation(t *testing.T, environment *coordinatedOmniEnvironment) {
	t.Helper()
	cancelContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	output := filepath.Join(environment.dir, "cancelled.txt")
	usageOutput := filepath.Join(environment.dir, "cancelled-usage.json")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(cancelContext, []string{
		"you", "models", "invoke", "llm", "--input", "prompt=Cancel while generating",
		"--input", "video=@" + environment.videoPath, "--output-map", "text=" + output,
		"--output-map", "usage=" + usageOutput,
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	done := make(chan error, 1)
	go func() { done <- environment.process.Execute(inputs.Input) }()
	<-environment.fixture.CancellationCallStarted()
	cancel()
	err := <-done
	if !errors.Is(err, models.ErrInferenceCancelled) {
		t.Fatalf("cancelled invocation error = %v, want ErrInferenceCancelled", err)
	}
	<-environment.fixture.CancellationObserved()
	t.Logf("command: you models invoke llm --input prompt=\"Cancel while generating\" --input video=@%s --output-map text=%s --output-map usage=%s", environment.videoPath, output, usageOutput)
	t.Logf("error: %v", err)
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	t.Log("fixture observed cancellation; output files absent")
	if stdout.Len() != 0 || !errors.Is(statAbsent(output), os.ErrNotExist) || !errors.Is(statAbsent(usageOutput), os.ErrNotExist) {
		t.Fatalf("cancelled invocation left stdout or output artifacts: stdout=%q output=%v usage=%v", stdout.String(), statAbsent(output), statAbsent(usageOutput))
	}
}

func runOmniFollowUpInvocation(t *testing.T, environment *coordinatedOmniEnvironment, response string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--input", "prompt=Follow up after cancellation",
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := environment.process.Execute(inputs.Input); err != nil {
		t.Fatalf("follow-up invocation after cancellation error = %v", err)
	}
	t.Log("command: you models invoke llm --input prompt=\"Follow up after cancellation\"")
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	if stdout.String() != response || stderr.Len() != 0 {
		t.Fatalf("follow-up output = stdout %q, stderr %q, want response without diagnostics", stdout.String(), stderr.String())
	}
}

func statAbsent(path string) error {
	_, err := os.Stat(path)
	return err
}

type coordinatedOmniProtocolFixture struct {
	mu                  sync.Mutex
	responses           []string
	requests            []models.InvocationProtocolRequest
	calls               int
	cancellationStarted chan struct{}
	cancellationSeen    chan struct{}
	startOnce           sync.Once
	cancelOnce          sync.Once
}

func newCoordinatedOmniProtocolFixture(responses ...string) *coordinatedOmniProtocolFixture {
	return &coordinatedOmniProtocolFixture{
		responses:           append([]string(nil), responses...),
		cancellationStarted: make(chan struct{}),
		cancellationSeen:    make(chan struct{}),
	}
}

func (fixture *coordinatedOmniProtocolFixture) Predict(
	ctx context.Context,
	request models.InvocationProtocolRequest,
) (models.InvocationProtocolResponse, error) {
	fixture.mu.Lock()
	fixture.calls++
	call := fixture.calls
	request.Inputs = append([]models.InvocationProtocolInput(nil), request.Inputs...)
	fixture.requests = append(fixture.requests, request)
	fixture.mu.Unlock()

	if call == 2 {
		fixture.startOnce.Do(func() { close(fixture.cancellationStarted) })
		select {
		case <-ctx.Done():
			fixture.cancelOnce.Do(func() { close(fixture.cancellationSeen) })
			return models.InvocationProtocolResponse{}, ctx.Err()
		}
	}
	responseIndex := call - 1
	if responseIndex >= len(fixture.responses) {
		responseIndex = len(fixture.responses) - 1
	}
	return models.InvocationProtocolResponse{Text: fixture.responses[responseIndex]}, nil
}

func (fixture *coordinatedOmniProtocolFixture) RequestAt(index int) models.InvocationProtocolRequest {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	request := fixture.requests[index]
	request.Inputs = append([]models.InvocationProtocolInput(nil), request.Inputs...)
	return request
}

func (fixture *coordinatedOmniProtocolFixture) Calls() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.calls
}

func (fixture *coordinatedOmniProtocolFixture) CancellationCallStarted() <-chan struct{} {
	return fixture.cancellationStarted
}

func (fixture *coordinatedOmniProtocolFixture) CancellationObserved() <-chan struct{} {
	return fixture.cancellationSeen
}
