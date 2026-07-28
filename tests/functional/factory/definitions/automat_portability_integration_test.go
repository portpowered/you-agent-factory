package definitions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestAutomatPortabilityFixture_IntegrationSmoke_CoversFlattenExpandAndBoundedReadiness(t *testing.T) {
	authoredFactoryDir, flattenedCfg, expandedDir := flattenAndExpandAutomatFixture(t)

	if flattenedCfg.ResourceManifest == nil {
		t.Fatal("expected flattened automat fixture to include resourceManifest")
	}
	assertAutomatRequiredToolsManifest(t, flattenedCfg.ResourceManifest.RequiredTools)

	bundledFiles := bundledFilesByTarget(flattenedCfg.ResourceManifest.BundledFiles)
	for targetLocation, sourcePath := range map[string]string{
		"factory/" + automatDependencyContract:      filepath.Join(authoredFactoryDir, automatDependencyContract),
		"factory/docs/portable-workflow.md":         filepath.Join(authoredFactoryDir, automatWorkflowGuide),
		"factory/scripts/prepare-automat-slice.ps1": filepath.Join(authoredFactoryDir, automatPrepareScript),
		"factory/scripts/verify-external-tools.ps1": filepath.Join(authoredFactoryDir, automatVerifyToolsScript),
	} {
		assertAutomatBundledFileContent(t, bundledFiles, targetLocation, sourcePath)
	}

	assertAutomatPersistedFactoryJSONUsesThinBundledFileContract(t, expandedDir, authoredFactoryDir)
	assertAutomatDependencyContract(t, expandedDir)
	assertAutomatExpandedBundledFile(t, expandedDir, automatWorkflowGuide, filepath.Join(authoredFactoryDir, automatWorkflowGuide))
	assertAutomatExpandedBundledFile(t, expandedDir, automatPrepareScript, filepath.Join(authoredFactoryDir, automatPrepareScript))
	assertAutomatExpandedBundledFile(t, expandedDir, automatVerifyToolsScript, filepath.Join(authoredFactoryDir, automatVerifyToolsScript))
	assertAutomatExpandedBundledFile(t, expandedDir, automatDependencyContract, filepath.Join(authoredFactoryDir, automatDependencyContract))

	if err := os.RemoveAll(authoredFactoryDir); err != nil {
		t.Fatalf("remove authored fixture after expand: %v", err)
	}

	testutil.WriteSeedRequest(t, expandedDir, work.SubmitRequest{
		WorkID:     automatDispatchReadyWorkID,
		WorkTypeID: "chapter",
		TraceID:    "trace-automat-ready",
		Payload:    []byte("portable automat readiness"),
	})

	scriptRunner := &automatDispatchReadyRunner{
		expandedDir: expandedDir,
		authoredDir: authoredFactoryDir,
	}
	providerRunner := support.NewRecordingCommandRunner("automat integration smoke must not execute providers")
	activateAutomatRequiredToolsOnPath(t)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, expandedDir, serviceedges.Edges{
		ScriptCommandRunner:   scriptRunner,
		ProviderCommandRunner: providerRunner,
	}, 10*time.Second)
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for script-only automat integration smoke", providerRunner.CallCount())
	}
	for placeID, want := range map[string]int{
		"chapter:ready":  1,
		"chapter:init":   0,
		"chapter:staged": 0,
		"chapter:failed": 0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}

	if issues := scriptRunner.Issues(); len(issues) > 0 {
		t.Fatalf("automat integration smoke issues:\n%s", strings.Join(issues, "\n"))
	}

	assertListedWorkPayload(t, listed, "chapter", "ready", "required-tools:"+automatExternalMangaka+","+automatExternalMagick)
}
