package admission_test

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const submissionDefaultSessionID = factorysessions.DefaultSessionID

func openSubmissionSession(t *testing.T, server *support.FunctionalAPIServer) (string, string) {
	t.Helper()
	dir := support.ScaffoldFactory(t, submissionInputPreservingFactoryConfig())
	configureSubmissionCodexWorkers(t, dir, "worker-a")
	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	id := opened.Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, server.URL(), id) })
	return dir, id
}
