package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestRunHostedRuntimePreparesHomeBeforeRuntimeStart(t *testing.T) {
	var output bytes.Buffer
	var events []string
	err := runHostedRuntime(
		t.Context(),
		RunConfig{
			HomeDir:                           "operator-home",
			StartupOutput:                     &output,
			DeferHomeDisclosureUntilHostReady: true,
			WithServer:                        true,
			StartupPreparation: func(_ context.Context, discloseHome bool, writer io.Writer) error {
				if !discloseHome {
					t.Fatal("hosted startup did not request the home disclosure")
				}
				events = append(events, "home")
				_, err := fmt.Fprintln(writer, "Home directory: operator-home")
				return err
			},
		},
		zap.NewNop(),
		nil,
		resolvedRunRecordPath{},
		nil,
		nil,
		nil,
		false,
		0,
		func(
			_ context.Context,
			_ *factorysessions.SessionStartRequest,
			_ initializer.InvocationCancellation,
			_ factorysessions.VisualizationSinkID,
		) (initializer.LocalRuntimeRunner, error) {
			events = append(events, "runtime log and metrics")
			return runFuncRunner(func(context.Context) error { return nil }), nil
		},
		func(RunConfig, *workers.MockWorkersConfig) *factorysessions.SessionStartRequest {
			return &factorysessions.SessionStartRequest{}
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("runHostedRuntime() error = %v", err)
	}
	if got, want := strings.Join(events, ","), "home,runtime log and metrics"; got != want {
		t.Fatalf("startup events = %q, want %q", got, want)
	}
	if got, want := output.String(), "Home directory: operator-home\n"; !strings.HasPrefix(got, want) {
		t.Fatalf("startup disclosure = %q, want prefix %q", got, want)
	}
}

func TestRunHostedRuntimeUsesHostedInvocationCapability(t *testing.T) {
	sessions := &hostedInvocationCapabilityFake{}
	request := invocationRequestFromText("summarize the dispatch")
	const sessionID = "session-explicit"

	err := runHostedRuntime(
		t.Context(),
		RunConfig{Output: io.Discard, WithServer: true, FactorySessionID: sessionID},
		zap.NewNop(),
		request,
		resolvedRunRecordPath{},
		testInvocationOperation{},
		nil,
		nil,
		true,
		0,
		func(
			_ context.Context,
			_ *factorysessions.SessionStartRequest,
			_ initializer.InvocationCancellation,
			_ factorysessions.VisualizationSinkID,
		) (initializer.LocalRuntimeRunner, error) {
			return WithHostedInvocation(hostedInvocationCompletionRunner{}, sessions), nil
		},
		func(RunConfig, *workers.MockWorkersConfig) *factorysessions.SessionStartRequest {
			return &factorysessions.SessionStartRequest{}
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("runHostedRuntime() error = %v", err)
	}
	if !sessions.invoked {
		t.Fatal("hosted Factory Sessions invocation was not used")
	}
	if sessions.invokedSessionID != sessionID {
		t.Fatalf("invoked session = %q, want %q", sessions.invokedSessionID, sessionID)
	}
}

func TestHostedRuntimeConfigClearsFiniteWorkFile(t *testing.T) {
	cleanConfig := hostedRuntimeConfig(RunConfig{CleanInvocation: true, WorkFile: "work.json"}, true)
	if cleanConfig.WorkFile != "" {
		t.Fatalf("clean runtime WorkFile = %q, want empty after owner projection", cleanConfig.WorkFile)
	}

	ordinaryConfig := hostedRuntimeConfig(RunConfig{WorkFile: "work.json"}, false)
	if ordinaryConfig.WorkFile != "work.json" {
		t.Fatalf("ordinary runtime WorkFile = %q, want original batch input", ordinaryConfig.WorkFile)
	}
}

func TestRunHostedRuntimeClosesVisualizationSinkAfterFailure(t *testing.T) {
	owner := &runSinkOwnerProbe{}
	want := context.Canceled
	var sinkID factorysessions.VisualizationSinkID
	err := runHostedRuntime(
		t.Context(), RunConfig{Output: io.Discard}, zap.NewNop(), nil,
		resolvedRunRecordPath{}, nil, testResponsePresentation(), nil, false, 0,
		func(_ context.Context, _ *factorysessions.SessionStartRequest, _ initializer.InvocationCancellation, id factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			sinkID = id
			return runFuncRunner(func(context.Context) error { return want }), nil
		},
		func(RunConfig, *workers.MockWorkersConfig) *factorysessions.SessionStartRequest {
			return &factorysessions.SessionStartRequest{}
		}, nil, owner,
	)
	if !errors.Is(err, want) {
		t.Fatalf("runHostedRuntime() error = %v, want %v", err, want)
	}
	if sinkID == "" {
		t.Fatal("runtime did not receive a visualization sink")
	}
	if _, ok := owner.RuntimeSink(factoryvisualization.RuntimeSinkID(sinkID)); ok {
		t.Fatal("visualization sink remained registered after run failure")
	}
}

func TestRunHostedRuntimeKeepsRuntimeFailureAfterReplayOpens(t *testing.T) {
	want := errors.New("runtime failed")
	err := runHostedRuntime(
		t.Context(), RunConfig{ReplayPath: "recording.jsonl"}, zap.NewNop(), nil,
		resolvedRunRecordPath{}, nil, nil, nil, false, 0,
		func(context.Context, *factorysessions.SessionStartRequest, initializer.InvocationCancellation, factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
			return runFuncRunner(func(context.Context) error { return want }), nil
		},
		func(RunConfig, *workers.MockWorkersConfig) *factorysessions.SessionStartRequest {
			return &factorysessions.SessionStartRequest{}
		}, nil, nil,
	)
	if err != want {
		t.Fatalf("runtime failure = %v, want unchanged %v", err, want)
	}
}

type runSinkOwnerProbe struct {
	sink   factoryvisualization.Sink
	closed bool
}

func (owner *runSinkOwnerProbe) RegisterRuntimeSink(sink factoryvisualization.Sink) (factoryvisualization.RuntimeSinkID, error) {
	owner.sink = sink
	return "run-sink", nil
}

func (owner *runSinkOwnerProbe) RuntimeSink(id factoryvisualization.RuntimeSinkID) (factoryvisualization.Sink, bool) {
	return owner.sink, id == "run-sink" && !owner.closed
}

func (owner *runSinkOwnerProbe) CloseRuntimeSink(id factoryvisualization.RuntimeSinkID) {
	if id == "run-sink" {
		owner.closed = true
	}
}

type hostedInvocationCompletionRunner struct{}

func (hostedInvocationCompletionRunner) Run(context.Context) error { return nil }

func (hostedInvocationCompletionRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	return completion(ctx)
}

type hostedInvocationCapabilityFake struct {
	invoked          bool
	invokedSessionID string
}

func (fake *hostedInvocationCapabilityFake) InvokeFactorySession(
	_ context.Context,
	sessionID string,
	_ factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	fake.invoked = true
	fake.invokedSessionID = sessionID
	return factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "completed",
		}},
	}, nil
}

func (*hostedInvocationCapabilityFake) SubscribeFactoryEventsForSession(
	context.Context,
	string,
	*interfaces.FactoryEventReconnectCursor,
) (*interfaces.FactoryEventStream, error) {
	return nil, nil
}

func (*hostedInvocationCapabilityFake) ReadDurableFactorySessionEventStream(
	context.Context,
	string,
	factorysessions.EventReconnectRequest,
) (*interfaces.FactoryEventStream, error) {
	return nil, nil
}

var _ HostedInvocationOperation = (*hostedInvocationCapabilityFake)(nil)
