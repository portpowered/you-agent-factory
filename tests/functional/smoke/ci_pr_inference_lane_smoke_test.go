package smoke_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestCIPRInferenceLaneSmoke_WiresRequiredApprovalJob(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	ciWorkflow := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	longWorkflow := filepath.Join(repoRoot, ".github", "workflows", "long-local-inference.yml")

	ciContents, err := os.ReadFile(ciWorkflow)
	if err != nil {
		t.Fatalf("read ci workflow: %v", err)
	}
	longContents, err := os.ReadFile(longWorkflow)
	if err != nil {
		t.Fatalf("read long local inference workflow: %v", err)
	}

	ciText := string(ciContents)
	longText := string(longContents)

	for _, want := range []string{
		"pr-inference-approval:",
		"name: PR Inference Approval",
		"make verify-pr-inference",
		"./scripts/ci/install-omnivoice-command.sh",
		"INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS",
	} {
		if !strings.Contains(ciText, want) {
			t.Fatalf("ci workflow missing %q", want)
		}
	}

	if strings.Contains(ciText, "make long-tests-functional-runtime") {
		t.Fatal("ci workflow should run the narrow PR lane, not the broader specialty aggregate")
	}

	for _, want := range []string{
		"name: Long Local Inference",
		"make long-tests-managed-runtime",
		"make long-tests-functional-runtime",
	} {
		if !strings.Contains(longText, want) {
			t.Fatalf("long local inference workflow missing %q", want)
		}
	}
}
