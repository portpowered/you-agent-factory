package inference_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each group owns one immutable cache/environment/locator edge shape and reuses
// its process for the causally ordered failure, repair, cold and warm actions.
// Pull is invocation-local; hosted inference/Session proof lives in the host journey.
func TestModelsGalleryBootstrapRelease(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	for _, fault := range []string{"metadata", "pair"} {
		t.Run(fault, func(t *testing.T) { t.Parallel(); galleryPullRecovery(t, fault, "", "") })
	}
}
func TestModelsGalleryBootstrapIntegrityRetry(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	for _, fault := range []string{"checksum", "download", "interrupted", "publication", "cached-corrupt"} {
		t.Run(fault, func(t *testing.T) { t.Parallel(); galleryPullRecovery(t, fault, "", "") })
	}
}
func TestModelsGalleryBootstrapInstallation(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	for _, fault := range []string{"command", "exit", "missing-script"} {
		t.Run(fault, func(t *testing.T) { t.Parallel(); galleryPullRecovery(t, fault, "", "") })
	}
}
func TestModelsGalleryBootstrapCachePrecedence(t *testing.T) {
	requireGalleryPlatform(t)
	t.Parallel()
	for _, selection := range []struct{ name, override, located string }{
		{"override", "selected-override", "ignored-path"}, {"path", "", "selected-path"},
	} {
		t.Run(selection.name, func(t *testing.T) {
			t.Parallel()
			galleryPullRecovery(t, "command", selection.override, selection.located)
		})
	}
}
func requireGalleryPlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("production gallery graph selects Linux amd64; own-PR Linux functional CI owns this witness")
	}
}

const galleryBinaryName = "local-ai-v1-linux-amd64"

var galleryBinary = []byte("synthetic verified executable; controlled runner never launches it")

type galleryPullFixture struct {
	model           *pullToReadyAssetClient
	cache           string
	fault           string
	failed          atomic.Bool
	downloads       atomic.Int32
	requests        atomic.Int32
	commands        atomic.Int32
	wantedCommand   string
	downloadStarted chan struct{}
}

func galleryPullRecovery(t *testing.T, fault, override, located string) {
	t.Helper()
	home := t.TempDir()
	cache := t.TempDir()
	body := []byte("gallery-backed customer ASR weights")
	writeGenericModelSourceOverride(t, home, pullToReadyModelName, pullToReadySource, "localai-whisper")
	fixture := &galleryPullFixture{model: newPullToReadyAssetClient(body, nil, ""), cache: cache, fault: fault}
	fixture.failed.Store(true)
	if override != "" {
		fixture.wantedCommand = override
	} else if located != "" {
		fixture.wantedCommand = located
	} else {
		fixture.wantedCommand = filepath.Join(cache, "you", "localai", "v1", galleryBinaryName)
	}
	if fault == "cached-corrupt" {
		path := fixture.wantedCommand
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("unverified cache bytes"), 0o700); err != nil {
			t.Fatal(err)
		}
		// This cell proves replacement on the first successful pull, not a failure.
		fixture.failed.Store(false)
	}
	edges := galleryInferenceEdges(t, galleryPullEdges(fixture, home, override, located))
	process := buildPullToReadyProcess(t, edges)
	if fixture.failed.Load() {
		galleryAssertPullFailure(t, process, home)
		galleryAssertNoUnverifiedInstallation(t, fixture)
	}
	fixture.failed.Store(false)
	selectedCache := filepath.Join(home, "managed-cache")
	pull := executePullToReadyCommand(t, process, home, "models", "pull", pullToReadyModelName)
	assertPullToReadySuccess(t, pull, body, selectedCache)
	assertPullToReadyInspect(t, executePullToReadyCommand(t, process, home, "models", "inspect", pullToReadyModelName), selectedCache, body)
	before := fixture.downloads.Load()
	if fault == "cached-corrupt" && before != 1 {
		t.Fatalf("corrupt cache replacement transfers = %d, want 1", before)
	}
	assertPullToReadyAlreadyPresent(t, executePullToReadyCommand(t, process, home, "models", "pull", pullToReadyModelName), body, selectedCache)
	if fixture.downloads.Load() != before {
		t.Fatal("warm pull retransferred binary")
	}
	if override != "" || located != "" {
		if before != 0 {
			t.Fatal("selected executable precedence downloaded release")
		}
	} else {
		installed, err := os.ReadFile(fixture.wantedCommand)
		if err != nil || !bytes.Equal(installed, galleryBinary) {
			t.Fatalf("verified executable bytes = %q, %v", installed, err)
		}
	}
	assertGalleryLifecycleInference(t, process, home)
}
func galleryAssertPullFailure(t *testing.T, process rootProcess, home string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "pull", pullToReadyModelName})
	inputs.Input.Env = isolatedModelEnvironment(home, filepath.Join(home, "managed-cache"))
	inputs.Input.WorkingDirectory = t.TempDir()
	err := process.Execute(inputs.Input)
	var diagnostic interface{ CLIErrorCode() string }
	if err == nil || !errors.As(err, &diagnostic) || diagnostic.CLIErrorCode() != "CLI_MODEL_PULL_FAILED" {
		t.Fatalf("gallery failure = %T %v, stdout=%s stderr=%s", err, err, inputs.Stdout(), inputs.Stderr())
	}
	// Backend selection fails before the pull response exists. The documented
	// public failure is the coded CLI error; it must emit no successful pull facts.
	if inputs.Stdout() != "" {
		t.Fatalf("gallery selection failure emitted success output: %s", inputs.Stdout())
	}
	diagnosticResponse := decodeFirstDiagnostic(t, inputs.Stderr())
	if string(diagnosticResponse.Code) != "CLI_MODEL_PULL_FAILED" {
		t.Fatalf("gallery diagnostic = %#v", diagnosticResponse)
	}

}

