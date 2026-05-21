package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	docscli "github.com/portpowered/infinite-you/pkg/cli/docs"
	factorycli "github.com/portpowered/infinite-you/pkg/cli/factory"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type errWriter struct {
	err error
}

func (w errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestNewRootCommand_HasSubcommands(t *testing.T) {
	root := NewRootCommand()

	want := map[string]bool{
		"config":  false,
		"docs":    false,
		"factory": false,
		"init":    false,
		"run":     false,
		"submit":  false,
		"work":    false,
	}

	for _, sub := range root.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("expected subcommand %q to be registered", name)
		}
	}
}

func TestRootCommand_UsesInstalledBinaryName(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Usage:\n  you",
		"Running you with no arguments starts the out-of-the-box flow",
		"you docs workstation",
		"you run --dir factory",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	for _, disallowed := range []string{
		"Usage:\n  infinite-you",
		"Running infinite-you with no arguments",
		"infinite-you docs workstation",
	} {
		if strings.Contains(help, disallowed) {
			t.Fatalf("root help still contains old executable token %q:\n%s", disallowed, help)
		}
	}
}

func TestFactoryQueryCommand_PortFlagMapsToConfig(t *testing.T) {
	originalQueryFactory := queryFactory
	defer func() {
		queryFactory = originalQueryFactory
	}()

	var got factorycli.QueryConfig
	queryFactory = func(cfg factorycli.QueryConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query", "--port", "9090", "--json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query: %v", err)
	}

	if got.Port != 9090 {
		t.Fatalf("port = %d, want 9090", got.Port)
	}
	if !got.JSON {
		t.Fatal("expected json output flag")
	}
	if got.Output == nil {
		t.Fatal("expected output writer")
	}
}

func TestFactoryQueryCommand_DefaultPortMapsToSharedLocalPort(t *testing.T) {
	originalQueryFactory := queryFactory
	defer func() {
		queryFactory = originalQueryFactory
	}()

	var got factorycli.QueryConfig
	queryFactory = func(cfg factorycli.QueryConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query: %v", err)
	}

	if got.Port != 7437 {
		t.Fatalf("port = %d, want 7437", got.Port)
	}
}

func TestFactoryQueryCommand_DefaultRootOutput(t *testing.T) {
	factoryDir := t.TempDir()
	srv := rootCurrentFactoryServer(t, factoryapi.Factory{
		Name:             apisurface.DefaultCurrentFactoryName,
		FactoryDirectory: &factoryDir,
	})
	defer srv.Close()

	out := executeRootCommand(t, "factory", "query", "--port", strconv.Itoa(rootServerPort(t, srv)))
	want := "NAME\tKIND\tID\tFACTORY DIRECTORY\n" +
		fmt.Sprintf("%s\tdefault-root\t\t%s\n", apisurface.DefaultCurrentFactoryName, factoryDir)
	if got := string(out); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestFactoryQueryCommand_NamedFactoryOutput(t *testing.T) {
	factoryID := "customer-factory"
	srv := rootCurrentFactoryServer(t, factoryapi.Factory{
		Name: "beta",
		Id:   &factoryID,
	})
	defer srv.Close()

	out := executeRootCommand(t, "factory", "query", "--port", strconv.Itoa(rootServerPort(t, srv)))
	want := "NAME\tKIND\tID\tFACTORY DIRECTORY\n" +
		"beta\tnamed\tcustomer-factory\t\n"
	if got := string(out); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestFactoryQueryCommand_DefaultRootJSONOutput(t *testing.T) {
	factoryDir := t.TempDir()
	srv := rootCurrentFactoryServer(t, factoryapi.Factory{
		Name:             apisurface.DefaultCurrentFactoryName,
		FactoryDirectory: &factoryDir,
	})
	defer srv.Close()

	out := executeRootCommand(t, "factory", "query", "--json", "--port", strconv.Itoa(rootServerPort(t, srv)))
	if bytes.Contains(out, []byte("NAME\tKIND")) {
		t.Fatalf("json output included human-readable text: %q", string(out))
	}

	var got factoryapi.Factory
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("json output is not valid Factory JSON: %v\n%s", err, string(out))
	}
	if got.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name = %q, want %q", got.Name, apisurface.DefaultCurrentFactoryName)
	}
	if got.FactoryDirectory == nil || *got.FactoryDirectory != factoryDir {
		t.Fatalf("factory directory = %#v, want %q", got.FactoryDirectory, factoryDir)
	}
}

