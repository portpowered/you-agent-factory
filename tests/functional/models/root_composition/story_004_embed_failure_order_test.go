package root_composition_test

import (
	"encoding/json"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type story004EmbedHarness struct {
	assetFixture *story004EmbedAssetHTTP
	selection    serviceedges.ModelBackendArtifactSelection
	process      support.Process
	factoryDir   string
	environment  []string
}

func newStory004EmbedHarness(t *testing.T, configure func(*story004EmbedAssetHTTP)) *story004EmbedHarness {
	t.Helper()
	hostServer := story004HostServer(t)
	home := functionalTempDir(t)
	modelBody := []byte("story-004-embedding-model-download")
	backendBody := []byte("story-004-localai-backend-download")
	selection := story004EmbedBackendSelection(backendBody)
	assetFixture := &story004EmbedAssetHTTP{modelBody: modelBody, backendBody: backendBody, selection: selection}
	if configure != nil {
		configure(assetFixture)
	}
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	process := functionalBuildProcess(t, story004EmbedEdges(
		home, assetFixture, hostServer.Client(), &recordingModelHostLauncher{endpoint: hostServer.URL},
		&joinedProtocolNegotiator{}, &joinedCompatibilityChecker{}, selection, newStory004EmbedFixture(),
	))
	support.CleanupProcess(t, process)
	return &story004EmbedHarness{
		assetFixture: assetFixture,
		selection:    selection,
		process:      process,
		factoryDir:   factoryDir,
		environment:  functionalHomeEnvironment(home),
	}
}

func (harness *story004EmbedHarness) run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return runStory004CLI(t, harness.process, harness.factoryDir, harness.environment,
		append([]string{"you"}, args...))
}

// TestModelsEmbedAcquiresBackendBeforeModelContentThroughRootBuildProcess
// proves the joined invoke checks backend assets before it downloads model
// content, and that the byte estimate is exact for the missing set.
func TestModelsEmbedAcquiresBackendBeforeModelContentThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	harness := newStory004EmbedHarness(t, nil)

	stdout, stderr, err := harness.run(t, "models", "invoke", "embed", "--input", "text=ordered")
	if err != nil || stdout != `[0.1,0.2,0.3,0.4]` {
		t.Fatalf("EMBED joined invoke = err %v stdout %q stderr %q", err, stdout, stderr)
	}
	if !strings.Contains(stderr, `models asset estimate modelName="embed" backendBytes=34 modelBytes=34 totalBytes=68`) {
		t.Fatalf("asset estimate stderr = %q, want exact missing-byte estimate", stderr)
	}
	requests := harness.assetFixture.Requests()
	manifest, modelContent, backendFirst := -1, -1, -1
	for index, request := range requests {
		switch {
		case strings.Contains(request, "/resolve/"):
			if modelContent < 0 {
				modelContent = index
			}
		case strings.HasSuffix(request, "/"+harness.selection.Name):
			if backendFirst < 0 {
				backendFirst = index
			}
		case strings.HasSuffix(request, "/models/Qwen/Qwen3-Embedding-0.6B-GGUF"):
			if manifest < 0 {
				manifest = index
			}
		}
	}
	if manifest < 0 || modelContent < 0 || backendFirst < 0 {
		t.Fatalf("asset requests = %#v, want manifest, model content, and backend requests", requests)
	}
	if backendFirst > modelContent {
		t.Fatalf("backend asset request followed model content: backend=%d model=%d requests=%#v", backendFirst, modelContent, requests)
	}
}

