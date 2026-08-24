package root_composition_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const deliveredEmbedBackendName = "localai-backend-localai-llamacpp-functional.tar.gz"

// TestDeliveredEmbedCLIArtifactReachesProtocolFixture builds and runs the
// actual cmd/factory artifact. The fixture endpoint supplies both the model
// assets and the generic backend exchange; no real weights, backend process,
// release asset, or port-7437 service participates in this proof.
func TestDeliveredEmbedCLIArtifactReachesProtocolFixture(t *testing.T) {
	fixture := newDeliveredEmbedFixture(t)
	binary := buildDeliveredEmbedYouBinary(t)
	workDir := support.ScaffoldFactory(t, builtInOnlyModelFactoryConfig())
	home := t.TempDir()

	first := runDeliveredEmbedCLI(t, binary, workDir, home, fixture.server.URL,
		"models", "invoke", "embed", "--input", "text=Find similar work")
	if first.exitCode != 0 || string(first.stdout) != `[0.1,0.2,0.3,0.4]` || len(first.stderr) != 0 {
		t.Fatalf("delivered EMBED command exit=%d stdout=%q stderr=%q; want one vector and empty stderr", first.exitCode, first.stdout, first.stderr)
	}
	t.Logf("delivered command: you models invoke embed --input text=Find similar work")
	t.Logf("delivered output: exit=%d stdout=%q stderr=%q", first.exitCode, first.stdout, first.stderr)
	missCalls := fixture.assetCalls()
	if missCalls < 3 {
		t.Fatalf("delivered EMBED cache-miss asset calls = %d, want manifest, model, and backend exchanges", missCalls)
	}

	second := runDeliveredEmbedCLI(t, binary, workDir, home, fixture.server.URL,
		"--json", "models", "invoke", "embed", "--input", "text=Find similar work",
		"--input", `parameters=json:{"normalize":true}`)
	if second.exitCode != 0 || len(second.stderr) != 0 {
		t.Fatalf("delivered EMBED JSON command exit=%d stdout=%q stderr=%q", second.exitCode, second.stdout, second.stderr)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal(second.stdout, &response); err != nil {
		t.Fatalf("decode delivered EMBED JSON output: %v\n%s", err, second.stdout)
	}
	if len(response.Outputs) != 1 || response.Outputs[0].Name != "embedding" ||
		response.Outputs[0].Content == nil || *response.Outputs[0].Content != `[0.1,0.2,0.3,0.4]` {
		t.Fatalf("delivered EMBED JSON response = %#v, want one named embedding vector", response)
	}
	t.Logf("delivered command: you --json models invoke embed --input text=Find similar work --input parameters=json:{\"normalize\":true}")
	t.Logf("delivered output: exit=%d stdout=%q stderr=%q", second.exitCode, second.stdout, second.stderr)
	if got := fixture.assetCalls(); got != missCalls {
		t.Fatalf("delivered EMBED cache-hit asset calls = %d, want unchanged %d", got, missCalls)
	}

	invocations := fixture.invocations()
	if len(invocations) != 2 {
		t.Fatalf("delivered EMBED backend invocations = %d, want two", len(invocations))
	}
	if invocations[0].ModelName != "embed" || invocations[0].Operation != string(models.OperationEMBED) ||
		len(invocations[0].Inputs) != 1 || invocations[0].Inputs[0].Name != "text" ||
		invocations[0].Inputs[0].Content != "Find similar work" {
		t.Fatalf("delivered first backend request = %#v, want canonical text EMBED request", invocations[0])
	}
	if len(invocations[1].Inputs) != 2 || invocations[1].Inputs[1].Name != "parameters" ||
		invocations[1].Inputs[1].Content != `{"normalize":true}` {
		t.Fatalf("delivered second backend request = %#v, want ordered JSON parameters input", invocations[1])
	}
}

// TestDeliveredEmbedHTTPArtifactReachesProtocolFixture starts the real
// delivered server command and sends the generic HTTP request through it. The
// CLI and root-composition HTTP parity cells then cover both public entry
// points without substituting an in-process handler for the shipped binary.
func TestDeliveredEmbedHTTPArtifactReachesProtocolFixture(t *testing.T) {
	fixture := newDeliveredEmbedFixture(t)
	binary := buildDeliveredEmbedYouBinary(t)
	serverRoot := t.TempDir()
	factoryDir := filepath.Join(serverRoot, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create delivered server factory directory: %v", err)
	}
	factoryConfig, err := json.Marshal(builtInOnlyModelFactoryConfig())
	if err != nil {
		t.Fatalf("encode delivered server factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), factoryConfig, 0o644); err != nil {
		t.Fatalf("write delivered server factory config: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve delivered HTTP port: %v", err)
	}
	listenAddress := listener.Addr().String()
	_ = listener.Close()

	home := t.TempDir()
	command := exec.CommandContext(t.Context(), binary, "server", "--listen", listenAddress)
	command.Dir = serverRoot
	command.Env = deliveredEmbedCLIEnvironment(home, fixture.server.URL)
	var serverStdout, serverStderr bytes.Buffer
	command.Stdout = &serverStdout
	command.Stderr = &serverStderr
	if err := command.Start(); err != nil {
		t.Fatalf("start delivered HTTP server: %v", err)
	}
	serverStopped := false
	stopServer := func() {
		if serverStopped {
			return
		}
		serverStopped = true
		_ = command.Process.Kill()
		_ = command.Wait()
	}
	defer stopServer()

	textValue := "Find similar work"
	inputs := []factoryapi.ModelInvocationInput{{
		Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &textValue,
	}}
	payload, err := json.Marshal(factoryapi.GenericModelInvocationRequest{
		Scope: "factory-session:delivered-http", Holder: "delivered-embed-http",
		Model: factoryapi.ModelReference{NameOrUri: "embed"}, Inputs: &inputs,
	})
	if err != nil {
		t.Fatalf("encode delivered HTTP request: %v", err)
	}

	var response factoryapi.GenericModelInvocationResponse
	var responseBody []byte
	deadline := time.Now().Add(20 * time.Second)
	for {
		request, requestErr := http.NewRequestWithContext(t.Context(), http.MethodPost,
			"http://"+listenAddress+"/models/invocations", bytes.NewReader(payload))
		if requestErr != nil {
			t.Fatalf("build delivered HTTP request: %v", requestErr)
		}
		request.Header.Set("Content-Type", "application/json")
		httpResponse, doErr := http.DefaultClient.Do(request)
		if doErr == nil {
			responseBody, _ = io.ReadAll(httpResponse.Body)
			_ = httpResponse.Body.Close()
			if httpResponse.StatusCode == http.StatusOK {
				if err := json.Unmarshal(responseBody, &response); err != nil {
					t.Fatalf("decode delivered HTTP response: %v\n%s", err, responseBody)
				}
				break
			}
		}
		if time.Now().After(deadline) {
			stopServer()
			t.Fatalf("delivered HTTP request did not become ready: error=%v status/body=%q server-stdout=%q server-stderr=%q", doErr, responseBody, serverStdout.String(), serverStderr.String())
		}
		// The shipped server is a real OS process; readiness is observable only
		// through its HTTP boundary, so this bounded retry is the process-level
		// synchronization required by this proof cell.
		time.Sleep(100 * time.Millisecond)
	}
	stopServer()
	t.Logf("delivered HTTP command: POST /models/invocations payload=%s", payload)
	t.Logf("delivered HTTP output: response=%s server-stdout=%q server-stderr=%q", responseBody, serverStdout.String(), serverStderr.String())
	assertDeliveredEmbedHTTPResponse(t, response)
	if fixture.assetCalls() < 3 {
		t.Fatalf("delivered HTTP cache-miss asset calls = %d, want manifest, model, and backend exchanges", fixture.assetCalls())
	}
}

func assertDeliveredEmbedHTTPResponse(t *testing.T, response factoryapi.GenericModelInvocationResponse) {
	t.Helper()
	if len(response.Outputs) != 1 {
		t.Fatalf("delivered HTTP outputs = %#v, want one named JSON vector", response.Outputs)
	}
	output := response.Outputs[0]
	if output.Name != "embedding" || output.Modality != factoryapi.ModelInvocationContentTypeJSON ||
		output.Content == nil || *output.Content != `[0.1,0.2,0.3,0.4]` {
		t.Fatalf("delivered HTTP response = %#v, want one named JSON vector", response)
	}
}

type deliveredEmbedCLIResult struct {
	exitCode int
	stdout   []byte
	stderr   []byte
}

func buildDeliveredEmbedYouBinary(t *testing.T) string {
	t.Helper()
	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	command := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binaryPath, "./cmd/factory")
	command.Dir = testutil.MustRepoRoot(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build delivered you binary: %v\n%s", err, output)
	}
	return binaryPath
}

func runDeliveredEmbedCLI(t *testing.T, binary, workDir, home, endpoint string, args ...string) deliveredEmbedCLIResult {
	t.Helper()
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Dir = workDir
	command.Env = deliveredEmbedCLIEnvironment(home, endpoint)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := deliveredEmbedCLIResult{stdout: stdout.Bytes(), stderr: stderr.Bytes()}
	if err == nil {
		return result
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("run delivered EMBED CLI: %v", err)
	}
	result.exitCode = exitError.ExitCode()
	return result
}

func deliveredEmbedCLIEnvironment(home, endpoint string) []string {
	keys := []string{"HOME", "USERPROFILE", "YOU_MODELS_BACKEND_ENDPOINT"}
	environment := make([]string, 0, len(os.Environ())+len(keys))
	for _, value := range os.Environ() {
		remove := false
		for _, key := range keys {
			if strings.HasPrefix(value, key+"=") {
				remove = true
				break
			}
		}
		if !remove {
			environment = append(environment, value)
		}
	}
	return append(environment, "HOME="+home, "USERPROFILE="+home, "YOU_MODELS_BACKEND_ENDPOINT="+endpoint)
}

type deliveredEmbedInput struct {
	Name          string
	Modality      string
	ContentType   string
	MediaType     string
	ContentBase64 string
	Content       string
}

type deliveredEmbedInvocation struct {
	ModelName string
	Operation string
	Inputs    []deliveredEmbedInput
}

type deliveredEmbedFixture struct {
	server *httptest.Server
	mu     sync.Mutex
	assets []string
	inputs []deliveredEmbedInvocation
}

func newDeliveredEmbedFixture(t *testing.T) *deliveredEmbedFixture {
	t.Helper()
	fixture := &deliveredEmbedFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *deliveredEmbedFixture) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	switch {
	case request.URL.Path == "/invoke":
		fixture.serveInvocation(writer, request)
	case strings.HasSuffix(request.URL.Path, "/models/Qwen/Qwen3-Embedding-0.6B"):
		fixture.serveModelManifest(writer, request)
	case strings.Contains(request.URL.Path, "/Qwen/Qwen3-Embedding-0.6B/resolve/"):
		fixture.recordAsset(request)
		writeDeliveredEmbedBytes(writer, http.StatusOK, []byte("story-004-embedding-model-download"), "application/octet-stream")
	case strings.HasSuffix(request.URL.Path, "/"+deliveredEmbedBackendName):
		fixture.recordAsset(request)
		writeDeliveredEmbedBytes(writer, http.StatusOK, []byte("story-004-localai-backend"), "application/octet-stream")
	default:
		http.NotFound(writer, request)
	}
}

