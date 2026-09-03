package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestOpenHostedRuntimePreparesHomeBeforeRuntimeOpening(t *testing.T) {
	var output bytes.Buffer
	var events []string
	operation, err := openHostedRuntime(
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
		nil,
		false,
		0,
		func(
			_ context.Context,
			_ *factorysessions.RuntimeOpeningRequest,
			_ initializer.InvocationCancellation,
			_ factorysessions.VisualizationSinkID,
		) (initializer.LocalRuntimeRunner, error) {
			events = append(events, "runtime log and metrics")
			return runFuncRunner(func(context.Context) error { return nil }), nil
		},
		func(RunConfig, *workers.MockWorkersConfig) *factorysessions.RuntimeOpeningRequest {
			return &factorysessions.RuntimeOpeningRequest{}
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("openHostedRuntime() error = %v", err)
	}
	if operation == nil {
		t.Fatal("openHostedRuntime() operation = nil")
	}
	if got, want := strings.Join(events, ","), "home,runtime log and metrics"; got != want {
		t.Fatalf("startup events = %q, want %q", got, want)
	}
	if got, want := output.String(), "Home directory: operator-home\n"; got != want {
		t.Fatalf("startup disclosure = %q, want %q after successful runtime opening", got, want)
	}
}

func TestOpenHostedRuntimeUsesOpenedHostedInvocationCapability(t *testing.T) {
	sessions := &hostedInvocationCapabilityFake{}
	request := invocationRequestFromText("summarize the dispatch")
	const sessionID = "session-explicit"

	operation, err := openHostedRuntime(
		t.Context(),
		RunConfig{Output: io.Discard, WithServer: true, FactorySessionID: sessionID},
		zap.NewNop(),
		request,
		resolvedRunRecordPath{},
		testInvocationOperation{},
		nil,
		nil,
		nil,
		true,
		0,
		func(
			_ context.Context,
			_ *factorysessions.RuntimeOpeningRequest,
			_ initializer.InvocationCancellation,
			_ factorysessions.VisualizationSinkID,
		) (initializer.LocalRuntimeRunner, error) {
			return WithHostedInvocation(hostedInvocationCompletionRunner{}, sessions), nil
		},
		func(RunConfig, *workers.MockWorkersConfig) *factorysessions.RuntimeOpeningRequest {
			return &factorysessions.RuntimeOpeningRequest{}
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("openHostedRuntime() error = %v", err)
	}
	if err := operation.Run(t.Context()); err != nil {
		t.Fatalf("Operation.Run() error = %v", err)
	}
	if !sessions.invoked {
		t.Fatal("hosted Factory Sessions invocation was not used")
	}
	if sessions.invokedSessionID != sessionID {
		t.Fatalf("invoked session = %q, want %q", sessions.invokedSessionID, sessionID)
	}
}

func TestPrepareHostedInvocationClearsFiniteWorkFile(t *testing.T) {
	cleanOperation, cleanConfig, err := prepareHostedInvocation(
		t.Context(),
		RunConfig{CleanInvocation: true, WorkFile: "work.json"},
		zap.NewNop(),
		invocationRequestFromText("one-shot input"),
		resolvedRunRecordPath{},
		testInvocationOperation{},
		nil,
		nil,
		true,
	)
	if err != nil {
		t.Fatalf("clean prepareHostedInvocation() error = %v", err)
	}
	if cleanOperation == nil {
		t.Fatal("clean operation = nil")
	}
	if cleanConfig.WorkFile != "" {
		t.Fatalf("clean runtime WorkFile = %q, want empty after owner projection", cleanConfig.WorkFile)
	}

	_, ordinaryConfig, err := prepareHostedInvocation(
		t.Context(),
		RunConfig{WorkFile: "work.json"},
		zap.NewNop(),
		nil,
		resolvedRunRecordPath{},
		testInvocationOperation{},
		nil,
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("ordinary prepareHostedInvocation() error = %v", err)
	}
	if ordinaryConfig.WorkFile != "work.json" {
		t.Fatalf("ordinary runtime WorkFile = %q, want original batch input", ordinaryConfig.WorkFile)
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
