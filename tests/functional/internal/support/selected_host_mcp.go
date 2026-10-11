package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

// Remote clients share initialized operator storage. Each connection still owns
// its context, pipes and working directory; real hosts retain their own stores.
type SelectedHostClientProcess struct {
	Process
	environment []string
}

// Environment returns the prepared profile for additional public client setup.
func (process SelectedHostClientProcess) Environment() []string {
	return append([]string(nil), process.environment...)
}

func (process SelectedHostClientProcess) Execute(input root.Input) error {
	input.Env = append([]string(nil), process.environment...)
	return process.Process.Execute(input)
}

func NewSelectedHostClientProcess(t *testing.T) SelectedHostClientProcess {
	t.Helper()
	home := t.TempDir()
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") || strings.EqualFold(name, runcli.ModelCacheDirEnvironment) {
			continue
		}
		environment = append(environment, entry)
	}
	environment = append(environment, "HOME="+home, "USERPROFILE="+home,
		runcli.ModelCacheDirEnvironment+"="+filepath.Join(home, "models"))
	process := SelectedHostClientProcess{
		Process:     BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: rejectSelectedHostProvider{t: t}}),
		environment: environment,
	}
	// Finish bootstrap before any parallel connection can use this home.
	inputs := FakeInputs(t.Context(), []string{"you", "init", "--provider", "codex"})
	inputs.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("initialize remote-client home: %v\n%s", err, inputs.Stderr())
	}
	return process
}

type rejectSelectedHostProvider struct{ t *testing.T }

func (r rejectSelectedHostProvider) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.t.Error("selected-host MCP executed a local provider")
	return platformprocess.CommandResult{}, errors.New("local provider execution forbidden")
}
func StartSelectedHostMCP(t *testing.T, process Process, host string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	session, ctx, _ := StartCancellableSelectedHostMCP(t, process, host)
	return session, ctx
}

func StartCancellableSelectedHostMCP(t *testing.T, process Process, host string, workspace ...string) (*mcp.ClientSession, context.Context, context.CancelFunc) {
	t.Helper()
	// The session spans host shutdown/recovery and independent profile setup.
	// Bound individual protocol operations, rather than expiring the transport
	// while the scenario is preparing its next host.
	ctx, cancel := context.WithCancel(t.Context())
	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()
	done := make(chan error, 1)
	workDir := t.TempDir()
	if len(workspace) > 0 {
		workDir = workspace[0]
	}
	go func() {
		err := process.Execute(root.Input{
			Args: []string{"you", "--server", host, "server", "mcp"}, Context: ctx,
			WorkingDirectory: workDir,
			Stdin:            stdinRead, Stdout: stdoutWrite, Stderr: io.Discard,
		})
		// Command exit ends its protocol streams, including startup refusal.
		// Otherwise a client can remain blocked writing initialize to the pipe.
		_ = stdinRead.CloseWithError(err)
		_ = stdoutWrite.CloseWithError(err)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		_ = stdinWrite.Close()
		_ = stdinRead.Close()
		_ = stdoutWrite.Close()
		_ = stdoutRead.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("MCP execute: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("MCP execute did not join")
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "selected-host-parity", Version: "test"}, nil)
	connectCtx, cancelConnect := context.WithTimeout(ctx, 30*time.Second)
	defer cancelConnect()
	session, err := client.Connect(connectCtx, &mcp.IOTransport{Reader: stdoutRead, Writer: stdinWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx, cancel
}

func CallMCPTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func CallSubagentAction(t *testing.T, ctx context.Context, session *mcp.ClientSession, action string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	input := make(map[string]any, len(args)+1)
	for key, value := range args {
		input[key] = value
	}
	input["action"] = action
	return CallMCPTool(t, ctx, session, "you.subagent", input)
}

func GetHostJSON(t *testing.T, url string) any {
	t.Helper()
	return RequestHostJSON(t, http.MethodGet, url)
}

func RequestHostJSON(t *testing.T, method, url string) any {
	t.Helper()
	request, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var value any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func AssertJSONEqual(t *testing.T, actual, expected any) {
	t.Helper()
	left, _ := json.Marshal(actual)
	right, _ := json.Marshal(expected)
	var normalized any
	if err := json.Unmarshal(left, &normalized); err != nil {
		t.Fatal(err)
	}
	left, _ = json.Marshal(normalized)
	normalized = nil
	if err := json.Unmarshal(right, &normalized); err != nil {
		t.Fatal(err)
	}
	right, _ = json.Marshal(normalized)
	if !bytes.Equal(left, right) {
		t.Fatalf("MCP=%s HTTP=%s", left, right)
	}
}

func AssertMCPToolError(t *testing.T, result *mcp.CallToolResult, code string, retryable bool) {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || envelope.Error.Code != code || envelope.Error.Retryable != retryable || strings.Contains(string(encoded), "secret-from-host") {
		t.Fatalf("error result = %s isError=%v", encoded, result.IsError)
	}
}

func PostHostJSON(t *testing.T, ctx context.Context, url string, payload any, status int) any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status %d, want %d: %s", url, response.StatusCode, status, body)
	}
	var result any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func CapturedCodexSuccessOutput(t *testing.T) []byte {
	t.Helper()
	fixture := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(ProviderSessionFixturePath("codex", "success")))
	output, err := os.ReadFile(filepath.Join(fixture, "stdout.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func AssertSubagentAdmission(t *testing.T, result *mcp.CallToolResult, id string) {
	t.Helper()
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("START result: %+v", result)
	}
	encoded := result.Content[0].(*mcp.TextContent).Text
	var envelope struct {
		Result struct {
			ID       string `json:"workerSessionId"`
			Accepted bool   `json:"accepted"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(encoded), &envelope); err != nil {
		t.Fatal(err)
	}
	if result.IsError || !envelope.Result.Accepted || envelope.Result.ID != id {
		t.Fatalf("START admission: %s", encoded)
	}
}
