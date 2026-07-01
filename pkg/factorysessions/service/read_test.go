package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
)

func TestService_ListFactorySessions_DelegatesToControlPlane(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		sessionIDs: []string{"sess-1"},
		sessions: map[string]*factorysessions.LiveSession{
			"sess-1": {
				ID: "sess-1",
				SessionState: factorysessions.SessionState{
					FactoryDir: "/tmp/factory",
				},
			},
		},
	}
	gateway := factorysessionservice.New(host)

	response, err := gateway.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].Id != "sess-1" {
		t.Fatalf("sessions = %#v, want sess-1", response.Sessions)
	}
}

func TestService_GetFactorySession_ReturnsNotFoundForMissingSession(t *testing.T) {
	t.Parallel()

	host := &openTestHost{
		requireSessionE: fmt.Errorf("%w: missing", apisurface.ErrFactorySessionNotFound),
	}
	gateway := factorysessionservice.New(host)

	_, err := gateway.GetFactorySession(context.Background(), "missing")
	if err == nil || !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("GetFactorySession error = %v, want not found", err)
	}
}

func TestService_GetFactorySession_RejectsDurableSessionID(t *testing.T) {
	t.Parallel()

	gateway := factorysessionservice.New(&openTestHost{})

	_, err := gateway.GetFactorySession(context.Background(), "dur-sess-js-run-n-001")
	if err == nil || !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("GetFactorySession error = %v, want not found", err)
	}
}
