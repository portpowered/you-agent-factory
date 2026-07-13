// Package main is the entry point for the agent-factory CLI.
package main

import (
	"os"

	"github.com/portpowered/infinite-you/pkg/root"
)

var runProcess = root.Main
var exitProcess = os.Exit

func main() {
	exitProcess(runProcess())
}
