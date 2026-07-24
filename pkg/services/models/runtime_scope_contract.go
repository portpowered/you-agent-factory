package models

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidRuntimeBinding classifies missing or invalid runtime-scope inputs
// supplied to ForRuntime. Peers fail closed on this typed outcome without
// importing local-runtime construction or process-launcher types.
var ErrInvalidRuntimeBinding = errors.New("models runtime binding is invalid")

// ValidateRuntimeBinding checks the plain runtime-scope inputs required to bind
// a constructed Models service to one Factory Session. It does not start host
// processes or touch local-runtime implementation packages.
func ValidateRuntimeBinding(binding RuntimeBinding) error {
	if strings.TrimSpace(binding.CacheDirectory) == "" {
		return fmt.Errorf("%w: cache directory is required", ErrInvalidRuntimeBinding)
	}
	if binding.RuntimeConfig == nil {
		return fmt.Errorf("%w: runtime configuration lookup is required", ErrInvalidRuntimeBinding)
	}
	return nil
}
