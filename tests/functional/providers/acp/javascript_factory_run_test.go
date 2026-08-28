package acp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func writeACPJavaScriptFactory(t *testing.T) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))
	workflow := `return (async function () {
  const child = await agent.run({
    prompt: "complete the JavaScript ACP child",
    label: "javascript-acp",
    executorProvider: "ACP",
    modelProvider: "cursor-acp",
    model: "test-model",
  });
  return child;
})();`
	if err := os.WriteFile(filepath.Join(dir, "acp.js"), []byte(workflow), 0o600); err != nil {
		t.Fatalf("write ACP JavaScript Factory: %v", err)
	}
	return dir
}
