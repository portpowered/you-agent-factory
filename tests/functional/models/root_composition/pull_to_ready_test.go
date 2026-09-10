package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	pullToReadyModelName = modelsservice.BuiltInModelNameASR
	pullToReadyRevision  = "5359861c739e955e79d9a303bcbc70fb988958b1"
	pullToReadyAsset     = "ggml-base.en.bin"
)

// TestModelsPullToReadySurvivesProcessReconstruction proves the public local
// Models lifecycle without a daemon or network. The HTTP effect is replaced at
// the root.BuildProcess edge, while the pull, inspect, and list commands remain
// ordinary customer Process.Execute invocations.
func TestModelsPullToReadySurvivesProcessReconstruction(t *testing.T) {
	t.Parallel()
	assetBody := []byte("network-free ASR asset")
	backendBody := []byte("network-free ASR backend")
	backendSelection := pullToReadyBackendSelection(backendBody)
	assetClient := newPullToReadyAssetClient(assetBody, backendBody, backendSelection.Location)
	homeDirectory := functionalTempDir(t)
	selectedCache := filepath.Join(homeDirectory, "managed-cache")
	edges := pullToReadyEdges(assetClient, homeDirectory, backendSelection)

	firstProcess := buildPullToReadyProcess(t, edges)
	pull := executePullToReadyCommand(t, firstProcess, homeDirectory, "models", "pull", pullToReadyModelName)
	assertPullToReadySuccess(t, pull, assetBody, selectedCache)
	inspect := executePullToReadyCommand(t, firstProcess, homeDirectory, "models", "inspect", pullToReadyModelName)
	assertPullToReadyInspect(t, inspect, selectedCache, assetBody)
	listed := executePullToReadyCommand(t, firstProcess, homeDirectory, "models", "list")
	assertPullToReadyList(t, listed, assetBody, selectedCache)
	closePullToReadyProcess(t, firstProcess)

	assetClient.SetOffline()
	secondProcess := buildPullToReadyProcess(t, pullToReadyEdges(assetClient, homeDirectory, backendSelection))
	warmPull := executePullToReadyCommand(t, secondProcess, homeDirectory, "models", "pull", pullToReadyModelName)
	assertPullToReadyAlreadyPresent(t, warmPull, assetBody, selectedCache)
	secondInspect := executePullToReadyCommand(t, secondProcess, homeDirectory, "models", "inspect", pullToReadyModelName)
	assertPullToReadyInspect(t, secondInspect, selectedCache, assetBody)
	secondList := executePullToReadyCommand(t, secondProcess, homeDirectory, "models", "list")
	assertPullToReadyList(t, secondList, assetBody, selectedCache)
	var pulled factoryapi.ModelPullResponse
	decodePullToReadyJSON(t, pull.raw, &pulled)
	if pulled.ManagedRuntimePull.CachePath == nil {
		t.Fatal("initial pull did not publish a cache path for removal accounting")
	}
	beforeRemoveBytes := story003RegularFileBytes(t, *pulled.ManagedRuntimePull.CachePath)
	t.Logf("selected-root removal setup: model=%q cachePath=%q bytes=%d selectedRoot=%q", pulled.ModelName, *pulled.ManagedRuntimePull.CachePath, beforeRemoveBytes, selectedCache)
	removed := executePullToReadyCommand(t, secondProcess, homeDirectory, "models", "remove", pullToReadyModelName)
	assertPullToReadyRemoved(t, removed, selectedCache, beforeRemoveBytes)
	_, repeatedRemoveErr := executePullToReadyCommandResult(t, secondProcess, homeDirectory, "models", "remove", pullToReadyModelName)
	if repeatedRemoveErr == nil || !errors.Is(repeatedRemoveErr, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("repeated selected-root remove error = %v, want ErrModelCacheNotFound", repeatedRemoveErr)
	}
	missingList := executePullToReadyCommand(t, secondProcess, homeDirectory, "models", "list")
	assertPullToReadyMissingList(t, missingList)
	missingInspect, missingInspectErr := executePullToReadyCommandResult(t, secondProcess, homeDirectory, "models", "inspect", pullToReadyModelName)
	if missingInspectErr == nil {
		assertPullToReadyMissingInspect(t, missingInspect)
	} else if !errors.Is(missingInspectErr, modelscli.ErrModelCacheNotFound) {
		t.Fatalf("selected-root missing inspect error = %v, want ErrModelCacheNotFound or missing projection", missingInspectErr)
	}
	closePullToReadyProcess(t, secondProcess)
	if _, err := os.Stat(filepath.Join(homeDirectory, ".agent-factory", "models")); !os.IsNotExist(err) {
		t.Fatalf("default cache root stat = %v, want absent after selected-root lifecycle", err)
	}
	assertPullToReadySafeOutput(t, map[string]string{
		"pull":            pull.raw,
		"inspect":         inspect.raw,
		"list":            listed.raw,
		"warm pull":       warmPull.raw,
		"restart inspect": secondInspect.raw,
		"restart list":    secondList.raw,
		"remove":          removed.raw,
		"missing list":    missingList.raw,
		"missing inspect": missingInspect.raw,
	}, []string{
		backendSelection.Location,
		"https://assets.invalid",
		string(assetBody),
		string(backendBody),
		"download=true",
	}...)

	if got := assetClient.Calls(); got != 6 {
		t.Fatalf("asset edge requests = %d (%#v), want only the first pull resolution attempts", got, assetClient.Requests())
	}
	if got := assetClient.TransferBytes(); got != int64(len(assetBody)+len(backendBody)) {
		t.Fatalf("asset transfer bytes = %d, want first-pull artifact bytes %d", got, len(assetBody)+len(backendBody))
	}
	wantRequests := []string{
		http.MethodHead + " " + backendSelection.Location,
		http.MethodGet + " https://api.invalid/models/ggerganov/whisper.cpp?revision=" + pullToReadyRevision,
		http.MethodHead + " " + backendSelection.Location,
		http.MethodGet + " https://api.invalid/models/ggerganov/whisper.cpp?revision=" + pullToReadyRevision,
		http.MethodGet + " " + backendSelection.Location,
		http.MethodGet + " https://assets.invalid/ggerganov/whisper.cpp/resolve/" + pullToReadyRevision + "/" + pullToReadyAsset + "?download=true",
	}
	if got := strings.Join(assetClient.Requests(), "\n"); got != strings.Join(wantRequests, "\n") {
		t.Fatalf("asset edge request order = %q, want %q", got, strings.Join(wantRequests, "\n"))
	}
	t.Logf(
		"pull-to-ready commands passed: downloadedBytes=%d inspectCacheBytes=%d restartInspectCacheBytes=%d warmPullBytes=%d cacheRoot=%s assetRequests=%d transferBytes=%d",
		pull.downloadedBytes, inspect.cacheBytes, secondInspect.cacheBytes,
		warmPull.downloadedBytes, filepath.Join(homeDirectory, ".agent-factory", "models"), assetClient.Calls(), assetClient.TransferBytes(),
	)
}

