package subsystems

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func testSubsystemNow() time.Time { return time.Now() }

func testTokenTransformer(net *state.Net) *token_transformer.Transformer {
	return token_transformer.New(net.Places, net.WorkTypes, petri.NewWorkIDGenerator())
}

// Each execution builds its own mutable component fixture. Capture and Noop
// operations start together, so neither policy owns the peer's state or sink.
func assertLoggerParity(t *testing.T, execute func(logging.Logger) (*interfaces.TickResult, error)) (*interfaces.TickResult, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	policies := []logging.Logger{logging.NewZapLogger(zap.New(core), false), logging.NoopLogger{}}
	type observation struct {
		result *interfaces.TickResult
		err    error
	}
	outputs := []chan observation{make(chan observation, 1), make(chan observation, 1)}
	start := make(chan struct{})
	for i, logger := range policies {
		go func() {
			<-start
			result, err := execute(logger)
			outputs[i] <- observation{result, err}
		}()
	}
	close(start)
	captured, quiet := <-outputs[0], <-outputs[1]
	if !reflect.DeepEqual(captured.result, quiet.result) || fmt.Sprint(captured.err) != fmt.Sprint(quiet.err) {
		t.Fatalf("logger policies changed behavior: capture=(%#v, %v), Noop=(%#v, %v)", captured.result, captured.err, quiet.result, quiet.err)
	}
	return captured.result, logs
}

func assertDiagnostic(t *testing.T, logs *observer.ObservedLogs, level zapcore.Level, message string, fields map[string]any) {
	t.Helper()
	entries := logs.FilterMessage(message).All()
	if len(entries) != 1 || entries[0].Level != level {
		t.Fatalf("diagnostic %q = %#v, want exactly one %s record", message, entries, level)
	}
	got := entries[0].ContextMap()
	for key, want := range fields {
		if !reflect.DeepEqual(got[key], want) {
			t.Errorf("diagnostic %q field %s = %#v, want %#v", message, key, got[key], want)
		}
	}
}

func loggerParityNow() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }

func loggerParityToken(id, place string) *factorytoken.Token {
	return &factorytoken.Token{
		ID: id, PlaceID: place, CreatedAt: loggerParityNow(), EnteredAt: loggerParityNow(),
		Color: factorytoken.Color{WorkID: id, WorkTypeID: "task", DataType: factorytoken.DataTypeWork, RequestID: "request-1", TraceID: "trace-1"},
	}
}

func loggerParityMarking(tokens map[string]*factorytoken.Token) petri.MarkingSnapshot {
	places := make(map[string][]string)
	for id, token := range tokens {
		places[token.PlaceID] = append(places[token.PlaceID], id)
	}
	return petri.MarkingSnapshot{Tokens: tokens, PlaceTokens: places}
}

type loggerParityScheduler struct{ decisions []interfaces.FiringDecision }

func (s loggerParityScheduler) Select(_ []interfaces.EnabledTransition, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	return s.decisions
}

func TestDispatcherSelectedLoggerParity(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"dispatch", "invalid and duplicate claims", "no enabled transitions", "seeded replay"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			result, logs := assertLoggerParity(t, func(logger logging.Logger) (*interfaces.TickResult, error) {
				return executeLoggerParityDispatch(scenario, logger)
			})
			if scenario == "no enabled transitions" {
				if result != nil {
					t.Fatalf("idle result = %#v, want nil", result)
				}
				assertDiagnostic(t, logs, zapcore.DebugLevel, "dispatcher: no enabled transitions", nil)
				return
			}
			if scenario == "seeded replay" {
				if result != nil && (len(result.Dispatches) != 0 || len(result.Mutations) != 0) {
					t.Fatalf("seeded result = %#v, want no dispatch/consumption", result)
				}
				assertDiagnostic(t, logs, zapcore.InfoLevel, "dispatcher: preserving seeded replay Work without an unrecorded dispatch", map[string]any{"transitionID": "t1", "workIDs": []any{"work-1"}})
				return
			}
			if result == nil || len(result.Dispatches) != 1 || len(result.Mutations) != 1 || result.Mutations[0].TokenID != "work-1" || result.Dispatches[0].Dispatch.DispatchID != "dispatch-1" {
				t.Fatalf("dispatch result = %#v, want one correlated dispatch and consumption", result)
			}
			assertDiagnostic(t, logs, zapcore.InfoLevel, "dispatcher: dispatching work to worker", map[string]any{"request_id": "request-1", "trace_id": "trace-1", "work_id": "work-1", "transition_id": "t1", "worker_type": "script", "input_tokens": int64(1)})
			if scenario == "invalid and duplicate claims" {
				assertDiagnostic(t, logs, zapcore.WarnLevel, "dispatcher: skipping firing decision with missing transition id", nil)
				assertDiagnostic(t, logs, zapcore.WarnLevel, "dispatcher: transition from firing decision not found in net", map[string]any{"transitionID": "unknown", "workerType": "script"})
				assertDiagnostic(t, logs, zapcore.WarnLevel, "dispatcher: skipping decision due to duplicate token claim", map[string]any{"tokenID": "work-1"})
			}
		})
	}
}

