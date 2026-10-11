package acceptance

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	acpfixture "github.com/portpowered/infinite-you/tests/functional/workers/invoke_continue/testdata/acpfixture"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// A distinct immutable ACP edge shape shares one selected host. Direct sessions
// own their directory, protocol peer and identity; no Factory Session is opened
// per START. The stdio peer withholds completion without holding a fixture lock.
func TestSubagentOpenCodeDefault(t *testing.T) {
	t.Parallel()
	functionalevidence.Covers(t, "cli/you.init", "cli/you.worker-sessions.start", "rest/startWorkerSession", "rest/getWorkerSessionObservationByWorkerSessionId")
	root := t.TempDir()
	hostDir, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	peers := map[string]*subagentACPPeer{}
	cases := []string{"default", "model-override", "unadvertised-model", "default-http", "default-mcp", "model-override-http", "model-override-mcp", "provider-override-cli", "provider-override-http", "provider-override-mcp"}
	for _, name := range cases {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		peers[dir] = &subagentACPPeer{observed: make(chan string, 2)}
	}
	host, err := startInvokeContinuePackageProcessWithEdges(t, hostDir, home, &invokeContinueStaticCommandRoute{}, serviceedges.Edges{
		PlatformProcessCommandFactory: acpfixture.CommandFactory(),
		ProvidersExecutableLocator:    continuationACPLocator{},
		ProviderCatalogProbe: func(_ context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
			descriptor.Readiness = providers.ReadinessReady
			return descriptor, nil
		},
		ProvidersStdioPipeFactory: acpfixture.StdioFactory(func(dir string, reader io.Reader, writer io.Writer) {
			if peer := peers[dir]; peer != nil {
				peer.starts.Add(1)
				peer.serve(reader, writer)
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := host.command.stop(); err != nil {
			t.Error(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := host.process.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	profile := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"generic-model","futureDefault":"retained","subagent":{"provider":"opencode","model":"controlled-free-model"}},"futureRoot":{"owner":"retained"},"messaging":{"enabled":false,"retentionDays":7,"limits":{"maxHop":0}}}`)
	if err := os.WriteFile(configPath, profile, 0o600); err != nil {
		t.Fatal(err)
	}
	profile = preserveSavedSubagentRoute(t, host, home, hostDir, configPath, profile)
	baseline := subagentFactorySessionIDs(t, host.baseURL)
	t.Cleanup(func() {
		if after := subagentFactorySessionIDs(t, host.baseURL); !slices.Equal(after, baseline) {
			t.Error("direct START changed Factory Session membership")
		}
	})
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertSubagentACPDefault(t, host, home, root, name, peers, configPath, profile)
		})
	}
}

// Configure through the supported isolated file surface, then exercise the
// public generic-settings update before START reloads the saved dedicated route.
// This document-only phase precedes parallel admissions and owns no session.
func preserveSavedSubagentRoute(t *testing.T, host invokeContinueStartedProcess, home, dir, path string, before []byte) []byte {
	t.Helper()
	input := support.FakeInputs(t.Context(), []string{"you", "init", "--provider", "claude", "--model", "updated-generic-model"})
	input.Input.Env, input.Input.WorkingDirectory = invokeContinueEnvironment(home), dir
	if err := host.process.Execute(input.Input); err != nil {
		t.Fatalf("generic settings update: %v %s %s", err, input.Stdout(), input.Stderr())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var original, saved map[string]any
	if err := json.Unmarshal(before, &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &saved); err != nil {
		t.Fatal(err)
	}
	defaults := saved["defaults"].(map[string]any)
	wantRoute := map[string]any{"provider": "OPENCODE", "model": "controlled-free-model"}
	if !reflect.DeepEqual(defaults["subagent"], wantRoute) || defaults["workerModelProvider"] != "claude" || defaults["workerModel"] != "updated-generic-model" ||
		defaults["futureDefault"] != original["defaults"].(map[string]any)["futureDefault"] || !reflect.DeepEqual(saved["futureRoot"], original["futureRoot"]) || !reflect.DeepEqual(saved["messaging"], original["messaging"]) {
		t.Fatalf("settings update did not preserve the dedicated route and siblings: %s", after)
	}
	return after
}

func assertSubagentACPDefault(t *testing.T, host invokeContinueStartedProcess, home, root, name string, peers map[string]*subagentACPPeer, configPath string, profile []byte) {
	t.Helper()
	dir := filepath.Join(root, name)
	peer := peers[dir]
	model := "controlled-free-model"
	if strings.HasPrefix(name, "model-override") || strings.HasPrefix(name, "provider-override") {
		model = "controlled-override"
	}
	if name == "unadvertised-model" {
		model = "unadvertised-model"
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	mode := "cli"
	if strings.HasSuffix(name, "-http") {
		mode = "http"
	}
	if strings.HasSuffix(name, "-mcp") {
		mode = "mcp"
	}
	body := executeSubagentPrompt(t, host, home, dir, mode, subagentACPDocument(name, dir, model), http.StatusAccepted)
	if !strings.Contains(body, `"accepted":true`) || !strings.Contains(body, `"workerSessionId":"`+name+`"`) {
		t.Fatalf("%s START = %s", mode, body)
	}
	if name == "unadvertised-model" {
		assertSubagentDeadlineObservation(t, ctx, host.baseURL, name, "FAILED", "family=terminal type=permanent_bad_request")
		if peer.prompts.Load() != 0 || peer.starts.Load() != 1 {
			t.Fatalf("invalid model executed inference or fallback: starts=%d prompts=%d", peer.starts.Load(), peer.prompts.Load())
		}
		return
	}
	select {
	case selected := <-peer.observed:
		if selected != model {
			t.Fatalf("selected ACP model = %q, want %q", selected, model)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	assertSubagentDeadlineObservation(t, ctx, host.baseURL, name, "RUNNING", "")
	status, observation := t7HTTP(t, ctx, http.MethodGet, host.baseURL+"/worker-sessions/"+name, nil)
	var admitted struct {
		Direct bool    `json:"direct"`
		WorkID *string `json:"workId"`
		Model  string  `json:"model"`
	}
	if status != http.StatusOK || json.Unmarshal([]byte(observation), &admitted) != nil || !admitted.Direct || admitted.WorkID != nil || admitted.Model != model {
		t.Fatal("valid ACP START lost direct identity or requested model")
	}
	assertSubagentACPReplay(t, ctx, host.baseURL, name, dir, model, peer)
	after, err := os.ReadFile(configPath)
	if err != nil || string(after) != string(profile) {
		t.Fatalf("START changed operator defaults/siblings: %v", err)
	}
}

func assertSubagentACPReplay(t *testing.T, ctx context.Context, baseURL, id, dir, model string, peer *subagentACPPeer) {
	t.Helper()
	document := subagentACPDocument(id, dir, model)
	var joined sync.WaitGroup
	for range 2 {
		joined.Add(1)
		go func() {
			defer joined.Done()
			status, body := t7HTTP(t, ctx, http.MethodPost, baseURL+"/worker-sessions", document)
			if status != http.StatusAccepted || !strings.Contains(body, `"workerSessionId":"`+id+`"`) {
				t.Errorf("concurrent replay = %d %s", status, body)
			}
		}()
	}
	joined.Wait()
	if peer.prompts.Load() != 1 || peer.starts.Load() != 1 {
		t.Fatalf("replay launched %d ACP attempts and %d prompts", peer.starts.Load(), peer.prompts.Load())
	}
}

func subagentACPDocument(id, dir, model string) map[string]any {
	document := map[string]any{"requestId": id + "-request", "workerSessionId": id, "prompt": "controlled ACP prompt", "workingRoot": dir}
	if strings.HasPrefix(id, "model-override") || strings.HasPrefix(id, "provider-override") || id == "unadvertised-model" {
		document["model"] = model
	}
	if strings.HasPrefix(id, "provider-override") {
		document["provider"] = "opencode"
	}
	return document
}

func subagentFactorySessionIDs(t *testing.T, baseURL string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status, body := t7HTTP(t, ctx, http.MethodGet, baseURL+"/factory-sessions", nil)
	var listing struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &listing) != nil {
		t.Fatal("Factory Session list unavailable")
	}
	ids := make([]string, 0, len(listing.Sessions))
	for _, session := range listing.Sessions {
		ids = append(ids, session.ID)
	}
	slices.Sort(ids)
	return ids
}

// The selected external ACP effect advertises two opaque models and records
// which selection arrives with the prompt. It supplies no real-provider proof.
type subagentACPPeer struct {
	observed chan string
	prompts  atomic.Int32
	starts   atomic.Int32
}

func (peer *subagentACPPeer) serve(reader io.Reader, writer io.Writer) {
	scanner := bufio.NewScanner(reader)
	model := "initial-model"
	for scanner.Scan() {
		var call continuationACPRPC
		if json.Unmarshal(scanner.Bytes(), &call) != nil {
			return
		}
		result := `{}`
		switch call.Method {
		case "initialize":
			result = `{"protocolVersion":1,"agentCapabilities":{}}`
		case "session/new":
			result = `{"sessionId":"controlled-opencode","configOptions":[{"id":"model","name":"Model","category":"model","type":"select","currentValue":"initial-model","options":[{"value":"initial-model","name":"Initial"},{"value":"controlled-free-model","name":"Controlled free"},{"value":"controlled-override","name":"Override"}]}]}`
		case "session/set_config_option":
			var params struct {
				Value string `json:"value"`
			}
			if json.Unmarshal(call.Params, &params) != nil {
				return
			}
			model = params.Value
		case "session/prompt":
			var params struct{ Prompt []struct{ Text string } }
			if json.Unmarshal(call.Params, &params) != nil || len(params.Prompt) != 1 || params.Prompt[0].Text != "controlled ACP prompt" {
				model = fmt.Sprintf("invalid prompt: %s", call.Params)
			}
			peer.prompts.Add(1)
			peer.observed <- model
			// Withhold the response until host cleanup closes this owned stream.
			continue
		}
		if len(call.ID) != 0 && sendContinuationACPRPC(writer, map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": json.RawMessage(result)}) != nil {
			return
		}
	}
}
