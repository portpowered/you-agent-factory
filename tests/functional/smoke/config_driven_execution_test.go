package smoke

import (
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestConfigDrivenExecution_HappyPath(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Config-driven happy path"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
	)

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want one terminal work item", status.Categories)
	}

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}

func TestConfigDrivenExecution_HappyPathFailureRouting(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will fail"}`))

	provider := testutil.NewMockProviderWithErrors(
		[]workerexecution.InferenceResponse{{Content: ""}},
		[]error{fmt.Errorf("something went wrong")},
	)

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Failed != 1 || status.Categories.Terminal != 0 {
		t.Fatalf("status categories = %+v, want one failed work item", status.Categories)
	}
	if provider.CallCount() != 1 {
		t.Errorf("expected failed provider called once, got %d", provider.CallCount())
	}
}

func TestConfigDrivenExecution_AddWorkType(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_work_type"))

	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "New request"}`))
	testutil.WriteSeedFile(t, dir, "review", []byte(`{"title": "New review"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Request handled. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Review handled. COMPLETE"},
	)

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 2 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want two terminal work items", status.Categories)
	}

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}