// rootProcess keeps the test's public process capability narrow while allowing
// the test to close the exact process returned by root.BuildProcess.
type rootProcess interface {
	Execute(root.Input) error
	Close(context.Context) error
}

func buildPullToReadyProcess(t *testing.T, edges serviceedges.Edges) rootProcess {
	t.Helper()
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	return process
}

func pullToReadyEdges(
	assetClient *pullToReadyAssetClient,
	homeDirectory string,
	backendSelection serviceedges.ModelBackendArtifactSelection,
) serviceedges.Edges {
	return serviceedges.Edges{
		ModelAssetHTTPClient: assetClient,
		ModelAssetEndpoints: modelsservice.RuntimeAssetEndpoints{
			BaseURL:    "https://assets.invalid",
			APIBaseURL: "https://api.invalid",
		},
		ModelAssetResolveHomeDirectory: func() (string, error) {
			return homeDirectory, nil
		},
		ModelResolveBackendArtifact: func(
			context.Context,
			serviceedges.ModelBackendArtifactSelectionRequest,
		) (serviceedges.ModelBackendArtifactSelection, error) {
			return backendSelection, nil
		},
	}
}

type pullToReadyCapture struct {
	raw             string
	downloadedBytes int64
	cacheBytes      int64
}

func executePullToReadyCommand(
	t *testing.T,
	process rootProcess,
	homeDirectory string,
	arguments ...string,
) pullToReadyCapture {
	t.Helper()
	capture, err := executePullToReadyCommandResult(t, process, homeDirectory, arguments...)
	if err != nil {
		t.Fatalf("Process.Execute(%s) error = %v\nstdout:\n%s", strings.Join(arguments, " "), err, capture.raw)
	}
	return capture
}

