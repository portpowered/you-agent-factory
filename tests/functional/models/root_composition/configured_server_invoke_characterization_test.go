package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/localai"
)

// TestLocalAIConfiguredServerInvokeCharacterization records the two remaining
// cycle-164 public observations after the retained transport correction: a
// named output mapping crosses --server and publishes atomically, while a
// request whose server withholds response headers is bounded by the caller
// context and publishes no output.
// The server is root-composed and the client crosses its actual loopback HTTP
// boundary; the LocalAI edge remains the controlled backend fixture.
func TestLocalAIConfiguredServerInvokeCharacterization(t *testing.T) {
	t.Parallel()

	fixture := functionalStartLocalAI(t, localai.Options{EmbeddingDimensions: 5})
	factoryDir := functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig())

	serverPath := newLocalAIConfiguredEmbedOwnerPath(t, fixture)
	blockingBackend := &localAIConfiguredEmbedBlockingBackend{next: fixture.InvocationBackend}
	serverPath.invocation = &localAIConfiguredEmbedInvocationRecorder{
		next: blockingBackend.Invoke,
	}
	serverPath.edges.ModelInvocationBackend = serverPath.invocation.Invoke
	server, serverURL := startLocalAIConfiguredEmbedServer(t, factoryDir, serverPath)
	client := newLocalAIConfiguredEmbedClientPath(t, fixture)
	warmLocalAIConfiguredEmbedCache(
		t, client.process, factoryDir, cleanModelsEnvironment(client.home), serverURL, false,
		serverPath.invocation,
	)

	t.Run("named output mapping", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "embedding.json")
		result := executeLocalAIConfiguredEmbedOutputMap(
			t, client.process, factoryDir, cleanModelsEnvironment(client.home), serverURL,
			"embedding="+outputPath, outputPath,
		)
		t.Logf(
			"configured-server named mapping: err=%v stdout=%q stderr=%q output=%q server-backend-calls=%d",
			result.Err, result.Stdout, result.Stderr, result.Output, len(serverPath.invocation.Signatures()),
		)
		if result.Err != nil {
			t.Fatalf("configured-server named mapping error = %v, want corrected remote mapping", result.Err)
		}
		if result.Output != `[0.1,0.2,0.3,0.4,0.5]` {
			t.Fatalf("configured-server named mapping output = %q, want controlled vector", result.Output)
		}
		var response factoryapi.GenericModelInvocationResponse
		if err := json.Unmarshal([]byte(result.Stdout), &response); err != nil {
			t.Fatalf("decode configured-server named mapping response: %v; stdout=%q", err, result.Stdout)
		}
		if len(response.Outputs) != 1 || response.Outputs[0].Name != "embedding" ||
			response.Outputs[0].Content == nil || *response.Outputs[0].Content != result.Output ||
			response.Outputs[0].ContentType == nil || *response.Outputs[0].ContentType != "application/json" ||
			response.Outputs[0].MediaType == nil || *response.Outputs[0].MediaType != "application/json" {
			t.Fatalf("configured-server named mapping response = %#v, want ordered JSON embedding metadata", response)
		}
	})

	t.Run("invalid output mapping", func(t *testing.T) {
		beforeCalls := len(serverPath.invocation.Signatures())
		outputPath := filepath.Join(t.TempDir(), "should-not-exist.json")
		result := executeLocalAIConfiguredEmbedOutputMap(
			t, client.process, factoryDir, cleanModelsEnvironment(client.home), serverURL,
			"embedding", outputPath,
		)
		if result.Err == nil || !strings.Contains(result.Err.Error(), "--output is not supported with explicit generic inputs") {
			t.Fatalf("invalid configured-server output mapping error = %v, want current typed preflight result", result.Err)
		}
		if result.Stdout != "" || result.Output != "" {
			t.Fatalf("invalid configured-server output mapping published output: stdout=%q output=%q", result.Stdout, result.Output)
		}
		if got := len(serverPath.invocation.Signatures()); got != beforeCalls {
			t.Fatalf("invalid configured-server output mapping backend calls = %d, want unchanged %d", got, beforeCalls)
		}
	})

	t.Run("withheld headers cancel", func(t *testing.T) {
		started := make(chan struct{})
		blockingBackend.BlockNext(started)
		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()
		result := executeLocalAIConfiguredEmbedCommandWithContext(
			t, ctx, client.process, factoryDir, cleanModelsEnvironment(client.home), serverURL,
		)
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("withheld-header characterization did not reach the server backend")
		}
		if result.Err == nil || !strings.Contains(result.Err.Error(), "context deadline exceeded") {
			t.Fatalf("withheld-header result error = %v, want caller deadline", result.Err)
		}
		if result.Stdout != "" {
			t.Fatalf("withheld-header result published partial output: stdout=%q", result.Stdout)
		}
		select {
		case <-blockingBackend.Canceled():
		case <-time.After(2 * time.Second):
			t.Fatal("withheld-header backend did not observe caller cancellation")
		}
		t.Logf("withheld-header characterization: err=%v stdout=%q stderr=%q", result.Err, result.Stdout, result.Stderr)
	})

	if err := client.process.Close(context.Background()); err != nil {
		t.Fatalf("close configured-server characterization client: %v", err)
	}
	server.Close(t)
	assertLocalAIConfiguredServerReleased(t, server, serverURL)
	assertLocalAIDirectHostReleased(t, serverPath.launcher)
	if err := fixture.Close(); err != nil {
		t.Fatalf("close configured-server characterization fixture: %v", err)
	}
	assertLocalAIOMNIFixtureListenerReleased(t, fixture.Endpoint())
}

