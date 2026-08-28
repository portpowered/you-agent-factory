package providers

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMockWorkers_ServiceCommandRunnerCompletesModelAndScriptWorkers(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker service-runner sweep")
	for _, test := range []struct {
		name    string
		fixture string
		workID  string
		traceID string
	}{
		{name: "model worker", fixture: "executor_success", workID: sharedMockServiceModelWorkID, traceID: "trace-shared-mock-service-model"},
		{name: "script worker", fixture: "script_executor_dir", workID: sharedMockServiceScriptWorkID, traceID: "trace-shared-mock-service-script"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, test.fixture))
			testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
				WorkID:     test.workID,
				WorkTypeID: "task",
				TraceID:    test.traceID,
				Payload:    []byte("mock-worker service payload"),
			})

			var runner platformprocess.CommandRunner = support.NewStaticSuccessCommandRunner("mock worker accepted")
			if test.fixture == "executor_success" {
				support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
				runner = support.NewShapedProviderCommandRunner(
					platformprocess.CommandResult{Stdout: []byte("mock worker accepted\nCOMPLETE")},
				)
			}
			scenario, listed := runSharedMockFactory(t, dir, runner, 5*time.Second)
			if len(listed.Results) != 1 || listed.Results[0].State == nil ||
				listed.Results[0].State.Type != factoryapi.WorkStateTypeTERMINAL {
				t.Fatalf("GET /work results = %#v, want one terminal work", listed.Results)
			}
			if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
				t.Fatalf("task:failed token count = %d, want zero", got)
			}

			fixture := scenario.Fixture()
			scenario.Stop(t)
			record := findSharedRuntimeLogRecord(t, fixture, dir, 0)
			if _, ok := record["stdout"]; ok {
				t.Fatal("command runner completion should omit stdout on success")
			}
			if _, ok := record["stderr"]; ok {
				t.Fatal("command runner completion should omit stderr on success")
			}
		})
	}
}
