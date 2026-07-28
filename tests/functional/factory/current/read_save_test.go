package current

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestAPIGetAndSaveCurrentFactoryWithinOneSession proves that a Factory Session
// can read its Current Factory, save a valid updated definition through the
// public session API, and read back the saved customer-visible topology within
// the same session.
func TestAPIGetAndSaveCurrentFactoryWithinOneSession(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)

	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	assertFactoryWorkType(t, current, "alpha-task", "initial current factory")

	saved := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		functionalNamedFactoryBody("alpha", "story", advancedFactoryVersion(t, current.Version)),
	)
	assertFactoryWorkType(t, saved, "story", "save response")

	reloaded := getCurrentFactory(t, server.URL())
	if reloaded.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("reloaded current factory name = %q, want alpha", reloaded.Name)
	}
	assertFactoryWorkType(t, reloaded, "story", "subsequent get within session")
}