type loggerParityReplay struct{}

func (loggerParityReplay) DispatchIDForDispatch(work.WorkDispatch) (string, bool) { return "", false }

func TestHistorySelectedLoggerParity(t *testing.T) {
	t.Parallel()
	result, logs := assertLoggerParity(t, func(logger logging.Logger) (*interfaces.TickResult, error) {
		snapshot := workerBatchSnapshot("")
		snapshot.Topology = workerBatchTestNet()
		snapshot.Dispatches["dispatch-1"].ConsumedTokens[0].History = workers.History{TotalVisits: map[string]int{"t1": 2}, ConsecutiveFailures: map[string]int{"t1": 1}}
		peer := snapshot.Dispatches["dispatch-1"].ConsumedTokens[0]
		peer.Color.WorkID = "peer"
		peer.History = workers.History{TotalVisits: map[string]int{"t1": 100}}
		snapshot.Dispatches["dispatch-1"].ConsumedTokens = append(snapshot.Dispatches["dispatch-1"].ConsumedTokens, peer)
		snapshot.Results = []workers.WorkResult{
			{DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workers.OutcomeFailed},
			{DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workers.OutcomeAccepted},
			{DispatchID: "dispatch-1", TransitionID: "t1", Outcome: workers.OutcomeFailed, FailureMetadata: &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyThrottle}},
		}
		return NewHistory(logger).Execute(context.Background(), snapshot)
	})
	if result == nil || len(result.Histories) != 3 {
		t.Fatalf("histories = %#v, want three in result order", result)
	}
	for i, failures := range []int{2, 0, 1} {
		if result.Histories[i].TotalVisits["t1"] != 3 || result.Histories[i].ConsecutiveFailures["t1"] != failures {
			t.Fatalf("history %d = %#v, want visits=3 failures=%d without peer history", i, result.Histories[i], failures)
		}
	}
	assertDiagnostic(t, logs, zapcore.DebugLevel, "history: computed token histories", map[string]any{"count": int64(3)})
	if len(logs.All()[0].ContextMap()) != 1 {
		t.Fatal("history diagnostic gained identity/payload fields")
	}
}

