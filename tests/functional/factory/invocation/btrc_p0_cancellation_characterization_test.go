package oneshot_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBTRCP0OneShotCancellationCharacterization records the current public
// cancellation boundary before any later runtime-opening ownership change.
// The provider gate is released only by the invocation context cancellation,
// so the test has no timing-dependent readiness or completion assumption.
func TestBTRCP0OneShotCancellationCharacterization(t *testing.T) {
	provider := newBTRCBlockingProvider()
	factoryDir := scaffoldBTRCOneShotFactory(t)
	artifactPath := filepath.Join(t.TempDir(), "btrc-one-shot-canceled.replay.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--factory", filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		"--record", artifactPath,
		"--output", "response-stream",
		"btrc one-shot cancellation",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = factoryDir
	invokeContext, cancelInvoke := context.WithCancel(inputs.Input.Context)
	defer cancelInvoke()
	inputs.Input.Context = invokeContext
	provider.cancel = cancelInvoke

	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: provider})
	support.CleanupProcess(t, process)
	command := support.StartProcessCommand(t, process, inputs.Input)
	select {
	case <-provider.started:
	case <-t.Context().Done():
		t.Fatal("test context canceled before provider start")
	}
	command.AcceptError()
	select {
	case <-command.Done():
	case <-t.Context().Done():
		t.Fatal("canceled Process.Execute did not finish")
	}
	// Stop is intentionally idempotent after the terminal invocation has
	// completed. The test calls it twice here; StartProcessCommand's cleanup
	// calls it once more.
	command.Stop(t)
	command.Stop(t)
	if err := command.Err(); err == nil || !strings.Contains(err.Error(), string(factoryapi.INVOCATIONCANCELED)) {
		t.Fatalf("canceled Process.Execute error = %v, want %s terminal error", err, factoryapi.INVOCATIONCANCELED)
	}
	if got := provider.runs.Load(); got != 1 {
		t.Fatalf("provider command calls = %d, want exactly one after repeated cancellation", got)
	}
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertBTRCOneShotCanceledEventOrder(t, artifact.Events)
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	assertBTRCOneShotResponse(t, response, factoryapi.InvocationTerminalStatusCanceled, response.RequestId, response.TraceId, "")
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONCANCELED {
		t.Fatalf("canceled response errorCode = %#v, want %s", response.ErrorCode, factoryapi.INVOCATIONCANCELED)
	}
	if response.Message == nil || *response.Message != "invocation was canceled while waiting for primary result" {
		t.Fatalf("canceled response message = %#v, want primary-result cancellation diagnostic", response.Message)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("canceled response primaryResult = %#v, want nil", response.PrimaryResult)
	}
	assertBTRCOneShotTerminalSession(t, artifact.Events, btrcOneShotSessionSucceeded)
	assertBTRCOneShotResponseStreamHasOneTerminalRecord(t, inputs.Stdout())
	lines := strings.Split(strings.TrimSpace(inputs.Stdout()), "\n")
	lastLine := ""
	if len(lines) > 0 {
		lastLine = lines[len(lines)-1]
	}
	if lastLine == "" || !strings.Contains(lastLine, `"recordType":"invocation_result"`) {
		t.Fatalf("response stream last record = %q, want the single terminal invocation_result", lastLine)
	}
}

var btrcOneShotCanceledEventOrders = [][]interfaces.FactoryEventType{
	{
		interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeInitialStructureRequest,
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeFactoryStateResponse,
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		interfaces.FactoryEventTypeModelRequest,
		interfaces.FactoryEventTypeModelResponse,
		interfaces.FactoryEventTypeAgentRunResponse,
		interfaces.FactoryEventTypeFactoryStateResponse,
		interfaces.FactoryEventTypeRunResponse,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionCompleted,
	},
	{
		interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeInitialStructureRequest,
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeFactoryStateResponse,
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
		interfaces.FactoryEventTypeModelRequest,
		interfaces.FactoryEventTypeFactoryStateResponse,
		interfaces.FactoryEventTypeModelResponse,
		interfaces.FactoryEventTypeAgentRunResponse,
		interfaces.FactoryEventTypeRunResponse,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionCompleted,
	},
}

func assertBTRCOneShotCanceledEventOrder(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	types := make([]interfaces.FactoryEventType, len(events))
	for index, event := range events {
		types[index] = event.Type
		if event.Context.Sequence != index {
			t.Fatalf("canceled event[%d] sequence = %d, want %d", index, event.Context.Sequence, index)
		}
		if strings.TrimSpace(event.Id) == "" {
			t.Fatalf("canceled event[%d] id is empty", index)
		}
	}
	for _, want := range btrcOneShotCanceledEventOrders {
		if reflect.DeepEqual(types, want) {
			return
		}
	}
	t.Fatalf("canceled canonical event order = %v, want one of %v", types, btrcOneShotCanceledEventOrders)
}

type btrcBlockingProvider struct {
	started chan struct{}
	cancel  context.CancelFunc
	runs    atomic.Int32
}

func newBTRCBlockingProvider() *btrcBlockingProvider {
	return &btrcBlockingProvider{started: make(chan struct{}, 1)}
}

func (provider *btrcBlockingProvider) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	provider.runs.Add(1)
	select {
	case provider.started <- struct{}{}:
	default:
	}
	if provider.cancel != nil {
		provider.cancel()
	}
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

var _ platformprocess.CommandRunner = (*btrcBlockingProvider)(nil)