func executePullToReadyCommandResult(
	t *testing.T,
	process rootProcess,
	homeDirectory string,
	arguments ...string,
) (pullToReadyCapture, error) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you", "--json"}, arguments...))
	inputs.Input.Env = append(inputs.Input.Env,
		"HOME="+homeDirectory,
		"USERPROFILE="+homeDirectory,
		runcli.ModelCacheDirEnvironment+"="+filepath.Join(homeDirectory, "managed-cache"),
	)
	inputs.Input.WorkingDirectory = functionalTempDir(t)
	err := process.Execute(inputs.Input)
	return pullToReadyCapture{raw: inputs.Stdout(), downloadedBytes: pullToReadyDownloadedBytes(t, arguments, inputs.Stdout()), cacheBytes: pullToReadyCacheBytes(t, arguments, inputs.Stdout())}, err
}

func closePullToReadyProcess(t *testing.T, process rootProcess) {
	t.Helper()
	closeContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := process.Close(closeContext); err != nil {
		t.Fatalf("close root.BuildProcess() process: %v", err)
	}
}

func pullToReadyDownloadedBytes(t *testing.T, arguments []string, raw string) int64 {
	t.Helper()
	if len(arguments) < 2 || arguments[0] != "models" || arguments[1] != "pull" {
		return 0
	}
	var response factoryapi.ModelPullResponse
	decodePullToReadyJSON(t, raw, &response)
	var total int64
	for _, file := range response.DownloadedFiles {
		total += file.Bytes
	}
	return total
}

func pullToReadyCacheBytes(t *testing.T, arguments []string, raw string) int64 {
	t.Helper()
	var cacheBytes *int64
	switch {
	case len(arguments) >= 2 && arguments[0] == "models" && arguments[1] == "inspect":
		var response factoryapi.ModelDetail
		decodePullToReadyJSON(t, raw, &response)
		cacheBytes = response.ManagedRuntime.CacheBytes
	case len(arguments) >= 2 && arguments[0] == "models" && arguments[1] == "list":
		var response factoryapi.ListModelsResponse
		decodePullToReadyJSON(t, raw, &response)
		for _, model := range response.Results {
			if model.Name == pullToReadyModelName {
				cacheBytes = model.ManagedRuntime.CacheBytes
				break
			}
		}
	}
	if cacheBytes == nil {
		return 0
	}
	return *cacheBytes
}

func decodePullToReadyJSON(t *testing.T, raw string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		t.Fatalf("decode Models JSON output: %v\noutput:\n%s", err, raw)
	}
}

func assertPullToReadySuccess(t *testing.T, capture pullToReadyCapture, assetBody []byte, cacheRoot string) {
	t.Helper()
	var response factoryapi.ModelPullResponse
	decodePullToReadyJSON(t, capture.raw, &response)
	if response.ModelName != pullToReadyModelName || response.Outcome != factoryapi.ModelPullOutcomePULLED {
		t.Fatalf("models pull response = %#v, want asr/PULLED", response)
	}
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY ||
		response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("models pull managed result = %#v, want INSTALLED_SUCCESSFULLY/READY", response.ManagedRuntimePull)
	}
	if len(response.DownloadedFiles) != 1 || response.DownloadedFiles[0].Path != pullToReadyAsset ||
		response.DownloadedFiles[0].Bytes != int64(len(assetBody)) {
		t.Fatalf("models pull downloaded files = %#v, want %s/%d bytes", response.DownloadedFiles, pullToReadyAsset, len(assetBody))
	}
	if response.ManagedRuntimePull.CachePath == nil || *response.ManagedRuntimePull.CachePath == "" {
		t.Fatalf("models pull managed cache path = %#v, want installed path", response.ManagedRuntimePull.CachePath)
	}
	if !strings.HasPrefix(*response.ManagedRuntimePull.CachePath, cacheRoot) {
		t.Fatalf("models pull cache path = %q, want selected root %q", *response.ManagedRuntimePull.CachePath, cacheRoot)
	}
	if capture.downloadedBytes != int64(len(assetBody)) {
		t.Fatalf("models pull downloaded bytes = %d, want %d", capture.downloadedBytes, len(assetBody))
	}
}

