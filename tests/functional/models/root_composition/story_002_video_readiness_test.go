package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsProjectorAwareVideoReadinessThroughRootBuildProcess proves the
// public discovery and joined invocation edges use one durable projector fact:
// missing projector omits VIDEO in CLI/HTTP projections and rejects video
// before backend/host/protocol effects, while text remains usable and a later
// verified projector restores VIDEO without a stale projection.
func TestModelsProjectorAwareVideoReadinessThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	fixture := newProjectorReadinessFixture(t)
	assertUnavailableProjectorPublicSurfaces(t, fixture, "missing projector")
	videoPath := filepath.Join(fixture.cliDir, "clip.mp4")
	if err := os.WriteFile(videoPath, []byte("controlled video"), 0o600); err != nil {
		t.Fatalf("write controlled video: %v", err)
	}
	missingFailure := assertProjectorCLIVideoFailure(t, fixture, videoPath, "missing")
	assertHTTPProjectorVideoFailure(t, fixture, "missing", missingFailure)
	assertMissingProjectorAllowsText(t, fixture)
	writeVideoReadinessModelSource(t, fixture.modelSource, true)
	writeVideoReadinessManagedCache(t, fixture.home, true)
	assertVerifiedProjectorPublicSurfaces(t, fixture)
	corruptCachedVideoReadinessProjector(t, fixture.home)
	assertUnavailableProjectorPublicSurfaces(t, fixture, "digest-corrupt projector")
	corruptFailure := assertProjectorCLIVideoFailure(t, fixture, videoPath, "digest-corrupt")
	assertHTTPProjectorVideoFailure(t, fixture, "digest-corrupt", corruptFailure)
	writeVideoReadinessManagedCache(t, fixture.home, true)
	assertVerifiedProjectorPublicSurfaces(t, fixture)
}

type projectorReadinessFixture struct {
	home             string
	modelSource      string
	cliDir           string
	server           *support.FunctionalAPIServer
	rejectingNetwork *rejectingModelAssetHTTP
	hostLauncher     *recordingModelHostLauncher
	protocol         *omniTextProtocolFixture
}

func newProjectorReadinessFixture(t *testing.T) *projectorReadinessFixture {
	t.Helper()
	home := functionalTempDir(t)
	modelSource := filepath.Join(home, "llm-source")
	writeVideoReadinessModelSource(t, modelSource, false)
	writeVideoReadinessOperatorConfig(t, home, modelSource)
	writeVideoReadinessManagedCache(t, home, false)
	selection := genericLlamaBackendSelection()
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, []byte("localai-llamacpp/linux-amd64"))

	modelServer := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)
	rejectingNetwork := &rejectingModelAssetHTTP{}
	assetFiles := functionalModelAssetFileSystem{home: home}
	hostLauncher := &recordingModelHostLauncher{endpoint: modelServer.URL}
	protocol := &omniTextProtocolFixture{response: "text remains usable"}
	edges := genericHTTPInvocationEdges(
		rejectingNetwork, assetFiles, hostLauncher, &joinedProtocolNegotiator{}, nil,
		modelServer, nil, nil,
	)
	edges.ModelInvocationProtocolClient = protocol
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	cliDir := functionalTempDir(t)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       videoReadinessEnvironment(home),
		Edges:                     edges,
	})
	return &projectorReadinessFixture{
		home: home, modelSource: modelSource, cliDir: cliDir,
		server: server, rejectingNetwork: rejectingNetwork,
		hostLauncher: hostLauncher, protocol: protocol,
	}
}

func assertUnavailableProjectorPublicSurfaces(
	t *testing.T,
	fixture *projectorReadinessFixture,
	projectorState string,
) {
	t.Helper()
	missingHTTPList := support.GetJSON[factoryapi.ListModelsResponse](t, fixture.server.URL()+"/models")
	missingHTTPModel := findVideoReadinessModel(t, missingHTTPList.Results, "HTTP list with "+projectorState)
	assertVideoReadinessOmitted(t, missingHTTPModel.Operations, missingHTTPModel.Modalities, missingHTTPModel.ManagedRuntime.Diagnostics)
	missingHTTPDetail := support.GetJSON[factoryapi.ModelDetail](t, fixture.server.URL()+"/models/llm")
	assertVideoReadinessOmitted(t, missingHTTPDetail.Operations, missingHTTPDetail.Modalities, nil)
	if missingHTTPDetail.Diagnostics["videoReadiness"] != "required projector artifact is missing or invalid" {
		t.Fatalf("HTTP detail video diagnostic with %s = %#v, want stable unavailable-projector reason", projectorState, missingHTTPDetail.Diagnostics)
	}
	missingCLIList := executeVideoReadinessJSON[factoryapi.ListModelsResponse](
		t, fixture.server, fixture.home, fixture.cliDir, []string{"you", "--json", "models", "list"},
	)
	missingCLIModel := findVideoReadinessModel(t, missingCLIList.Results, "CLI list with "+projectorState)
	assertVideoReadinessOmitted(t, missingCLIModel.Operations, missingCLIModel.Modalities, missingCLIModel.ManagedRuntime.Diagnostics)
	if missingCLIModel.ManagedRuntime.Diagnostics == nil ||
		(*missingCLIModel.ManagedRuntime.Diagnostics)["videoReadiness"] != "required projector artifact is missing or invalid" {
		t.Fatalf("CLI list video diagnostic with %s = %#v, want stable unavailable-projector reason", projectorState, missingCLIModel.ManagedRuntime.Diagnostics)
	}
}

