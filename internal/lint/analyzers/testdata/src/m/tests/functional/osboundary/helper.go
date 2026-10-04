package osboundary

import (
	"context"
	"log"
	local "m/pkg/osboundary/localexec"
	"os/exec" // want "import 'os/exec' is not allowed.*real OS proof belongs in integration"
)

func Launch() {
	_ = exec.Command("unused")                              // want "use of `exec.Command` forbidden.*functional tests must replace process effects"
	_ = exec.CommandContext(context.Background(), "unused") // want "use of `exec.CommandContext` forbidden.*functional tests must replace process effects"
	local.Command()
	Command()
	log.Print("logging exclusions must not hide OS diagnostics")
}

func Command() {}
