// replayboundarycheck enforces the closed replay-contract functional boundary.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/functionalscenarios"
)

const successOutput = "[agent-factory:replay-boundary] 0 replay-contract files require reviewed exceptions; removed paths: replay_event_stream_artifact_smoke_long_test.go, replay_live_helpers_test.go, replay_record_end_to_end_long_test.go, replay_regression_harness_long_test.go\n"

func main() {
	if err := run(".", os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string, stdout io.Writer) error {
	if err := functionalscenarios.CheckReplayContractBoundaries(root); err != nil {
		return err
	}
	_, err := io.WriteString(stdout, successOutput)
	return err
}