func assertProjectorCLIVideoFailure(
	t *testing.T,
	fixture *projectorReadinessFixture,
	videoPath string,
	projectorState string,
) *models.InvocationFailure {
	t.Helper()
	textOutputPath := filepath.Join(fixture.cliDir, "rejected-video-"+projectorState+"-text.txt")
	usageOutputPath := filepath.Join(fixture.cliDir, "rejected-video-"+projectorState+"-usage.json")
	videoInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=" + projectorState + " video should be rejected",
		"--input", "video=@" + videoPath,
		"--output-map", "text=" + textOutputPath,
		"--output-map", "usage=" + usageOutputPath,
	})
	videoInputs.Input.Env = videoReadinessEnvironment(fixture.home)
	videoInputs.Input.WorkingDirectory = fixture.cliDir
	var stdout bytes.Buffer
	videoInputs.Input.Stdout = &stdout
	videoInputs.Input.Stderr = bytes.NewBuffer(nil)
	before := [3]int{fixture.hostLauncher.Calls(), fixture.protocol.Calls(), fixture.rejectingNetwork.Calls()}
	videoErr := fixture.server.Execute(t, videoInputs.Input)
	var failure *models.InvocationFailure
	if !errors.As(videoErr, &failure) || failure.Class != models.InvocationFailureClassMediaCapability || failure.Slot != "video" {
		t.Fatalf("%s projector video invocation error = %v, failure=%#v, want typed MEDIA_CAPABILITY/video", projectorState, videoErr, failure)
	}
	if stdout.Len() != 0 {
		t.Fatalf("rejected %s video invocation stdout = %q, want no successful output", projectorState, stdout.String())
	}
	assertVideoReadinessOutputAbsent(t, textOutputPath)
	assertVideoReadinessOutputAbsent(t, usageOutputPath)
	after := [3]int{fixture.hostLauncher.Calls(), fixture.protocol.Calls(), fixture.rejectingNetwork.Calls()}
	if after != before {
		t.Fatalf("rejected %s video effects = host %d, protocol %d, network %d; before=%#v want unchanged", projectorState, after[0], after[1], after[2], before)
	}
	return failure
}

func assertHTTPProjectorVideoFailure(
	t *testing.T,
	fixture *projectorReadinessFixture,
	projectorState string,
	typedFailure *models.InvocationFailure,
) {
	t.Helper()
	prompt := "video should be rejected"
	mediaType := "video/mp4"
	videoContent := []byte("controlled video")
	operation := factoryapi.ModelOperationName(models.OperationOMNI)
	inputs := []factoryapi.ModelInvocationInput{
		{Name: "prompt", Modality: factoryapi.ModelInvocationContentTypeText, Content: &prompt},
		{
			Name: "video", Modality: factoryapi.ModelInvocationContentTypeVideo,
			MediaType: &mediaType, ContentBase64: &videoContent,
		},
	}
	requestBody, err := json.Marshal(factoryapi.GenericModelInvocationRequest{
		Scope:     "factory-session:projector-readiness-" + projectorState,
		Holder:    "functional-projector-readiness-http",
		Model:     factoryapi.ModelReference{NameOrUri: models.BuiltInModelNameLLM},
		Operation: &operation,
		Inputs:    &inputs,
	})
	if err != nil {
		t.Fatalf("marshal %s HTTP video request: %v", projectorState, err)
	}
	requestContext, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestContext, http.MethodPost, fixture.server.URL()+"/models/invocations", bytes.NewReader(requestBody),
	)
	if err != nil {
		t.Fatalf("build %s HTTP video request: %v", projectorState, err)
	}
	request.Header.Set("Content-Type", "application/json")
	before := [3]int{fixture.hostLauncher.Calls(), fixture.protocol.Calls(), fixture.rejectingNetwork.Calls()}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s projector HTTP video request: %v", projectorState, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s HTTP video response: %v", projectorState, err)
	}
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(responseBody, &failure); err != nil {
		t.Fatalf("decode %s HTTP video failure: %v; body=%s", projectorState, err, responseBody)
	}
	if response.StatusCode != http.StatusBadRequest || failure.Code != factoryapi.ErrorResponseCode("BAD_REQUEST") ||
		failure.Family != factoryapi.ErrorFamilyBadRequest || failure.Message != typedFailure.Error() {
		t.Fatalf("%s HTTP video failure = status %d, %#v; want typed media-capability BAD_REQUEST", projectorState, response.StatusCode, failure)
	}
	var responseFields map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &responseFields); err != nil {
		t.Fatalf("decode %s HTTP video response fields: %v", projectorState, err)
	}
	if _, hasOutputs := responseFields["outputs"]; hasOutputs {
		t.Fatalf("%s rejected HTTP video response unexpectedly contains successful outputs: %s", projectorState, responseBody)
	}
	after := [3]int{fixture.hostLauncher.Calls(), fixture.protocol.Calls(), fixture.rejectingNetwork.Calls()}
	if after != before {
		t.Fatalf("%s rejected HTTP video effects = host %d, protocol %d, network %d; before=%#v want unchanged", projectorState, after[0], after[1], after[2], before)
	}
}

func assertVideoReadinessOutputAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("rejected video output %q exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect rejected video output %q: %v", path, err)
	}
}

func assertMissingProjectorAllowsText(t *testing.T, fixture *projectorReadinessFixture) {
	t.Helper()
	textInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=text remains usable",
	})
	textInputs.Input.Env = videoReadinessEnvironment(fixture.home)
	textInputs.Input.WorkingDirectory = fixture.cliDir
	var textStdout, textStderr bytes.Buffer
	textInputs.Input.Stdout = &textStdout
	textInputs.Input.Stderr = &textStderr
	if err := fixture.server.Execute(t, textInputs.Input); err != nil {
		t.Fatalf("text-only invocation with missing projector: %v", err)
	}
	if textStdout.String() != fixture.protocol.response {
		t.Fatalf("text-only output = stdout %q stderr %q, want stdout %q", textStdout.String(), textStderr.String(), fixture.protocol.response)
	}
	if fixture.hostLauncher.Calls() != 1 || fixture.protocol.Calls() != 1 {
		t.Fatalf("text-only effects = host %d protocol %d, want one each", fixture.hostLauncher.Calls(), fixture.protocol.Calls())
	}
}

func assertVerifiedProjectorPublicSurfaces(t *testing.T, fixture *projectorReadinessFixture) {
	t.Helper()
	restoredHTTPDetail := support.GetJSON[factoryapi.ModelDetail](t, fixture.server.URL()+"/models/llm")
	assertVideoReadinessPresent(t, restoredHTTPDetail.Operations, restoredHTTPDetail.Modalities)
	if restoredHTTPDetail.Diagnostics["videoReadiness"] != "verified-projector" {
		t.Fatalf("restored HTTP detail diagnostic = %#v, want verified-projector", restoredHTTPDetail.Diagnostics)
	}
	restoredCLIList := executeVideoReadinessJSON[factoryapi.ListModelsResponse](
		t, fixture.server, fixture.home, fixture.cliDir, []string{"you", "--json", "models", "list"},
	)
	restoredCLIModel := findVideoReadinessModel(t, restoredCLIList.Results, "CLI list restored projector")
	assertVideoReadinessPresent(t, restoredCLIModel.Operations, restoredCLIModel.Modalities)
}

func executeVideoReadinessJSON[T any](
	t *testing.T,
	server *support.FunctionalAPIServer,
	home, factoryDir string,
	args []string,
) T {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = videoReadinessEnvironment(home)
	inputs.Input.WorkingDirectory = factoryDir
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v): %v\nstdout=%s\nstderr=%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	var result T
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("decode Process.Execute(%v): %v\nstdout=%s", args, err, inputs.Stdout())
	}
	return result
}

func videoReadinessEnvironment(home string) []string {
	environment := functionalHomeEnvironment(home)
	return append(environment, runcli.ModelCacheDirEnvironment+"="+filepath.Join(home, ".agent-factory", "models"))
}

func findVideoReadinessModel(
	t *testing.T,
	results []factoryapi.ModelSummary,
	surface string,
) factoryapi.ModelSummary {
	t.Helper()
	for _, result := range results {
		if result.Name == models.BuiltInModelNameLLM {
			return result
		}
	}
	t.Fatalf("%s did not include %q: %#v", surface, models.BuiltInModelNameLLM, results)
	return factoryapi.ModelSummary{}
}

