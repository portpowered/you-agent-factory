package named_invocation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	packagedGoalFactoryName             = "@you/goal"
	packagedGoalExecuteWorkstationName  = "execute-goal"
	packagedSubagentFactoryName         = "@you/subagent"
	packagedSubagentWorkerName          = "subagent-worker"
	packagedSubagentRunWorkstationName  = "run-subagent"
	wantHermeticInvocationPrimaryResult = "mock worker accepted"
)

type listenerStartObservation struct {
	calls atomic.Int32
}

func (observation *listenerStartObservation) Start(
	context.Context,
	platformhttpserver.StartRequest,
) error {
	observation.calls.Add(1)
	return errors.New("one-shot named invocation attempted to start an HTTP listener")
}

func TestRun_NamedGoalHermeticInvocationSucceedsWithoutListeningServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for hermetic named goal no-server invocation")
	}

	goalText := "hermetic no-server named goal prompt"
	stdout, listenerStarts := runHermeticNamedInvocation(
		t,
		packagedGoalFactoryName,
		goalText,
		workers.MockWorkersConfig{
			UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      "goal-executor",
				WorkstationName: packagedGoalExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
	)

	if stdout != wantHermeticInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want primary result %s", stdout, wantHermeticInvocationPrimaryResult)
	}
	if listenerStarts != 0 {
		t.Fatalf("HTTP listener start calls = %d, want 0", listenerStarts)
	}
}

func TestRun_NamedSubagentHermeticInvocationSucceedsWithoutListeningServer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for hermetic named subagent no-server invocation")
	}

	requestText := "hermetic no-server named subagent prompt"
	stdout, listenerStarts := runHermeticNamedInvocation(
		t,
		packagedSubagentFactoryName,
		requestText,
		workers.MockWorkersConfig{
			UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      packagedSubagentWorkerName,
				WorkstationName: packagedSubagentRunWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
	)

	if stdout != wantHermeticInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want agent response %s", stdout, wantHermeticInvocationPrimaryResult)
	}
	if stdout == requestText {
		t.Fatalf("stdout echoed submitted request text instead of agent response")
	}
	if listenerStarts != 0 {
		t.Fatalf("HTTP listener start calls = %d, want 0", listenerStarts)
	}
}

func TestRun_NamedGoalCanonicalResponseStreamWorksInShortSuite(t *testing.T) {
	goalText := "short-suite canonical response-stream prompt"
	stdout, listenerStarts := runHermeticNamedInvocationWithOutput(
		t,
		packagedGoalFactoryName,
		goalText,
		workers.MockWorkersConfig{
			UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      "goal-executor",
				WorkstationName: packagedGoalExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
		true,
	)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	var factoryEvents int
	for index, line := range lines {
		var record struct {
			RecordType string `json:"recordType"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode response-stream line %d: %v\nstdout:\n%s", index, err, stdout)
		}
		switch record.RecordType {
		case "factory_event":
			if index == len(lines)-1 {
				t.Fatal("Factory Event appeared after the terminal record")
			}
			factoryEvents++
		case "invocation_result":
			if index != len(lines)-1 {
				t.Fatalf("terminal record index = %d, want %d", index, len(lines)-1)
			}
		default:
			t.Fatalf("unexpected response-stream record type %q", record.RecordType)
		}
	}
	if factoryEvents == 0 {
		t.Fatalf("stdout contains no canonical Factory Event records:\n%s", stdout)
	}
	if listenerStarts != 0 {
		t.Fatalf("HTTP listener start calls = %d, want 0", listenerStarts)
	}
}

func runHermeticNamedInvocation(
	t *testing.T,
	factoryName string,
	requestText string,
	mockWorkers workers.MockWorkersConfig,
) (string, int) {
	return runHermeticNamedInvocationWithOutput(t, factoryName, requestText, mockWorkers, false)
}

func runHermeticNamedInvocationWithOutput(
	t *testing.T,
	factoryName string,
	requestText string,
	mockWorkers workers.MockWorkersConfig,
	responseStream bool,
) (string, int) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	listenerStarts := &listenerStartObservation{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: listenerStarts.Start,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	initStdout, initStderr := executeCustomerCommand(
		t,
		process,
		environment,
		workingDirectory,
		[]string{"you", "--json", "config", "init"},
	)
	if initStderr != "" {
		t.Fatalf("config init stderr = %q, want empty; stdout=%s", initStderr, initStdout)
	}
	assertPackagedFactoryInstalled(t, initStdout, factoryName)

	mockWorkersPath := writeMockWorkersConfig(t, mockWorkers)
	args := []string{
		"you", "run", "--named", factoryName,
		"--with-mock-workers", "--no-record", "--quiet",
		mockWorkersPath, requestText,
	}
	if responseStream {
		args = []string{
			"you", "--json", "run", "--named", factoryName,
			"--with-mock-workers", "--no-record", "--output", "response-stream",
			mockWorkersPath, requestText,
		}
	}
	stdout, stderr := executeCustomerCommand(
		t,
		process,
		environment,
		workingDirectory,
		args,
	)
	if stderr != "" {
		t.Fatalf("named invocation stderr = %q, want empty; stdout=%s", stderr, stdout)
	}
	return stdout, int(listenerStarts.calls.Load())
}

type customerProcess interface {
	Execute(root.Input) error
}

func executeCustomerCommand(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	args []string,
) (string, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	stdinIsTTY := true
	stdoutIsTTY := false
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := process.Execute(root.Input{
		Args:             args,
		Env:              environment,
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          ctx,
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	if err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v; stdout=%q stderr=%q",
			args,
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return stdout.String(), stderr.String()
}

func assertPackagedFactoryInstalled(t *testing.T, payload, name string) {
	t.Helper()
	var result struct {
		PackagedFactories []struct {
			Name       string `json:"name"`
			FactoryDir string `json:"factoryDirectory"`
		} `json:"packagedFactories"`
	}
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		t.Fatalf("decode config init result: %v\nstdout:\n%s", err, payload)
	}
	for _, factory := range result.PackagedFactories {
		if factory.Name != name {
			continue
		}
		if _, err := os.Stat(filepath.Join(factory.FactoryDir, "factory.json")); err != nil {
			t.Fatalf("installed packaged Factory %q: %v", name, err)
		}
		return
	}
	t.Fatalf("config init result omitted packaged Factory %q: %#v", name, result.PackagedFactories)
}

func writeMockWorkersConfig(t *testing.T, config workers.MockWorkersConfig) string {
	t.Helper()
	payload, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}