func TestTransitionerSelectedLoggerParity(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"accepted", "retry timeout", "terminal failure", "canceled", "unknown transition"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			result, logs := assertLoggerParity(t, func(logger logging.Logger) (*interfaces.TickResult, error) {
				return executeLoggerParityTransition(scenario, logger)
			})
			if scenario == "unknown transition" {
				if result != nil {
					t.Fatalf("unknown transition result = %#v, want nil", result)
				}
				assertDiagnostic(t, logs, zapcore.ErrorLevel, "transitioner: unknown transition in result", map[string]any{"transitionID": "unknown"})
				assertDiagnostic(t, logs, zapcore.ErrorLevel, "transitioner: error processing result", map[string]any{"transition": "unknown"})
				if !strings.Contains(fmt.Sprint(logs.FilterMessage("transitioner: error processing result").All()[0].ContextMap()["error"]), "unknown transition unknown") {
					t.Fatal("missing wrapped processing error")
				}
				return
			}
			if result == nil || len(result.CompletedDispatches) != 1 || result.CompletedDispatches[0].DispatchID != "dispatch-1" {
				t.Fatalf("result = %#v, want correlated completion", result)
			}
			fields := map[string]any{"dispatch_id": "dispatch-1", "transition_id": "t1", "work_id": "work-source", "work_name": "source"}
			message, level, destination := "transitioner: result accepted", zapcore.InfoLevel, "task:complete"
			switch scenario {
			case "retry timeout", "terminal failure":
				message, level = "transitioner: result failed", zapcore.ErrorLevel
				fields["failure_reason"] = string(workers.WorkFailureTypeTimeout)
				destination = "task:init"
				if scenario == "terminal failure" {
					destination = "task:failed"
					fields["failure_reason"] = string(workers.WorkFailureTypePermanentBadRequest)
				}
			case "canceled":
				message, destination = "transitioner: result canceled", "task:init"
				fields["cancellation_reason"] = string(workers.DispatchCancellationReasonSuperseded)
				assertCanceledDispatchRestoration(t, result, loggerParityNow())
			}
			if len(result.Mutations) != 1 || result.Mutations[0].ToPlace != destination {
				t.Fatalf("mutations = %#v, want one to %s", result.Mutations, destination)
			}
			assertDiagnostic(t, logs, level, message, fields)
			for _, entry := range logs.All() {
				if strings.Contains(fmt.Sprint(entry.ContextMap()), "raw-secret-payload") {
					t.Fatalf("raw failure leaked in diagnostic %q", entry.Message)
				}
			}
		})
	}
}

func TestCircuitBreakerSelectedLoggerParity(t *testing.T) {
	t.Parallel()
	for _, breached := range []bool{false, true} {
		t.Run(fmt.Sprintf("breached=%t", breached), func(t *testing.T) {
			t.Parallel()
			result, logs := assertLoggerParity(t, func(logger logging.Logger) (*interfaces.TickResult, error) {
				n := workerBatchTestNet()
				n.Limits.MaxTokenAge = time.Hour
				token := loggerParityToken("work-1", "task:init")
				if breached {
					token.CreatedAt = loggerParityNow().Add(-2 * time.Hour)
					token.History.TotalVisits = map[string]int{"t1": 3}
				}
				n.Transitions["exhausted"] = &petri.Transition{ID: "exhausted", Type: petri.TransitionExhaustion, InputArcs: []petri.Arc{{PlaceID: "task:init", Guard: &petri.VisitCountGuard{TransitionID: "t1", MaxVisits: 3}}}, OutputArcs: []petri.Arc{{PlaceID: "task:failed"}}}
				return NewCircuitBreakerWithClock(n, loggerParityNow, logger, nil).Execute(context.Background(), &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{"work-1": token, "peer": loggerParityToken("peer", "task:init"), "terminal": loggerParityToken("terminal", "task:complete")}}})
			})
			if !breached {
				if result != nil || logs.Len() != 0 {
					t.Fatalf("within limits = %#v, %d logs, want no effect", result, logs.Len())
				}
				return
			}
			if result == nil || len(result.Mutations) != 1 || result.Mutations[0].TokenID != "work-1" || result.Mutations[0].ToPlace != "task:failed" || len(result.Mutations[0].FailureRecords) != 1 {
				t.Fatalf("breach result = %#v, want only work-1 failure", result)
			}
			assertDiagnostic(t, logs, zapcore.InfoLevel, "circuit-breaker: limit breached", map[string]any{"token": "work-1", "place": "task:init", "reason": "token age 2h0m0s exceeds max 1h0m0s"})
			assertDiagnostic(t, logs, zapcore.InfoLevel, "circuit-breaker: breaking token ", map[string]any{"tokenID": "work-1", "from": "task:init", "to": "task:failed"})
		})
	}
}