func assertVideoReadinessOmitted(
	t *testing.T,
	operations []factoryapi.ModelInvocationOperation,
	modalities []factoryapi.ModelInvocationContentType,
	diagnostics *factoryapi.StringMap,
) {
	t.Helper()
	if modelOperationsHaveVideo(operations) || containsVideoModality(modalities) {
		t.Fatalf("effective video projection retained VIDEO: operations=%#v modalities=%#v", operations, modalities)
	}
	if diagnostics != nil && (*diagnostics)["videoReadiness"] != "required projector artifact is missing or invalid" {
		t.Fatalf("managed-runtime video diagnostic = %#v, want stable missing-projector reason", diagnostics)
	}
}

func assertVideoReadinessPresent(
	t *testing.T,
	operations []factoryapi.ModelInvocationOperation,
	modalities []factoryapi.ModelInvocationContentType,
) {
	t.Helper()
	if !modelOperationsHaveVideo(operations) || !containsVideoModality(modalities) {
		t.Fatalf("effective video projection did not restore VIDEO: operations=%#v modalities=%#v", operations, modalities)
	}
}

func modelOperationsHaveVideo(operations []factoryapi.ModelInvocationOperation) bool {
	for _, operation := range operations {
		if operation.Inputs == nil {
			continue
		}
		for _, slot := range *operation.Inputs {
			if slot.Name == "video" || (slot.Modality != nil && *slot.Modality == factoryapi.ModelInvocationContentTypeVideo) {
				return true
			}
		}
	}
	return false
}

func containsVideoModality(modalities []factoryapi.ModelInvocationContentType) bool {
	for _, modality := range modalities {
		if modality == factoryapi.ModelInvocationContentTypeVideo {
			return true
		}
	}
	return false
}

func writeVideoReadinessModelSource(t *testing.T, source string, includeProjector bool) {
	t.Helper()
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("create video readiness model source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "model.gguf"), []byte("controlled model"), 0o644); err != nil {
		t.Fatalf("write video readiness model: %v", err)
	}
	projector := filepath.Join(source, "mmproj-F16.gguf")
	if includeProjector {
		if err := os.WriteFile(projector, []byte("controlled projector"), 0o644); err != nil {
			t.Fatalf("write video readiness projector: %v", err)
		}
		return
	}
	if err := os.Remove(projector); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove video readiness projector: %v", err)
	}
}

func writeVideoReadinessOperatorConfig(t *testing.T, home, source string) {
	t.Helper()
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create video readiness operator config: %v", err)
	}
	data, err := json.Marshal(map[string]any{
		"models": map[string]any{models.BuiltInModelNameLLM: map[string]any{"source": source}},
	})
	if err != nil {
		t.Fatalf("marshal video readiness operator config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write video readiness operator config: %v", err)
	}
}

func writeVideoReadinessManagedCache(t *testing.T, home string, includeProjector bool) {
	t.Helper()
	const revision = "video-readiness-revision"
	root := filepath.Join(home, ".agent-factory", "models", "LLM")
	revisionPath := filepath.Join(root, revision)
	if err := os.RemoveAll(revisionPath); err != nil {
		t.Fatalf("reset video readiness managed cache: %v", err)
	}
	if err := os.MkdirAll(revisionPath, 0o755); err != nil {
		t.Fatalf("create video readiness managed cache: %v", err)
	}
	files := []struct {
		name string
		body []byte
	}{
		{name: "model.gguf", body: []byte("controlled model")},
	}
	if includeProjector {
		files = append(files, struct {
			name string
			body []byte
		}{name: "mmproj-F16.gguf", body: []byte("controlled projector")})
	}
	metadataFiles := make([]map[string]any, 0, len(files))
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(revisionPath, file.name), file.body, 0o644); err != nil {
			t.Fatalf("write video readiness managed artifact %q: %v", file.name, err)
		}
		digest := sha256.Sum256(file.body)
		metadataFiles = append(metadataFiles, map[string]any{
			"path": file.name, "bytes": len(file.body), "sha256": hex.EncodeToString(digest[:]),
		})
	}
	metadata, err := json.Marshal(map[string]any{
		"modelName": models.BuiltInModelNameLLM,
		"revision":  revision,
		"files":     metadataFiles,
	})
	if err != nil {
		t.Fatalf("marshal video readiness managed metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".managed-cache.json"), metadata, 0o644); err != nil {
		t.Fatalf("write video readiness managed metadata: %v", err)
	}
}

func corruptCachedVideoReadinessProjector(t *testing.T, home string) {
	t.Helper()
	path := filepath.Join(home, ".agent-factory", "models", "LLM", "video-readiness-revision", "mmproj-F16.gguf")
	if err := os.WriteFile(path, []byte("digest-corrupt projector"), 0o644); err != nil {
		t.Fatalf("corrupt cached video readiness projector: %v", err)
	}
}