func TestFactoryQueryCommand_NamedFactoryJSONOutput(t *testing.T) {
	factoryID := "customer-factory"
	workers := []factoryapi.Worker{{Name: "executor"}}
	srv := rootCurrentFactoryServer(t, factoryapi.Factory{
		Name:    "beta",
		Id:      &factoryID,
		Workers: &workers,
	})
	defer srv.Close()

	out := executeRootCommand(t, "factory", "query", "--port", strconv.Itoa(rootServerPort(t, srv)), "--json")
	if bytes.Contains(out, []byte("NAME\tKIND")) {
		t.Fatalf("json output included human-readable text: %q", string(out))
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("json output is not valid Factory JSON: %v\n%s", err, string(out))
	}
	if got["name"] != "beta" || got["id"] != factoryID {
		t.Fatalf("factory JSON = %#v, want name beta and id %q", got, factoryID)
	}
	workerPayloads, ok := got["workers"].([]any)
	if !ok || len(workerPayloads) != 1 {
		t.Fatalf("workers = %#v, want one API worker payload", got["workers"])
	}
}

func TestFactoryQueryCommand_CurrentFactoryNotFoundFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory/~current" {
			t.Fatalf("path = %q, want /factory/~current", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.NOTFOUND,
			Message: "Current named factory not found.",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	out, err := executeRootCommandErr(t, "factory", "query", "--json", "--port", strconv.Itoa(rootServerPort(t, srv)))
	if err == nil {
		t.Fatal("expected missing current factory to fail")
	}
	if len(out) != 0 {
		t.Fatalf("stdout = %q, want no success output", string(out))
	}
	want := "running service has no active current factory; start a factory or activate a named factory: current factory not found: Current named factory not found."
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestFactoryQueryCommand_UnreachableServerFails(t *testing.T) {
	out, err := executeRootCommandErr(t, "factory", "query", "--port", "1")
	if err == nil {
		t.Fatal("expected unreachable server to fail")
	}
	if len(out) != 0 {
		t.Fatalf("stdout = %q, want no success output", string(out))
	}
	if !strings.Contains(err.Error(), "factory not reachable at http://localhost:1/factory/~current") {
		t.Fatalf("error = %q, want reachability context", err.Error())
	}
}

func TestFactoryCommand_HelpListsQuerySubcommand(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Inspect factory runtime state from a running infinite-you service.",
		"query",
		"Show the current active factory",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory help missing %q:\n%s", want, help)
		}
	}
}

func TestFactoryQueryCommand_HelpDocumentsOutputModesAndPort(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query --help: %v", err)
	}

	help := out.String()
	for _, want := range []string{
		"Show the current active factory from a running infinite-you service.",
		"human-readable table",
		"Use --json for the API-shaped current-factory payload",
		"--port",
		"--json",
		"you factory query --json",
		"you factory query --port 9090 --json",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("factory query help missing %q:\n%s", want, help)
		}
	}
}

func TestDocsCommand_HelpDocumentsSupportedTopics(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"docs"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute docs help: %v", err)
	}

	help := out.String()
	for _, want := range append(
		[]string{
			"Print packaged markdown reference topics from the installed binary.",
			"Use one of the supported topic subcommands to print the authored markdown page with no wrapper formatting.",
		},
		docscli.SupportedTopics()...,
	) {
		if !strings.Contains(help, want) {
			t.Fatalf("docs help missing %q:\n%s", want, help)
		}
	}
}

func TestRootCommand_HelpDocumentsSupportedDocsTopics(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}

	help := out.String()
	for _, want := range append(
		[]string{
			"Packaged reference topics are also available through you docs <topic>.",
			"Supported docs topics:",
			"you docs workstation",
		},
		docscli.SupportedTopics()...,
	) {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestDocsCommand_SupportedTopicsPrintRawPackagedMarkdown(t *testing.T) {
	t.Parallel()

	for _, topic := range docscli.SupportedTopics() {
		topic := topic
		t.Run(topic, func(t *testing.T) {
			t.Parallel()

			want, err := docscli.Markdown(topic)
			if err != nil {
				t.Fatalf("Markdown(%q): %v", topic, err)
			}

			got := string(executeRootCommand(t, "docs", topic))
			if got != want {
				t.Fatalf("docs %s output did not match packaged markdown", topic)
			}
		})
	}
}

func TestDocsCommand_RejectsUnsupportedTopic(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"docs", "unknown"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unsupported docs topic to fail")
	}
	if !strings.Contains(err.Error(), `unknown command "unknown"`) {
		t.Fatalf("unsupported docs topic error = %q", err.Error())
	}
}

