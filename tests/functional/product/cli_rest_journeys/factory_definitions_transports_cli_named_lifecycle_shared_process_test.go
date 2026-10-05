package cli_rest_journeys_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var namedLifecycleProcess support.ApplicationProcess

func initializeFactorydefinitionstransportsclinamedlifecycleFixture(t *testing.T) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build named Factory lifecycle process: %v\n", err)
		t.Fatal("customer fixture setup failed; see preceding diagnostic")
	}
	namedLifecycleProcess = process
	t.Cleanup(func() {
		exitCode := 0
		//nolint:testsleep // This deadline bounds process teardown after all scenario-owned commands have joined.
		closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := process.Close(closeContext); err != nil {
			fmt.Fprintf(os.Stderr, "close named Factory lifecycle process: %v\n", err)
			exitCode = 1
		}
		if exitCode != 0 {
			t.Error("customer fixture cleanup failed; see preceding diagnostic")
		}
	})
}
func resetfactorydefinitionstransportsclinamedlifecycle2State() {
	var freshNamedLifecycleProcess support.ApplicationProcess
	namedLifecycleProcess = freshNamedLifecycleProcess
}
