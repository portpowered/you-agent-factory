package root_composition_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	home := t.TempDir()
	writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))

	dir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	promptPath := filepath.Join(dir, "timing.txt")
	videoPath := filepath.Join(dir, "clip.mp4")
	inputFiles := map[string][]byte{
		promptPath: []byte("What happens at 0:30?"),
		videoPath:  []byte("MP4-CLIP"),
	}
	fixture := newCoordinatedOmniProtocolFixture(timingResponse, followUpResponse)
	assetFiles := functionalModelAssetFileSystem{home: home}
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &joinedProtocolNegotiator{}
	compatibility := &joinedCompatibilityChecker{}
	edges := genericHTTPInvocationEdges(
		rejectingNetwork, assetFiles, hostLauncher, protocol, compatibility, modelServer,
	)
	edges.ModelCLIInputReadFile = func(path string) ([]byte, error) {
		data, ok := inputFiles[path]
		if !ok {
			return nil, fmt.Errorf("unexpected fixture input path %q", path)
		}
		return append([]byte(nil), data...), nil
	}
	edges.ModelResolveBackendArtifact = func(
		context.Context,
		serviceedges.ModelBackendArtifactSelectionRequest,
	) (serviceedges.ModelBackendArtifactSelection, error) {
		return selection, nil
	}
	edges.ModelInvocationProtocolClient = fixture
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)

	var videoStdout bytes.Buffer
	var videoStderr bytes.Buffer
	videoInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm",
		"--input", "prompt=What happens at 0:30?",
		"--input", "video=@" + videoPath,
	})
	videoInputs.Input.Env = functionalHomeEnvironment(home)
	videoInputs.Input.WorkingDirectory = dir
	videoInputs.Input.Stdout = &videoStdout
	videoInputs.Input.Stderr = &videoStderr
	if err := process.Execute(videoInputs.Input); err != nil {
		t.Fatalf("video timing invocation error = %v", err)
	}
	if videoStdout.String() != timingResponse {
		t.Fatalf("video timing stdout = %q, want %q", videoStdout.String(), timingResponse)
	}
	if videoStderr.Len() != 0 {
		t.Fatalf("video timing stderr = %q, want no diagnostics", videoStderr.String())
	}
	videoRequest := fixture.RequestAt(0)
	if videoRequest.Prompt != "What happens at 0:30?" || len(videoRequest.Inputs) != 2 {
		t.Fatalf("video protocol request = %#v, want prompt plus video", videoRequest)
	}
	if videoRequest.Inputs[0].Slot != "prompt" ||
		videoRequest.Inputs[0].MediaType != "text/plain" ||
		videoRequest.Inputs[0].Content != "What happens at 0:30?" {
		t.Fatalf("video prompt input = %#v, want timing prompt", videoRequest.Inputs[0])
	}
	if videoRequest.Inputs[1].Slot != "video" ||
		videoRequest.Inputs[1].Modality != models.ModalityVideo ||
		videoRequest.Inputs[1].MediaType != "video/mp4" ||
		videoRequest.Inputs[1].Content != "MP4-CLIP" {
		t.Fatalf("video media input = %#v, want ordered video media", videoRequest.Inputs[1])
	}

	unsupportedOutput := filepath.Join(dir, "unsupported.txt")
	unsupportedUsageOutput := filepath.Join(dir, "unsupported-usage.json")
	var unsupportedStdout bytes.Buffer
	var unsupportedStderr bytes.Buffer
	unsupportedInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm",
		"--input", "prompt=Reject this modality",
		"--input", "audio=@" + videoPath,
		"--output-map", "text=" + unsupportedOutput,
		"--output-map", "usage=" + unsupportedUsageOutput,
	})
	unsupportedInputs.Input.Env = functionalHomeEnvironment(home)
	unsupportedInputs.Input.WorkingDirectory = dir
	unsupportedInputs.Input.Stdout = &unsupportedStdout
	unsupportedInputs.Input.Stderr = &unsupportedStderr
	unsupportedErr := process.Execute(unsupportedInputs.Input)
	var unsupportedFailure *models.InvocationFailure
	if !errors.As(unsupportedErr, &unsupportedFailure) ||
		unsupportedFailure.Class != models.InvocationFailureClassMediaCapability ||
		unsupportedFailure.Slot != "audio" {
		t.Fatalf("unsupported audio/video error = %v, failure = %#v, want typed audio capability failure", unsupportedErr, unsupportedFailure)
	}
	if unsupportedStdout.Len() != 0 {
		t.Fatalf("unsupported modality stdout = %q, want empty", unsupportedStdout.String())
	}
	if _, err := os.Stat(unsupportedOutput); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported modality output stat error = %v, want no output artifact", err)
	}
	if _, err := os.Stat(unsupportedUsageOutput); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported modality usage stat error = %v, want no output artifact", err)
	}
	if fixture.Calls() != 1 {
		t.Fatalf("unsupported modality protocol calls = %d, want no generation call", fixture.Calls())
	}

	cancelContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancelledOutput := filepath.Join(dir, "cancelled.txt")
	cancelledUsageOutput := filepath.Join(dir, "cancelled-usage.json")
	var cancelledStdout bytes.Buffer
	var cancelledStderr bytes.Buffer
	cancelledInputs := support.FakeInputs(cancelContext, []string{
		"you", "models", "invoke", "llm",
		"--input", "prompt=Cancel while generating",
		"--input", "video=@" + videoPath,
		"--output-map", "text=" + cancelledOutput,
		"--output-map", "usage=" + cancelledUsageOutput,
	})
	cancelledInputs.Input.Env = functionalHomeEnvironment(home)
	cancelledInputs.Input.WorkingDirectory = dir
	cancelledInputs.Input.Stdout = &cancelledStdout
	cancelledInputs.Input.Stderr = &cancelledStderr
	cancelledDone := make(chan error, 1)
	go func() { cancelledDone <- process.Execute(cancelledInputs.Input) }()
	<-fixture.CancellationCallStarted()
	cancel()
	cancelledErr := <-cancelledDone
	if !errors.Is(cancelledErr, models.ErrInferenceCancelled) {
		t.Fatalf("cancelled invocation error = %v, want ErrInferenceCancelled", cancelledErr)
	}
	<-fixture.CancellationObserved()
	if cancelledStdout.Len() != 0 {
		t.Fatalf("cancelled invocation stdout = %q, want no partial output", cancelledStdout.String())
	}
	if _, err := os.Stat(cancelledOutput); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled invocation output stat error = %v, want no output artifact", err)
	}
	if _, err := os.Stat(cancelledUsageOutput); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled invocation usage stat error = %v, want no output artifact", err)
	}

	var followUpStdout bytes.Buffer
	var followUpStderr bytes.Buffer
	followUpInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm",
		"--input", "prompt=Follow up after cancellation",
	})
	followUpInputs.Input.Env = functionalHomeEnvironment(home)
	followUpInputs.Input.WorkingDirectory = dir
	followUpInputs.Input.Stdout = &followUpStdout
	followUpInputs.Input.Stderr = &followUpStderr
	if err := process.Execute(followUpInputs.Input); err != nil {
		t.Fatalf("follow-up invocation after cancellation error = %v", err)
	}
	if followUpStdout.String() != followUpResponse {
		t.Fatalf("follow-up stdout = %q, want %q", followUpStdout.String(), followUpResponse)
	}
	if followUpStderr.Len() != 0 {
		t.Fatalf("follow-up stderr = %q, want no diagnostics", followUpStderr.String())
	}
	if fixture.Calls() != 3 {
		t.Fatalf("protocol calls = %d, want video, cancelled, and follow-up generations only", fixture.Calls())
	}
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
