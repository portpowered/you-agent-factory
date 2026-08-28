package model_invoke_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBuiltCLIExplicitServerAdverseInputMatrix keeps the adverse remote input
// proof at the OS-process boundary. The package-level tests prove context and
// typed-error identity; this witness proves a fresh binary cannot turn those
// server outcomes into stdout success and that concurrent processes keep their
// file-backed bytes isolated.
func TestBuiltCLIExplicitServerAdverseInputMatrix(t *testing.T) {
	binary := buildExplicitServerAdverseBinary(t)
	workDir := t.TempDir()
	homeDir := t.TempDir()
	firstImage := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff, 0x01}
	secondImage := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x10, 0x80, 0x02}
	firstPath := writeExplicitServerImage(t, workDir, "first.png", firstImage)
	secondPath := writeExplicitServerImage(t, workDir, "second.png", secondImage)

	fixture := newBuiltExplicitServerFixture(t)
	endpoint := fixture.server.URL + "?token=built-process-secret"

	fixture.setMode(builtExplicitServerSuccess)
	success := runExplicitServerCLI(t, t.Context(), binary, workDir, homeDir, endpoint,
		"models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=first prompt", "--input", "image=@"+firstPath,
	)
	if success.err != nil || success.exitCode != 0 {
		t.Fatalf("built happy invocation exit=%d err=%v stdout=%q stderr=%q", success.exitCode, success.err, success.stdout, success.stderr)
	}
	if string(success.stdout) != "controlled response-"+hashExplicitServerBytes(firstImage) || len(success.stderr) != 0 {
		t.Fatalf("built happy streams stdout=%q stderr=%q, want exact response and empty stderr", success.stdout, success.stderr)
	}
	fixture.assertLastInput(t, "first prompt", firstImage)

	fixture.setMode(builtExplicitServerTypedFailure)
	typed := runExplicitServerCLI(t, t.Context(), binary, workDir, homeDir, endpoint,
		"models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=typed failure", "--input", "image=@"+firstPath,
	)
	assertBuiltExplicitServerFailure(t, typed, "MODEL_BACKEND_NOT_READY", factoryapi.ErrorFamilyInternalServerError, "built typed server failure")

	fixture.setMode(builtExplicitServerMalformed)
	malformed := runExplicitServerCLI(t, t.Context(), binary, workDir, homeDir, endpoint,
		"models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=malformed", "--input", "image=@"+firstPath,
	)
	assertBuiltExplicitServerFailure(t, malformed, "MODEL_BACKEND_FAILURE", factoryapi.ErrorFamilyInternalServerError, "")

	fixture.setMode(builtExplicitServerRecovery)
	firstRecovery := runExplicitServerCLI(t, t.Context(), binary, workDir, homeDir, endpoint,
		"models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=recovery", "--input", "image=@"+firstPath,
	)
	assertBuiltExplicitServerFailure(t, firstRecovery, "MODEL_BACKEND_NOT_READY", factoryapi.ErrorFamilyInternalServerError, "built recovery first failure")
	secondRecovery := runExplicitServerCLI(t, t.Context(), binary, workDir, homeDir, endpoint,
		"models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=recovery", "--input", "image=@"+firstPath,
	)
	if secondRecovery.err != nil || secondRecovery.exitCode != 0 ||
		string(secondRecovery.stdout) != "recovered response-"+hashExplicitServerBytes(firstImage) || len(secondRecovery.stderr) != 0 {
		t.Fatalf("built recovery second invocation exit=%d err=%v stdout=%q stderr=%q", secondRecovery.exitCode, secondRecovery.err, secondRecovery.stdout, secondRecovery.stderr)
	}

	fixture.setMode(builtExplicitServerConcurrent)
	concurrentContext, cancelConcurrent := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancelConcurrent()
	results := make(chan builtExplicitServerCLIResult, 2)
	go func() {
		results <- runExplicitServerCLI(t, concurrentContext, binary, workDir, homeDir, fixture.server.URL,
			"models", "invoke", "llm", "--operation", "OMNI",
			"--input", "prompt=first", "--input", "image=@"+firstPath,
		)
	}()
	go func() {
		results <- runExplicitServerCLI(t, concurrentContext, binary, workDir, homeDir, fixture.server.URL,
			"models", "invoke", "llm", "--operation", "OMNI",
			"--input", "prompt=second", "--input", "image=@"+secondPath,
		)
	}()
	select {
	case <-fixture.concurrentReady:
		close(fixture.concurrentRelease)
	case <-concurrentContext.Done():
		t.Fatalf("built concurrent invocations did not reach the server: %v", concurrentContext.Err())
	}
	for range 2 {
		result := <-results
		if result.err != nil || result.exitCode != 0 || len(result.stderr) != 0 {
			t.Fatalf("built concurrent invocation exit=%d err=%v stdout=%q stderr=%q", result.exitCode, result.err, result.stdout, result.stderr)
		}
		wantFirst := "controlled response-" + hashExplicitServerBytes(firstImage)
		wantSecond := "controlled response-" + hashExplicitServerBytes(secondImage)
		if string(result.stdout) != wantFirst && string(result.stdout) != wantSecond {
			t.Fatalf("built concurrent stdout=%q, want one exact response %q or %q", result.stdout, wantFirst, wantSecond)
		}
	}
	fixture.assertConcurrentInputs(t)

	fixture.setMode(builtExplicitServerBlock)
	cancelContext, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cancelledResult := make(chan builtExplicitServerCLIResult, 1)
	go func() {
		cancelledResult <- runExplicitServerCLI(t, cancelContext, binary, workDir, homeDir, fixture.server.URL,
			"models", "invoke", "llm", "--operation", "OMNI",
			"--input", "prompt=cancelled", "--input", "image=@"+firstPath,
		)
	}()
	select {
	case <-fixture.blockRequestStarted:
		cancel()
	case <-cancelContext.Done():
		t.Fatalf("built cancellation invocation did not reach the server: %v", cancelContext.Err())
	}
	var cancelled builtExplicitServerCLIResult
	select {
	case cancelled = <-cancelledResult:
	case <-time.After(2 * time.Second):
		t.Fatal("built cancellation invocation did not return after context cancellation")
	}
	close(fixture.blockRelease)
	if cancelled.err == nil || cancelled.exitCode == 0 || len(cancelled.stdout) != 0 {
		t.Fatalf("built cancellation exit=%d err=%v stdout=%q stderr=%q, want nonzero without success output", cancelled.exitCode, cancelled.err, cancelled.stdout, cancelled.stderr)
	}
	select {
	case <-fixture.blockRequestDone:
	case <-time.After(2 * time.Second):
		t.Fatal("built cancellation did not close the blocked request")
	}

	fixture.server.Close()
	unreachable := runExplicitServerCLI(t, t.Context(), binary, workDir, homeDir, endpoint,
		"models", "invoke", "llm", "--operation", "OMNI",
		"--input", "prompt=unreachable", "--input", "image=@"+firstPath,
	)
	if unreachable.err == nil || unreachable.exitCode == 0 || len(unreachable.stdout) != 0 || strings.Contains(string(unreachable.stderr), "built-process-secret") {
		t.Fatalf("built unreachable exit=%d err=%v stdout=%q stderr=%q, want safe non-success without token", unreachable.exitCode, unreachable.err, unreachable.stdout, unreachable.stderr)
	}
	support.RequireSafeCLIDiagnostic(t, string(unreachable.stderr))

	t.Logf("fresh binary=%s server=%s success-sha256=%s", binary, fixture.server.URL, hashExplicitServerBytes(firstImage))
}

