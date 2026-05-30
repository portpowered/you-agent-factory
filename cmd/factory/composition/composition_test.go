package composition

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
)

func TestBuildFactoryService_MatchesServiceBuilder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, err := BuildFactoryService(ctx, nil)
	_, serviceErr := service.BuildFactoryService(ctx, nil)
	if (err == nil) != (serviceErr == nil) {
		t.Fatalf(
			"composition error presence = %v, service.BuildFactoryService = %v",
			err,
			serviceErr,
		)
	}
	if err != nil && serviceErr != nil && err.Error() != serviceErr.Error() {
		t.Fatalf(
			"composition err = %q, service.BuildFactoryService err = %q",
			err,
			serviceErr,
		)
	}
}
