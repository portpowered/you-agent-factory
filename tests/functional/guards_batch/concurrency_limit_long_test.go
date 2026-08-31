//go:build functionallong

package guards_batch

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConcurrencyLimit_BlocksExcessDispatches(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit blocking sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "concurrency_limit_dir")

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-1"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-2"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "item-3"}`))

	session, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsProviderOutput("item 1 done. COMPLETE"),
			sharedGuardsProviderOutput("item 2 done. COMPLETE"),
			sharedGuardsProviderOutput("item 3 done. COMPLETE"),
		),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 3, "task:init": 0, "task:failed": 0})
	assertSharedGuardsProviderCalls(t, dir, 3)

	assertGuardResourceAvailability(t, session, "executor-slot", 2)
}

func TestConcurrencyLimit_ResourceTokensConsumedDuringProcessing(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit resource-consumption sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "concurrency_limit_dir")

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "B"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "C"}`))

	session, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsProviderOutput("A done. COMPLETE"),
			sharedGuardsProviderOutput("B done. COMPLETE"),
			sharedGuardsProviderOutput("C done. COMPLETE"),
		),
	}, 30*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 3})
	assertSharedGuardsProviderCalls(t, dir, 3)
	assertGuardResourceAvailability(t, session, "executor-slot", 2)
}

func TestConcurrencyLimit_ResourceReleasedOnFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit failure-release sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "resource_contention")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "X"}`))

	session, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(sharedGuardsCommandError(errors.New("processor failed"))),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0})
	assertSharedGuardsProviderCalls(t, dir, 1)

	assertGuardResourceAvailability(t, session, "slot", 1)
}

func TestConcurrencyLimit_ReducedCapacityStillCompletes(t *testing.T) {
	support.SkipLongFunctional(t, "slow concurrency-limit reduced-capacity sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "resource_contention")

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "X"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Y"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Z"}`))

	session, listed := runSharedGuardsFactoryToCompletionWithRouteAndWork(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsProviderOutput("X done. COMPLETE"),
			sharedGuardsProviderOutput("Y done. COMPLETE"),
			sharedGuardsProviderOutput("Z done. COMPLETE"),
		),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"task:complete": 3, "task:init": 0})
	assertSharedGuardsProviderCalls(t, dir, 3)

	assertGuardResourceAvailability(t, session, "slot", 1)
}