type builtExplicitServerMode string

const (
	builtExplicitServerSuccess      builtExplicitServerMode = "success"
	builtExplicitServerTypedFailure builtExplicitServerMode = "typed-failure"
	builtExplicitServerMalformed    builtExplicitServerMode = "malformed"
	builtExplicitServerRecovery     builtExplicitServerMode = "recovery"
	builtExplicitServerConcurrent   builtExplicitServerMode = "concurrent"
	builtExplicitServerBlock        builtExplicitServerMode = "block"
)

type builtExplicitServerFixture struct {
	server *httptest.Server

	mu                  sync.Mutex
	mode                builtExplicitServerMode
	received            []builtExplicitServerInput
	handlerErrors       []error
	concurrentCalls     atomic.Int32
	concurrentPostCalls atomic.Int32
	concurrentReady     chan struct{}
	concurrentRelease   chan struct{}
	blockRequestStarted chan struct{}
	blockRelease        chan struct{}
	blockRequestDone    chan struct{}
	postCalls           atomic.Int32
}

type builtExplicitServerInput struct {
	prompt string
	image  []byte
}

func newBuiltExplicitServerFixture(t *testing.T) *builtExplicitServerFixture {
	t.Helper()
	fixture := &builtExplicitServerFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(func() {
		fixture.server.Close()
	})
	return fixture
}

func (fixture *builtExplicitServerFixture) setMode(mode builtExplicitServerMode) {
	fixture.mu.Lock()
	fixture.mode = mode
	fixture.received = nil
	fixture.handlerErrors = nil
	fixture.concurrentCalls.Store(0)
	fixture.concurrentPostCalls.Store(0)
	fixture.concurrentReady = make(chan struct{})
	fixture.concurrentRelease = make(chan struct{})
	fixture.blockRequestStarted = make(chan struct{})
	fixture.blockRelease = make(chan struct{})
	fixture.blockRequestDone = make(chan struct{})
	fixture.postCalls.Store(0)
	fixture.mu.Unlock()
}

