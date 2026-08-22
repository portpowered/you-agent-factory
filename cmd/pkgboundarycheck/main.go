package main

import (
	"fmt"
	"io"
	"os"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

func main() {
	cfg := parseConfig()
	if cfg.writeTestServiceImportBaseline {
		if err := createTestServiceImportBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writeSupportServiceImportBaseline {
		if err := createSupportServiceImportBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writeTransportBehaviorBaseline {
		if err := createTransportBehaviorBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writeProductionDefaultBaseline {
		if err := createProductionDefaultBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writeTestBehaviorBaseline {
		if err := createTestBehaviorBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if cfg.writePetriPublicSurfaceBaseline {
		if err := createPetriPublicSurfaceBaseline(cfg); err != nil {
			fmt.Fprintln(stderrWriter, err)
			exitFunc(1)
		}
		return
	}
	if err := run(cfg, stdoutWriter, stderrWriter); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitFunc(1)
	}
}
