package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestConfigDriven_ResourceContention(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "resource_contention"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Work item A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Work item B"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().PlaceTokenCount("task:complete", 2)

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times total, got %d", provider.CallCount())
	}
}