func assertPullToReadyAlreadyPresent(t *testing.T, capture pullToReadyCapture, assetBody []byte, cacheRoot string) {
	t.Helper()
	var response factoryapi.ModelPullResponse
	decodePullToReadyJSON(t, capture.raw, &response)
	if response.ModelName != pullToReadyModelName || response.Outcome != factoryapi.ModelPullOutcomeALREADYPRESENT {
		t.Fatalf("warm models pull response = %#v, want asr/ALREADY_PRESENT", response)
	}
	if response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT &&
		response.ManagedRuntimePull.PullOutcome != factoryapi.ManagedRuntimePullOutcomeALREADYREADY {
		t.Fatalf("warm models pull managed result = %#v, want ALREADY_PRESENT or ALREADY_READY", response.ManagedRuntimePull)
	}
	if response.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("warm models pull readiness = %s, want READY", response.ManagedRuntimePull.ReadinessState)
	}
	if len(response.DownloadedFiles) != 1 || response.DownloadedFiles[0].Path != pullToReadyAsset ||
		response.DownloadedFiles[0].Bytes != int64(len(assetBody)) {
		t.Fatalf("warm models pull cached files = %#v, want persisted %s/%d facts", response.DownloadedFiles, pullToReadyAsset, len(assetBody))
	}
	if response.ManagedRuntimePull.CachePath == nil || *response.ManagedRuntimePull.CachePath == "" {
		t.Fatalf("warm models pull cache path = %#v, want persisted path", response.ManagedRuntimePull.CachePath)
	}
	if !strings.HasPrefix(*response.ManagedRuntimePull.CachePath, cacheRoot) {
		t.Fatalf("warm models pull cache path = %q, want selected root %q", *response.ManagedRuntimePull.CachePath, cacheRoot)
	}
}

func assertPullToReadySafeOutput(t *testing.T, outputs map[string]string, forbidden ...string) {
	t.Helper()
	for phase, output := range outputs {
		for _, value := range forbidden {
			if value != "" && strings.Contains(output, value) {
				t.Fatalf("%s output leaked controlled source value %q:\n%s", phase, value, output)
			}
		}
	}
}

func assertPullToReadyInspect(t *testing.T, capture pullToReadyCapture, cacheRoot string, assetBody []byte) {
	t.Helper()
	var response factoryapi.ModelDetail
	decodePullToReadyJSON(t, capture.raw, &response)
	if response.Name != pullToReadyModelName ||
		response.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		response.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("models inspect response = %#v, want asr READY/INSTALLED", response)
	}
	if response.ManagedRuntime.CachePath == nil || !strings.HasPrefix(*response.ManagedRuntime.CachePath, cacheRoot) {
		t.Fatalf("models inspect cache path = %#v, want selected cache root %q", response.ManagedRuntime.CachePath, cacheRoot)
	}
	if response.ManagedRuntime.CacheBytes == nil || *response.ManagedRuntime.CacheBytes != int64(len(assetBody)) {
		t.Fatalf("models inspect cache bytes = %#v, want %d", response.ManagedRuntime.CacheBytes, len(assetBody))
	}
}

func assertPullToReadyList(t *testing.T, capture pullToReadyCapture, assetBody []byte, cacheRoot string) {
	t.Helper()
	var response factoryapi.ListModelsResponse
	decodePullToReadyJSON(t, capture.raw, &response)
	for _, model := range response.Results {
		if model.Name != pullToReadyModelName {
			continue
		}
		if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
			model.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
			t.Fatalf("models list ASR managed result = %#v, want READY/INSTALLED", model.ManagedRuntime)
		}
		if model.ManagedRuntime.CacheBytes == nil || *model.ManagedRuntime.CacheBytes != int64(len(assetBody)) {
			t.Fatalf("models list ASR cache bytes = %#v, want %d", model.ManagedRuntime.CacheBytes, len(assetBody))
		}
		if model.ManagedRuntime.CachePath == nil || !strings.HasPrefix(*model.ManagedRuntime.CachePath, cacheRoot) {
			t.Fatalf("models list ASR cache path = %#v, want selected cache root %q", model.ManagedRuntime.CachePath, cacheRoot)
		}
		return
	}
	t.Fatalf("models list did not contain %q: %#v", pullToReadyModelName, response.Results)
}

