package baseline_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const goalFailureProcessUnreachableServer = "http://127.0.0.1:1"

const goalFailureProcessInvalidTopologyJSON = `{
  "name": "@you/goal",
  "workTypes": [{
    "name": "goal",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "plan", "type": "PROCESSING"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "goal-planner", "type": "AGENT_WORKER"}],
  "workstations": [{
    "name": "plan-goal",
    "type": "AGENT_RUN",
    "worker": "goal-planner",
    "inputs": [{"workType": "goal", "state": "init"}],
    "outputs": [{"workType": "goal", "state": "missing-plan-state"}],
    "onFailure": [{"workType": "goal", "state": "failed"}]
  }]
}`

var goalFailureProcessQuietForbiddenMarkers = []string{
	"Factory initiated", "Dashboard URL", "Runtime log", "Opening dashboard", "Factory:", "Recording saved",
}

type goalFailureProcessResult struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
	err    error
}

func executeGoalFailureProcess(
	t *testing.T,
	homeDir string,
	workingDirectory string,
	edges serviceedges.Edges,
	args ...string,
) goalFailureProcessResult {
	t.Helper()
	if homeDir == "" {
		homeDir = t.TempDir()
	}
	if workingDirectory == "" {
		workingDirectory = t.TempDir()
	}
	process, err := root.BuildProcess(t.Context(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	result := goalFailureProcessResult{}
	stdinIsTTY := true
	result.err = process.Execute(root.Input{
		Args:             append([]string{"you"}, args...),
		Env:              goalFailureProcessEnvironment(homeDir),
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
		Stdout:           &result.stdout,
		Stderr:           &result.stderr,
		StdinIsTTY:       &stdinIsTTY,
	})
	return result
}

func goalFailureProcessEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func writeGoalFailureInvalidTopology(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(goalFailureProcessInvalidTopologyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir, factoryPath
}

func TestFailureBaseline_QuietInvalidTopologyWritesStructuredInvocationFailure(t *testing.T) {
	dir, factoryPath := writeGoalFailureInvalidTopology(t)
	result := executeGoalFailureProcess(t, "", dir, serviceedges.Edges{},
		"run", "--factory", factoryPath, "--no-record", "--quiet", "invalid-topology-baseline",
	)
	if result.err == nil {
		t.Fatal("expected invalid goal topology to fail before invocation")
	}
	if !strings.Contains(result.err.Error(), "invalid graph references") {
		t.Fatalf("error = %q, want invalid graph references guidance", result.err.Error())
	}
	if result.stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no quiet terminal value", result.stdout.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(result.stderr.Bytes(), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\n%s", err, result.stderr.String())
	}
	if response.Code != factoryapi.ErrorResponseCode(runcli.InvocationErrorCodeFailed) ||
		response.Family != factoryapi.ErrorFamilyInternalServerError ||
		!strings.Contains(response.Message, "invalid graph references") {
		t.Fatalf("ErrorResponse = %#v", response)
	}
	for _, marker := range goalFailureProcessQuietForbiddenMarkers {
		if strings.Contains(result.stderr.String(), marker) {
			t.Fatalf("stderr contains forbidden marker %q: %q", marker, result.stderr.String())
		}
	}
}

func TestFailureBaseline_NoServer_ModelsListCommandReportsUnreachableEndpoint(t *testing.T) {
	result := executeGoalFailureProcess(t, "", "", serviceedges.Edges{},
		"models", "list", "--server", goalFailureProcessUnreachableServer,
	)
	if result.err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "models endpoint not reachable at http://127.0.0.1:1/models"
	if !strings.Contains(result.err.Error(), want) {
		t.Fatalf("error = %q, want %q", result.err.Error(), want)
	}
}

func TestFailureBaseline_AbsentDefault_RunCommandRejectsUnresolvedDefaultProvider(t *testing.T) {
	result := executeGoalFailureProcess(t, "", "", serviceedges.Edges{},
		"run", "--default-worker-model-provider", "DEFAULT", "--no-record",
	)
	if result.err == nil {
		t.Fatal("expected unresolved DEFAULT provider error")
	}
	if !strings.Contains(result.err.Error(), "DEFAULT requires a concrete provider") {
		t.Fatalf("error = %q, want unresolved DEFAULT guidance", result.err.Error())
	}
}

func TestFailureBaseline_InvalidTopology_RunFactoryCommandRejectsGoalShapedGraphReferences(t *testing.T) {
	dir, factoryPath := writeGoalFailureInvalidTopology(t)
	result := executeGoalFailureProcess(t, "", dir, serviceedges.Edges{},
		"run", "--factory", factoryPath, "--no-record", "--quiet", "invalid-topology-baseline",
	)
	if result.err == nil {
		t.Fatal("expected invalid goal topology to fail before invocation")
	}
	if !strings.Contains(result.err.Error(), "invalid graph references") {
		t.Fatalf("error = %q, want invalid graph references guidance", result.err.Error())
	}
	if !strings.Contains(result.err.Error(), "Blocking findings:") {
		t.Fatalf("error = %q, want blocking findings section", result.err.Error())
	}
	if strings.Count(result.err.Error(), "you factory config validate") != 1 {
		t.Fatalf("error = %q, want exactly one recovery command", result.err.Error())
	}
}

func TestFailureBaseline_NamedPath_RunNamedMissingLocalFactoryRejectsBeforeInvocation(t *testing.T) {
	result := executeGoalFailureProcess(t, "", "", serviceedges.Edges{},
		"run", "--named", "missing-alpha", "--no-record", "--quiet", "named-path baseline prompt",
	)
	if result.err == nil {
		t.Fatal("expected missing named factory to fail before invocation")
	}
	if !strings.Contains(result.err.Error(), `resolve named factory "missing-alpha"`) {
		t.Fatalf("error = %q, want named-path resolution guidance", result.err.Error())
	}
	if !strings.Contains(result.err.Error(), `named factory "missing-alpha" not found`) {
		t.Fatalf("error = %q, want named factory not found guidance", result.err.Error())
	}
}

func TestFailureBaseline_NamedPath_RunNamedUnknownBuiltInGoalStyleNameRejectsBeforeInvocation(t *testing.T) {
	result := executeGoalFailureProcess(t, "", "", serviceedges.Edges{},
		"run", "--named", "@you/missing", "--no-record", "--quiet", "named-path baseline prompt",
	)
	if result.err == nil {
		t.Fatal("expected unknown built-in named factory to fail before invocation")
	}
	if !strings.Contains(result.err.Error(), `resolve named factory "@you/missing"`) {
		t.Fatalf("error = %q, want built-in named-path resolution guidance", result.err.Error())
	}
	if !strings.Contains(result.err.Error(), "project root") || !strings.Contains(result.err.Error(), "global root") {
		t.Fatalf("error = %q, want cross-root named-path context", result.err.Error())
	}
}
