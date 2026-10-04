package osalias

import (
	"context"
	process "os/exec" // want "import 'os/exec' is not allowed"
	"testing"
)

func TestAlias(t *testing.T) {
	_ = process.Command("unused")                              // want "use of `process.Command` forbidden.*functional tests must replace process effects"
	_ = process.CommandContext(context.Background(), "unused") // want "use of `process.CommandContext` forbidden.*functional tests must replace process effects"
}
