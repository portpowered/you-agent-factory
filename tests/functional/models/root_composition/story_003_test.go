package root_composition_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelservice "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	story003ModelName       = "OMNIVOICE_Q4_K_M"
	story003Repository      = "Serveurperso/OmniVoice-GGUF"
	story003Revision        = "functional-workflow-revision"
	story003BaseAsset       = "omnivoice-base-Q4_K_M.gguf"
	story003TokenizerAsset  = "omnivoice-tokenizer-Q4_K_M.gguf"
	story003WorkflowTimeout = 15 * time.Second
)

// TestModelsPublicPullWorkflowProvesTruthfulTerminalState drives one complete
// managed local-model workflow through root.BuildProcess and Process.Execute.
// The source server gates the mid-pull observation, so this test does not use
// sleeps or extend a client deadline to manufacture a transitional state.
func TestModelsPublicPullWorkflowProvesTruthfulTerminalState(t *testing.T) {
	t.Parallel()
	baseBody := []byte("functional-omnivoice-base-asset")
	tokenizerBody := []byte("functional-omnivoice-tokenizer-asset")
	source := newControlledModelSource(baseBody, tokenizerBody)
	sourceServer := functionalNewHTTPServer(t, source)
	t.Cleanup(sourceServer.Close)

	factoryDir := functionalScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig(sourceServer.URL))
	cacheDirectory := functionalTempDir(t)
	homeDirectory := functionalTempDir(t)
	environment := append(
		os.Environ(),
		runcli.ModelCacheDirEnvironment+"="+cacheDirectory,
		"HOME="+homeDirectory,
		"USERPROFILE="+homeDirectory,
	)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
		Edges: serviceedges.Edges{
			ModelAssetHTTPClient: sourceServer.Client(),
			ModelAssetEndpoints: modelservice.RuntimeAssetEndpoints{
				BaseURL:    sourceServer.URL,
				APIBaseURL: sourceServer.URL,
			},
		},
	})

	inspectProcess := functionalBuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, inspectProcess)

	beforeList := executeStory003List(t, inspectProcess, server.URL(), "before pull")
	assertStory003StatePair(t, beforeList.ManagedRuntime.ReadinessState, beforeList.ManagedRuntime.LifecycleState,
		factoryapi.ManagedRuntimeReadinessStateMISSING, factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED)
	before := executeStory003Inspect(t, inspectProcess, server.URL(), cacheDirectory, "before pull")
	assertStory003State(t, before, factoryapi.ManagedRuntimeReadinessStateMISSING, factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED)

	pullProcess := functionalBuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, pullProcess)
	pullInputs := support.FakeInputs(t.Context(), story003ModelsPullArgs(server.URL()))
	pullInputs.Input.WorkingDirectory = factoryDir
	pullStartedAt := time.Now().UTC()
	pullCommand := support.StartProcessCommand(t, pullProcess, pullInputs.Input)

	waitStory003Signal(t, source.manifestServed, "manifest request")
	waitStory003Signal(t, source.firstDownloadStarted, "first asset download")
	during := executeStory003Inspect(t, inspectProcess, server.URL(), cacheDirectory, "during pull")
	assertStory003State(t, during, factoryapi.ManagedRuntimeReadinessStateLOADING, factoryapi.ManagedRuntimeLifecycleStateINSTALLING)
	duringList := executeStory003List(t, inspectProcess, server.URL(), "during pull")
	assertStory003StatePair(t, duringList.ManagedRuntime.ReadinessState, duringList.ManagedRuntime.LifecycleState,
		factoryapi.ManagedRuntimeReadinessStateLOADING, factoryapi.ManagedRuntimeLifecycleStateINSTALLING)

	close(source.releaseFirstDownload)
	select {
	case <-pullCommand.Done():
	case <-time.After(story003WorkflowTimeout):
		t.Fatal("timed out waiting for synchronous model pull to finish")
	}
	pullFinishedAt := time.Now().UTC()
	t.Logf("models pull command=%q start=%s finish=%s exitCode=0", strings.Join(story003ModelsPullArgs(server.URL()), " "), pullStartedAt.Format(time.RFC3339Nano), pullFinishedAt.Format(time.RFC3339Nano))
	if err := pullCommand.Err(); err != nil {
		t.Fatalf("Process.Execute(models pull) error = %v\nstdout:\n%s\nstderr:\n%s", err, pullInputs.Stdout(), pullInputs.Stderr())
	}

	var pullResponse factoryapi.ModelPullResponse
	if err := json.Unmarshal([]byte(pullInputs.Stdout()), &pullResponse); err != nil {
		t.Fatalf("decode successful models pull output: %v\nstdout:\n%s", err, pullInputs.Stdout())
	}
	if pullResponse.Outcome != factoryapi.ModelPullOutcomePULLED ||
		pullResponse.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY ||
		pullResponse.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("successful pull response = %#v, want PULLED/INSTALLED_SUCCESSFULLY/READY", pullResponse)
	}
	assertStory003DownloadedFiles(t, pullResponse.DownloadedFiles, map[string][]byte{
		story003BaseAsset:      baseBody,
		story003TokenizerAsset: tokenizerBody,
	})

	afterList := assertStory003ListAfterPull(t, inspectProcess, server.URL(), cacheDirectory)

	after := executeStory003Inspect(t, inspectProcess, server.URL(), cacheDirectory, "after pull")
	if after.Detail.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("after-pull readiness = %s, want READY", after.Detail.ManagedRuntime.ReadinessState)
	}
	if after.Detail.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED &&
		after.Detail.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateLOADED {
		t.Fatalf("after-pull lifecycle = %s, want INSTALLED or LOADED", after.Detail.ManagedRuntime.LifecycleState)
	}
	assertStory003ReadyParity(t, inspectProcess, server.URL(), afterList, after)
	if before.state() == during.state() || during.state() == after.state() || before.state() == after.state() {
		t.Fatalf("inspect state pairs were not all distinct: before=%s during=%s after=%s", before.state(), during.state(), after.state())
	}
	if after.ArtifactBytes != int64(len(baseBody)+len(tokenizerBody)) {
		t.Fatalf("final on-disk artifact bytes = %d, want %d", after.ArtifactBytes, len(baseBody)+len(tokenizerBody))
	}

	t.Run("controlled source failure", testStory003ControlledSourceFailure)
}

