package invocation

import (
	"context"
	"errors"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

type runtimeOpeningStub struct{}

func (*runtimeOpeningStub) OpenInvocationRuntime(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
) (roles.OpenedInvocationRuntime, error) {
	return roles.OpenedInvocationRuntime{}, errors.New("runtime unavailable")
}
