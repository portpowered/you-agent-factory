package runtime_api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/review"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_PackagedReviewReturnsApprovedCandidate(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("candidate work")},
		workers.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved candidate work"}`)},
	)

	host, stream := startPackagedReviewInvocationHost(t, runner)
	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "customer request", nil))
	assertPackagedReviewCompletedWithText(t, response, "approved candidate work")
	assertPackagedReviewDispatches(t, stream, []packagedReviewDispatch{
		{transitionID: review.PackagedExecuteWorkstationName, outcome: factoryapi.WorkOutcomeAccepted},
		{transitionID: review.PackagedReviewWorkstationName, outcome: factoryapi.WorkOutcomeAccepted},
	})
}

func TestSessionInvocationAPI_PackagedReviewRejectsThenApprovesRevision(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("first candidate")},
		workers.CommandResult{Stdout: []byte(`{"decision":"REJECTED","feedback":"add the missing release date"}`)},
		workers.CommandResult{Stdout: []byte("revised candidate")},
		workers.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved revised candidate"}`)},
	)

	host, stream := startPackagedReviewInvocationHost(t, runner)
	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "write release notes", nil))
	assertPackagedReviewCompletedWithText(t, response, "approved revised candidate")
	assertPackagedReviewDispatches(t, stream, []packagedReviewDispatch{
		{transitionID: review.PackagedExecuteWorkstationName, outcome: factoryapi.WorkOutcomeAccepted},
		{transitionID: review.PackagedReviewWorkstationName, outcome: factoryapi.WorkOutcomeRejected},
		{transitionID: review.PackagedExecuteWorkstationName, outcome: factoryapi.WorkOutcomeAccepted},
		{transitionID: review.PackagedReviewWorkstationName, outcome: factoryapi.WorkOutcomeAccepted},
	})
}

func TestSessionInvocationAPI_PackagedReviewWorkerFailureReturnsFailedStatus(t *testing.T) {
	host, stream := startPackagedReviewInvocationHost(t, packagedReviewFailingCommandRunner{})
	response := postInvocation(t, host.Endpoint(), textInvocationRequest(t, "customer request", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil after worker failure", response.PrimaryResult)
	}
	if response.WorkState == nil || *response.WorkState != "reviewable-work:failed" {
		t.Fatalf("workState = %#v, want reviewable-work:failed", response.WorkState)
	}
	assertPackagedReviewDispatches(t, stream, []packagedReviewDispatch{
		{transitionID: review.PackagedExecuteWorkstationName, outcome: factoryapi.WorkOutcomeFailed},
	})
	assertInvocationWorkFailedPublicly(t, host.Endpoint(), response)
}

type packagedReviewFailingCommandRunner struct{}

func (packagedReviewFailingCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, errors.New("mock provider failure")
}

func startPackagedReviewInvocationHost(t *testing.T, runner workers.CommandRunner) (*support.RootRunFunctionalHost, *factoryEventHTTPStream) {
	t.Helper()
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), review.PackagedFactoryName, review.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot:        dir,
		SystemRoot:         t.TempDir(),
		DisableMockWorkers: true,
		FunctionalEdges: wire.FunctionalEdges{
			ProviderCommandRunner: runner,
		},
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)
	return host, stream
}

type packagedReviewDispatch struct {
	transitionID string
	outcome      factoryapi.WorkOutcome
}

func assertPackagedReviewDispatches(t *testing.T, stream *factoryEventHTTPStream, want []packagedReviewDispatch) {
	t.Helper()

	for index, expected := range want {
		for {
			event := stream.next(10 * time.Second)
			if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
				continue
			}
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode packaged review DISPATCH_RESPONSE %d: %v", index, err)
			}
			if payload.TransitionId != expected.transitionID || payload.Outcome != expected.outcome {
				t.Fatalf("packaged review DISPATCH_RESPONSE %d = transition %q outcome %q, want transition %q outcome %q", index, payload.TransitionId, payload.Outcome, expected.transitionID, expected.outcome)
			}
			break
		}
	}
}

func assertInvocationWorkFailedPublicly(t *testing.T, baseURL string, response factoryapi.InvocationResponse) {
	t.Helper()
	if response.WorkId == nil || *response.WorkId == "" {
		t.Fatalf("failed invocation workId = %#v, want customer-readable work identity", response.WorkId)
	}

	works := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(baseURL, "/work"))
	for _, candidate := range works.Results {
		if support.StringPointerValue(candidate.WorkId) != *response.WorkId {
			continue
		}
		if generatedWorkStateName(candidate.State) != "failed" || generatedWorkStateType(candidate.State) != factoryapi.WorkStateTypeFAILED {
			t.Fatalf("failed invocation GET /work state = %#v, want failed/FAILED", candidate.State)
		}
		return
	}
	t.Fatalf("GET /work missing failed invocation work %q; response = %#v", *response.WorkId, works)
}

func assertPackagedReviewCompletedWithText(t *testing.T, response factoryapi.InvocationResponse, want string) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if got := primaryResultText(t, response); got != want {
		t.Fatalf("primaryResult text = %q, want %q", got, want)
	}
}