func assertStory003ReadyParity(
	t *testing.T,
	process support.Process,
	serverURL string,
	listed factoryapi.ModelSummary,
	inspected story003InspectCapture,
) {
	t.Helper()
	assertStory003StatePair(t, listed.ManagedRuntime.ReadinessState, listed.ManagedRuntime.LifecycleState,
		factoryapi.ManagedRuntimeReadinessStateREADY, factoryapi.ManagedRuntimeLifecycleStateINSTALLED)
	if listed.ManagedRuntime.ReadinessState != inspected.Detail.ManagedRuntime.ReadinessState ||
		listed.ManagedRuntime.LifecycleState != inspected.Detail.ManagedRuntime.LifecycleState ||
		listed.ManagedRuntime.Revision == nil || inspected.Detail.ManagedRuntime.Revision == nil ||
		*listed.ManagedRuntime.Revision != *inspected.Detail.ManagedRuntime.Revision ||
		listed.ManagedRuntime.CacheBytes == nil || inspected.Detail.ManagedRuntime.CacheBytes == nil ||
		*listed.ManagedRuntime.CacheBytes != *inspected.Detail.ManagedRuntime.CacheBytes {
		t.Fatalf("list/inspect managed runtime diverged: list=%#v inspect=%#v", listed.ManagedRuntime, inspected.Detail.ManagedRuntime)
	}
	httpList := support.GetJSON[factoryapi.ListModelsResponse](t, serverURL+"/models")
	httpDetail := support.GetJSON[factoryapi.ModelDetail](t, serverURL+"/models/"+story003ModelName)
	listedModel := findStory003Model(t, httpList.Results, "HTTP list")
	assertStory003StatePair(t, listedModel.ManagedRuntime.ReadinessState, listedModel.ManagedRuntime.LifecycleState,
		factoryapi.ManagedRuntimeReadinessStateREADY, factoryapi.ManagedRuntimeLifecycleStateINSTALLED)
	if httpDetail.ManagedRuntime.ReadinessState != listedModel.ManagedRuntime.ReadinessState ||
		httpDetail.ManagedRuntime.LifecycleState != listedModel.ManagedRuntime.LifecycleState {
		t.Fatalf("HTTP list/detail managed runtime diverged: list=%#v detail=%#v", listedModel.ManagedRuntime, httpDetail.ManagedRuntime)
	}
	if httpDetail.Diagnostics["readinessState"] != string(httpDetail.ManagedRuntime.ReadinessState) ||
		httpDetail.Diagnostics["lifecycleState"] != string(httpDetail.ManagedRuntime.LifecycleState) {
		t.Fatalf("HTTP detail diagnostics diverged from managed runtime: diagnostics=%#v managedRuntime=%#v", httpDetail.Diagnostics, httpDetail.ManagedRuntime)
	}
	if inspected.Detail.Diagnostics["readinessState"] != string(inspected.Detail.ManagedRuntime.ReadinessState) ||
		inspected.Detail.Diagnostics["lifecycleState"] != string(inspected.Detail.ManagedRuntime.LifecycleState) {
		t.Fatalf("inspect diagnostics diverged from managed runtime: diagnostics=%#v managedRuntime=%#v", inspected.Detail.Diagnostics, inspected.Detail.ManagedRuntime)
	}
	assertStory003HumanOutput(t, process, "models list", story003ModelsHumanListArgs(serverURL), "READY", "INSTALLED")
	assertStory003HumanOutput(t, process, "models inspect", story003ModelsHumanInspectArgs(serverURL), "READY", "INSTALLED")
}

