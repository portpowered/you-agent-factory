package replay_contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
)

func TestJavaScriptDocumentedFailureContracts(t *testing.T) {
	t.Run("authoring rejects unsupported host access with a stable code", func(t *testing.T) {
		result := workflowvalidation.Validate(workflowvalidation.Request{
			Source:    `const filesystem = require("fs");`,
			SourceRef: "unsupported-host.workflow.js",
		})
		if !hasValidationCode(result.Issues, workflowvalidation.CodeForbiddenHostAccess) {
			t.Fatalf("issues = %#v, want a %q diagnostic", result.Issues, workflowvalidation.CodeForbiddenHostAccess)
		}
	})

	t.Run("child policy denial is bounded before execution", func(t *testing.T) {
		err := workflowpolicy.ValidateChildRequest(
			workflowpolicy.EffectivePolicy{AllowedModels: []string{"allowed-model"}},
			workflowpolicy.ChildRequest{Label: "review", Model: "denied-model"},
		)
		if err == nil || !strings.Contains(err.Error(), `policy denied: model "denied-model"`) {
			t.Fatalf("error = %v, want bounded model policy denial", err)
		}
	})

	t.Run("replay rejects an incompatible schema before reconstruction", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "incompatible.replay.json")
		if err := os.WriteFile(path, []byte(`{"schemaVersion":"agent-factory.replay.v99","events":[]}`), 0o600); err != nil {
			t.Fatalf("write incompatible replay: %v", err)
		}
		_, err := replay.Load(path)
		if err == nil || !strings.Contains(err.Error(), "unsupported replay artifact schemaVersion") {
			t.Fatalf("Load() error = %v, want stable incompatible-schema diagnostic", err)
		}
	})
}

func hasValidationCode(issues []workflowvalidation.Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
