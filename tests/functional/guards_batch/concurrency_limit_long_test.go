//go:build functionallong

package guards_batch

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestConcurrencyLimit_BlocksExcessDispatches(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit blocking sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "concurrency_limit_dir"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-1"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-2"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-3"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "item 1 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "item 2 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "item 3 done. COMPLETE"},
	)
	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 3, "task:init": 0, "task:failed": 0})

	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}

	assertGuardResourceAvailability(t, session, "executor-slot", 2)
}

func TestConcurrencyLimit_ResourceTokensConsumedDuringProcessing(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit resource-consumption sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "concurrency_limit_dir"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "B"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "C"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "A done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "B done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "C done. COMPLETE"},
	)
	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 30*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 3})
	assertGuardResourceAvailability(t, session, "executor-slot", 2)
}

func TestConcurrencyLimit_ResourceReleasedOnFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit failure-release sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "X"}`))

	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{{Content: ""}},
		[]error{errors.New("processor failed")},
	)
	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0})

	if provider.CallCount() != 1 {
		t.Errorf("expected provider called 1 time, got %d", provider.CallCount())
	}

	assertGuardResourceAvailability(t, session, "slot", 1)
}

func TestConcurrencyLimit_ReducedCapacityStillCompletes(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit reduced-capacity sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "X"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Y"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Z"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "X done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Y done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Z done. COMPLETE"},
	)
	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 3, "task:init": 0})

	if provider.CallCount() != 3 {
		t.Errorf("expected provider called 3 times, got %d", provider.CallCount())
	}

	assertGuardResourceAvailability(t, session, "slot", 1)
}
