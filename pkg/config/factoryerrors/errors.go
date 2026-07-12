package factoryerrors

import "errors"

// ErrInvalidNamedFactory reports that the submitted named-factory payload could
// not be normalized into a runnable named-factory layout.
var ErrInvalidNamedFactory = errors.New("invalid named factory")

// Is reports whether err matches the named-factory validation sentinel.
func Is(err error) bool {
	return errors.Is(err, ErrInvalidNamedFactory)
}