type galleryExecutableLocator struct{ command string }

func (locator galleryExecutableLocator) LookPath(string) (string, error) {
	if locator.command == "" {
		return "", errors.New("controlled executable missing")
	}
	return locator.command, nil
}
func (fixture *galleryPullFixture) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	fixture.commands.Add(1)
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	if request.Command != fixture.wantedCommand || len(request.Args) != 4 || request.Args[0] != "backends" || request.Args[1] != "install" || request.Args[2] != "--backends-path="+filepath.Join(fixture.cache, "you", "localai-backends") || request.Args[3] != "cpu-whisper" {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected controlled gallery command: %#v", request)
	}
	if fixture.failed.Load() {
		switch fixture.fault {
		case "command":
			return platformprocess.CommandResult{}, errors.New("controlled gallery execution failure")
		case "exit":
			return platformprocess.CommandResult{ExitCode: 17}, nil
		case "missing-script":
			return platformprocess.CommandResult{}, nil
		}
	}
	directory := filepath.Join(fixture.cache, "you", "localai-backends", request.Args[3])
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return platformprocess.CommandResult{}, err
	}
	err := os.WriteFile(filepath.Join(directory, "run.sh"), []byte("#!/bin/sh\n"), 0o700)
	return platformprocess.CommandResult{}, err
}
func (fixture *galleryPullFixture) Do(request *http.Request) (*http.Response, error) {
	fixture.requests.Add(1)
	var body []byte
	status := http.StatusOK
	switch request.URL.Path {
	case "/repos/mudler/LocalAI/releases/latest":
		if fixture.failed.Load() && fixture.fault == "metadata" {
			status = http.StatusServiceUnavailable
		}
		assets := []map[string]any{{"name": galleryBinaryName, "browser_download_url": "https://gallery.invalid/binary", "size": len(galleryBinary)}, {"name": "LocalAI-v1-checksums.txt", "browser_download_url": "https://gallery.invalid/checksums"}}
		if fixture.failed.Load() && fixture.fault == "pair" {
			assets = assets[:1]
		}
		body, _ = json.Marshal(map[string]any{"tag_name": "v1", "assets": assets})
	case "/checksums":
		digest := sha256.Sum256(galleryBinary)
		body = []byte(fmt.Sprintf("%x  %s\n", digest, galleryBinaryName))
	case "/binary":
		return fixture.binaryResponse(request), nil
	default:
		return fixture.model.Do(request)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Request: request}, nil
}

func (fixture *galleryPullFixture) binaryResponse(request *http.Request) *http.Response {
	fixture.downloads.Add(1)
	body := galleryBinary
	status := http.StatusOK
	var reader io.Reader = bytes.NewReader(body)
	if fixture.failed.Load() {
		switch fixture.fault {
		case "download":
			status = http.StatusServiceUnavailable
		case "cancellation":
			reader = io.MultiReader(bytes.NewReader(body[:2]), galleryCanceledReader{request.Context(), fixture.downloadStarted})
		case "checksum":
			reader = bytes.NewReader(bytes.Repeat([]byte("x"), len(body)))
		case "interrupted":
			reader = io.MultiReader(bytes.NewReader(body[:2]), galleryInterruptedReader{})
		}
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(reader), Request: request}
}

type galleryInterruptedReader struct{}

type galleryCanceledReader struct {
	context context.Context
	started chan struct{}
}

func (reader galleryCanceledReader) Read([]byte) (int, error) {
	close(reader.started)
	<-reader.context.Done()
	return 0, reader.context.Err()
}

func (galleryInterruptedReader) Read([]byte) (int, error) {
	return 0, errors.New("controlled interrupted transfer")
}

func galleryPullEdges(fixture *galleryPullFixture, home, override, located string) serviceedges.Edges {
	edges := pullToReadyEdges(fixture.model, home, serviceedges.ModelBackendArtifactSelection{})
	edges.ModelResolveBackendArtifact = nil // Exercise the canonical production resolver.
	edges.ModelAssetHostPlatform.OperatingSystem = "linux"
	edges.ModelAssetHostPlatform.Architecture = "amd64"
	edges.ModelAssetHostPlatform.Accelerator = "cpu"
	edges.ModelAssetHTTPClient = fixture
	edges.ModelAssetResolveEnvironment = func(key string) string {
		if key == "LOCALAI_BINARY" {
			return override
		}
		return ""
	}
	edges.ModelAssetExecutableLocator = galleryExecutableLocator{located}
	edges.ModelAssetResolveCacheDirectory = func() (string, error) { return fixture.cache, nil }
	edges.ModelAssetRenamePath = func(from, to string) error {
		if fixture.failed.Load() && fixture.fault == "publication" && filepath.Base(to) == galleryBinaryName {
			return errors.New("controlled publication denial")
		}
		return os.Rename(from, to)
	}
	edges.ModelRuntimeCommandRunner = fixture
	return edges
}
func galleryAssertNoUnverifiedInstallation(t *testing.T, fixture *galleryPullFixture) {
	t.Helper()
	fault, cache := fixture.fault, fixture.cache
	if fault != "command" && fault != "exit" && fault != "missing-script" && fixture.commands.Load() != 0 {
		t.Fatal("unverified binary reached installation command")
	}
	if fault == "checksum" || fault == "download" || fault == "interrupted" || fault == "publication" || fault == "cancellation" {
		if _, err := os.Stat(fixture.wantedCommand); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed transfer published binary: %v", err)
		}
		staged, err := filepath.Glob(filepath.Join(cache, "you", "localai", "v1", ".local-ai-*"))
		if err != nil || len(staged) != 0 {
			t.Fatalf("partial transfer remains: %v %v", staged, err)
		}
	}
}