func assertStory003ListAfterPull(
	t *testing.T,
	process support.Process,
	serverURL string,
	cacheDirectory string,
) factoryapi.ModelSummary {
	t.Helper()
	listedModel := executeStory003List(t, process, serverURL, "after pull")
	if listedModel.ManagedRuntime.CacheBytes == nil {
		t.Fatalf("models list after pull = %#v, want one cached model with exact bytes", listedModel)
	}
	independentRevisionBytes := story003RegularFileBytes(
		t, filepath.Join(cacheDirectory, story003ModelName, story003Revision),
	)
	if got := *listedModel.ManagedRuntime.CacheBytes; got != independentRevisionBytes {
		t.Fatalf("models list cacheBytes = %d, independent revision sum = %d", got, independentRevisionBytes)
	}
	assertStory003StatePair(t, listedModel.ManagedRuntime.ReadinessState, listedModel.ManagedRuntime.LifecycleState,
		factoryapi.ManagedRuntimeReadinessStateREADY, factoryapi.ManagedRuntimeLifecycleStateINSTALLED)
	t.Logf(
		"models list after pull stdout=%s independentRevisionBytes=%d cacheBytes=%d revisionPath=%s",
		strings.TrimSpace(mustStory003ModelJSON(t, listedModel)),
		independentRevisionBytes,
		*listedModel.ManagedRuntime.CacheBytes,
		filepath.Join(cacheDirectory, story003ModelName, story003Revision),
	)
	return listedModel
}

