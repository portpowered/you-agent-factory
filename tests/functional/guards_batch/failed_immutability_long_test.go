//go:build functionallong

package guards_batch

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFailedImmutability_CannotBeReDispatched(t *testing.T) {
	support.SkipLongFunctional(t, "slow failed-immutability redispatch sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "code_review")
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "broken"}`))
	_, listed, events := runSharedGuardsFactoryToCompletionWithRouteAndObservations(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(sharedGuardsCommandError(errors.New("build error"))),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{
		"code-change:failed": 1, "code-change:init": 0, "code-change:in-review": 0, "code-change:complete": 0,
	})

	if got := len(sharedGuardsProviderRequests(t, dir)); got != 1 {
		t.Errorf("expected one provider dispatch, got %d", got)
	}
	assertSharedGuardsDispatchTransitions(t, events, "coding")
}

func TestFailedImmutability_ReviewerFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow failed-immutability reviewer-failure sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "code_review")
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "risky-change"}`))
	_, listed, events := runSharedGuardsFactoryToCompletionWithRouteAndObservations(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsProviderOutput(support.AcceptedProviderResponse().Content),
			sharedGuardsCommandError(errors.New("critical security issue")),
		),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"code-change:failed": 1, "code-change:complete": 0})

	if got := len(sharedGuardsProviderRequests(t, dir)); got != 2 {
		t.Errorf("expected coding and reviewer dispatches, got %d", got)
	}
	assertSharedGuardsDispatchTransitions(t, events, "coding", "review")
}

func TestFailedImmutability_NoDuplicateTokens(t *testing.T) {
	support.SkipLongFunctional(t, "slow failed-immutability duplicate sweep")
	enterSharedGuardsScenario(t)
	dir := sharedGuardsScenario(t, "code_review")
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "a"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "b"}`))
	_, listed, events := runSharedGuardsFactoryToCompletionWithRouteAndObservations(t, dir, sharedGuardsRouteConfig{
		provider: sharedGuardsProviderSequence(
			sharedGuardsCommandError(errors.New("crash")),
			sharedGuardsCommandError(errors.New("crash")),
		),
	}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{
		"code-change:failed": 2, "code-change:init": 0, "code-change:in-review": 0, "code-change:complete": 0,
	})
	if got := len(sharedGuardsProviderRequests(t, dir)); got != 2 {
		t.Errorf("expected one provider dispatch per token, got %d", got)
	}
	assertSharedGuardsDispatchTransitions(t, events, "coding", "coding")
}