func TestCascadingFailureSelectedLoggerParity(t *testing.T) {
	t.Parallel()
	result, logs := assertLoggerParity(t, func(logger logging.Logger) (*interfaces.TickResult, error) {
		n := workerBatchTestNet()
		child, grandchild := loggerParityToken("child", "task:init"), loggerParityToken("grandchild", "task:init")
		child.Color.Relations = []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "parent"}}
		grandchild.Color.Relations = []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "child"}}
		terminal := loggerParityToken("terminal", "task:complete")
		terminal.Color.Relations = []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "parent"}}
		snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{"parent": loggerParityToken("parent", "task:failed"), "child": child, "grandchild": grandchild, "peer": loggerParityToken("peer", "task:init"), "terminal": terminal}}}
		return NewCascadingFailure(n, logger, loggerParityNow).Execute(context.Background(), snapshot)
	})
	if result == nil || len(result.Mutations) != 2 {
		t.Fatalf("cascade = %#v, want two BFS moves", result)
	}
	entries := logs.FilterMessage("cascading-failure: propagating failure").All()
	if len(entries) != 2 {
		t.Fatalf("cascade diagnostics = %#v, want two records", entries)
	}
	for i, id := range []string{"child", "grandchild"} {
		m := result.Mutations[i]
		if m.TokenID != id || m.ToPlace != "task:failed" || len(m.FailureRecords) != 1 || m.FailureRecords[0].Timestamp != loggerParityNow() {
			t.Fatalf("cascade mutation %d = %#v", i, m)
		}
		dependency := []string{"parent", "child"}[i]
		if entries[i].Level != zapcore.InfoLevel || !reflect.DeepEqual(entries[i].ContextMap(), map[string]any{"token": id, "dependency": dependency, "to_place": "task:failed"}) {
			t.Fatalf("cascade diagnostic %d = %#v", i, entries[i])
		}
	}
}

func TestTerminationSelectedLoggerParity(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"empty", "terminal", "failed", "drained duplicate Work", "runnable", "in flight", "awaiting retirement", "missing resource", "service"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			result, logs := assertLoggerParity(t, func(logger logging.Logger) (*interfaces.TickResult, error) {
				return executeLoggerParityTermination(scenario, logger)
			})
			classified := scenario == "empty" || scenario == "terminal" || scenario == "failed" || scenario == "drained duplicate Work"
			if !classified {
				if result != nil || logs.FilterMessage("termination-check: finite runtime classified").Len() != 0 {
					t.Fatalf("active/service result = %#v, want no classification", result)
				}
				return
			}
			classification, count := interfaces.TerminationClassificationComplete, 0
			if scenario == "drained duplicate Work" {
				classification, count = interfaces.TerminationClassificationIncomplete, 1
			}
			if result == nil || !result.ShouldTerminate || result.Termination.Classification != classification || result.Termination.NonTerminalWorkCount != count {
				t.Fatalf("termination = %#v, want %s count=%d", result, classification, count)
			}
			assertDiagnostic(t, logs, zapcore.InfoLevel, "termination-check: finite runtime classified", map[string]any{"classification": classification, "non_terminal_work_items": int64(count), "in_flight": int64(0)})
		})
	}
}

func executeLoggerParityDispatch(scenario string, logger logging.Logger) (*interfaces.TickResult, error) {
	n := workerBatchTestNet()
	n.Transitions["t1"].Type = petri.TransitionNormal
	n.Transitions["t1"].InputArcs = []petri.Arc{{PlaceID: "task:init", Direction: petri.ArcInput}}
	decision := interfaces.FiringDecision{TransitionID: "t1", ConsumeTokens: []string{"work-1"}, WorkerType: "script"}
	decisions := []interfaces.FiringDecision{decision}
	if scenario == "invalid and duplicate claims" {
		decisions = []interfaces.FiringDecision{{}, {TransitionID: "unknown", WorkerType: "script"}, decision, decision}
	}
	tokens := map[string]*factorytoken.Token{"work-1": loggerParityToken("work-1", "task:init")}
	if scenario == "no enabled transitions" {
		tokens = nil
	}
	d := NewDispatcher(n, loggerParityScheduler{decisions}, nil, logger, nil, loggerParityNow, func() string { return "dispatch-1" })
	if scenario == "seeded replay" {
		d = NewDispatcherWithSeededReplay(n, loggerParityScheduler{decisions}, nil, logger, nil, loggerParityNow, func() string { return "dispatch-1" }, loggerParityReplay{}, map[string]struct{}{"work-1": {}})
	}
	return d.Execute(context.Background(), &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: loggerParityMarking(tokens)})
}

