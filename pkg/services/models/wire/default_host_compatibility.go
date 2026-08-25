package wire

import (
	"context"
	"fmt"

	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// NewDefaultHostCompatibilityChecker constructs the production host policy
// from the same pinned artifact matrix used to select managed backend assets.
// Keeping the check here makes normal composition fail before process start
// when the current platform cannot run the selected backend.
func NewDefaultHostCompatibilityChecker() (HostCompatibilityChecker, error) {
	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		return nil, fmt.Errorf("construct default host compatibility selector: %w", err)
	}
	return defaultHostCompatibilityChecker{resolve: resolver}, nil
}

type defaultHostCompatibilityChecker struct {
	resolve BackendArtifactResolver
}

func (checker defaultHostCompatibilityChecker) Check(
	ctx context.Context,
	request HostCompatibilityRequest,
) error {
	_, err := checker.resolve(ctx, BackendArtifactSelectionRequest{
		Backend:         request.Backend,
		Platform:        request.Platform,
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	})
	if err != nil {
		return fmt.Errorf(
			"select pinned backend %q for model %q: %w",
			request.Backend, request.ModelName, err,
		)
	}
	return nil
}

var _ HostCompatibilityChecker = defaultHostCompatibilityChecker{}
