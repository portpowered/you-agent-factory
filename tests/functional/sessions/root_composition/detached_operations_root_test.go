package root_composition_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/sessionfixtures"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
)

func TestFactorySessionsRootPublishesDetachedOperations(t *testing.T) {
	t.Parallel()

	service, err := sessionfixtures.NewService()
	if err != nil {
		t.Fatalf("construct Factory Sessions root: %v", err)
	}

	operations, err := factorysessionwire.NewDetachedOperations(service)
	if err != nil {
		t.Fatalf("bind detached operations from Factory Sessions root: %v", err)
	}
	if operations == nil {
		t.Fatal("Factory Sessions root published nil detached operations")
	}
}
