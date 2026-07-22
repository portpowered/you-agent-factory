package process

import (
	"os/exec"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

// SubprocessTree supervises a command process tree for cancel and post-run cleanup.
type SubprocessTree struct {
	tree *commandProcessTree
}

// ConfigureSubprocessTree configures cmd for supervised process-tree cleanup.
func ConfigureSubprocessTree(cmd *exec.Cmd) {
	configureCommandProcessTree(cmd)
}

// AttachSubprocessTree attaches supervision after cmd.Start.
func AttachSubprocessTree(cmd *exec.Cmd) (SubprocessTree, error) {
	tree, err := attachCommandProcessTree(cmd)
	if err != nil {
		return SubprocessTree{}, err
	}
	return SubprocessTree{tree: tree}, nil
}

// TerminateSubprocessTree force-terminates the supervised tree on cancel or timeout.
func TerminateSubprocessTree(cmd *exec.Cmd, tree SubprocessTree) error {
	cancelCleanup := newCommandProcessCleanupContext(logging.EnsureLogger(nil), CommandRequest{}, commandProcessCleanupReasonCancel)
	return terminateCommandProcessTree(cmd, tree.tree, platformclock.Real{}, cancelCleanup)
}

// CloseSubprocessTree runs post-run supervised process-tree cleanup.
func CloseSubprocessTree(cmd *exec.Cmd, tree SubprocessTree) {
	postRunCleanup := newCommandProcessCleanupContext(logging.EnsureLogger(nil), CommandRequest{}, commandProcessCleanupReasonPostRun)
	closeCommandProcessTree(cmd, tree.tree, platformclock.Real{}, postRunCleanup)
}