func TestDocsCommand_SupportedTopicReturnsConfiguredWriterFailure(t *testing.T) {
	topic := docscli.SupportedTopics()[0]
	wantErr := fmt.Errorf("write %s docs output: boom", topic)

	root := NewRootCommand()
	root.SetOut(errWriter{err: wantErr})
	root.SetErr(io.Discard)
	root.SetArgs([]string{"docs", topic})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected docs topic write to fail")
	}
	if err != wantErr {
		t.Fatalf("docs topic write error = %v, want %v", err, wantErr)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-cli-flatten-fixture review=2026-07-18 removal=split-cli-flatten-fixture-before-next-config-cli-change
func TestConfigFlattenCommand_WritesCanonicalLoadableFactoryJSON(t *testing.T) {
	factoryDir := t.TempDir()
	writeFlattenCommandFixture(t, factoryDir)
	assertSplitLayoutLoadUsesCanonicalWorkerFields(t, factoryDir)

	out := executeRootCommand(t, "config", "flatten", factoryDir)
	payload := decodeFlattenPayload(t, out)
	assertCanonicalFlattenPayload(t, payload)
	assertFlattenedOutputParses(t, out)
	standaloneDir := writeStandaloneFlattenOutput(t, out)
	assertStandaloneRuntimeConfigLoads(t, standaloneDir)

	fileOut := executeRootCommand(t, "config", "flatten", filepath.Join(standaloneDir, interfaces.FactoryConfigFile))
	if _, err := factoryconfig.FactoryConfigFromOpenAPIJSON(fileOut); err != nil {
		t.Fatalf("standalone file flatten output should parse through normal factory config path: %v", err)
	}
}

func writeFlattenCommandFixture(t *testing.T, factoryDir string) {
	t.Helper()

	writeRootTestFile(t, filepath.Join(factoryDir, interfaces.FactoryConfigFile), `{
		"name":"flatten-command-fixture",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"resources":[{"name":"agent-slot","capacity":2}]
		}]
	}`)
	writeRootTestFile(t, filepath.Join(factoryDir, "workers", "executor", "AGENTS.md"), `---
type: MODEL_WORKER
model: claude-sonnet-4-6
modelProvider: CLAUDE
executorProvider: SCRIPT_WRAP
stopToken: COMPLETE
---

You are the split-layout executor.`)
	writeRootTestFile(t, filepath.Join(factoryDir, "workstations", "execute-story", "AGENTS.md"), `---
type: MODEL_WORKSTATION
worker: executor
limits:
  maxExecutionTime: 20m
  maxRetries: 2
stopWords: ["DONE"]
---

Process {{ (index .Inputs 0).WorkID }}.`)
}

func executeRootCommand(t *testing.T, args ...string) []byte {
	t.Helper()

	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root command %v: %v", args, err)
	}
	return out.Bytes()
}

func executeRootCommandErr(t *testing.T, args ...string) ([]byte, error) {
	t.Helper()

	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	err := root.Execute()
	return out.Bytes(), err
}

func rootCurrentFactoryServer(t *testing.T, current factoryapi.Factory) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/factory/~current" {
			t.Fatalf("path = %q, want /factory/~current", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(current); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
}

func rootServerPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()

	var port int
	if _, err := fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return port
}

func decodeFlattenPayload(t *testing.T, out []byte) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("flattened output is not JSON: %v\n%s", err, string(out))
	}
	return payload
}

