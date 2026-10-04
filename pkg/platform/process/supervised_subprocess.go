package process

import (
	"io"
	"os"
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

// TerminateSubprocessTreeWithEffects force-terminates the supervised tree using
// the caller's selected clock and logger. Both effects must be supplied.
func TerminateSubprocessTreeWithEffects(cmd *exec.Cmd, tree SubprocessTree, clock platformclock.Source, logger logging.Logger) error {
	cleanup := commandProcessCleanupContext{logger: logger, reason: commandProcessCleanupReasonCancel}
	return terminateCommandProcessTree(cmd, tree.tree, clock, cleanup)
}

// CloseSubprocessTreeWithEffects performs post-run cleanup using the caller's
// selected clock and logger. It retains the existing process-tree cleanup policy.
func CloseSubprocessTreeWithEffects(cmd *exec.Cmd, tree SubprocessTree, clock platformclock.Source, logger logging.Logger) {
	cleanup := commandProcessCleanupContext{logger: logger, reason: commandProcessCleanupReasonPostRun}
	closeCommandProcessTree(cmd, tree.tree, clock, cleanup)
}

// StdioChannel is one parent-owned duplex standard-stream channel for a child
// process.
//
// The parent keeps its own request and response handles for as long as it wants
// to read from the child, independently of any wait on the process itself, and
// it holds copies of the child's two pipe ends only until the child is started.
// Detach releases those copies: keeping them would leave the parent holding the
// response end's own writer, so a response reader could never observe a clean
// end of stream. Close releases every end the parent still owns, including the
// child's ends when the child never started.
//
// The channel is policy-free. It decides nothing about what travels over the
// stream, how long a reader may block, or how the bytes are interpreted.
type StdioChannel interface {
	// Attach installs the child's ends as cmd's stdin and stdout. It must run
	// before cmd.Start.
	Attach(cmd *exec.Cmd)
	// Detach releases this process's copies of the child's ends. It must run
	// once the start succeeded, because the child owns its own handles from
	// that moment on.
	Detach()
	// Close releases every end this process still owns. It is safe after
	// Detach and safe to call more than once.
	Close()
	// Requests is the parent-owned end the child reads as standard input.
	Requests() io.WriteCloser
	// Responses is the parent-owned end fed by the child's standard output.
	Responses() io.ReadCloser
}

// StdioPipeFactory creates one fresh parent-owned standard-stream channel.
// Production pipe creation is selected at the composition boundary so no service
// or protocol package opens a host pipe for itself.
type StdioPipeFactory func() (StdioChannel, error)

// NewParentOwnedStdio opens parent-owned handles independent of command Wait.
func NewParentOwnedStdio() (StdioChannel, error) {
	return openParentOwnedStdio(os.Pipe, func(file *os.File) error { return file.Close() })
}

type hostParentOwnedStdio struct {
	requests  *os.File
	responses *os.File
	childEnds []*os.File
	closeFile func(*os.File) error
}

func openParentOwnedStdio(open func() (*os.File, *os.File, error), closeFile func(*os.File) error) (StdioChannel, error) {
	childStdin, requests, err := open()
	if err != nil {
		return nil, err
	}
	responses, childStdout, err := open()
	if err != nil {
		_ = closeFile(childStdin)
		_ = closeFile(requests)
		return nil, err
	}
	return &hostParentOwnedStdio{requests: requests, responses: responses, childEnds: []*os.File{childStdin, childStdout}, closeFile: closeFile}, nil
}

func (channel *hostParentOwnedStdio) Attach(cmd *exec.Cmd) {
	// Attaching after Detach or Close would hand the child a handle this
	// process has already released, so a released channel attaches nothing and
	// the caller observes an unset command instead of a silently reopened pipe.
	if len(channel.childEnds) != 2 {
		return
	}
	cmd.Stdin, cmd.Stdout = channel.childEnds[0], channel.childEnds[1]
}

func (channel *hostParentOwnedStdio) Detach() {
	for _, end := range channel.childEnds {
		_ = channel.closeFile(end)
	}
	channel.childEnds = nil
}

func (channel *hostParentOwnedStdio) Close() {
	channel.Detach()
	if channel.requests != nil {
		_ = channel.closeFile(channel.requests)
		channel.requests = nil
	}
	if channel.responses != nil {
		_ = channel.closeFile(channel.responses)
		channel.responses = nil
	}
}

func (channel *hostParentOwnedStdio) Requests() io.WriteCloser { return channel.requests }

func (channel *hostParentOwnedStdio) Responses() io.ReadCloser { return channel.responses }

var _ StdioChannel = (*hostParentOwnedStdio)(nil)
