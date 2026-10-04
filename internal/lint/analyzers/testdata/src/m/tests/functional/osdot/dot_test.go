package osdot

import (
	"context"
	. "os/exec" // want "import 'os/exec' is not allowed"
)

// Examples cannot provide an escape from the functional OS boundary.
func ExampleCommand() {
	_ = Command("unused")                              // want "use of `Command` forbidden.*functional tests must replace process effects"
	_ = CommandContext(context.Background(), "unused") // want "use of `CommandContext` forbidden.*functional tests must replace process effects"
}
