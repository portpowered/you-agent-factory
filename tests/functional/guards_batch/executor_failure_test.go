package guards_batch

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestExecutorFailure_NoFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work payload"))

	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{{}},
		[]error{errors.New("executor crashed")},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 5*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertGuardSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:processing": 0})
	for _, event := range server.GetFactoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Error == nil || *payload.Error == "" {
			t.Error("public dispatch response error is empty")
		}
		server.Stop(t)
		return
	}
	t.Fatal("Factory Event history has no dispatch response")
}

func TestExecutorFailure_OutcomeFailed_NoFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_no_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work"))
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte("provider unavailable"),
		ExitCode: 1,
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 5*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:processing": 0})
}

func TestExecutorFailure_WithFailureArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_failure_with_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work"))
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte("intentional failure"),
		ExitCode: 1,
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 5*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
}

func TestExecutorSuccess_TokenAtOutputPlace(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 5*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0})
}
