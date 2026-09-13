package factorydefinitions

import (
	"context"
	"errors"
	"fmt"
	"os"
)

type activationReadBypassContextKey struct{}

// WithActivationReadBypass marks a Definitions read performed by an owner
// activation callback that already holds the activation lock. It prevents a
// non-reentrant lock from being acquired twice while keeping ordinary public
// reads serialized with activation.
func WithActivationReadBypass(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, activationReadBypassContextKey{}, true)
}

// ActivationReadBypassed reports whether a read is already inside the
// owner-held activation critical section.
func ActivationReadBypassed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	bypassed, _ := ctx.Value(activationReadBypassContextKey{}).(bool)
	return bypassed
}

// CurrentFactoryPointerRemover is the optional filesystem capability used to
// restore a root that did not have a current-factory pointer before a
// transition began.
type CurrentFactoryPointerRemover interface {
	RemoveCurrentPointer(rootDir string) error
}

// CurrentFactoryPointerRemoverFunc adapts an injected removal callback to the
// optional rollback capability without exposing filesystem implementations.
type CurrentFactoryPointerRemoverFunc func(string) error

func (f CurrentFactoryPointerRemoverFunc) RemoveCurrentPointer(rootDir string) error {
	if f == nil {
		return fmt.Errorf("current Factory pointer remover is required")
	}
	return f(rootDir)
}

// CurrentFactoryPointerState is the detached state captured before a durable
// current-factory pointer transition. It lets an activation rollback restore
// both an existing pointer and the absence of a pointer.
type CurrentFactoryPointerState struct {
	Name   string
	Exists bool
}

// ReadCurrentFactoryPointerState captures the current pointer without
// treating a missing pointer as an error. Other read failures remain
// actionable because they could make rollback target the wrong Factory.
func ReadCurrentFactoryPointerState(
	read CurrentFactoryPointerReader,
	rootDir string,
) (CurrentFactoryPointerState, error) {
	if read == nil {
		return CurrentFactoryPointerState{}, fmt.Errorf("current Factory pointer reader is required")
	}
	name, err := read(rootDir)
	if err == nil {
		return CurrentFactoryPointerState{Name: name, Exists: true}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return CurrentFactoryPointerState{}, nil
	}
	return CurrentFactoryPointerState{}, err
}

// WriteCurrentFactoryPointerAfterState publishes name and returns the exact
// inverse transition. The caller owns invoking the returned rollback until
// the activation transaction has completed successfully.
func WriteCurrentFactoryPointerAfterState(
	state CurrentFactoryPointerState,
	rootDir string,
	name string,
	write CurrentFactoryPointerWriter,
	remove CurrentFactoryPointerRemover,
) (func() error, error) {
	if write == nil {
		return nil, fmt.Errorf("current Factory pointer writer is required")
	}
	restore := func() error {
		if state.Exists {
			return write(rootDir, state.Name)
		}
		if remove == nil {
			return fmt.Errorf("current Factory pointer remover is required to restore an absent pointer")
		}
		return remove.RemoveCurrentPointer(rootDir)
	}
	if err := write(rootDir, name); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return nil, errors.Join(err, fmt.Errorf("restore current Factory pointer after write failure: %w", restoreErr))
		}
		return nil, err
	}
	return restore, nil
}

// WriteCurrentFactoryPointerTransaction captures the selected pointer and
// publishes name as one activation transaction. The returned callback restores
// the captured state when a later runtime swap fails.
func WriteCurrentFactoryPointerTransaction(
	rootDir string,
	name string,
	read CurrentFactoryPointerReader,
	write CurrentFactoryPointerWriter,
	remove CurrentFactoryPointerRemover,
) (func() error, error) {
	state, err := ReadCurrentFactoryPointerState(read, rootDir)
	if err != nil {
		return nil, err
	}
	return WriteCurrentFactoryPointerAfterState(state, rootDir, name, write, remove)
}