func assertCanonicalFlattenPayload(t *testing.T, payload map[string]any) {
	t.Helper()

	if _, ok := payload["workTypes"]; !ok {
		t.Fatalf("expected canonical workTypes key in flattened output")
	}
	if _, ok := payload["work_types"]; ok {
		t.Fatalf("expected flattened output not to include legacy work_types key")
	}

	workstations, ok := payload["workstations"].([]any)
	if !ok || len(workstations) != 1 {
		t.Fatalf("expected one workstation in flattened output, got %#v", payload["workstations"])
	}
	workstation, ok := workstations[0].(map[string]any)
	if !ok {
		t.Fatalf("expected workstation object, got %#v", workstations[0])
	}
	if _, ok := workstation["resources"]; !ok {
		t.Fatalf("expected canonical resources key in flattened workstation")
	}
	workersPayload, ok := payload["workers"].([]any)
	if !ok || len(workersPayload) != 1 {
		t.Fatalf("expected one worker in flattened output, got %#v", payload["workers"])
	}
	workerPayload, ok := workersPayload[0].(map[string]any)
	if !ok {
		t.Fatalf("expected flattened worker to include inline definition, got %#v", workerPayload)
	}
	if workerPayload["model"] != "claude-sonnet-4-6" || workerPayload["modelProvider"] != "CLAUDE" {
		t.Fatalf("expected flattened worker definition to preserve model/provider, got %#v", workerPayload)
	}
	if workerPayload["executorProvider"] != "SCRIPT_WRAP" {
		t.Fatalf("expected flattened worker definition to preserve canonical executorProvider, got %#v", workerPayload)
	}
	for _, retired := range []string{"provider", "sessionId", "concurrency"} {
		if _, ok := workerPayload[retired]; ok {
			t.Fatalf("expected flattened worker definition not to include retired %q field, got %#v", retired, workerPayload)
		}
	}
	if workerPayload["body"] != "You are the split-layout executor." {
		t.Fatalf("expected flattened worker body, got %#v", workerPayload["body"])
	}
	if workstation["type"] != "MODEL_WORKSTATION" {
		t.Fatalf("expected flattened workstation runtime type, got %#v", workstation)
	}
	if workstation["body"] != "Process {{ (index .Inputs 0).WorkID }}." {
		t.Fatalf("expected flattened workstation body to preserve prompt content, got %#v", workstation)
	}
	if _, ok := workstation["promptTemplate"]; ok {
		t.Fatalf("expected flattened workstation to omit promptTemplate, got %#v", workstation)
	}
	if _, ok := workstation["definition"]; ok {
		t.Fatalf("expected flattened workstation runtime config to be flat, got %#v", workstation)
	}
}

func assertFlattenedOutputParses(t *testing.T, out []byte) {
	t.Helper()

	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(out)
	if err != nil {
		t.Fatalf("flattened output should parse through normal factory config path: %v", err)
	}
	if len(cfg.Workers) != 1 || len(cfg.Workstations) != 1 {
		t.Fatalf("expected flattened output to preserve workers/workstations, got %d/%d", len(cfg.Workers), len(cfg.Workstations))
	}
}

func writeStandaloneFlattenOutput(t *testing.T, out []byte) string {
	t.Helper()

	standaloneDir := t.TempDir()
	writeRootTestFile(t, filepath.Join(standaloneDir, interfaces.FactoryConfigFile), string(out))
	return standaloneDir
}

func assertStandaloneRuntimeConfigLoads(t *testing.T, standaloneDir string) {
	t.Helper()

	loaded, err := factoryconfig.LoadRuntimeConfig(standaloneDir, nil)
	if err != nil {
		t.Fatalf("flattened output should load as standalone factory config: %v", err)
	}
	if len(loaded.FactoryConfig().Workers) != 1 || len(loaded.FactoryConfig().Workstations) != 1 {
		t.Fatalf("expected standalone load to preserve workers/workstations, got %d/%d", len(loaded.FactoryConfig().Workers), len(loaded.FactoryConfig().Workstations))
	}
	workerDef, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected standalone flattened worker definition to load")
	}
	if workerDef.Model != "claude-sonnet-4-6" || workerDef.Body != "You are the split-layout executor." {
		t.Fatalf("standalone flattened worker definition = %#v", workerDef)
	}
	if workerDef.SessionID != "" || workerDef.Concurrency != 0 {
		t.Fatalf("standalone flattened worker definition leaked internal-only runtime fields: %#v", workerDef)
	}
	workstationDef, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("expected standalone flattened workstation definition to load")
	}
	if workstationDef.PromptTemplate != "Process {{ (index .Inputs 0).WorkID }}." || workstationDef.Limits.MaxRetries != 2 {
		t.Fatalf("standalone flattened workstation definition = %#v", workstationDef)
	}
}

