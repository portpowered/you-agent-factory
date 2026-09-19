// Package managedchild provides a policy-free, supervised long-running child
// process with bounded terminal output evidence.
package managedchild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"os/exec"
	"sync"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

const (
	// DefaultOutputTailLimit bounds retained output for each stream while the
	// stream digest and byte count continue to cover every observed byte.
	DefaultOutputTailLimit = 64 << 10
	managedChildWaitDelay  = 5 * time.Second
)

// Spec describes one policy-free managed child launch.
type Spec struct {
	Command         string
	Args            []string
	Env             []string
	WorkDir         string
	OutputTailLimit int
}

// ExitClass classifies the terminal result of a managed child.
type ExitClass string

const (
	ExitClassExited     ExitClass = "EXITED"
	ExitClassNonzero    ExitClass = "NONZERO_EXIT"
	ExitClassWaitFailed ExitClass = "WAIT_FAILED"
)

// StreamSnapshot contains bounded retained output and complete stream facts.
type StreamSnapshot struct {
	Bytes     uint64
	SHA256    string
	Truncated bool
	Tail      []byte
}

// Snapshot is the immutable terminal state of a managed child.
type Snapshot struct {
	ExitClass     ExitClass
	ExitCode      int
	ExitCodeKnown bool
	Stdout        StreamSnapshot
	Stderr        StreamSnapshot
}

// Process owns one started child and its supervised process tree.
type Process struct {
	cmd  *exec.Cmd
	tree platformprocess.SubprocessTree

	stdout *streamCapture
	stderr *streamCapture

	waitFn      func() error
	terminateFn func() error
	closeFn     func()
	pid         int

	done chan struct{}

	mu       sync.Mutex
	waitErr  error
	snapshot Snapshot
	terminal bool
	stopErr  error
	stopOnce sync.Once
}

type startDependencies struct {
	newCommand   func(string, ...string) *exec.Cmd
	configure    func(*exec.Cmd)
	startCommand func(*exec.Cmd) error
	attach       func(*exec.Cmd) (platformprocess.SubprocessTree, error)
	waitCommand  func(*exec.Cmd) error
	terminate    func(*exec.Cmd, platformprocess.SubprocessTree) error
	close        func(*exec.Cmd, platformprocess.SubprocessTree)
}

// Start starts and supervises one long-running child. A nil context is
// treated as context.Background; cancellation after a successful start stops
// the supervised process tree.
func Start(ctx context.Context, spec Spec) (*Process, error) {
	return startWithDependencies(ctx, spec, startDependencies{
		newCommand:   exec.Command,
		configure:    platformprocess.ConfigureSubprocessTree,
		startCommand: func(cmd *exec.Cmd) error { return cmd.Start() },
		attach:       platformprocess.AttachSubprocessTree,
		waitCommand:  func(cmd *exec.Cmd) error { return cmd.Wait() },
		terminate:    platformprocess.TerminateSubprocessTree,
		close:        platformprocess.CloseSubprocessTree,
	})
}

func startWithDependencies(ctx context.Context, spec Spec, dependencies startDependencies) (*Process, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cmd := dependencies.newCommand(spec.Command, spec.Args...)
	if spec.Env != nil {
		cmd.Env = append([]string(nil), spec.Env...)
	}
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}

	dependencies.configure(cmd)
	stdout := newStreamCapture(outputTailLimit(spec.OutputTailLimit))
	stderr := newStreamCapture(outputTailLimit(spec.OutputTailLimit))
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// A descendant that inherits a pipe must not make terminalization
	// unbounded after the started child has exited.
	cmd.WaitDelay = managedChildWaitDelay

	if err := dependencies.startCommand(cmd); err != nil {
		return nil, err
	}
	tree, err := dependencies.attach(cmd)
	if err != nil {
		terminateErr := dependencies.terminate(cmd, platformprocess.SubprocessTree{})
		waitErr := dependencies.waitCommand(cmd)
		return nil, errors.Join(err, terminateErr, waitErr)
	}

	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	managed := &Process{
		cmd:         cmd,
		tree:        tree,
		stdout:      stdout,
		stderr:      stderr,
		waitFn:      func() error { return dependencies.waitCommand(cmd) },
		terminateFn: func() error { return dependencies.terminate(cmd, tree) },
		closeFn:     func() { dependencies.close(cmd, tree) },
		pid:         pid,
		done:        make(chan struct{}),
	}
	go managed.supervise()
	go managed.stopWhenContextDone(ctx)
	return managed, nil
}

