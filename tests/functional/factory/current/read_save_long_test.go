//go:build functionallong

package current

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCurrentFactoryActivationSwitchesPersistedFactories proves that activating
// a second persisted Factory through the public session API becomes the Current
// Factory and resolves the correct customer-visible name and directory. The
// server fixture composes the runtime through root.BuildProcess and public CLI
// argv/stdio exactly as a customer invocation would.
func TestCurrentFactoryActivationSwitchesPersistedFactories(t *testing.T) {
	support.SkipLongFunctional(t, "slow current-factory activation persistence smoke")

	rootDir := t.TempDir()
	alphaDir := seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	betaDir := createNamedFactoryFixture(
		t,
		rootDir,
		"beta",
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	support.WaitForRuntimeIdle(t, server.URL(), 5*time.Second)

	assertCurrentFactoryNameAndDirectory(t, server.URL(), "alpha", alphaDir)

	activateNamedPersistedFactoryOverHTTP(
		t,
		server.URL(),
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	assertCurrentFactoryNameAndDirectory(t, server.URL(), "beta", betaDir)
}
