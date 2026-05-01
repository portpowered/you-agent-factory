package guards_batch

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestFailedImmutability_CannotBeReDispatched(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "broken"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{}},
		[]error{errors.New("build error")},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:failed").
		PlaceTokenCount("code-change:failed", 1).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:complete")

	if got := len(support.ProviderCallsForWorker(provider, "swe")); got != 1 {
		t.Errorf("expected swe called once, got %d", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "reviewer")); got != 0 {
		t.Errorf("expected reviewer never called, got %d", got)
	}
}

func TestFailedImmutability_ReviewerFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "risky-change"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{
			support.AcceptedProviderResponse(),
			{},
		},
		[]error{
			nil,
			errors.New("critical security issue"),
		},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:failed").
		PlaceTokenCount("code-change:failed", 1).
		HasNoTokenInPlace("code-change:complete")

	if got := len(support.ProviderCallsForWorker(provider, "reviewer")); got != 1 {
		t.Errorf("expected reviewer called once, got %d", got)
	}
}

func TestFailedImmutability_NoDuplicateTokens(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "a"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "b"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{}, {}},
		[]error{
			errors.New("crash"),
			errors.New("crash"),
		},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("code-change:failed", 2).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:complete")
}