// TestModelsPublicRemoveWorkflowProvesReclamationAndInUseRefusal proves removal reclaims unused assets and refuses assets still in use.
func TestModelsPublicRemoveWorkflowProvesReclamationAndInUseRefusal(t *testing.T) {
	t.Parallel()
	cacheDirectory := functionalTempDir(t)
	writeCachedOmniVoiceAssets(t, cacheDirectory)
	factoryDir := functionalScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	environment := append(
		functionalHomeEnvironment(cacheDirectory),
		runcli.ModelCacheDirEnvironment+"="+cacheDirectory,
	)
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true, Env: environment,
	})
	t.Cleanup(func() { server.Stop(t) })

	process := functionalBuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	revisionPath := filepath.Join(cacheDirectory, story003ModelName, "cached-revision")
	beforeBytes := story003RegularFileBytes(t, revisionPath)
	removeInputs := support.FakeInputs(t.Context(), story003ModelsRemoveArgs(server.URL()))
	removeInputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(removeInputs.Input); err != nil {
		t.Fatalf("Process.Execute(models remove) error = %v\nstdout=%s\nstderr=%s", err, removeInputs.Stdout(), removeInputs.Stderr())
	}
	var removed factoryapi.ModelRemoveResponse
	if err := json.Unmarshal([]byte(removeInputs.Stdout()), &removed); err != nil {
		t.Fatalf("decode models remove output: %v\nstdout=%s", err, removeInputs.Stdout())
	}
	if removed.ModelName != story003ModelName || removed.Revision != "cached-revision" ||
		removed.BytesRemoved != beforeBytes || removed.Outcome != factoryapi.REMOVED {
		t.Fatalf("models remove response = %#v, want selected revision and %d removed bytes", removed, beforeBytes)
	}
	if _, err := os.Stat(revisionPath); !os.IsNotExist(err) {
		t.Fatalf("removed revision stat error = %v, want not-exist", err)
	}
	afterBytes := story003RegularFileBytesIfPresent(t, revisionPath)
	if afterBytes != 0 {
		t.Fatalf("removed revision bytes = %d, want 0 after removal", afterBytes)
	}
	t.Logf(
		"models remove beforeRevisionBytes=%d afterRevisionBytes=%d remainingModelCacheBytes=%d response=%s cachePath=%s",
		beforeBytes,
		afterBytes,
		story003RegularFileBytes(t, filepath.Join(cacheDirectory, story003ModelName)),
		strings.TrimSpace(removeInputs.Stdout()),
		removed.CachePath,
	)

	missingInputs := support.FakeInputs(t.Context(), story003ModelsRemoveArgs(server.URL()))
	missingInputs.Input.WorkingDirectory = factoryDir
	missingErr := process.Execute(missingInputs.Input)
	if missingErr == nil || !errors.Is(missingErr, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("missing models remove = err %v stdout=%q stderr=%q, want ErrModelCacheNotFound", missingErr, missingInputs.Stdout(), missingInputs.Stderr())
	}

	t.Run("in-use response", testModelsPublicRemoveRefusesInUseCache)
}

func testModelsPublicRemoveRefusesInUseCache(t *testing.T) {
	server := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/models/"+story003ModelName {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
			Message: "managed model cache is in use",
			Family:  factoryapi.ErrorFamilyConflict,
			Code:    factoryapi.ErrorResponseCode("MODEL_CACHE_IN_USE"),
		})
	}))
	t.Cleanup(server.Close)

	process := functionalBuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), story003ModelsRemoveArgs(server.URL))
	err := process.Execute(inputs.Input)
	if err == nil || !errors.Is(err, modelscli.ErrModelCacheInUse) {
		t.Fatalf("in-use models remove = err %v stdout=%q stderr=%q, want ErrModelCacheInUse", err, inputs.Stdout(), inputs.Stderr())
	}
	t.Logf("models remove in-use classification error=%v stderr=%s", err, strings.TrimSpace(inputs.Stderr()))
}

