package root_composition_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

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

	missingHTTPList := support.GetJSON[factoryapi.ListModelsResponse](t, server.URL()+"/models")
	missingHTTPModel := findVideoReadinessModel(t, missingHTTPList.Results, "HTTP list missing projector")
	assertVideoReadinessOmitted(t, missingHTTPModel.Operations, missingHTTPModel.Modalities, missingHTTPModel.ManagedRuntime.Diagnostics)
	missingHTTPDetail := support.GetJSON[factoryapi.ModelDetail](t, server.URL()+"/models/llm")
	assertVideoReadinessOmitted(t, missingHTTPDetail.Operations, missingHTTPDetail.Modalities, nil)
	if missingHTTPDetail.Diagnostics["videoReadiness"] != "required projector artifact is missing or invalid" {
		t.Fatalf("HTTP detail video diagnostic = %#v, want stable missing-projector reason", missingHTTPDetail.Diagnostics)
	}

	process := functionalBuildProcess(t, edges)
	missingCLIList := executeVideoReadinessJSON[factoryapi.ListModelsResponse](
		t, process, home, cliDir, []string{"you", "--json", "models", "list"},
	)
	missingCLIModel := findVideoReadinessModel(t, missingCLIList.Results, "CLI list missing projector")
	assertVideoReadinessOmitted(t, missingCLIModel.Operations, missingCLIModel.Modalities, missingCLIModel.ManagedRuntime.Diagnostics)
	if missingCLIModel.ManagedRuntime.Diagnostics == nil ||
		(*missingCLIModel.ManagedRuntime.Diagnostics)["videoReadiness"] != "required projector artifact is missing or invalid" {
		t.Fatalf("CLI list video diagnostic = %#v, want stable missing-projector reason", missingCLIModel.ManagedRuntime.Diagnostics)
	}

	videoPath := filepath.Join(cliDir, "clip.mp4")
	if err := os.WriteFile(videoPath, []byte("controlled video"), 0o600); err != nil {
		t.Fatalf("write controlled video: %v", err)
	}
	videoInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=video should be rejected",
		"--input", "video=@" + videoPath,
	})
	videoInputs.Input.Env = videoReadinessEnvironment(home)
	videoInputs.Input.WorkingDirectory = cliDir
	videoInputs.Input.Stdout = bytes.NewBuffer(nil)
	videoInputs.Input.Stderr = bytes.NewBuffer(nil)
	videoErr := process.Execute(videoInputs.Input)
	var failure *models.InvocationFailure
	if !errors.As(videoErr, &failure) || failure.Class != models.InvocationFailureClassMediaCapability || failure.Slot != "video" {
		t.Fatalf("missing-projector video invocation error = %v, failure=%#v, want typed MEDIA_CAPABILITY/video", videoErr, failure)
	}
	if hostLauncher.Calls() != 0 || protocol.Calls() != 0 || rejectingNetwork.Calls() != 0 {
		t.Fatalf("rejected video effects = host %d, protocol %d, network %d; want 0/0/0", hostLauncher.Calls(), protocol.Calls(), rejectingNetwork.Calls())
	}

	textInputs := support.FakeInputs(t.Context(), []string{
		"you", "models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=text remains usable",
	})
	textInputs.Input.Env = videoReadinessEnvironment(home)
	textInputs.Input.WorkingDirectory = cliDir
	var textStdout, textStderr bytes.Buffer
	textInputs.Input.Stdout = &textStdout
	textInputs.Input.Stderr = &textStderr
	if err := process.Execute(textInputs.Input); err != nil {
		t.Fatalf("text-only invocation with missing projector: %v", err)
	}
	if textStdout.String() != protocol.response {
		t.Fatalf("text-only output = stdout %q stderr %q, want stdout %q", textStdout.String(), textStderr.String(), protocol.response)
	}
	if hostLauncher.Calls() != 1 || protocol.Calls() != 1 {
		t.Fatalf("text-only effects = host %d protocol %d, want one each", hostLauncher.Calls(), protocol.Calls())
	}

	writeVideoReadinessModelSource(t, modelSource, true)
	writeVideoReadinessManagedCache(t, home, true)
	restoredHTTPDetail := support.GetJSON[factoryapi.ModelDetail](t, server.URL()+"/models/llm")
	assertVideoReadinessPresent(t, restoredHTTPDetail.Operations, restoredHTTPDetail.Modalities)
	if restoredHTTPDetail.Diagnostics["videoReadiness"] != "verified-projector" {
		t.Fatalf("restored HTTP detail diagnostic = %#v, want verified-projector", restoredHTTPDetail.Diagnostics)
	}
	restoredCLIList := executeVideoReadinessJSON[factoryapi.ListModelsResponse](
		t, process, home, cliDir, []string{"you", "--json", "models", "list"},
	)
	restoredCLIModel := findVideoReadinessModel(t, restoredCLIList.Results, "CLI list restored projector")
	assertVideoReadinessPresent(t, restoredCLIModel.Operations, restoredCLIModel.Modalities)
	closeRootProcess(t, process, "close projector readiness root process")
}

func executeVideoReadinessJSON[T any](
	t *testing.T,
	process support.Process,
	home, factoryDir string,
	args []string,
) T {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = videoReadinessEnvironment(home)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
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
