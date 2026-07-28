package root_composition_test

import (
	"testing"

	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

// TestMain registers Operator Settings composition hooks before functional proofs run.
func TestMain(m *testing.M) {
	settingswire.RegisterTestComposition()
	m.Run()
}
