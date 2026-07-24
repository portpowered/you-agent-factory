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

func TestFailedImmutability_CannotBeReDispatched(t *testing.T) {
	support.SkipLongFunctional(t, "slow failed-immutability redispatch sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "broken"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{{}},
		[]error{errors.New("build error")},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{
		"code-change:failed": 1, "code-change:init": 0, "code-change:in-review": 0, "code-change:complete": 0,
	})

	if got := len(support.ProviderCallsForWorker(provider, "swe")); got != 1 {
		t.Errorf("expected swe called once, got %d", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "reviewer")); got != 0 {
		t.Errorf("expected reviewer never called, got %d", got)
	}
}

func TestFailedImmutability_ReviewerFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow failed-immutability reviewer-failure sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "risky-change"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{
			support.AcceptedProviderResponse(),
			{},
		},
		[]error{
			nil,
			errors.New("critical security issue"),
		},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{"code-change:failed": 1, "code-change:complete": 0})

	if got := len(support.ProviderCallsForWorker(provider, "reviewer")); got != 1 {
		t.Errorf("expected reviewer called once, got %d", got)
	}
}

func TestFailedImmutability_NoDuplicateTokens(t *testing.T) {
	support.SkipLongFunctional(t, "slow failed-immutability duplicate sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "a"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "b"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{{}, {}},
		[]error{
			errors.New("crash"),
			errors.New("crash"),
		},
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertGuardSessionPlaces(t, listed, map[string]int{
		"code-change:failed": 2, "code-change:init": 0, "code-change:in-review": 0, "code-change:complete": 0,
	})
}
