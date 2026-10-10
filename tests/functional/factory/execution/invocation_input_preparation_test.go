package execution_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type preparationWorkflowFiles struct{ platformfilesystem.Local }

func (preparationWorkflowFiles) ReadFile(path string) ([]byte, error) {
	if filepath.Base(path) == "denied-workflow.js" {
		return nil, fs.ErrPermission
	}
	return os.ReadFile(path)
}

// Public preparation rejects unreadable files before dispatch, while a peer
// invocation expands its customer text through the same process and succeeds.
func TestFactoryInvocationInputPreparation(t *testing.T) {
	t.Parallel()
	host := newFactorySessionHost(t, serviceedges.Edges{FactoryRuntimeWorkflowSources: preparationWorkflowFiles{}})
	for _, name := range []string{"B08-S", "B08-F", "B08-W", "B08-E"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runPreparationCase(t, host, name)
		})
	}
}

func assertEmptyPreparationHost(t *testing.T, host *factorySessionHost, dir string, runner *support.RecordingCommandRunner) {
	t.Helper()
	support.ClearSeedInputs(t, dir)
	baseURL, id := host.Open(t, dir, runner)
	endpoint := baseURL + "/factory-sessions/" + id + "/work"
	before := support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
	if len(before.Results) != 0 || runner.CallCount() != 0 {
		t.Fatalf("empty startup admitted Work: %#v, calls=%d", before.Results, runner.CallCount())
	}
	support.SubmitSessionWorkAt(t, baseURL, id, factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: "explicit customer value"})
	support.WaitForSessionTerminalStatus(t, baseURL, id, 30*time.Second)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
	if support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")) != 1 || runner.CallCount() != 1 {
		t.Fatalf("explicit Work = %#v, calls=%d", listed.Results, runner.CallCount())
	}
}

func scaffoldPreparationFactory(t *testing.T) string {
	t.Helper()
	dir := scaffoldBTRCOneShotFactory(t)
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\nselected {{ (index .Inputs 0).Payload }}\n")
	return dir
}

func runPreparationCase(t *testing.T, host *factorySessionHost, name string) {
	t.Helper()
	dir := scaffoldPreparationFactory(t)
	runner := support.NewRecordingCommandRunner("prepared customer result COMPLETE")
	if name == "B08-E" {
		assertEmptyPreparationHost(t, host, dir, runner)
		return
	}
	key, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	host.commands.mu.Lock()
	host.commands.routes[key] = runner
	host.commands.mu.Unlock()
	t.Cleanup(func() { host.commands.mu.Lock(); delete(host.commands.routes, key); host.commands.mu.Unlock() })
	file := filepath.Join(dir, "customer.txt")
	if err := os.WriteFile(file, []byte("selected file customer value"), 0600); err != nil {
		t.Fatal(err)
	}
	if name == "B08-F" {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(dir, "factory.json")
	if name == "B08-W" {
		source = filepath.Join(dir, "denied-workflow.js")
		if err := os.WriteFile(source, []byte(`return agent.run({prompt: "selected file customer value"});`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	args := []string{"you", "--json", "run", "--factory", source, "--session", uuid.NewString(), "--no-record", "--output", "primary"}
	if name != "B08-W" {
		args = append(args, "--to-file", file)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.WorkingDirectory = dir
	err = host.server.Execute(t, inputs.Input)
	if name != "B08-S" {
		assertPreparationRejected(t, name, err, runner.CallCount(), inputs.Stdout())
		return
	}
	if err != nil {
		t.Fatalf("file invocation: %v / %s", err, inputs.Stderr())
	}
	requests := runner.Requests()
	if len(requests) != 1 || !strings.Contains(string(requests[0].Stdin), "selected file customer value") {
		t.Fatalf("file value did not reach selected provider: calls=%d", len(requests))
	}
	if !strings.Contains(inputs.Stdout(), "prepared customer result COMPLETE") {
		t.Fatalf("successful output = %s", inputs.Stdout())
	}
}

func assertPreparationRejected(t *testing.T, name string, err error, calls int, stdout string) {
	t.Helper()
	if err == nil || calls != 0 {
		t.Fatalf("rejected preparation = %v, calls=%d, stdout=%q", err, calls, stdout)
	}
	if name == "B08-W" {
		var failure struct {
			Code  string `json:"code"`
			Field string `json:"field"`
		}
		if decodeErr := json.Unmarshal([]byte(stdout), &failure); decodeErr != nil || failure.Code != "SESSION_EXECUTION_VALIDATION_FAILED" || failure.Field != "source" {
			t.Fatalf("workflow rejection = %v, code=%s, field=%s, decode=%v", err, failure.Code, failure.Field, decodeErr)
		}
	}
	if name == "B08-F" && (!errors.Is(err, fs.ErrNotExist) || stdout != "") {
		t.Fatalf("input-read error = %v", err)
	}
}