func assertSplitLayoutLoadUsesCanonicalWorkerFields(t *testing.T, factoryDir string) {
	t.Helper()

	workerDef, err := factoryconfig.LoadWorkerConfig(filepath.Join(factoryDir, "workers", "executor"))
	if err != nil {
		t.Fatalf("LoadWorkerConfig(source split layout): %v", err)
	}
	if workerDef.ModelProvider != "claude" || workerDef.ExecutorProvider != "script_wrap" || workerDef.StopToken != "COMPLETE" {
		t.Fatalf("source split worker definition = %#v, want canonical worker fields", workerDef)
	}
	if workerDef.SessionID != "" || workerDef.Concurrency != 0 {
		t.Fatalf("source split worker definition leaked runtime-only fields: %#v", workerDef)
	}
}

func TestConfigExpandCommand_WritesSplitFactoryLayout(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	writeRootTestFile(t, factoryPath, `{
		"name":"expand-command-fixture",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"config", "expand", factoryPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute config expand: %v", err)
	}
	if !strings.Contains(out.String(), "Expanded factory config into") {
		t.Fatalf("expected expand result output, got %q", out.String())
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("expanded layout should load through normal runtime config path: %v", err)
	}
	if len(loaded.FactoryConfig().Workers) != 1 || len(loaded.FactoryConfig().Workstations) != 1 {
		t.Fatalf("expected expanded layout to preserve workers/workstations, got %d/%d", len(loaded.FactoryConfig().Workers), len(loaded.FactoryConfig().Workstations))
	}
	if _, ok := loaded.Worker("executor"); !ok {
		t.Fatal("expected expanded worker AGENTS.md to load")
	}
	if _, ok := loaded.Workstation("execute-story"); !ok {
		t.Fatal("expected expanded workstation AGENTS.md to load")
	}

	workerAgents := string(readRootTestFile(t, filepath.Join(dir, "workers", "executor", "AGENTS.md")))
	for _, expected := range []string{"type: MODEL_WORKER"} {
		if !strings.Contains(workerAgents, expected) {
			t.Fatalf("expanded worker AGENTS.md missing %q:\n%s", expected, workerAgents)
		}
	}
	for _, retired := range []string{"modelProvider:", "stopToken:", "skipPermissions:"} {
		if strings.Contains(workerAgents, retired) {
			t.Fatalf("expanded worker AGENTS.md should not contain retired %q:\n%s", retired, workerAgents)
		}
	}

	workstationAgents := string(readRootTestFile(t, filepath.Join(dir, "workstations", "execute-story", "AGENTS.md")))
	for _, expected := range []string{"type: MODEL_WORKSTATION", "worker: executor"} {
		if !strings.Contains(workstationAgents, expected) {
			t.Fatalf("expanded workstation AGENTS.md missing %q:\n%s", expected, workstationAgents)
		}
	}
	for _, retired := range []string{"promptFile:", "outputSchema:", "stopWords:"} {
		if strings.Contains(workstationAgents, retired) {
			t.Fatalf("expanded workstation AGENTS.md should not contain retired %q:\n%s", retired, workstationAgents)
		}
	}
}

func TestNewRootCommand_DoesNotExposeRemovedAuditStateSurfaces(t *testing.T) {
	root := NewRootCommand()

	for _, subcommand := range root.Commands() {
		if subcommand.Name() == "audit" {
			t.Fatal("audit command should not be registered")
		}
		if subcommand.Name() == "status" {
			t.Fatal("status command should not be registered")
		}
	}

	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"audit", "state-surfaces"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected removed audit state-surfaces command to fail")
	}
}

func TestExecute_ExitsWithStatusOneWhenRootCommandFails(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestExecute_HelperProcess", "--", "docs", "unknown")
	cmd.Env = append(os.Environ(), "GO_WANT_EXECUTE_HELPER=1")

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected Execute helper process to exit with failure")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("Execute helper error = %T, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("Execute helper exit code = %d, want 1", exitErr.ExitCode())
	}
}

func TestExecute_HelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_EXECUTE_HELPER") != "1" {
		return
	}

	argsIndex := 1
	for ; argsIndex < len(os.Args); argsIndex++ {
		if os.Args[argsIndex] == "--" {
			argsIndex++
			break
		}
	}
	if argsIndex > len(os.Args) {
		fmt.Fprintln(os.Stderr, "missing helper args")
		os.Exit(2)
	}

	originalArgs := os.Args
	defer func() {
		os.Args = originalArgs
	}()
	os.Args = append([]string{originalArgs[0]}, originalArgs[argsIndex:]...)

	Execute()
	os.Exit(0)
}

func writeRootTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readRootTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