// Wait waits for terminalization and returns the exact wait error produced by
// os/exec to every observer. The returned error is stable for the lifetime of
// the Process.
func (p *Process) Wait() error {
	if p == nil || p.done == nil {
		return nil
	}
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

// Stop force-terminates the supervised process tree once, then waits for the
// terminal snapshot. Repeated and concurrent calls share the same cleanup.
func (p *Process) Stop(ctx context.Context) error {
	if p == nil || p.done == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.stopOnce.Do(func() {
		var err error
		if p.terminateFn != nil {
			err = p.terminateFn()
		}
		p.mu.Lock()
		p.stopErr = err
		p.mu.Unlock()
	})

	p.mu.Lock()
	stopErr := p.stopErr
	p.mu.Unlock()
	if stopErr != nil {
		return stopErr
	}
	select {
	case <-p.done:
		return nil
	default:
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Snapshot returns a deep copy of the one terminal snapshot, if terminalized.
func (p *Process) Snapshot() (Snapshot, bool) {
	if p == nil || p.done == nil {
		return Snapshot{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.terminal {
		return Snapshot{}, false
	}
	return cloneSnapshot(p.snapshot), true
}

// PID returns the host process identifier assigned at start, or zero when the
// host did not expose one. It is a policy-free fact used by platform evidence.
func (p *Process) PID() int {
	if p == nil {
		return 0
	}
	return p.pid
}

func (p *Process) supervise() {
	waitErr := p.waitFn()
	if p.closeFn != nil {
		p.closeFn()
	}
	snapshot := terminalSnapshot(waitErr, p.stdout, p.stderr)
	p.mu.Lock()
	p.waitErr = waitErr
	p.snapshot = snapshot
	p.terminal = true
	close(p.done)
	p.mu.Unlock()
}

func (p *Process) stopWhenContextDone(ctx context.Context) {
	if ctx == nil || ctx.Done() == nil {
		return
	}
	select {
	case <-ctx.Done():
		_ = p.Stop(context.Background())
	case <-p.done:
	}
}

type streamCapture struct {
	mu        sync.Mutex
	digest    hash.Hash
	bytes     uint64
	tail      []byte
	limit     int
	truncated bool
}

func newStreamCapture(limit int) *streamCapture {
	return &streamCapture{digest: sha256.New(), limit: limit}
}

func (capture *streamCapture) Write(chunk []byte) (int, error) {
	if len(chunk) == 0 {
		return 0, nil
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	_, _ = capture.digest.Write(chunk)
	capture.bytes += uint64(len(chunk))
	if len(capture.tail)+len(chunk) <= capture.limit {
		capture.tail = append(capture.tail, chunk...)
		return len(chunk), nil
	}
	capture.truncated = true
	combined := make([]byte, 0, capture.limit)
	combined = append(combined, capture.tail...)
	combined = append(combined, chunk...)
	if len(combined) > capture.limit {
		combined = combined[len(combined)-capture.limit:]
	}
	capture.tail = append(capture.tail[:0], combined...)
	return len(chunk), nil
}

func (capture *streamCapture) snapshot() StreamSnapshot {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	digest := capture.digest.Sum(nil)
	return StreamSnapshot{
		Bytes:     capture.bytes,
		SHA256:    hex.EncodeToString(digest),
		Truncated: capture.truncated,
		Tail:      append([]byte(nil), capture.tail...),
	}
}

func outputTailLimit(requested int) int {
	if requested <= 0 || requested > DefaultOutputTailLimit {
		return DefaultOutputTailLimit
	}
	return requested
}

func terminalSnapshot(waitErr error, stdout, stderr *streamCapture) Snapshot {
	result := Snapshot{
		ExitCode: -1,
		Stdout:   stdout.snapshot(),
		Stderr:   stderr.snapshot(),
	}
	if waitErr == nil {
		result.ExitClass = ExitClassExited
		result.ExitCode = 0
		result.ExitCodeKnown = true
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		result.ExitClass = ExitClassNonzero
		if exitCode := exitErr.ExitCode(); exitCode >= 0 {
			result.ExitCode = exitCode
			result.ExitCodeKnown = true
		}
		return result
	}
	result.ExitClass = ExitClassWaitFailed
	return result
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Stdout.Tail = append([]byte(nil), snapshot.Stdout.Tail...)
	snapshot.Stderr.Tail = append([]byte(nil), snapshot.Stderr.Tail...)
	return snapshot
}
