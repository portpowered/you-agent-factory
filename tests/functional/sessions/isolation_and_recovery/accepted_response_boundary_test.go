package isolation_and_recovery_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logging is an injected external effect. Holding its write at result routing
// leaves the real accepted response recorded, but not yet applied to Work.
func TestAcceptedResponseSurvivesAcceptanceBeforePause(t *testing.T) {
	t.Parallel()
	core := &acceptedApplicationLogGate{entered: make(chan struct{}), release: make(chan struct{})}
	providerGate := make(chan struct{})
	runner := &pausedResponseRunner{runner: support.NewGatedSuccessCommandRunner("COMPLETE", providerGate), arrived: make(chan struct{}, 8)}
	dir := support.ScaffoldFactory(t, acceptedResponsePauseFactory())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	for _, station := range []string{"process", "finish"} {
		support.WriteWorkstationConfig(t, dir, station, "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir, WaitForServiceModeRuntime: true,
		Args: []string{"--debug"}, Edges: serviceedges.Edges{ProviderCommandRunner: runner, ProcessLogger: zap.New(core)}})
	t.Cleanup(func() { server.Stop(t) })
	t.Cleanup(core.releaseApplication)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	work := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: "accept before pause"})
	select {
	case <-runner.arrived:
	case <-ctx.Done():
		t.Fatal("original execution did not start")
	}
	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	core.armed.Store(true)
	close(providerGate)
	select {
	case <-core.entered:
	case <-ctx.Done():
		t.Fatal("accepted result did not reach application boundary")
	}
	// Pause may await the blocked engine snapshot before returning. Its durable
	// event is the acknowledgement that scheduling has been paused.
	pauseDone := make(chan struct{})
	go func() {
		defer close(pauseDone)
		postSessionsLifecycleControl(t, server.URL(), factorysessions.DefaultSessionID, factoryapi.FactorySessionLifecycleControlKindPause)
	}()
	assertAcceptancePrecedesPause(t, ctx, stream)
	core.releaseApplication()
	select {
	case <-pauseDone:
	case <-ctx.Done():
		t.Fatal("pause did not finish after releasing application")
	}
	waitAcceptedPauseDispatchResponse(t, ctx, stream, *work.WorkId)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertAcceptedPauseOutput(t, listed, *work.WorkId)
	if len(listed.Results) != 1 || !support.HasWorkAtCustomerState(listed, *work.WorkId, "task:processed") {
		t.Fatalf("paused accepted Work = %#v, want one processed Work", listed.Results)
	}
	session := support.GetDefaultSession(t, server.URL())
	if session.Runtime.LifecycleControlStatus == nil || *session.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused || runner.calls.Load() != 1 {
		t.Fatalf("accepted boundary lost pause or dispatched downstream: %#v calls=%d", session.Runtime, runner.calls.Load())
	}
	postSessionsLifecycleControl(t, server.URL(), factorysessions.DefaultSessionID, factoryapi.FactorySessionLifecycleControlKindResume)
	support.WaitForRuntimeIdle(t, server.URL(), time.Minute)
	if events := support.ObserveDispatchEvents(t, support.GetFactoryEventsAt(t, server.URL())); len(events) != 2 || runner.calls.Load() != 2 {
		t.Fatalf("resume dispatches=%d calls=%d, want original and downstream once", len(events), runner.calls.Load())
	}
}

func assertAcceptancePrecedesPause(t *testing.T, ctx context.Context, stream *support.FactoryEventStream) {
	t.Helper()
	accepted := false
	for {
		event := stream.NextEventContext(ctx)
		switch string(event.Type) {
		case "AGENT_RUN_RESPONSE":
			response, err := event.Payload.AsAgentRunResponseEventPayload()
			if err != nil || string(response.Outcome) != "ACCEPTED" {
				t.Fatalf("agent acceptance = %#v, %v", response, err)
			}
			accepted = true
		case "DISPATCH_RESPONSE":
			t.Fatal("result applied before pause boundary")
		case "SESSION_PAUSED":
			if !accepted {
				t.Fatal("pause did not follow recorded acceptance")
			}
			return
		}
	}
}

type acceptedApplicationLogGate struct {
	armed            atomic.Bool
	entered, release chan struct{}
	once             sync.Once
}

func (*acceptedApplicationLogGate) Enabled(zapcore.Level) bool             { return true }
func (core *acceptedApplicationLogGate) With([]zapcore.Field) zapcore.Core { return core }
func (core *acceptedApplicationLogGate) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return checked.AddCore(entry, core)
}
func (core *acceptedApplicationLogGate) Write(entry zapcore.Entry, _ []zapcore.Field) error {
	if entry.Message == "transitioner: processing results" && core.armed.CompareAndSwap(true, false) {
		close(core.entered)
		<-core.release
	}
	return nil
}
func (*acceptedApplicationLogGate) Sync() error { return nil }
func (core *acceptedApplicationLogGate) releaseApplication() {
	core.once.Do(func() { close(core.release) })
}