func assertPullToReadyRemoved(t *testing.T, capture pullToReadyCapture, cacheRoot string, beforeBytes int64) {
	t.Helper()
	var response factoryapi.ModelRemoveResponse
	decodePullToReadyJSON(t, capture.raw, &response)
	if response.ModelName != strings.ToUpper(strings.TrimSpace(pullToReadyModelName)) ||
		response.Outcome != factoryapi.REMOVED || response.BytesRemoved != beforeBytes {
		t.Fatalf("models remove response = %#v, want selected %s/%d bytes", response, cacheRoot, beforeBytes)
	}
	if strings.TrimSpace(response.CachePath) == "" || !strings.HasPrefix(response.CachePath, cacheRoot) {
		t.Fatalf("models remove cache path = %#v, want selected cache root %q", response.CachePath, cacheRoot)
	}
}

func assertPullToReadyMissingList(t *testing.T, capture pullToReadyCapture) {
	t.Helper()
	var response factoryapi.ListModelsResponse
	decodePullToReadyJSON(t, capture.raw, &response)
	for _, model := range response.Results {
		if model.Name != pullToReadyModelName {
			continue
		}
		if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING ||
			model.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
			t.Fatalf("missing models list runtime = %#v, want MISSING/NOT_INSTALLED", model.ManagedRuntime)
		}
		return
	}
	t.Fatalf("missing models list did not contain %q: %#v", pullToReadyModelName, response.Results)
}

func assertPullToReadyMissingInspect(t *testing.T, capture pullToReadyCapture) {
	t.Helper()
	var response factoryapi.ModelDetail
	decodePullToReadyJSON(t, capture.raw, &response)
	if response.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING ||
		response.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("missing models inspect runtime = %#v, want MISSING/NOT_INSTALLED", response.ManagedRuntime)
	}
}

type pullToReadyAssetClient struct {
	manifest   []byte
	model      []byte
	backend    []byte
	backendURL string
	calls      atomic.Int64
	transfers  atomic.Int64
	offline    atomic.Bool
	requests   []string
}

func newPullToReadyAssetClient(modelBody, backendBody []byte, backendURL string) *pullToReadyAssetClient {
	digest := sha256.Sum256(modelBody)
	manifest, err := json.Marshal(map[string]any{
		"sha": pullToReadyRevision,
		"siblings": []map[string]any{{
			"rfilename": pullToReadyAsset,
			"size":      len(modelBody),
			"lfs": map[string]any{
				"oid":  hex.EncodeToString(digest[:]),
				"size": len(modelBody),
			},
		}},
	})
	if err != nil {
		panic(fmt.Sprintf("marshal pull-to-ready manifest: %v", err))
	}
	return &pullToReadyAssetClient{
		manifest:   manifest,
		model:      append([]byte(nil), modelBody...),
		backend:    append([]byte(nil), backendBody...),
		backendURL: backendURL,
	}
}

func (client *pullToReadyAssetClient) Do(request *http.Request) (*http.Response, error) {
	if client.offline.Load() {
		return nil, fmt.Errorf("controlled asset source is offline")
	}
	client.calls.Add(1)
	client.requests = append(client.requests, request.Method+" "+request.URL.String())
	var body []byte
	switch request.URL.Path {
	case "/models/ggerganov/whisper.cpp":
		body = client.manifest
	case "/ggerganov/whisper.cpp/resolve/" + pullToReadyRevision + "/" + pullToReadyAsset:
		body = client.model
	default:
		if request.URL.String() == client.backendURL {
			body = client.backend
			break
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("unexpected asset request")),
			Request:    request,
		}, nil
	}
	if request.Method == http.MethodGet &&
		(request.URL.Path == "/ggerganov/whisper.cpp/resolve/"+pullToReadyRevision+"/"+pullToReadyAsset || request.URL.String() == client.backendURL) {
		client.transfers.Add(int64(len(body)))
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		ContentLength: func() int64 {
			if request.Method == http.MethodHead {
				return int64(len(body))
			}
			return -1
		}(),
		Request: request,
	}, nil
}

func (client *pullToReadyAssetClient) SetOffline() {
	client.offline.Store(true)
}

func pullToReadyBackendSelection(body []byte) serviceedges.ModelBackendArtifactSelection {
	digest := sha256.Sum256(body)
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "pull-to-ready-backend.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/pull-to-ready-fixture/pull-to-ready-backend.tar.gz",
		Bytes:    int64(len(body)),
		SHA256:   hex.EncodeToString(digest[:]),
	}
}

func (client *pullToReadyAssetClient) Calls() int64 {
	return client.calls.Load()
}

func (client *pullToReadyAssetClient) TransferBytes() int64 {
	return client.transfers.Load()
}

func (client *pullToReadyAssetClient) Requests() []string {
	return append([]string(nil), client.requests...)
}
