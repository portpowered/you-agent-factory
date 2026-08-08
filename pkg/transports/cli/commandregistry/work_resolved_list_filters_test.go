package commandregistry_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
)

func TestResolvedWorkListRejectsMutuallyExclusiveTerminalityBeforeHandler(t *testing.T) {
	called := false
	list := commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{
		ListWork: func(workcli.ListConfig) error {
			called = true
			return nil
		},
	})
	err := executeResolvedListError(t, list, []string{
		"work", "list", "--terminal", "--non-terminal",
	}, io.Discard, io.Discard, context.Background())
	if err == nil || !strings.Contains(err.Error(), "terminal") || !strings.Contains(err.Error(), "non-terminal") {
		t.Fatalf("mutually-exclusive flags error = %v, want relationship validation error", err)
	}
	if called {
		t.Fatal("list handler was called after mutually-exclusive flag validation")
	}
}
