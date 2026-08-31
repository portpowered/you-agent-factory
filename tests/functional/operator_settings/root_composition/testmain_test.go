package root_composition_test

import (
	"fmt"
	"os"
	"testing"

	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

// TestMain registers Operator Settings composition hooks before functional proofs run.
func TestMain(m *testing.M) {
	settingswire.RegisterTestComposition()
	exitCode := m.Run()
	if err := closeSharedOperatorSettingsFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "shared Operator Settings fixture cleanup: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}
