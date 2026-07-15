package providers

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/packageassets"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const packagedRuntimeScriptPath = "scripts/pf2-003-runtime-fixture.sh"

func TestPackagedScriptRuntime_FreshInstallExecutesFactoryRelativeScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable shebang scripts are not supported on Windows")
	}

	factoryDir := installPackagedScriptRuntimeFixture(t, "packaged-script-runtime-success", "#!/bin/sh\nprintf 'packaged runtime success\\n'\n")
	testutil.WriteSeedFile(t, factoryDir, "task", []byte("input-payload"))

	h := testutil.NewServiceTestHarness(t, factoryDir, testutil.WithFullWorkerPoolAndScriptWrap())
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
	assertTokenPayload(t, h.Marking(), "task:complete", "packaged runtime success")
}

func TestPackagedScriptRuntime_NonZeroExitUsesStandardFailureOutcome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable shebang scripts are not supported on Windows")
	}

	factoryDir := installPackagedScriptRuntimeFixture(t, "packaged-script-runtime-failure", "#!/bin/sh\nprintf 'packaged runtime failure\\n' >&2\nexit 23\n")
	testutil.WriteSeedFile(t, factoryDir, "task", []byte("input-payload"))

	h := testutil.NewServiceTestHarness(t, factoryDir, testutil.WithFullWorkerPoolAndScriptWrap())
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("dispatch history = %#v, want one completed dispatch", snapshot.DispatchHistory)
	}
	dispatch := snapshot.DispatchHistory[0]
	if dispatch.Outcome != workerexecution.OutcomeFailed || dispatch.Reason != "packaged runtime failure" {
		t.Fatalf("dispatch outcome = %s reason = %q, want FAILED with script stderr", dispatch.Outcome, dispatch.Reason)
	}

	assertPackagedScriptExitEvent(t, h, 23, "packaged runtime failure\n")
}

func installPackagedScriptRuntimeFixture(t *testing.T, packageName, scriptBody string) string {
	t.Helper()

	payload, err := packageassets.Assemble(packageassets.Definition{
		Package: packageName,
		FactoryJSON: []byte(fmt.Sprintf(`{
  "name":%q,
  "workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers":[{"name":"runner","type":"SCRIPT_WORKER","command":%q}],
  "workstations":[{"name":"run-script","worker":"runner","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}]}]
}`, packageName, packagedRuntimeScriptPath)),
		Assets: fstest.MapFS{
			packagedRuntimeScriptPath: {Data: []byte(scriptBody), Mode: 0o600},
		},
	})
	if err != nil {
		t.Fatalf("packageassets.Assemble: %v", err)
	}

	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), packageName, payload)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	return factoryDir
}

func assertPackagedScriptExitEvent(t *testing.T, h *testutil.ServiceTestHarness, wantExitCode int, wantStderr string) {
	t.Helper()

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
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

func packagedScriptEventTypes(events []factoryapi.FactoryEvent) []string {
	types := make([]string, len(events))
	for i, event := range events {
		types[i] = string(event.Type)
	}
	return types
}
