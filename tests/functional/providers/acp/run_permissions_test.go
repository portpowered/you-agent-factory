package acp_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection(t *testing.T) {
	tests := []struct {
		name            string
		skipPermissions bool
		mode            string
	}{
		{name: "default rejects", mode: "permission-reject"},
		{name: "skipPermissions allows", skipPermissions: true, mode: "permission-allow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"golden ACP permission"}`))
			writeACPWorkerPolicy(t, dir, test.skipPermissions)
			t.Setenv(goldenACPModeEnvironment, test.mode)
			t.Setenv("YOU_ACP_GOLDEN_SENTINEL", "preserved")

			var starts atomic.Int32
			_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: goldenACPCommandFactory(&starts),
				ProvidersExecutableLocator:    availableExecutableLocator{},
			}, 20*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
				t.Fatalf("completed work = %d, want 1", got)
			}
			if starts.Load() != 1 {
				t.Fatalf("ACP process starts = %d, want 1", starts.Load())
			}
		})
	}
}

func writeACPWorkerPolicy(t *testing.T, factoryDir string, skipPermissions bool) {
	t.Helper()
	value := "false"
	if skipPermissions {
		value = "true"
	}
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	content := "---\n" +
		"executorProvider: ACP\n" +
		"modelProvider: cursor-acp\n" +
		"model: test-model\n" +
		"skipPermissions: " + value + "\n" +
		"stopToken: COMPLETE\n" +
		"type: MODEL_WORKER\n" +
		"---\n\nTest golden ACP worker.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write ACP worker policy: %v", err)
	}
}
