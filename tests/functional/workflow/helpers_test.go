package workflow

import (
	"context"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

func newAdhocProcessReviewHarness(
	t *testing.T,
	responses []interfaces.InferenceResponse,
) (string, *testutil.MockWorkerMapProvider, *testutil.ServiceTestHarness) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, testutil.MustRepoPath(t, "tests/adhoc/factory"))
	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": responses,
	})
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithExecutionBaseDir(dir),
	)

	harness.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		WorkID:     "task-process-review-contract",
		TraceID:    "trace-process-review-contract",
		Name:       "align-process-review-loop-contract",
		Payload:    []byte("process review contract coverage"),
	}})

	return dir, provider, harness
}

func dispatchesForWorkstation(
	history []interfaces.CompletedDispatch,
	workstationName string,
) []interfaces.CompletedDispatch {
	dispatches := make([]interfaces.CompletedDispatch, 0, len(history))
	for _, dispatch := range history {
		if dispatch.WorkstationName == workstationName {
			dispatches = append(dispatches, dispatch)
		}
	}
	return dispatches
}

func assertProviderCallWorkstations(
	t *testing.T,
	calls []interfaces.ProviderInferenceRequest,
	want []string,
) {
	t.Helper()

	if len(calls) != len(want) {
		t.Fatalf("provider call count = %d, want %d", len(calls), len(want))
	}
	for i, workstationName := range want {
		if calls[i].Dispatch.WorkstationName != workstationName {
			t.Fatalf("provider call %d workstation = %q, want %q", i, calls[i].Dispatch.WorkstationName, workstationName)
		}
	}
}

func assertDispatchHasOutputToPlace(
	t *testing.T,
	dispatch interfaces.CompletedDispatch,
	placeID string,
) {
	t.Helper()

	for _, mutation := range dispatch.OutputMutations {
		if mutation.ToPlace == placeID {
			return
		}
	}

	t.Fatalf("dispatch %#v missing output mutation to %q", dispatch, placeID)
}

func assertDispatchOutputTagAbsent(
	t *testing.T,
	dispatch interfaces.CompletedDispatch,
	key string,
) {
	t.Helper()

	for _, mutation := range dispatch.OutputMutations {
		if mutation.Token == nil || mutation.Token.Color.Tags == nil {
			continue
		}
		if _, ok := mutation.Token.Color.Tags[key]; ok {
			t.Fatalf("dispatch %#v unexpectedly set tag %q", dispatch, key)
		}
	}
}

func assertDispatchHistoryContainsWorkstationRoute(
	t *testing.T,
	history []interfaces.CompletedDispatch,
	workstationName string,
	terminalPlace string,
) {
	t.Helper()

	for _, dispatch := range history {
		if dispatch.WorkstationName != workstationName {
			continue
		}
		for _, mutation := range dispatch.OutputMutations {
			if mutation.ToPlace == terminalPlace {
				return
			}
		}
	}

	t.Fatalf(
		"dispatch history missing %q route to %q: %#v",
		workstationName,
		terminalPlace,
		history,
	)
}

func firstInputToken(rawTokens any) interfaces.Token {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return interfaces.Token{}
		}
		tok, ok := tokens[0].(interfaces.Token)
		if !ok {
			return interfaces.Token{}
		}
		return tok
	case []interfaces.Token:
		if len(tokens) == 0 {
			return interfaces.Token{}
		}
		return tokens[0]
	default:
		return interfaces.Token{}
	}
}