type localAIConfiguredEmbedOutputMapResult struct {
	Stdout string
	Stderr string
	Output string
	Err    error
}

func executeLocalAIConfiguredEmbedOutputMap(
	t *testing.T,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL, outputMapping, outputPath string,
) localAIConfiguredEmbedOutputMapResult {
	t.Helper()
	return executeLocalAIConfiguredEmbedOutputMapWithContext(
		t, context.Background(), process, factoryDir, environment, serverURL, outputMapping, outputPath,
	)
}

func executeLocalAIConfiguredEmbedOutputMapWithContext(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL, outputMapping, outputPath string,
) localAIConfiguredEmbedOutputMapResult {
	t.Helper()
	args := []string{
		"you", "--json", "--server", strings.TrimSuffix(serverURL, "/"), "models", "invoke",
		models.BuiltInModelNameEmbed, "--operation", models.OperationEMBED,
	}
	args = append(args, "--input", localAIConfiguredEmbedInputSpecs(t, true)[0])
	args = append(args, "--input", localAIConfiguredEmbedInputSpecs(t, true)[1])
	args = append(args, "--output", outputMapping)
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = factoryDir
	var stdout, stderr bytes.Buffer
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := process.Execute(inputs.Input)
	result := localAIConfiguredEmbedOutputMapResult{
		Stdout: stdout.String(), Stderr: stderr.String(), Err: err,
	}
	if contents, readErr := os.ReadFile(outputPath); readErr == nil {
		result.Output = string(contents)
	} else if !os.IsNotExist(readErr) {
		t.Fatalf("read configured-server output mapping %q: %v", outputPath, readErr)
	}
	return result
}

func executeLocalAIConfiguredEmbedCommandWithContext(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	factoryDir string,
	environment []string,
	serverURL string,
) localAIConfiguredEmbedInvokeResult {
	t.Helper()
	args := []string{"you", "--json", "--server", strings.TrimSuffix(serverURL, "/"), "models", "invoke", models.BuiltInModelNameEmbed, "--operation", models.OperationEMBED}
	for _, inputSpec := range localAIConfiguredEmbedInputSpecs(t, true) {
		args = append(args, "--input", inputSpec)
	}
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = append([]string(nil), environment...)
	inputs.Input.WorkingDirectory = factoryDir
	var stdout, stderr bytes.Buffer
	inputs.Input.Stdout = &stdout
	inputs.Input.Stderr = &stderr
	err := process.Execute(inputs.Input)
	return localAIConfiguredEmbedInvokeResult{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}

// The blocking backend is a controlled external edge. It withholds the
// response until the caller context is canceled, then records that the
// cancellation reached the server-owned backend invocation.
type localAIConfiguredEmbedBlockingBackend struct {
	mu       sync.Mutex
	next     serviceedges.ModelInvocationBackend
	blocked  *localAIConfiguredEmbedBlock
	canceled <-chan struct{}
}

type localAIConfiguredEmbedBlock struct {
	started  chan<- struct{}
	canceled chan struct{}
}

func (backend *localAIConfiguredEmbedBlockingBackend) BlockNext(started chan<- struct{}) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.blocked = &localAIConfiguredEmbedBlock{
		started: started, canceled: make(chan struct{}),
	}
	backend.canceled = backend.blocked.canceled
}

func (backend *localAIConfiguredEmbedBlockingBackend) Canceled() <-chan struct{} {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.canceled
}

func (backend *localAIConfiguredEmbedBlockingBackend) Invoke(
	ctx context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	backend.mu.Lock()
	blocked := backend.blocked
	backend.blocked = nil
	next := backend.next
	backend.mu.Unlock()
	if blocked != nil {
		close(blocked.started)
		<-ctx.Done()
		close(blocked.canceled)
		return nil, nil, ctx.Err()
	}
	return next(ctx, request)
}
