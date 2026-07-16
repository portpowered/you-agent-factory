package runtime_api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/review"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_PackagedReviewReturnsApprovedCandidate(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("candidate work")},
		workers.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved candidate work"}`)},
	)

	response := postInvocation(t, startPackagedReviewInvocationServer(t, runner).URL(), textInvocationRequest(t, "customer request", nil))
	assertPackagedReviewCompletedWithText(t, response, "approved candidate work")
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want work then review", got)
	}
}

func TestSessionInvocationAPI_PackagedReviewRejectsThenApprovesRevision(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("first candidate")},
		workers.CommandResult{Stdout: []byte(`{"decision":"REJECTED","feedback":"add the missing release date"}`)},
		workers.CommandResult{Stdout: []byte("revised candidate")},
		workers.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved revised candidate"}`)},
	)

	server := startPackagedReviewInvocationServer(t, runner)
	response := postInvocation(t, server.URL(), textInvocationRequest(t, "write release notes", nil))
	assertPackagedReviewCompletedWithText(t, response, "approved revised candidate")
	if got := runner.CallCount(); got != 4 {
		t.Fatalf("provider invocation count = %d, want work, review, revised work, review", got)
	}

	requests := runner.Requests()
	if len(requests) != 4 {
		t.Fatalf("recorded provider requests = %d, want 4", len(requests))
	}
	secondWorkPrompt := strings.Join(requests[2].Args, " ")
	if !strings.Contains(secondWorkPrompt, "write release notes") || !strings.Contains(secondWorkPrompt, "first candidate") || !strings.Contains(secondWorkPrompt, "add the missing release date") {
		t.Fatalf("revised work prompt = %q, want request, rejected candidate, and review feedback", secondWorkPrompt)
	}
	completed := server.GetEngineStateSnapshot(t).DispatchHistory
	if len(completed) != 4 {
		t.Fatalf("completed dispatches = %#v, want four ordered work and review outcomes", completed)
	}
	if completed[0].WorkstationName != review.PackagedExecuteWorkstationName || completed[0].Outcome != workerexecution.OutcomeAccepted ||
		completed[1].WorkstationName != review.PackagedReviewWorkstationName || completed[1].Outcome != workerexecution.OutcomeRejected ||
		completed[2].WorkstationName != review.PackagedExecuteWorkstationName || completed[2].Outcome != workerexecution.OutcomeAccepted ||
		completed[3].WorkstationName != review.PackagedReviewWorkstationName || completed[3].Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("completed dispatches = %#v, want work accepted, review rejected, work accepted, review accepted", completed)
	}
}

func TestSessionInvocationAPI_PackagedReviewWorkerFailureReturnsFailedStatus(t *testing.T) {
	response := postInvocation(t, startPackagedReviewInvocationServer(t, packagedReviewFailingCommandRunner{}).URL(), textInvocationRequest(t, "customer request", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil after worker failure", response.PrimaryResult)
	}
	if response.WorkState == nil || *response.WorkState != "reviewable-work:failed" {
		t.Fatalf("workState = %#v, want reviewable-work:failed", response.WorkState)
	}
}

type packagedReviewFailingCommandRunner struct{}

func (packagedReviewFailingCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, errors.New("mock provider failure")
}

func startPackagedReviewInvocationServer(t *testing.T, runner workers.CommandRunner) *functionalAPIServer {
	t.Helper()
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), review.PackagedFactoryName, review.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	return startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, runner, nil)
	})
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
