package providers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedRuntimeScriptPath = "scripts/pf2-003-runtime-fixture.sh"

// TestPackagedScriptRuntime_FreshInstallExecutesFactoryRelativeScript is
// isolated because it proves Unix shebang permissions and Factory-relative
// executable selection through the real exec runner.
func TestPackagedScriptRuntime_FreshInstallExecutesFactoryRelativeScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable shebang scripts are not supported on Windows")
	}

	factoryDir := installPackagedScriptRuntimeFixture(t, "packaged-script-runtime-success", "#!/bin/sh\nprintf 'packaged runtime success\\n'\n")
	testutil.WriteSeedFile(t, factoryDir, "task", []byte("input-payload"))

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Edges:      packagedScriptRuntimeEdges(t),
	})
	defer server.Stop(t)
	support.WaitForTerminalStatus(t, server.URL(), 5*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	for placeID, want := range map[string]int{
		"task:complete": 1,
		"task:init":     0,
		"task:failed":   0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
	assertListedWorkText(t, support.ListDefaultSessionWork(t, server.URL()), "task", "complete", "packaged runtime success")
}

func packagedScriptRuntimeEdges(t *testing.T) serviceedges.Edges {
	t.Helper()
	runner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil, nil)
	if err != nil {
		t.Fatalf("construct packaged script runtime command runner: %v", err)
	}
	return serviceedges.Edges{ScriptCommandRunner: runner}
}

// TestPackagedScriptRuntime_NonZeroExitUsesStandardFailureOutcome is isolated
// because it proves real shell exit-code and stderr mapping through exec.
func TestPackagedScriptRuntime_NonZeroExitUsesStandardFailureOutcome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable shebang scripts are not supported on Windows")
	}

	factoryDir := installPackagedScriptRuntimeFixture(t, "packaged-script-runtime-failure", "#!/bin/sh\nprintf 'packaged runtime failure\\n' >&2\nexit 23\n")
	testutil.WriteSeedFile(t, factoryDir, "task", []byte("input-payload"))

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Edges:      packagedScriptRuntimeEdges(t),
	})
	defer server.Stop(t)
	support.WaitForTerminalStatus(t, server.URL(), 5*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	for placeID, want := range map[string]int{
		"task:failed":   1,
		"task:init":     0,
		"task:complete": 0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
	assertPackagedScriptExitEvent(t, server.GetFactoryEvents(t), 23, "packaged runtime failure\n")
}

func installPackagedScriptRuntimeFixture(t *testing.T, packageName, scriptBody string) string {
	t.Helper()

	factoryDir := t.TempDir()
	factoryJSON := []byte(fmt.Sprintf(`{
  "name":%q,
  "workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers":[{"name":"runner","type":"SCRIPT_WORKER","command":%q}],
  "workstations":[{"name":"run-script","worker":"runner","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}],"definition":{"type":"SCRIPT_RUN","worker":"runner","body":"Run the packaged script."}}]
}`, packageName, packagedRuntimeScriptPath))
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), factoryJSON, 0o600); err != nil {
		t.Fatalf("write customer factory.json: %v", err)
	}
	scriptPath := filepath.Join(factoryDir, filepath.FromSlash(packagedRuntimeScriptPath))
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("create customer script directory: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o700); err != nil {
		t.Fatalf("write customer script: %v", err)
	}
	return factoryDir
}

func assertPackagedScriptExitEvent(t *testing.T, events []factoryapi.FactoryEvent, wantExitCode int, wantStderr string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeScriptResponse {
			continue
		}
		response, err := event.Payload.AsScriptResponseEventPayload()
		if err != nil {
			t.Fatalf("decode script response event: %v", err)
		}
		if response.Outcome != factoryapi.ScriptExecutionOutcomeFailedExitCode || response.ExitCode == nil || *response.ExitCode != wantExitCode {
			t.Fatalf("script response outcome = %s exitCode = %#v, want FAILED_EXIT_CODE/%d", response.Outcome, response.ExitCode, wantExitCode)
		}
		if response.Stdout != "" || response.Stderr != wantStderr || response.FailureType != nil {
			t.Fatalf("script response = %#v, want ordinary non-zero exit diagnostics", response)
		}
		return
	}
	t.Fatalf("factory events %s do not contain a script response", strings.Join(packagedScriptEventTypes(events), ","))
}

func assertListedWorkText(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workType string,
	state string,
	want string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType ||
			item.State == nil || item.State.Name != state {
			continue
		}
		if item.Content == nil || len(*item.Content) == 0 {
			t.Fatalf("%s:%s Work has no content", workType, state)
		}
		part, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			t.Fatalf("decode Work text content: %v", err)
		}
		if part.Text != want {
			t.Fatalf("Work text = %q, want %q", part.Text, want)
		}
		return
	}
	t.Fatalf("no listed Work found in %s:%s", workType, state)
}

func packagedScriptEventTypes(events []factoryapi.FactoryEvent) []string {
	types := make([]string, len(events))
	for i, event := range events {
		types[i] = string(event.Type)
	}
	return types
}