func (fixture *deliveredEmbedFixture) serveModelManifest(writer http.ResponseWriter, request *http.Request) {
	fixture.recordAsset(request)
	modelBody := []byte("story-004-embedding-model-download")
	digest := fmt.Sprintf("%x", sha256.Sum256(modelBody))
	payload := map[string]any{
		"sha": "97b0c614be4d77ee51c0cef4e5f07c00f9eb65b3",
		"siblings": []map[string]any{{
			"rfilename": "Qwen3-Embedding-0.6B.gguf", "size": len(modelBody),
			"lfs": map[string]any{"oid": digest, "size": len(modelBody)},
		}},
	}
	writeDeliveredEmbedJSON(writer, http.StatusOK, payload)
}

func (fixture *deliveredEmbedFixture) serveInvocation(writer http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()
	var input struct {
		ModelName string `json:"modelName"`
		Operation string `json:"operation"`
		Inputs    []struct {
			Name          string `json:"name"`
			Modality      string `json:"modality"`
			ContentType   string `json:"contentType"`
			MediaType     string `json:"mediaType"`
			ContentBase64 string `json:"contentBase64"`
		} `json:"inputs"`
	}
	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	invocation := deliveredEmbedInvocation{ModelName: input.ModelName, Operation: input.Operation}
	for _, value := range input.Inputs {
		content, err := base64.StdEncoding.DecodeString(value.ContentBase64)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		invocation.Inputs = append(invocation.Inputs, deliveredEmbedInput{
			Name: value.Name, Modality: value.Modality, ContentType: value.ContentType,
			MediaType: value.MediaType, ContentBase64: value.ContentBase64, Content: string(content),
		})
	}
	fixture.mu.Lock()
	fixture.inputs = append(fixture.inputs, invocation)
	fixture.mu.Unlock()
	writeDeliveredEmbedJSON(writer, http.StatusOK, map[string]any{
		"outputs": []map[string]any{{
			"name": "embedding", "modality": string(models.ModalityJSON),
			"contentType": "application/json", "mediaType": "application/json",
			"contentBase64": base64.StdEncoding.EncodeToString([]byte(`[0.1,0.2,0.3,0.4]`)),
		}},
	})
}

func (fixture *deliveredEmbedFixture) recordAsset(request *http.Request) {
	fixture.mu.Lock()
	fixture.assets = append(fixture.assets, request.URL.String())
	fixture.mu.Unlock()
}

func (fixture *deliveredEmbedFixture) assetCalls() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return len(fixture.assets)
}

func (fixture *deliveredEmbedFixture) invocations() []deliveredEmbedInvocation {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	result := make([]deliveredEmbedInvocation, len(fixture.inputs))
	copy(result, fixture.inputs)
	return result
}

func writeDeliveredEmbedJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeDeliveredEmbedBytes(writer http.ResponseWriter, status int, body []byte, contentType string) {
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
}
