package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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

func TestModelsOmniUnsupportedVideoCapabilityFailsBeforeProtocolThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	const modelName = models.BuiltInModelNameEmbed
	environment := buildCoordinatedOmniEnvironmentWithOptions(t, &recordingModelHostLauncher{}, coordinatedOmniBuildOptions{
		configFactory: omniVideoCapabilityProbeFactoryConfig,
		modelName:     modelName,
		modelSource:   omniCapabilityProbeSource,
	}, "unused response")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", modelName, "--operation", models.OperationOMNI,
		"--input", "prompt=Reject unadvertised video", "--input", "video=" + "@" + environment.videoPath,
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := environment.process.Execute(inputs.Input)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMediaCapability || failure.Slot != "video" {
		t.Fatalf("unadvertised video error = %v, failure = %#v, want typed video capability failure", err, failure)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unadvertised video stdout = %q, want no successful output", stdout.String())
	}
	if environment.fixture.Calls() != 0 || environment.network.Calls() != 0 || environment.launcher.Calls() != 0 {
		t.Fatalf("unadvertised video effects = protocol %d network %d starts %d, want 0/0/0", environment.fixture.Calls(), environment.network.Calls(), environment.launcher.Calls())
	}
	closeRootProcess(t, environment.process, "close unsupported-video capability process")
}