func testStory003ControlledSourceFailure(t *testing.T) {
	failureSource := functionalNewHTTPServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/models/"+story003Repository {
			http.NotFound(writer, request)
			return
		}
		http.Error(writer, "controlled source failure", http.StatusServiceUnavailable)
	}))
	t.Cleanup(failureSource.Close)

	failureFactoryDir := functionalScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig(failureSource.URL))
	failureCacheDirectory := functionalTempDir(t)
	failureHomeDirectory := functionalTempDir(t)
	failureEnvironment := append(
		os.Environ(),
		runcli.ModelCacheDirEnvironment+"="+failureCacheDirectory,
		"HOME="+failureHomeDirectory,
		"USERPROFILE="+failureHomeDirectory,
	)
	failureServer := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                failureFactoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       failureEnvironment,
		Edges: serviceedges.Edges{
			ModelAssetHTTPClient: failureSource.Client(),
			ModelAssetEndpoints: modelservice.RuntimeAssetEndpoints{
				BaseURL:    failureSource.URL,
				APIBaseURL: failureSource.URL,
			},
		},
	})

	failureProcess := functionalBuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, failureProcess)
	failureInputs := support.FakeInputs(t.Context(), story003ModelsPullArgs(failureServer.URL()))
	failureInputs.Input.WorkingDirectory = failureFactoryDir
	cliErr := failureProcess.Execute(failureInputs.Input)
	if cliErr == nil {
		t.Fatalf("Process.Execute(failed models pull) error = nil, want non-zero exit\nstdout:\n%s", failureInputs.Stdout())
	}
	if !strings.Contains(cliErr.Error(), string(factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED)) {
		t.Fatalf("failed pull error = %v, want SOURCE_FETCH_FAILED classification", cliErr)
	}

	var failureResponse factoryapi.ModelPullResponse
	if decodeErr := json.Unmarshal([]byte(failureInputs.Stdout()), &failureResponse); decodeErr != nil {
		t.Fatalf("decode failed models pull response: %v\nstdout:\n%s\nstderr:\n%s", decodeErr, failureInputs.Stdout(), failureInputs.Stderr())
	}
	if failureResponse.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED ||
		failureResponse.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateFAILED ||
		failureResponse.Outcome != factoryapi.ModelPullOutcomeFAILED {
		t.Fatalf("failed pull response = %#v, want SOURCE_FETCH_FAILED/FAILED", failureResponse)
	}
	if failureResponse.Outcome == factoryapi.ModelPullOutcomePULLED ||
		failureResponse.Outcome == factoryapi.ModelPullOutcomeALREADYPRESENT {
		t.Fatalf("failed pull retained a success compatibility outcome: %#v", failureResponse)
	}
	if strings.TrimSpace(failureInputs.Stderr()) == "" {
		t.Fatal("failed pull stderr was empty, want the typed CLI diagnostic envelope")
	}
	var failureDiagnostic factoryapi.ErrorResponse
	if decodeErr := json.Unmarshal([]byte(failureInputs.Stderr()), &failureDiagnostic); decodeErr != nil {
		t.Fatalf("decode failed models pull diagnostic: %v\nstderr:\n%s", decodeErr, failureInputs.Stderr())
	}
	if failureDiagnostic.Code != factoryapi.ErrorResponseCode("CLI_MODEL_PULL_FAILED") ||
		failureDiagnostic.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("failed pull diagnostic = %#v, want CLI_MODEL_PULL_FAILED/BAD_REQUEST", failureDiagnostic)
	}
	for _, want := range []string{"pullOutcome=SOURCE_FETCH_FAILED", "readinessState=FAILED"} {
		if !strings.Contains(failureDiagnostic.Message, want) {
			t.Fatalf("failed pull diagnostic message = %q, want %q", failureDiagnostic.Message, want)
		}
	}
	if strings.Contains(failureInputs.Stderr(), "CLI_COMMAND_FAILED") ||
		strings.Contains(failureInputs.Stderr(), "controlled source failure") {
		t.Fatalf("failed pull diagnostic leaked generic fallback or source body: %q", failureInputs.Stderr())
	}

	assertStory003ControlledSourceHTTPFailure(t, failureServer.URL())
	t.Logf("controlled source failure CLI exitStatus=non-zero error=%q stdout=%s stderr=%s", cliErr, strings.TrimSpace(failureInputs.Stdout()), strings.TrimSpace(failureInputs.Stderr()))
}

