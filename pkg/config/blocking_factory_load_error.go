package config

import (
	"errors"
	"fmt"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

// BlockingFactoryLoadError reports blocking factory-load validation failures with
// structured canonical targets preserved for callers.
type BlockingFactoryLoadError struct {
	Targets []factoryvalidation.Target
}

func (e *BlockingFactoryLoadError) Error() string {
	if e == nil {
		return ErrInvalidNamedFactory.Error()
	}
	if len(e.Targets) == 0 {
		return fmt.Sprintf("%v: factory topology contains invalid graph references", ErrInvalidNamedFactory)
	}
	return fmt.Sprintf(
		"%v: factory topology contains invalid graph references (%d blocking validation targets)",
		ErrInvalidNamedFactory,
		len(e.Targets),
	)
}

func (e *BlockingFactoryLoadError) Is(target error) bool {
	return target == ErrInvalidNamedFactory
}

func newBlockingFactoryLoadError(result factoryvalidation.Result) error {
	if len(result.Targets) == 0 {
		return nil
	}
	return &BlockingFactoryLoadError{
		Targets: append([]factoryvalidation.Target(nil), result.Targets...),
	}
}

// IsInvalidNamedFactory reports whether err wraps ErrInvalidNamedFactory.
func IsInvalidNamedFactory(err error) bool {
	return errors.Is(err, ErrInvalidNamedFactory)
}

// AsBlockingFactoryLoadError returns structured blocking findings when err wraps
// a BlockingFactoryLoadError from materialization, upgrade, or factory load.
func AsBlockingFactoryLoadError(err error) (*BlockingFactoryLoadError, bool) {
	var loadErr *BlockingFactoryLoadError
	if !errors.As(err, &loadErr) {
		return nil, false
	}
	return loadErr, true
}

// BlockingFactoryLoadFindings returns config findings derived from structured
// blocking-load validation errors.
func BlockingFactoryLoadFindings(err error) []Finding {
	loadErr, ok := AsBlockingFactoryLoadError(err)
	if !ok {
		return nil
	}
	return canonicalTargetsToFindings(loadErr.Targets)
}