func executeLoggerParityTransition(scenario string, logger logging.Logger) (*interfaces.TickResult, error) {
	n := workerBatchTestNet()
	snapshot := workerBatchSnapshot("")
	if scenario == "retry timeout" || scenario == "terminal failure" {
		n.Transitions["t1"].FailureArcs = []petri.Arc{{ID: "retry", PlaceID: "task:init"}}
		snapshot.Results[0].Outcome = workers.OutcomeFailed
		snapshot.Results[0].Error = "raw-secret-payload"
		snapshot.Results[0].FailureMetadata = &workers.WorkFailureMetadata{Type: workers.WorkFailureTypeTimeout}
		if scenario == "terminal failure" {
			snapshot.Results[0].FailureMetadata.Family = workers.WorkFailureFamilyTerminal
			snapshot.Results[0].FailureMetadata.Type = workers.WorkFailureTypePermanentBadRequest
		}
	}
	if scenario == "canceled" {
		snapshot.Results[0].Outcome = workers.OutcomeCanceled
		snapshot.Results[0].Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonSuperseded}
		snapshot.Dispatches["dispatch-1"].HeldMutations = []interfaces.MarkingMutation{{Type: interfaces.MutationConsume, TokenID: "tok-source", FromPlace: "task:init"}}
	}
	if scenario == "unknown transition" {
		snapshot.Results[0].TransitionID = "unknown"
	}
	return NewTransitioner(n, logger, loggerParityNow, testTokenTransformer(n), nil, nil, nil, testWorkPropagationPolicy()).Execute(context.Background(), snapshot)
}

func TestTransitionerSelectedLoggerResourceParity(t *testing.T) {
	t.Parallel()
	for _, outcome := range []workers.WorkOutcome{workers.OutcomeFailed, workers.OutcomeCanceled} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			result, _ := assertLoggerParity(t, func(logger logging.Logger) (*interfaces.TickResult, error) {
				n := workerBatchTestNet()
				n.Places["slot:available"] = &petri.Place{ID: "slot:available", TypeID: "slot", State: "available"}
				snapshot := workerBatchSnapshot("")
				slot := loggerParityToken("slot-1", "slot:available")
				slot.Color.DataType, slot.Color.WorkTypeID = factorytoken.DataTypeResource, "slot"
				snapshot.Dispatches["dispatch-1"].ConsumedTokens = append(snapshot.Dispatches["dispatch-1"].ConsumedTokens, factorytoken.ToWorker(*slot))
				snapshot.Dispatches["dispatch-1"].HeldMutations = []interfaces.MarkingMutation{{Type: interfaces.MutationConsume, TokenID: "tok-source", FromPlace: "task:init"}, {Type: interfaces.MutationConsume, TokenID: "slot-1", FromPlace: "slot:available"}}
				snapshot.Results[0].Outcome = outcome
				if outcome == workers.OutcomeCanceled {
					snapshot.Results[0].Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonSuperseded}
				}
				return NewTransitioner(n, logger, loggerParityNow, testTokenTransformer(n), nil, nil, nil, testWorkPropagationPolicy()).Execute(context.Background(), snapshot)
			})
			if result == nil || len(result.Mutations) != 2 || len(result.CompletedDispatches) != 1 {
				t.Fatalf("resource result = %#v, want work routing/restoration and resource return", result)
			}
			var restored *workers.Token
			for _, mutation := range result.Mutations {
				if mutation.ToPlace == "slot:available" {
					restored = mutation.NewToken
				}
			}
			if restored == nil || restored.ID != "slot-1" || restored.Color.WorkID != "slot-1" || restored.CreatedAt != loggerParityNow() {
				t.Fatalf("restored resource = %#v, want original identity and creation time", restored)
			}
			if outcome == workers.OutcomeCanceled && result.CompletedDispatches[0].FailureMetadata != nil {
				t.Fatal("cancellation acquired business failure metadata")
			}
		})
	}
}