func assertStory003ControlledSourceHTTPFailure(t *testing.T, serverURL string) {
	t.Helper()
	httpFailure, err := http.Post(
		serverURL+"/models/"+story003ModelName+"/pull",
		"application/json",
		strings.NewReader("{}"),
	)
	if err != nil {
		t.Fatalf("POST /models/%s/pull: %v", story003ModelName, err)
	}
	defer httpFailure.Body.Close()
	httpFailureBody, err := io.ReadAll(httpFailure.Body)
	if err != nil {
		t.Fatalf("read POST /models/%s/pull response: %v", story003ModelName, err)
	}
	if httpFailure.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("POST /models/%s/pull status = %d body = %s, want 422", story003ModelName, httpFailure.StatusCode, httpFailureBody)
	}
	var httpFailureResponse factoryapi.ModelPullResponse
	if err := json.Unmarshal(httpFailureBody, &httpFailureResponse); err != nil {
		t.Fatalf("decode POST /models/%s/pull response: %v\nbody=%s", story003ModelName, err, httpFailureBody)
	}
	if httpFailureResponse.Outcome != factoryapi.ModelPullOutcomeFAILED ||
		httpFailureResponse.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED ||
		httpFailureResponse.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateFAILED {
		t.Fatalf("POST /models/%s/pull response = %#v, want FAILED/SOURCE_FETCH_FAILED/FAILED", story003ModelName, httpFailureResponse)
	}
	if httpFailureResponse.Outcome == factoryapi.ModelPullOutcomePULLED ||
		httpFailureResponse.Outcome == factoryapi.ModelPullOutcomeALREADYPRESENT {
		t.Fatalf("POST /models/%s/pull retained a success compatibility outcome: %#v", story003ModelName, httpFailureResponse)
	}
	t.Logf("controlled source failure HTTP status=%d body=%s", httpFailure.StatusCode, strings.TrimSpace(string(httpFailureBody)))
}

type story003InspectCapture struct {
	Detail        factoryapi.ModelDetail
	ArtifactBytes int64
	CacheBytes    int64
	ObservedAt    time.Time
}

func (capture story003InspectCapture) state() string {
	return string(capture.Detail.ManagedRuntime.ReadinessState) + "/" + string(capture.Detail.ManagedRuntime.LifecycleState)
}

func executeStory003Inspect(
	t *testing.T,
	process support.Process,
	serverURL string,
	cacheDirectory string,
	phase string,
) story003InspectCapture {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), story003ModelsInspectArgs(serverURL))
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models inspect %s) error = %v\nstdout:\n%s\nstderr:\n%s", phase, err, inputs.Stdout(), inputs.Stderr())
	}
	var detail factoryapi.ModelDetail
	if err := json.Unmarshal([]byte(inputs.Stdout()), &detail); err != nil {
		t.Fatalf("decode models inspect %s output: %v\nstdout:\n%s", phase, err, inputs.Stdout())
	}
	capture := story003InspectCapture{
		Detail:        detail,
		ArtifactBytes: story003ArtifactBytes(t, cacheDirectory),
		CacheBytes:    story003RegularFileBytes(t, cacheDirectory),
		ObservedAt:    time.Now().UTC(),
	}
	t.Logf(
		"inspect %s timestamp=%s exitCode=0 state=%s artifactBytes=%d cacheBytes=%d stdout=%s",
		phase,
		capture.ObservedAt.Format(time.RFC3339Nano),
		capture.state(),
		capture.ArtifactBytes,
		capture.CacheBytes,
		strings.TrimSpace(inputs.Stdout()),
	)
	return capture
}

