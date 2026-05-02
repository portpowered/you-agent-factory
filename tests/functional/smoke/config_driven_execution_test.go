package smoke

import (
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDrivenExecution_HappyPath(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Config-driven happy path"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}

func TestConfigDrivenExecution_HappyPathFailureRouting(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will fail"}`))

	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{Content: ""}},
		[]error{fmt.Errorf("something went wrong")},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete").
		TokenCount(1)
}

func TestConfigDrivenExecution_AddWorkType(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_work_type"))

	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "New request"}`))
	testutil.WriteSeedFile(t, dir, "review", []byte(`{"title": "New review"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Request handled. COMPLETE"},
		interfaces.InferenceResponse{Content: "Review handled. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("request:complete").
		HasTokenInPlace("review:complete").
		HasNoTokenInPlace("request:init").
		HasNoTokenInPlace("review:init").
		PlaceTokenCount("request:complete", 1).
		PlaceTokenCount("review:complete", 1)

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}
