package agypty

import (
	"os/exec"

	"github.com/portpowered/infinite-you/pkg/workers/process"
)

func processAttachForTest(cmd *exec.Cmd) (process.SubprocessTree, error) {
	return process.AttachSubprocessTree(cmd)
}

func processConfigureForTest(cmd *exec.Cmd) {
	process.ConfigureSubprocessTree(cmd)
}