func executeStory003List(t *testing.T, process support.Process, serverURL, phase string) factoryapi.ModelSummary {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), story003ModelsListArgs(serverURL))
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(models list %s) error = %v\nstdout=%s\nstderr=%s", phase, err, inputs.Stdout(), inputs.Stderr())
	}
	var listed factoryapi.ListModelsResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &listed); err != nil {
		t.Fatalf("decode models list %s output: %v\nstdout=%s", phase, err, inputs.Stdout())
	}
	t.Logf("models list %s stdout=%s", phase, strings.TrimSpace(inputs.Stdout()))
	return findStory003Model(t, listed.Results, "models list "+phase)
}

func findStory003Model(t *testing.T, results []factoryapi.ModelSummary, surface string) factoryapi.ModelSummary {
	t.Helper()
	for _, result := range results {
		if result.Name == story003ModelName {
			return result
		}
	}
	t.Fatalf("%s did not include declared model %q; results=%#v", surface, story003ModelName, results)
	return factoryapi.ModelSummary{}
}

func assertStory003StatePair(
	t *testing.T,
	readiness factoryapi.ManagedRuntimeReadinessState,
	lifecycle factoryapi.ManagedRuntimeLifecycleState,
	wantReadiness factoryapi.ManagedRuntimeReadinessState,
	wantLifecycle factoryapi.ManagedRuntimeLifecycleState,
) {
	t.Helper()
	if readiness != wantReadiness || lifecycle != wantLifecycle {
		t.Fatalf("managed runtime state = %s/%s, want %s/%s", readiness, lifecycle, wantReadiness, wantLifecycle)
	}
}

func mustStory003ModelJSON(t *testing.T, model factoryapi.ModelSummary) string {
	t.Helper()
	body, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal models list model: %v", err)
	}
	return string(body)
}

func assertStory003HumanOutput(
	t *testing.T,
	process support.Process,
	phase string,
	args []string,
	wants ...string,
) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%s) error = %v\nstdout=%s\nstderr=%s", phase, err, inputs.Stdout(), inputs.Stderr())
	}
	for _, want := range wants {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("%s output missing %q:\n%s", phase, want, inputs.Stdout())
		}
	}
	t.Logf("%s human output=%s", phase, strings.TrimSpace(inputs.Stdout()))
}

func assertStory003State(
	t *testing.T,
	capture story003InspectCapture,
	wantReadiness factoryapi.ManagedRuntimeReadinessState,
	wantLifecycle factoryapi.ManagedRuntimeLifecycleState,
) {
	t.Helper()
	if capture.Detail.ManagedRuntime.ReadinessState != wantReadiness ||
		capture.Detail.ManagedRuntime.LifecycleState != wantLifecycle {
		t.Fatalf("inspect state = %s, want %s/%s", capture.state(), wantReadiness, wantLifecycle)
	}
}

func assertStory003DownloadedFiles(t *testing.T, files []factoryapi.ModelPullDownloadedFile, want map[string][]byte) {
	t.Helper()
	if len(files) != len(want) {
		t.Fatalf("downloaded files = %#v, want %d files", files, len(want))
	}
	for _, file := range files {
		body, ok := want[file.Path]
		if !ok {
			t.Fatalf("downloaded unexpected file %q", file.Path)
		}
		if file.Bytes != int64(len(body)) {
			t.Fatalf("downloaded file %q bytes = %d, want %d", file.Path, file.Bytes, len(body))
		}
		checksum := sha256.Sum256(body)
		if file.Sha256 == nil || *file.Sha256 != hex.EncodeToString(checksum[:]) {
			t.Fatalf("downloaded file %q sha256 = %v, want %s", file.Path, file.Sha256, hex.EncodeToString(checksum[:]))
		}
	}
}

