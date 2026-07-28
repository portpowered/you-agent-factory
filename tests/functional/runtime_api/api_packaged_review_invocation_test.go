package runtime_api

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_PackagedReviewReturnsApprovedCandidate(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("candidate work")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved candidate work"}`)},
	)

	response := postInvocation(t, startPackagedReviewInvocationServer(t, runner).URL(), textInvocationRequest(t, "customer request", nil))
	assertPackagedReviewCompletedWithText(t, response, "approved candidate work")
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want work then review", got)
	}
}

func TestSessionInvocationAPI_PackagedReviewRejectsThenApprovesRevision(t *testing.T) {
	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("first candidate")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"REJECTED","feedback":"add the missing release date"}`)},
		platformprocess.CommandResult{Stdout: []byte("revised candidate")},
		platformprocess.CommandResult{Stdout: []byte(`{"decision":"ACCEPTED","output":"approved revised candidate"}`)},
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

func (packagedReviewFailingCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("mock provider failure")
}

func startPackagedReviewInvocationServer(t *testing.T, runner platformprocess.CommandRunner) *functionalAPIServer {
	t.Helper()
	dir := support.InstallPackagedFactory(t, t.TempDir(), factorydefinitions.PackagedReviewFactoryName)
	return startFunctionalServer(t, dir, false, withWorkerCommands(runner, nil))
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
