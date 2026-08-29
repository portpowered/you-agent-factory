package root_composition_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models"
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
	assetBody := []byte("network-free ASR asset")
	backendBody := []byte("network-free ASR backend")
	backendSelection := pullToReadyBackendSelection(backendBody)
	assetClient := newPullToReadyAssetClient(assetBody, backendBody, backendSelection.Location)
	homeDirectory := characterizationTempDir(t)
	edges := pullToReadyEdges(assetClient, homeDirectory, backendSelection)

	firstProcess := buildPullToReadyProcess(t, edges)
	pull := executePullToReadyCommand(t, firstProcess, homeDirectory, "models", "pull", pullToReadyModelName)
	assertPullToReadySuccess(t, pull, assetBody)
	inspect := executePullToReadyCommand(t, firstProcess, homeDirectory, "models", "inspect", pullToReadyModelName)
	assertPullToReadyInspect(t, inspect, homeDirectory, assetBody)
	listed := executePullToReadyCommand(t, firstProcess, homeDirectory, "models", "list")
	assertPullToReadyList(t, listed, assetBody)
	closePullToReadyProcess(t, firstProcess)

	secondProcess := buildPullToReadyProcess(t, pullToReadyEdges(assetClient, homeDirectory, backendSelection))
	secondInspect := executePullToReadyCommand(t, secondProcess, homeDirectory, "models", "inspect", pullToReadyModelName)
	assertPullToReadyInspect(t, secondInspect, homeDirectory, assetBody)
	secondList := executePullToReadyCommand(t, secondProcess, homeDirectory, "models", "list")
	assertPullToReadyList(t, secondList, assetBody)
	closePullToReadyProcess(t, secondProcess)

	if got := assetClient.Calls(); got != 6 {
		t.Fatalf("asset edge requests = %d (%#v), want the two scoped pull resolution attempts", got, assetClient.Requests())
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
		"pull-to-ready commands passed: downloadedBytes=%d inspectCacheBytes=%d restartInspectCacheBytes=%d cacheRoot=%s assetRequests=%d",
		pull.downloadedBytes, inspect.cacheBytes, secondInspect.cacheBytes,
		filepath.Join(homeDirectory, ".agent-factory", "models"), assetClient.Calls(),
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
	inputs := support.FakeInputs(t.Context(), append([]string{"you", "--json"}, arguments...))
	inputs.Input.Env = append(inputs.Input.Env,
		"HOME="+homeDirectory,
		"USERPROFILE="+homeDirectory,
		runcli.ModelCacheDirEnvironment+"="+filepath.Join(homeDirectory, "managed-cache"),
	)
	inputs.Input.WorkingDirectory = characterizationTempDir(t)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%s) error = %v\nstdout:\n%s\nstderr:\n%s",
			strings.Join(arguments, " "), err, inputs.Stdout(), inputs.Stderr(),
		)
	}
	return pullToReadyCapture{raw: inputs.Stdout(), downloadedBytes: pullToReadyDownloadedBytes(t, arguments, inputs.Stdout()), cacheBytes: pullToReadyCacheBytes(t, arguments, inputs.Stdout())}
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

func assertPullToReadySuccess(t *testing.T, capture pullToReadyCapture, assetBody []byte) {
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
	if capture.downloadedBytes != int64(len(assetBody)) {
		t.Fatalf("models pull downloaded bytes = %d, want %d", capture.downloadedBytes, len(assetBody))
	}
}

func assertPullToReadyInspect(t *testing.T, capture pullToReadyCapture, homeDirectory string, assetBody []byte) {
	t.Helper()
	var response factoryapi.ModelDetail
	decodePullToReadyJSON(t, capture.raw, &response)
	if response.Name != pullToReadyModelName ||
		response.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		response.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("models inspect response = %#v, want asr READY/INSTALLED", response)
	}
	if response.ManagedRuntime.CachePath == nil || !strings.HasPrefix(*response.ManagedRuntime.CachePath, filepath.Join(homeDirectory, ".agent-factory", "models")) {
		t.Fatalf("models inspect cache path = %#v, want isolated home cache", response.ManagedRuntime.CachePath)
	}
	if response.ManagedRuntime.CacheBytes == nil || *response.ManagedRuntime.CacheBytes != int64(len(assetBody)) {
		t.Fatalf("models inspect cache bytes = %#v, want %d", response.ManagedRuntime.CacheBytes, len(assetBody))
	}
}

func assertPullToReadyList(t *testing.T, capture pullToReadyCapture, assetBody []byte) {
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
		return
	}
	t.Fatalf("models list did not contain %q: %#v", pullToReadyModelName, response.Results)
}

type pullToReadyAssetClient struct {
	manifest   []byte
	model      []byte
	backend    []byte
	backendURL string
	calls      atomic.Int64
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
	c06Ledger.assetHTTPCalls.Add(1)
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

func (client *pullToReadyAssetClient) Requests() []string {
	return append([]string(nil), client.requests...)
}
