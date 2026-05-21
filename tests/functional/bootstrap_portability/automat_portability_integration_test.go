package bootstrap_portability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
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

	testutil.WriteSeedRequest(t, expandedDir, interfaces.SubmitRequest{
		WorkID:     automatDispatchReadyWorkID,
		WorkTypeID: "chapter",
		TraceID:    "trace-automat-ready",
		Payload:    []byte("portable automat readiness"),
	})

	runner := &automatDispatchReadyRunner{
		expandedDir: expandedDir,
		authoredDir: authoredFactoryDir,
	}
	activateAutomatRequiredToolsOnPath(t)
	harness := testutil.NewServiceTestHarness(t, expandedDir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		PlaceTokenCount("chapter:ready", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:staged").
		HasNoTokenInPlace("chapter:failed")

	if issues := runner.Issues(); len(issues) > 0 {
		t.Fatalf("automat integration smoke issues:\n%s", strings.Join(issues, "\n"))
	}

	assertTokenPayload(t, harness.Marking(), "chapter:ready", "required-tools:"+automatExternalMangaka+","+automatExternalMagick)
}
