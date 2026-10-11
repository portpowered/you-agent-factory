package workersessions

import (
	"errors"
	"fmt"
	"net/url"
)

// ErrDirectCapacityExceeded refuses a new direct attempt before identity or effects.
var ErrDirectCapacityExceeded = errors.New("direct Worker Session capacity is full")

// DirectCapacityFailure contains only public host identity and its finite bound.
type DirectCapacityFailure struct {
	Host     string
	Capacity int64
}

func (failure DirectCapacityFailure) Error() string {
	return fmt.Sprintf("Selected host %s has direct capacity %d. Retry after an attempt ends.", failure.PublicHost(), failure.Capacity)
}
func (failure DirectCapacityFailure) Unwrap() error { return ErrDirectCapacityExceeded }

// PublicHost returns the public HTTP origin, stripping credentials and resource paths.
func (failure DirectCapacityFailure) PublicHost() string {
	parsed, err := url.Parse(failure.Host)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Opaque != "" {
		return "local"
	}
	return parsed.Scheme + "://" + parsed.Host
}