func (fixture *builtExplicitServerFixture) currentMode() builtExplicitServerMode {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.mode
}

func (fixture *builtExplicitServerFixture) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		writeBuiltExplicitServerCatalog(writer)
	case http.MethodPost:
		fixture.postCalls.Add(1)
		mode := fixture.currentMode()
		switch mode {
		case builtExplicitServerTypedFailure:
			writeBuiltExplicitServerFailure(writer, http.StatusServiceUnavailable, "MODEL_BACKEND_NOT_READY", factoryapi.ErrorFamilyInternalServerError, "built typed server failure")
		case builtExplicitServerMalformed:
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"outputs":[{"name":"text","modality":"TEXT","content":"built response"}]} {}`))
		case builtExplicitServerRecovery:
			if fixture.postCalls.Load() == 1 {
				writeBuiltExplicitServerFailure(writer, http.StatusServiceUnavailable, "MODEL_BACKEND_NOT_READY", factoryapi.ErrorFamilyInternalServerError, "built recovery first failure")
				return
			}
			fixture.writeSuccess(writer, request, "recovered response-")
		case builtExplicitServerConcurrent:
			fixture.concurrentPostCalls.Add(1)
			if fixture.concurrentCalls.Add(1) == 2 {
				close(fixture.concurrentReady)
			}
			select {
			case <-fixture.concurrentRelease:
				fixture.writeSuccess(writer, request, "controlled response-")
			case <-request.Context().Done():
			}
		case builtExplicitServerBlock:
			close(fixture.blockRequestStarted)
			select {
			case <-request.Context().Done():
			case <-fixture.blockRelease:
			}
			close(fixture.blockRequestDone)
		default:
			fixture.writeSuccess(writer, request, "controlled response-")
		}
	default:
		http.NotFound(writer, request)
	}
}

func (fixture *builtExplicitServerFixture) writeSuccess(writer http.ResponseWriter, request *http.Request, prefix string) {
	input, err := decodeBuiltExplicitServerInput(request)
	if err != nil {
		fixture.recordHandlerError(err)
		http.Error(writer, "invalid invocation request", http.StatusBadRequest)
		return
	}
	fixture.mu.Lock()
	fixture.received = append(fixture.received, input)
	fixture.mu.Unlock()
	answer := prefix + hashExplicitServerBytes(input.image)
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(factoryapi.GenericModelInvocationResponse{
		Outputs: []factoryapi.ModelInvocationOutput{{
			Name: "text", Modality: factoryapi.ModelInvocationContentTypeText, Content: &answer,
		}},
	})
}

func decodeBuiltExplicitServerInput(request *http.Request) (builtExplicitServerInput, error) {
	defer request.Body.Close()
	var payload factoryapi.GenericModelInvocationRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		return builtExplicitServerInput{}, fmt.Errorf("decode generic request: %w", err)
	}
	if payload.Model.NameOrUri != "llm" || payload.Operation == nil || *payload.Operation != "OMNI" || payload.Inputs == nil || len(*payload.Inputs) != 2 {
		return builtExplicitServerInput{}, errors.New("generic request did not preserve model, operation, and two inputs")
	}
	inputs := *payload.Inputs
	if inputs[0].Name != "prompt" || inputs[0].Content == nil || inputs[1].Name != "image" || inputs[1].ContentBase64 == nil {
		return builtExplicitServerInput{}, errors.New("generic request did not preserve ordered prompt/image carriers")
	}
	return builtExplicitServerInput{prompt: *inputs[0].Content, image: append([]byte(nil), (*inputs[1].ContentBase64)...)}, nil
}

func (fixture *builtExplicitServerFixture) recordHandlerError(err error) {
	fixture.mu.Lock()
	fixture.handlerErrors = append(fixture.handlerErrors, err)
	fixture.mu.Unlock()
}

func (fixture *builtExplicitServerFixture) assertLastInput(t *testing.T, wantPrompt string, wantImage []byte) {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.handlerErrors) != 0 {
		t.Fatalf("built server handler errors = %v", fixture.handlerErrors)
	}
	if len(fixture.received) == 0 {
		t.Fatal("built server received no successful input")
	}
	got := fixture.received[len(fixture.received)-1]
	if got.prompt != wantPrompt || !bytes.Equal(got.image, wantImage) {
		t.Fatalf("built server input = prompt %q image sha256 %s, want prompt %q image sha256 %s", got.prompt, hashExplicitServerBytes(got.image), wantPrompt, hashExplicitServerBytes(wantImage))
	}
}

func (fixture *builtExplicitServerFixture) assertConcurrentInputs(t *testing.T) {
	t.Helper()
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.handlerErrors) != 0 {
		t.Fatalf("built concurrent server handler errors = %v", fixture.handlerErrors)
	}
	if len(fixture.received) != 2 {
		t.Fatalf("built concurrent server received %d inputs, want 2", len(fixture.received))
	}
	if fixture.concurrentPostCalls.Load() != 2 {
		t.Fatalf("built concurrent POST calls = %d, want exactly one per process", fixture.concurrentPostCalls.Load())
	}
	seen := make(map[string]struct{}, len(fixture.received))
	for _, input := range fixture.received {
		seen[input.prompt+":"+hashExplicitServerBytes(input.image)] = struct{}{}
	}
	for _, want := range []builtExplicitServerInput{
		{prompt: "first", image: []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff, 0x01}},
		{prompt: "second", image: []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x10, 0x80, 0x02}},
	} {
		if _, ok := seen[want.prompt+":"+hashExplicitServerBytes(want.image)]; !ok {
			t.Fatalf("built concurrent inputs = %#v, missing prompt %q image sha256 %s", fixture.received, want.prompt, hashExplicitServerBytes(want.image))
		}
	}
}

func writeBuiltExplicitServerCatalog(writer http.ResponseWriter) {
	required := true
	repeatable := true
	textModality := factoryapi.ModelInvocationContentTypeText
	imageModality := factoryapi.ModelInvocationContentTypeImage
	promptMedia := []string{"text/plain"}
	imageMedia := []string{"image/*"}
	inputs := []factoryapi.ModelInvocationSlot{
		{Name: "prompt", Modality: &textModality, Required: &required, MediaTypes: &promptMedia},
		{Name: "image", Modality: &imageModality, Required: &required, Repeatable: &repeatable, MediaTypes: &imageMedia},
	}
	outputs := []factoryapi.ModelInvocationSlot{{Name: "text", Modality: &textModality}}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(factoryapi.ModelDetail{
		Name: "llm",
		Operations: []factoryapi.ModelInvocationOperation{{
			Name: "OMNI", Inputs: &inputs, Outputs: &outputs,
		}},
	})
}

func writeBuiltExplicitServerFailure(writer http.ResponseWriter, status int, code string, family factoryapi.ErrorFamily, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
		Code: factoryapi.ErrorResponseCode(code), Family: family, Message: message,
	})
}

type builtExplicitServerCLIResult struct {
	exitCode int
	stdout   []byte
	stderr   []byte
	err      error
}

func buildExplicitServerAdverseBinary(t *testing.T) string {
	t.Helper()
	name := "you"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	command := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", path, "./cmd/factory")
	command.Dir = testutil.MustRepoRoot(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build fresh explicit-server binary: %v\n%s", err, output)
	}
	t.Logf("fresh explicit-server binary: go build -buildvcs=false -o %s ./cmd/factory", path)
	return path
}

func runExplicitServerCLI(t *testing.T, ctx context.Context, binary, workDir, homeDir, endpoint string, args ...string) builtExplicitServerCLIResult {
	t.Helper()
	commandArgs := append([]string{"--server", endpoint}, args...)
	command := exec.CommandContext(ctx, binary, commandArgs...)
	command.Dir = workDir
	command.Env = explicitServerCLIEnvironment(homeDir)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := builtExplicitServerCLIResult{stdout: stdout.Bytes(), stderr: stderr.Bytes(), err: err}
	if err == nil {
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
	}
	return result
}

func explicitServerCLIEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "HOME=") || strings.HasPrefix(value, "USERPROFILE=") || strings.HasPrefix(value, "YOU_MODELS_BACKEND_ENDPOINT=") {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func assertBuiltExplicitServerFailure(t *testing.T, result builtExplicitServerCLIResult, wantCode string, wantFamily factoryapi.ErrorFamily, wantMessage string) {
	t.Helper()
	if result.err == nil || result.exitCode == 0 || len(result.stdout) != 0 {
		t.Fatalf("built failure exit=%d err=%v stdout=%q stderr=%q, want nonzero/no stdout", result.exitCode, result.err, result.stdout, result.stderr)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(bytes.TrimSpace(result.stderr), &response); err != nil {
		t.Fatalf("decode built failure diagnostic: %v stderr=%q", err, result.stderr)
	}
	if response.Code != factoryapi.ErrorResponseCode(wantCode) || response.Family != wantFamily {
		t.Fatalf("built failure diagnostic = %#v, want code/family %s/%s", response, wantCode, wantFamily)
	}
	if wantMessage != "" && response.Message != wantMessage {
		t.Fatalf("built failure message = %q, want %q", response.Message, wantMessage)
	}
}

func writeExplicitServerImage(t *testing.T, directory, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write explicit-server image %s: %v", path, err)
	}
	return path
}

func hashExplicitServerBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
