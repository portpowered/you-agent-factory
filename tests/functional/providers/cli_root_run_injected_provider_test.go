package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	rootRunWorkType       = "root-run-task"
	rootRunWorker         = "root-run-worker"
	rootRunWorkstation    = "process-root-run-task"
	rootRunModel          = "functional-cursor-model"
	rootRunPrompt         = "prove the injected provider edge"
	rootRunProviderResult = "root.Run injected provider result COMPLETE"
)

type functionalEdgeGraphBuilder struct {
	edges wire.FunctionalEdges
}

func (builder functionalEdgeGraphBuilder) Build(
	ctx context.Context,
	request root.GraphRequest,
) (*root.ApplicationGraph, error) {
	return wire.BuildProcessGraphWithFunctionalEdges(ctx, request.Startup, request.Policy, builder.edges)
}

func TestRootRunInjectedProviderCommandRunner(t *testing.T) {
	factoryDir, providerWorkDir := scaffoldRootRunProviderFactory(t)
	runner := support.NewRecordingCommandRunner(string(support.CursorProviderSuccessStdout(rootRunProviderResult)))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	input := root.Input{
		Args: []string{
			"you", "run", "--factory", filepath.Join(factoryDir, interfaces.FactoryConfigFile),
			"--quiet", "--no-record", rootRunPrompt,
		},
		Env:     isolatedRootRunEnvironment(t.TempDir()),
		Stdin:   strings.NewReader(""),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Context: context.Background(),
	}
	dependencies := root.Dependencies{GraphBuilder: functionalEdgeGraphBuilder{edges: wire.FunctionalEdges{
		ProviderCommandRunner: runner,
	}}}
	exitCode := root.ExitFailure
	support.WithWorkingDirectory(t, factoryDir, func() {
		exitCode = root.Run(input, dependencies)
	})

	if exitCode != root.ExitSuccess {
		t.Fatalf("root.Run() exit code = %d, want %d; stdout=%q stderr=%q", exitCode, root.ExitSuccess, stdout.String(), stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != rootRunProviderResult {
		t.Fatalf("stdout = %q, want primary result %q", got, rootRunProviderResult)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty successful provider invocation diagnostics", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
	}
	assertRootRunProviderRequest(t, providerWorkDir, runner.LastRequest())
}

func scaffoldRootRunProviderFactory(t *testing.T) (string, string) {
	t.Helper()
	providerWorkDir := t.TempDir()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "root-run-injected-provider",
		"workTypes": []map[string]any{{
			"name":             rootRunWorkType,
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": rootRunWorker}},
		"workstations": []map[string]any{{
			"name":             rootRunWorkstation,
			"worker":           rootRunWorker,
			"workingDirectory": providerWorkDir,
			"inputs":           []map[string]string{{"workType": rootRunWorkType, "state": "init"}},
			"outputs":          []map[string]string{{"workType": rootRunWorkType, "state": "complete"}},
			"onFailure":        []map[string]string{{"workType": rootRunWorkType, "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, rootRunWorker, support.BuildModelWorkerConfig(
		interfaces.ModelProviderCursor,
		rootRunModel,
	))
	return dir, providerWorkDir
}

func isolatedRootRunEnvironment(home string) []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"USERPROFILE=" + home}
	case "plan9":
		return []string{"home=" + home}
	default:
		return []string{"HOME=" + home}
	}
}

func assertRootRunProviderRequest(t *testing.T, providerWorkDir string, request workers.CommandRequest) {
	t.Helper()
	if request.Command != string(interfaces.ModelProviderCursor) {
		t.Fatalf("provider command = %q, want %q", request.Command, interfaces.ModelProviderCursor)
	}
	support.AssertArgsContainSequence(t, request.Args, []string{"-p", "--model", rootRunModel})
	support.AssertArgsContainSequence(t, request.Args, []string{"--output-format", "stream-json", "--stream-partial-output"})
	if len(request.Args) == 0 {
		t.Fatal("provider args are empty, want rendered prompt argument")
	}
	renderedInput := request.Args[len(request.Args)-1]
	for _, want := range []string{"Process the input task.", "Do the work."} {
		if !strings.Contains(renderedInput, want) {
			t.Fatalf("rendered provider input = %q, want %q", renderedInput, want)
		}
	}
	inputTokens, err := json.Marshal(request.InputTokens)
	if err != nil {
		t.Fatalf("marshal provider input tokens: %v", err)
	}
	if !bytes.Contains(inputTokens, []byte(rootRunPrompt)) {
		t.Fatalf("provider input tokens = %s, want invocation prompt %q", inputTokens, rootRunPrompt)
	}
	if len(request.Stdin) != 0 {
		t.Fatalf("provider stdin = %q, want Cursor prompt in command args", string(request.Stdin))
	}
	if got, want := filepath.Clean(request.WorkDir), filepath.Clean(providerWorkDir); got != want {
		t.Fatalf("provider working directory = %q, want %q", got, want)
	}
	if request.DispatchID == "" || request.TransitionID == "" {
		t.Fatalf("provider dispatch metadata = dispatch %q transition %q, want populated", request.DispatchID, request.TransitionID)
	}
	if request.WorkerType != rootRunWorker || request.WorkstationName != rootRunWorkstation {
		t.Fatalf("provider routing metadata = worker %q workstation %q", request.WorkerType, request.WorkstationName)
	}
	if request.Execution.TraceID == "" || len(request.Execution.WorkIDs) != 1 {
		t.Fatalf("provider execution metadata = %+v, want trace and one work ID", request.Execution)
	}
}

var _ root.GraphBuilder = functionalEdgeGraphBuilder{}