func TestModelsOmniMappedOutputsPublishExactPairThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	const usage = `{"tokens":7,"media":"exact"}`
	environment := buildCoordinatedOmniEnvironmentWithOptions(t, &recordingModelHostLauncher{}, coordinatedOmniBuildOptions{
		configFactory: multiOutputModelFactoryConfig,
		usage:         usage,
	}, "exact mapped response")
	textPath := filepath.Join(environment.dir, "mapped-text.txt")
	usagePath := filepath.Join(environment.dir, "mapped-usage.json")
	stdout, stderr, err := executeOmniExactMappedInvocation(t, environment, textPath, usagePath)
	if err != nil {
		t.Fatalf("exact mapped OMNI invocation error = %v", err)
	}
	if stderr != "" {
		t.Fatalf("exact mapped OMNI stderr = %q, want no diagnostics", stderr)
	}
	assertFunctionalFile(t, textPath, "exact mapped response")
	assertFunctionalFile(t, usagePath, usage)
	assertExactOmniMediaRequest(t, environment.fixture.RequestAt(0), environment.media)
	if environment.fixture.Calls() != 1 || environment.network.Calls() != 0 {
		t.Fatalf("exact mapped effects = protocol %d network %d, want 1/0", environment.fixture.Calls(), environment.network.Calls())
	}
	var response struct {
		Outputs []struct {
			Name        string  `json:"name"`
			Content     *string `json:"content"`
			ContentType *string `json:"contentType"`
			MediaType   *string `json:"mediaType"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("decode exact mapped response metadata: %v\n%s", err, stdout)
	}
	if len(response.Outputs) != 2 || response.Outputs[0].Name != "text" || response.Outputs[1].Name != "usage" || response.Outputs[0].Content == nil || *response.Outputs[0].Content != "exact mapped response" || response.Outputs[1].Content == nil || *response.Outputs[1].Content != usage || response.Outputs[0].ContentType == nil || *response.Outputs[0].ContentType != "text/plain" || response.Outputs[0].MediaType == nil || *response.Outputs[0].MediaType != "text/plain" || response.Outputs[1].ContentType == nil || *response.Outputs[1].ContentType != "application/json" || response.Outputs[1].MediaType == nil || *response.Outputs[1].MediaType != "application/json" {
		t.Fatalf("exact mapped response metadata = %#v, want complete text/usage pair", response.Outputs)
	}
	closeRootProcess(t, environment.process, "close exact mapped-output process")
}

func TestModelsOmniMappedOutputsRemainAtomicOnSecondPublishFailure(t *testing.T) {
	t.Parallel()

	const usage = `{"tokens":8,"media":"rollback"}`
	effects := &genericCLIOutputFailureEffects{}
	environment := buildCoordinatedOmniEnvironmentWithOptions(t, &recordingModelHostLauncher{}, coordinatedOmniBuildOptions{
		configFactory: multiOutputModelFactoryConfig,
		usage:         usage,
		outputEffects: effects,
	}, "replacement response")
	textPath := filepath.Join(environment.dir, "rollback-text.txt")
	usagePath := filepath.Join(environment.dir, "rollback-usage.json")
	const oldText = "pre-existing text"
	const oldUsage = `{"tokens":1,"media":"old"}`
	if err := os.WriteFile(textPath, []byte(oldText), 0o644); err != nil {
		t.Fatalf("seed old text output: %v", err)
	}
	if err := os.WriteFile(usagePath, []byte(oldUsage), 0o644); err != nil {
		t.Fatalf("seed old usage output: %v", err)
	}
	effects.failedTarget = usagePath
	stdout, _, err := executeOmniExactMappedInvocation(t, environment, textPath, usagePath)
	if err == nil {
		t.Fatal("exact mapped output publication error = nil, want injected second-commit failure")
	}
	if stdout != "" {
		t.Fatalf("failed mapped output stdout = %q, want no partial response", stdout)
	}
	assertFunctionalFile(t, textPath, oldText)
	assertFunctionalFile(t, usagePath, oldUsage)
	if !effects.failed.Load() {
		t.Fatalf("mapped output failure effect was not reached; calls = %v", effects.Calls())
	}
	if environment.fixture.Calls() != 1 {
		t.Fatalf("failed mapped protocol calls = %d, want one generation before publication fault", environment.fixture.Calls())
	}
	closeRootProcess(t, environment.process, "close atomic mapped-output process")
}

func TestModelsOmniProtocolFailureReleasesStateForFreshIsolatedRoot(t *testing.T) {
	t.Parallel()

	first := buildCoordinatedOmniEnvironmentWithOptions(t, &recordingModelHostLauncher{}, coordinatedOmniBuildOptions{
		configFactory: multiOutputModelFactoryConfig,
		usage:         `{"tokens":9,"attempt":"failed"}`,
		protocolError: errors.New("controlled OMNI protocol failure"),
	}, "unused after protocol failure")
	failedTextPath := filepath.Join(first.dir, "failed-text.txt")
	failedUsagePath := filepath.Join(first.dir, "failed-usage.json")
	stdout, stderr, err := executeOmniExactMappedInvocation(t, first, failedTextPath, failedUsagePath)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassBackendProtocol {
		t.Fatalf("controlled protocol failure = %v, failure = %#v, want typed backend-protocol failure", err, failure)
	}
	if stdout != "" || !errors.Is(statAbsent(failedTextPath), os.ErrNotExist) || !errors.Is(statAbsent(failedUsagePath), os.ErrNotExist) {
		t.Fatalf("controlled protocol failure outputs = stdout %q text=%v usage=%v, want none", stdout, statAbsent(failedTextPath), statAbsent(failedUsagePath))
	}
	if first.fixture.Calls() != 1 || first.network.Calls() != 0 {
		t.Fatalf("controlled protocol failure effects = calls %d network %d, want 1/0", first.fixture.Calls(), first.network.Calls())
	}
	closeRootProcess(t, first.process, "close failed OMNI root process")

	second := buildCoordinatedOmniEnvironmentWithOptions(t, &recordingModelHostLauncher{}, coordinatedOmniBuildOptions{
		configFactory: multiOutputModelFactoryConfig,
		usage:         `{"tokens":10,"attempt":"fresh"}`,
	}, "fresh isolated response")
	textPath := filepath.Join(second.dir, "fresh-text.txt")
	usagePath := filepath.Join(second.dir, "fresh-usage.json")
	stdout, stderr, err = executeOmniExactMappedInvocation(t, second, textPath, usagePath)
	if err != nil {
		t.Fatalf("fresh isolated exact OMNI invocation error = %v", err)
	}
	if stdout == "" || stderr != "" {
		t.Fatalf("fresh isolated streams = stdout %q stderr %q, want response without diagnostics", stdout, stderr)
	}
	assertFunctionalFile(t, textPath, "fresh isolated response")
	assertFunctionalFile(t, usagePath, `{"tokens":10,"attempt":"fresh"}`)
	assertExactOmniMediaRequest(t, second.fixture.RequestAt(0), second.media)
	if second.fixture.Calls() != 1 || second.network.Calls() != 0 {
		t.Fatalf("fresh isolated effects = calls %d network %d, want 1/0", second.fixture.Calls(), second.network.Calls())
	}
	closeRootProcess(t, second.process, "close recovered OMNI root process")
}

func executeOmniExactMappedInvocation(
	t testing.TB,
	environment *coordinatedOmniEnvironment,
	textPath string,
	usagePath string,
) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", environment.modelName, "--operation", models.OperationOMNI,
		"--input", "prompt=@" + environment.media.promptPath,
		"--input", "image=@" + environment.media.image.ResolvedPath,
		"--input", "image=@" + environment.media.image.ResolvedPath,
		"--input", "video=@" + environment.media.video.ResolvedPath,
		"--output-map", "text=" + textPath, "--output-map", "usage=" + usagePath,
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := environment.process.Execute(inputs.Input)
	t.Logf("command: you models invoke %s --operation %s --input prompt=@%s --input image=@%s --input image=@%s --input video=@%s --output-map text=%s --output-map usage=%s", environment.modelName, models.OperationOMNI, environment.media.promptPath, environment.media.image.ResolvedPath, environment.media.image.ResolvedPath, environment.media.video.ResolvedPath, textPath, usagePath)
	t.Logf("protocol request: %s", omniRequestSummary(environment.fixture.RequestAt(0)))
	return stdout.String(), stderr.String(), err
}

func assertExactOmniMediaRequest(t testing.TB, request models.InvocationProtocolRequest, media omniExactMediaFixture) {
	t.Helper()
	if request.Operation != models.OperationOMNI || request.Prompt != string(media.promptBytes) || len(request.Inputs) != 4 {
		t.Fatalf("exact OMNI request identity = %s, want prompt/image/image/video", omniRequestSummary(request))
	}
	assertExactOmniProtocolInput(t, 0, request.Inputs[0], "prompt", models.ModalityText, "text/plain", media.promptBytes)
	assertExactOmniProtocolInput(t, 1, request.Inputs[1], "image", models.ModalityImage, media.image.MediaType, media.imageBytes)
	assertExactOmniProtocolInput(t, 2, request.Inputs[2], "image", models.ModalityImage, media.image.MediaType, media.imageBytes)
	assertExactOmniProtocolInput(t, 3, request.Inputs[3], "video", models.ModalityVideo, media.video.MediaType, media.videoBytes)
}

const omniCapabilityProbeSource = "hf://Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-f16.gguf@370f27d7550e0def9b39c1f16d3fbaa13aa67728"

func omniVideoCapabilityProbeFactoryConfig(endpoint string) map[string]any {
	const modelName = models.BuiltInModelNameEmbed
	config := multiOutputModelFactoryConfig(endpoint)
	resources := config["resources"].([]map[string]any)
	resources[0]["model"] = modelName
	workers := config["workers"].([]map[string]any)
	workers[0]["model"] = modelName
	workers[0]["operations"] = []map[string]any{{
		"name": "OMNI",
		"inputs": []map[string]any{
			{"name": "prompt", "contentTypes": []string{interfaces.ModelOperationContentTypeText}, "required": true},
			// Deliberately advertise the video-named slot as TEXT. The public
			// input preflight must reject the pinned MP4 before host/protocol
			// effects when this controlled model contract omits VIDEO support.
			{"name": "video", "contentTypes": []string{interfaces.ModelOperationContentTypeText}},
		},
		"outputs": []map[string]any{{
			"name": "text", "contentTypes": []string{interfaces.ModelOperationContentTypeText}, "required": true,
		}},
	}}
	return config
}

func writeGenericModelOverlay(t *testing.T, home, modelName, source string) {
	t.Helper()
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	config := map[string]any{
		"models": map[string]any{
			modelName: map[string]any{
				"source": source, "backend": "localai-llamacpp", "loadPolicy": "ON_DEMAND",
				"operations": []string{models.OperationOMNI},
			},
		},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal controlled OMNI model overlay: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create controlled OMNI model overlay directory: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write controlled OMNI model overlay: %v", err)
	}
}

type coordinatedOmniEnvironment struct {
	process   support.Process
	home      string
	dir       string
	modelName string
	videoPath string
	media     omniExactMediaFixture
	edges     serviceedges.Edges
	launcher  *recordingModelHostLauncher
	network   *rejectingModelAssetHTTP
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
	return buildCoordinatedOmniEnvironmentWithOptions(t, hostLauncher, coordinatedOmniBuildOptions{}, responses...)
}

type coordinatedOmniBuildOptions struct {
	configFactory func(string) map[string]any
	modelName     string
	modelSource   string
	usage         string
	protocolError error
	outputEffects *genericCLIOutputFailureEffects
}

func buildCoordinatedOmniEnvironmentWithOptions(
	t *testing.T,
	hostLauncher *recordingModelHostLauncher,
	options coordinatedOmniBuildOptions,
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
	if options.configFactory == nil {
		options.configFactory = builtInOnlyModelFactoryConfigWithEndpoint
	}
	if options.modelName == "" {
		options.modelName = models.BuiltInModelNameLLM
	}
	if options.modelSource == "" {
		writeGenericBuiltinModelCache(t, home, "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3")
	} else {
		writeGenericBuiltinModelCache(t, home, options.modelSource)
		writeGenericModelOverlay(t, home, options.modelName, options.modelSource)
	}
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))
	dir := functionalScaffoldFactory(t, options.configFactory(modelServer.URL))
	media := newExactOmniMediaFixture(t, dir)
	fixture := newCoordinatedOmniProtocolFixtureWithResponse(options.usage, options.protocolError, responses...)
	assetFiles := functionalModelAssetFileSystem{home: home}
	rejectingNetwork := &rejectingModelAssetHTTP{}
	hostLauncher.endpoint = modelServer.URL
	// Leave compatibility unset so this root-composition journey exercises the
	// same pinned artifact/platform default as the shipped CLI.
	edges := genericHTTPInvocationEdges(
		rejectingNetwork, assetFiles, hostLauncher,
		&joinedProtocolNegotiator{}, nil, modelServer, nil, nil,
	)
	edges.ModelCLIInputReadFile = media.inputReader.Read
	edges.ModelResolveBackendArtifact = func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
		return selection, nil
	}
	edges.ModelInvocationProtocolClient = fixture
	if options.outputEffects != nil {
		edges.ModelCLIOutputCreateTempFile = options.outputEffects.CreateTemp
		edges.ModelCLIOutputInspectPath = options.outputEffects.Inspect
		edges.ModelCLIOutputRemovePath = options.outputEffects.Remove
		edges.ModelCLIOutputRenamePath = options.outputEffects.Rename
	}
	process := functionalBuildProcess(t, edges)
	support.CleanupProcess(t, process)
	return &coordinatedOmniEnvironment{
		process: process, home: home, dir: dir, modelName: options.modelName,
		videoPath: media.video.ResolvedPath, media: media, edges: edges,
		launcher: hostLauncher, network: rejectingNetwork, fixture: fixture,
	}
}

func builtInOnlyModelFactoryConfigWithEndpoint(string) map[string]any {
	return builtInOnlyModelFactoryConfig()
}

func runOmniVideoInvocation(t *testing.T, environment *coordinatedOmniEnvironment, response string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", environment.modelName, "--input", "prompt=What happens at 0:30?",
		"--input", "video=@" + environment.videoPath,
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := environment.process.Execute(inputs.Input); err != nil {
		t.Fatalf("video timing invocation error = %v", err)
	}
	t.Logf("command: you models invoke %s --input prompt=\"What happens at 0:30?\" --input video=@%s", environment.modelName, environment.videoPath)
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	if stdout.String() != response || stderr.Len() != 0 {
		t.Fatalf("video output = stdout %q, stderr %q, want response without diagnostics", stdout.String(), stderr.String())
	}
	request := environment.fixture.RequestAt(0)
	t.Logf("protocol request: %s", omniRequestSummary(request))
	if request.Prompt != "What happens at 0:30?" || len(request.Inputs) != 2 || request.Inputs[0].Slot != "prompt" || request.Inputs[0].MediaType != "text/plain" || request.Inputs[0].Content != request.Prompt {
		t.Fatalf("video protocol request identity = %s, want prompt plus video input", omniRequestSummary(request))
	}
	assertExactOmniProtocolInput(t, 1, request.Inputs[1], "video", models.ModalityVideo, environment.media.video.MediaType, environment.media.videoBytes)
}

func runUnsupportedOmniInvocation(t *testing.T, environment *coordinatedOmniEnvironment) {
	t.Helper()
	output := filepath.Join(environment.dir, "unsupported.txt")
	usageOutput := filepath.Join(environment.dir, "unsupported-usage.json")
	var stdout, stderr bytes.Buffer
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", environment.modelName, "--input", "prompt=Reject this modality",
		"--input", "image=@" + environment.videoPath, "--output-map", "text=" + output,
		"--output-map", "usage=" + usageOutput,
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := environment.process.Execute(inputs.Input)
	var failure *models.InvocationFailure
	if !errors.As(err, &failure) || failure.Class != models.InvocationFailureClassMediaCapability || failure.Slot != "image" {
		t.Fatalf("wrong image/video error = %v, failure = %#v, want typed image capability failure", err, failure)
	}
	t.Logf("command: you models invoke %s --input prompt=\"Reject this modality\" --input image=@%s --output-map text=%s --output-map usage=%s", environment.modelName, environment.videoPath, output, usageOutput)
	t.Logf("error: %v", err)
	t.Logf("stdout:\n%s\n--- end stdout", stdout.String())
	t.Logf("stderr:\n%s\n--- end stderr", stderr.String())
	if stdout.Len() != 0 || !errors.Is(statAbsent(output), os.ErrNotExist) || !errors.Is(statAbsent(usageOutput), os.ErrNotExist) {
		t.Fatalf("wrong media left stdout or output artifacts: stdout=%q output=%v usage=%v", stdout.String(), statAbsent(output), statAbsent(usageOutput))
	}
	if environment.fixture.Calls() != 1 {
		t.Fatalf("wrong media protocol calls = %d, want no generation call", environment.fixture.Calls())
	}
	if environment.network.Calls() != 0 {
		t.Fatalf("wrong media asset network calls = %d, want no external bytes", environment.network.Calls())
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
		"you", "models", "invoke", environment.modelName, "--input", "prompt=Cancel while generating",
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
	t.Logf("command: you models invoke %s --input prompt=\"Cancel while generating\" --input video=@%s --output-map text=%s --output-map usage=%s", environment.modelName, environment.videoPath, output, usageOutput)
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
		"you", "models", "invoke", environment.modelName, "--input", "prompt=Follow up after cancellation",
	})
	inputs.Input.Env = functionalHomeEnvironment(environment.home)
	inputs.Input.WorkingDirectory = environment.dir
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	if err := environment.process.Execute(inputs.Input); err != nil {
		t.Fatalf("follow-up invocation after cancellation error = %v", err)
	}
	t.Logf("command: you models invoke %s --input prompt=\"Follow up after cancellation\"", environment.modelName)
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
	usage               string
	protocolError       error
	requests            []models.InvocationProtocolRequest
	calls               int
	cancellationStarted chan struct{}
	cancellationSeen    chan struct{}
	startOnce           sync.Once
	cancelOnce          sync.Once
}

func newCoordinatedOmniProtocolFixture(responses ...string) *coordinatedOmniProtocolFixture {
	return newCoordinatedOmniProtocolFixtureWithResponse("", nil, responses...)
}

func newCoordinatedOmniProtocolFixtureWithResponse(
	usage string,
	protocolError error,
	responses ...string,
) *coordinatedOmniProtocolFixture {
	return &coordinatedOmniProtocolFixture{
		responses: append([]string(nil), responses...), usage: usage, protocolError: protocolError,
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
	if fixture.protocolError != nil {
		return models.InvocationProtocolResponse{}, fixture.protocolError
	}
	responseIndex := call - 1
	responseText := ""
	if responseIndex >= 0 && responseIndex < len(fixture.responses) {
		responseText = fixture.responses[responseIndex]
	} else if len(fixture.responses) > 0 {
		responseIndex = len(fixture.responses) - 1
		responseText = fixture.responses[responseIndex]
	}
	return models.InvocationProtocolResponse{Text: responseText, Usage: fixture.usage}, nil
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
