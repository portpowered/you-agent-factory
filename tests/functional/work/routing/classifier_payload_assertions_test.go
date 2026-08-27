package routing

import (
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func assertClassifierRoutingBranchProviderPayloads(
	t *testing.T,
	runner *workRoutingScenarioCommandRunner,
	want string,
	wantBranchCalls int,
) {
	t.Helper()

	requests := runner.requestsSnapshot()
	if len(requests) != 1+wantBranchCalls {
		t.Fatalf("provider command count = %d, want %d", len(requests), 1+wantBranchCalls)
	}
	branchRequests := requests[len(requests)-wantBranchCalls:]
	for index, request := range branchRequests {
		if !classifierRoutingProviderRequestIncludesPayload(request, want) {
			t.Fatalf(
				"branch provider request %d missing payload %q; args=%#v stdin=%q workDir=%q",
				index,
				want,
				request.Args,
				string(request.Stdin),
				request.WorkDir,
			)
		}
	}
}

func classifierRoutingProviderRequestIncludesPayload(
	request platformprocess.CommandRequest,
	want string,
) bool {
	if strings.Contains(string(request.Stdin), want) {
		return true
	}
	for _, arg := range request.Args {
		if strings.Contains(arg, want) {
			return true
		}
	}
	return false
}

func assertClassifierRoutingOutputWorkBranches(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	wantBranches []string,
	wantPayload string,
) {
	t.Helper()

	classifierDispatches := filterWorkstationDispatches(dispatches, classifierRoutingWorkstation)
	if len(classifierDispatches) != 1 {
		t.Fatalf("classifier dispatch count = %d, want 1", len(classifierDispatches))
	}
	response := classifierDispatches[0].Response
	if response == nil || response.OutputWork == nil {
		t.Fatalf("classifier dispatch missing outputWork branches: response=%#v", response)
	}
	seen := make(map[string]bool, len(wantBranches))
	for _, item := range *response.OutputWork {
		location := support.WorkItemCustomerLocation(item)
		if location == "" {
			continue
		}
		seen[location] = true
		if got := workRoutingPublicWorkText(item); got != wantPayload {
			t.Fatalf(
				"classifier outputWork branch %s payload = %q, want %q preserved across fan-out",
				location,
				got,
				wantPayload,
			)
		}
	}
	for _, location := range wantBranches {
		if !seen[location] {
			t.Fatalf("classifier outputWork missing branch %s; seen=%#v", location, seen)
		}
	}
}

func classifierRoutingFailureSignature(dispatch support.DispatchEventObservation) string {
	if dispatch.Response == nil {
		return ""
	}
	parts := []string{string(dispatch.Response.Outcome)}
	if dispatch.Response.Error != nil {
		parts = append(parts, *dispatch.Response.Error)
	}
	if dispatch.Response.FailureDetail != nil {
		parts = append(parts, string(dispatch.Response.FailureDetail.Reason))
		parts = append(parts, dispatch.Response.FailureDetail.Message)
	}
	return strings.Join(parts, "|")
}
