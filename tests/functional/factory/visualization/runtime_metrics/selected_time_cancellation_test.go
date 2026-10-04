package runtime_metrics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// C06: one hosted process owns both explicit sessions. Routes and gates belong
// to their Factory directories; canceling one cannot release the peer's gate.
func TestSelectedTimeCancellationPreservesPeerSession(t *testing.T) {
	t.Parallel()
	base := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	facts := &selectedTimeNowOnlySource{clock: platformclock.NewDeterministic(base, time.Hour)}
	runner := &selectedCancellationRunner{
		cancelDir:     selectedCancellationFactory(t, "cancel-target"),
		peerDir:       selectedCancellationFactory(t, "surviving-peer"),
		cancelStarted: make(chan struct{}), peerStarted: make(chan struct{}),
		cancelled: make(chan struct{}), releasePeer: make(chan struct{}),
	}
	// The host has no seed: readiness cannot consume an unrelated provider
	// outcome before the two customer sessions open.
	idleDir := support.ScaffoldSingleStepFactory(t, "selected-time-idle-host")
	support.WriteAgentConfig(t, idleDir, "processor", support.BuildModelWorkerConfig("codex", "gpt-5-codex"))
	host := startSelectedTimeHost(t, idleDir, facts, nil, runner)
	idleWork := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(host.url, host.session, "/work"))
	if len(idleWork.Results) != 0 {
		t.Fatal("unseeded host admitted Work before customer sessions opened")
	}
	cancelled := support.OpenFactorySessionAt(t, host.url, runner.cancelDir).Session.Id
	peer := support.OpenFactorySessionAt(t, host.url, runner.peerDir).Session.Id
	awaitSelectedTimeSignal(t, runner.cancelStarted)
	awaitSelectedTimeSignal(t, runner.peerStarted)
	control := selectedCancellationControl(t, host.url, cancelled)
	if control.Operation != factoryapi.FactorySessionLifecycleControlKindCancel || control.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("cancel control = %#v, want accepted cancellation", control)
	}
	awaitSelectedTimeSignal(t, runner.cancelled)
	support.WaitForSessionStopped(t, host.url, cancelled, 30*time.Second)
	before := support.GetFactoryEventsForSessionAt(t, host.url, cancelled)
	assertSelectedCancellationState(t, host.url, cancelled)
	facts.clock.SetTick(1)
	close(runner.releasePeer)
	support.WaitForSessionTerminalStatus(t, host.url, peer, 30*time.Second)
	peerFixture := selectedTimeFixture{url: host.url, session: peer}
	assertSelectedTimeWork(t, peerFixture)
	assertSelectedRuntimeEventTime(t, peerFixture, factoryapi.FactoryEventTypeDispatchResponse, base.Add(time.Hour))
	if after := support.GetFactoryEventsForSessionAt(t, host.url, cancelled); !reflect.DeepEqual(after, before) {
		t.Fatal("closed session history changed when time advanced and peer completed")
	}
	if runner.cancelCalls.Load() != 1 || runner.peerCalls.Load() != 1 {
		t.Fatalf("provider attempts cancel=%d peer=%d, want one each", runner.cancelCalls.Load(), runner.peerCalls.Load())
	}
	host.command.Stop(t)
}

func selectedCancellationFactory(t *testing.T, name string) string {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, name)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"`+name+`"}`))
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig("codex", "gpt-5-codex"))
	return dir
}

type selectedCancellationRunner struct {
	cancelDir, peerDir                                 string
	cancelStarted, peerStarted, cancelled, releasePeer chan struct{}
	cancelCalls, peerCalls                             atomic.Int32
}

func (runner *selectedCancellationRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	switch filepath.Clean(request.WorkDir) {
	case filepath.Clean(runner.cancelDir):
		if runner.cancelCalls.Add(1) != 1 {
			return platformprocess.CommandResult{}, fmt.Errorf("unexpected canceled-session retry")
		}
		close(runner.cancelStarted)
		<-ctx.Done()
		close(runner.cancelled)
		return platformprocess.CommandResult{}, ctx.Err()
	case filepath.Clean(runner.peerDir):
		if runner.peerCalls.Add(1) != 1 {
			return platformprocess.CommandResult{}, fmt.Errorf("unexpected peer retry")
		}
		close(runner.peerStarted)
		return support.NewGatedSuccessCommandRunner("surviving peer COMPLETE", runner.releasePeer).Run(ctx, request)
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider command outside customer sessions: %s", request.WorkDir)
	}
}

func selectedCancellationControl(t *testing.T, baseURL, session string) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	response, err := http.Post(baseURL+"/factory-sessions/"+session+"/cancel", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("cancel status = %d", response.StatusCode)
	}
	var control factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(response.Body).Decode(&control); err != nil {
		t.Fatal(err)
	}
	return control
}

func assertSelectedCancellationState(t *testing.T, baseURL, session string) {
	t.Helper()
	state := support.GetJSON[factoryapi.FactorySession](t, baseURL+"/factory-sessions/"+session)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, session, "/work"))
	if support.CountWorkAtCustomerState(listed, "task:complete") != 0 {
		t.Fatal("canceled session completed Work")
	}
	if state.Runtime.LifecycleControlStatus == nil {
		t.Fatal("closed session lacks lifecycle control status")
	}
	if *state.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("closed live session control status = %s, want SUCCEEDED", *state.Runtime.LifecycleControlStatus)
	}
}