func TestTransitionerSelectedLoggerPeerIsolation(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zapcore.DebugLevel)
	start := make(chan struct{})
	results := make(chan *interfaces.TickResult, 2)
	for _, canceled := range []bool{false, true} {
		go func() {
			<-start
			logger := logging.Logger(logging.NoopLogger{})
			var result *interfaces.TickResult
			var err error
			if canceled {
				logger = logging.NewZapLogger(zap.New(core), false)
				result, err = executeLoggerParityTransition("canceled", logger)
			} else {
				n := workerBatchTestNet()
				snapshot := workerBatchSnapshot("")
				entry := snapshot.Dispatches["dispatch-1"]
				entry.DispatchID = "peer-dispatch"
				entry.ConsumedTokens[0].Color.WorkID = "peer-work"
				snapshot.Dispatches = map[string]*interfaces.DispatchEntry{"peer-dispatch": entry}
				snapshot.Results[0].DispatchID = "peer-dispatch"
				result, err = NewTransitioner(n, logger, loggerParityNow, testTokenTransformer(n), nil, nil, nil, testWorkPropagationPolicy()).Execute(context.Background(), snapshot)
			}
			if err != nil {
				results <- nil
				return
			}
			results <- result
		}()
	}
	close(start)
	outcomes := make(map[workers.WorkOutcome]bool)
	for range 2 {
		result := <-results
		if result == nil || len(result.CompletedDispatches) != 1 {
			t.Fatalf("peer result = %#v, want completion", result)
		}
		outcomes[result.CompletedDispatches[0].Outcome] = true
		wantDispatch := "dispatch-1"
		if result.CompletedDispatches[0].Outcome == workers.OutcomeAccepted {
			wantDispatch = "peer-dispatch"
		}
		if result.CompletedDispatches[0].DispatchID != wantDispatch {
			t.Fatalf("peer completion identity = %q, want %s", result.CompletedDispatches[0].DispatchID, wantDispatch)
		}
	}
	if !outcomes[workers.OutcomeAccepted] || !outcomes[workers.OutcomeCanceled] {
		t.Fatalf("peer outcomes = %#v, want independent success and cancellation", outcomes)
	}
	assertDiagnostic(t, logs, zapcore.InfoLevel, "transitioner: result canceled", map[string]any{"cancellation_reason": string(workers.DispatchCancellationReasonSuperseded)})
	if logs.FilterMessage("transitioner: result accepted").Len() != 0 {
		t.Fatal("Noop peer leaked into selected capture")
	}
}

func executeLoggerParityTermination(scenario string, logger logging.Logger) (*interfaces.TickResult, error) {
	n := workerBatchTestNet()
	n.Transitions = nil
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{}}}
	mode := interfaces.RuntimeModeBatch
	switch scenario {
	case "terminal":
		snapshot.Marking.Tokens["work-1"] = loggerParityToken("work-1", "task:complete")
	case "failed":
		snapshot.Marking.Tokens["work-1"] = loggerParityToken("work-1", "task:failed")
	case "drained duplicate Work":
		snapshot.Marking.Tokens["a"] = loggerParityToken("work-1", "task:init")
		snapshot.Marking.Tokens["b"] = loggerParityToken("work-1", "task:init")
		snapshot.Marking.Tokens["a"].ID = "a"
		snapshot.Marking.Tokens["b"].ID = "b"
	case "runnable":
		n.Transitions = map[string]*petri.Transition{"t1": {ID: "t1", Type: petri.TransitionNormal, InputArcs: []petri.Arc{{PlaceID: "task:init", Direction: petri.ArcInput}}}}
		snapshot.Marking.Tokens["work-1"] = loggerParityToken("work-1", "task:init")
	case "in flight":
		snapshot.InFlightCount = 1
	case "awaiting retirement":
		snapshot.Dispatches = map[string]*interfaces.DispatchEntry{"dispatch-1": {DispatchID: "dispatch-1"}}
	case "missing resource":
		n.Resources = map[string]*state.ResourceDef{"slot": {ID: "slot", Capacity: 1}}
	case "service":
		mode = interfaces.RuntimeModeService
	}
	snapshot.Marking = loggerParityMarking(snapshot.Marking.Tokens)
	return NewTerminationCheckWithRuntime(n, logger, mode, nil, loggerParityNow).Execute(context.Background(), snapshot)
}
