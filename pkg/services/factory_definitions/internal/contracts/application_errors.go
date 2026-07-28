package factorycontracts

import (
	"errors"
	"fmt"

	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

var ErrInvalidNamedFactory = errors.New("invalid named factory")
var ErrNamedFactoryAlreadyExists = errors.New("named factory already exists")
var ErrInvalidNamedFactoryName = namedfactorypath.ErrInvalidName
var ErrFactoryLayoutNotFound = errors.New("factory layout not found")
var ErrNamedFactoryNotFound = namedfactorypath.ErrNotFound
var ErrNamedFactoryIsCurrent = errors.New("cannot delete current factory")

type BlockingFactoryLoadError struct {
	Targets []ValidationTarget
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

func NewBlockingFactoryLoadError(result ValidationResult) error {
	if len(result.Targets) == 0 {
		return nil
	}
	return &BlockingFactoryLoadError{
		Targets: append([]ValidationTarget(nil), result.Targets...),
	}
}

func AsBlockingFactoryLoadError(err error) (*BlockingFactoryLoadError, bool) {
	var loadErr *BlockingFactoryLoadError
	if !errors.As(err, &loadErr) {
		return nil, false
	}
	return loadErr, true
}

func IsInvalidNamedFactoryName(err error) bool {
	return errors.Is(err, ErrInvalidNamedFactoryName)
}

func IsNamedFactoryNotFound(err error) bool {
	return errors.Is(err, ErrNamedFactoryNotFound)
}