// TestModelsEmbedUnavailableBackendStopsBeforeModelContentThroughRootBuildProcess
// proves an unreachable backend asset fails with MODEL_BACKEND_NOT_READY
// before any model manifest or content is requested.
func TestModelsEmbedUnavailableBackendStopsBeforeModelContentThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	harness := newStory004EmbedHarness(t, func(fixture *story004EmbedAssetHTTP) { fixture.failBackend = true })

	stdout, stderr, err := harness.run(t, "models", "invoke", "embed", "--input", "text=unavailable")
	if err == nil {
		t.Fatalf("EMBED invoke with unavailable backend returned nil; stdout %q stderr %q", stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("unavailable-backend stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "MODEL_BACKEND_NOT_READY") {
		t.Fatalf("unavailable-backend stderr = %q, want MODEL_BACKEND_NOT_READY", stderr)
	}
	requests := harness.assetFixture.Requests()
	if len(requests) == 0 {
		t.Fatal("unavailable-backend asset requests are empty, want backend reachability attempts")
	}
	for _, request := range requests {
		if !strings.HasSuffix(request, "/"+harness.selection.Name) {
			t.Fatalf("asset request %q, want backend reachability only: %#v", request, requests)
		}
	}
}

// TestModelsEmbedRepeatedSourceFailureRendersOneDeterministicDiagnosticThroughRootBuildProcess
// proves five failing invokes against a failing source render one safe typed
// diagnostic and one stable debug cause chain, in text and JSON modes.
func TestModelsEmbedRepeatedSourceFailureRendersOneDeterministicDiagnosticThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	harness := newStory004EmbedHarness(t, func(fixture *story004EmbedAssetHTTP) { fixture.failManifest = true })

	var baseline story004Diagnostic
	for attempt := 0; attempt < 5; attempt++ {
		stdout, stderr, err := harness.run(t, "--debug", "models", "invoke", "embed", "--input", "text=failing")
		if err == nil {
			t.Fatalf("failing invoke %d returned nil error", attempt+1)
		}
		if stdout != "" {
			t.Fatalf("failing invoke %d stdout = %q, want empty", attempt+1, stdout)
		}
		capture := decodeStory004Diagnostic(t, stderr)
		if strings.TrimSpace(string(capture.response.Code)) == "" || strings.TrimSpace(capture.response.Message) == "" || capture.debug == "" {
			t.Fatalf("failing invoke %d capture = %#v, want non-empty code/message/debug", attempt+1, capture)
		}
		if attempt == 0 {
			baseline = capture
		} else if capture != baseline {
			t.Fatalf("failing invoke %d capture = %#v, want baseline %#v", attempt+1, capture, baseline)
		}
	}

	stdout, stderr, err := harness.run(t, "--json", "--debug", "models", "invoke", "embed", "--input", "text=failing")
	if err == nil {
		t.Fatal("JSON failing invoke returned nil error")
	}
	if stdout != "" {
		var value any
		if decodeErr := json.Unmarshal([]byte(stdout), &value); decodeErr != nil {
			t.Fatalf("JSON failing invoke stdout is neither empty nor valid JSON: %v; stdout=%q", decodeErr, stdout)
		}
	}
	if jsonCapture := decodeStory004Diagnostic(t, stderr); jsonCapture != baseline {
		t.Fatalf("JSON failing invoke capture = %#v, want baseline %#v", jsonCapture, baseline)
	}
}

type story004Diagnostic struct {
	response story004DiagnosticResponse
	debug    string
}

type story004DiagnosticResponse struct {
	Code    factoryapi.ErrorResponseCode
	Family  factoryapi.ErrorFamily
	Message string
}

func decodeStory004Diagnostic(t testing.TB, stderr string) story004Diagnostic {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) < 2 {
		t.Fatalf("stderr = %q, want typed line and debug cause", stderr)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(lines[0]), &response); err != nil {
		t.Fatalf("decode typed diagnostic: %v; stderr=%q", err, stderr)
	}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "debug: ") {
			t.Fatalf("stderr = %q, want only debug continuation after typed line", stderr)
		}
	}
	return story004Diagnostic{
		response: story004DiagnosticResponse{Code: response.Code, Family: response.Family, Message: response.Message},
		debug:    strings.Join(lines[1:], "\n"),
	}
}