func story003ArtifactBytes(t *testing.T, cacheDirectory string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(cacheDirectory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".gguf") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatalf("walk model artifact cache: %v", err)
	}
	return total
}

func story003RegularFileBytes(t *testing.T, directory string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk model cache: %v", err)
	}
	return total
}

func story003RegularFileBytesIfPresent(t *testing.T, directory string) int64 {
	t.Helper()
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return 0
	}
	return story003RegularFileBytes(t, directory)
}

func story003ModelsInspectArgs(serverURL string) []string {
	return []string{
		"you", "--json", "--server", strings.TrimSuffix(serverURL, "/"),
		"models", "inspect", story003ModelName,
	}
}

func story003ModelsListArgs(serverURL string) []string {
	return []string{
		"you", "--json", "--server", strings.TrimSuffix(serverURL, "/"), "models", "list",
	}
}

func story003ModelsHumanListArgs(serverURL string) []string {
	return []string{
		"you", "--server", strings.TrimSuffix(serverURL, "/"), "models", "list",
	}
}

func story003ModelsHumanInspectArgs(serverURL string) []string {
	return []string{
		"you", "--server", strings.TrimSuffix(serverURL, "/"),
		"models", "inspect", story003ModelName,
	}
}

func story003ModelsRemoveArgs(serverURL string) []string {
	return []string{
		"you", "--json", "--server", strings.TrimSuffix(serverURL, "/"),
		"models", "remove", story003ModelName,
	}
}

func story003ModelsPullArgs(serverURL string) []string {
	return []string{
		"you", "--json", "--server", strings.TrimSuffix(serverURL, "/"),
		"models", "pull", story003ModelName,
	}
}

func waitStory003Signal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(story003WorkflowTimeout):
		t.Fatalf("timed out waiting for controlled source %s", label)
	}
}

type controlledModelSource struct {
	baseBody      []byte
	tokenizerBody []byte

	manifestServed       chan struct{}
	firstDownloadStarted chan struct{}
	releaseFirstDownload chan struct{}
	manifestOnce         sync.Once
	firstDownloadOnce    sync.Once
}

func newControlledModelSource(baseBody, tokenizerBody []byte) *controlledModelSource {
	return &controlledModelSource{
		baseBody:             baseBody,
		tokenizerBody:        tokenizerBody,
		manifestServed:       make(chan struct{}),
		firstDownloadStarted: make(chan struct{}),
		releaseFirstDownload: make(chan struct{}),
	}
}

func (source *controlledModelSource) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/models/" + story003Repository:
		source.manifestOnce.Do(func() { close(source.manifestServed) })
		manifest := map[string]any{
			"sha": story003Revision,
			"siblings": []map[string]any{
				story003ManifestSibling(story003BaseAsset, source.baseBody),
				story003ManifestSibling(story003TokenizerAsset, source.tokenizerBody),
			},
		}
		if err := json.NewEncoder(writer).Encode(manifest); err != nil {
			return
		}
	case "/" + story003Repository + "/resolve/" + story003Revision + "/" + story003BaseAsset:
		source.firstDownloadOnce.Do(func() { close(source.firstDownloadStarted) })
		select {
		case <-source.releaseFirstDownload:
			_, _ = writer.Write(source.baseBody)
		case <-request.Context().Done():
		}
	case "/" + story003Repository + "/resolve/" + story003Revision + "/" + story003TokenizerAsset:
		_, _ = writer.Write(source.tokenizerBody)
	default:
		http.NotFound(writer, request)
	}
}

func story003ManifestSibling(name string, body []byte) map[string]any {
	checksum := sha256.Sum256(body)
	checksumText := hex.EncodeToString(checksum[:])
	return map[string]any{
		"rfilename": name,
		"size":      len(body),
		"lfs": map[string]any{
			"oid":  checksumText,
			"size": len(body),
		},
	}
}
