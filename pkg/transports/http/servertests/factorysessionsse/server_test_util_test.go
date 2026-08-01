package factorysessionsse

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/work"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type factorySessionEventProgram struct {
	replay func(
		[]interfaces.FactoryEvent,
		interfaces.FactoryEventReconnectCursor,
		interfaces.FactoryEventReconnectScope,
	) ([]interfaces.FactoryEvent, error)
	stream interfaces.FactoryEventStream
}

// programmedFactorySessionEvents is a strict transport test implementation of
// the public WorkAPI role. Only Factory Session event subscription and probing
// are programmable; every unrelated Work operation fails loudly.
type programmedFactorySessionEvents struct {
	mu       sync.RWMutex
	sessions map[string]factorySessionEventProgram
}

type sseRequestPreparation struct {
	factorysessionshttp.RequestPreparation
}

func (sseRequestPreparation) PrepareEventReconnect(
	request factorysessions.EventReconnectRequest,
) (factorysessions.EventReconnectRequest, error) {
	return request, nil
}

func newProgrammedFactorySessionEvents() *programmedFactorySessionEvents {
	return &programmedFactorySessionEvents{sessions: make(map[string]factorySessionEventProgram)}
}

func (service *programmedFactorySessionEvents) SetSession(sessionID string, program factorySessionEventProgram) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.sessions[sessionID] = program
}

func (service *programmedFactorySessionEvents) SubscribeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) (*interfaces.FactoryEventStream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	service.mu.RLock()
	program, ok := service.sessions[sessionID]
	service.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", apisurface.ErrFactorySessionNotFound, sessionID)
	}

	history := append([]interfaces.FactoryEvent(nil), program.stream.History...)
	if reconnect != nil {
		if program.replay == nil {
			return nil, fmt.Errorf("factory event reconnect program is unavailable")
		}
		var err error
		history, err = program.replay(
			history,
			*reconnect,
			interfaces.FactoryEventReconnectScope{SessionID: sessionID},
		)
		if err != nil {
			return nil, err
		}
	}
	stream := program.stream
	stream.History = history
	return &stream, nil
}

func (service *programmedFactorySessionEvents) ProbeFactoryEventsForSession(
	ctx context.Context,
	sessionID string,
	reconnect *interfaces.FactoryEventReconnectCursor,
) error {
	_, err := service.SubscribeFactoryEventsForSession(ctx, sessionID, reconnect)
	return err
}

func (*programmedFactorySessionEvents) SubmitWorkRequestForSession(context.Context, string, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	panic("unexpected WorkAPI.SubmitWorkRequestForSession call")
}

func (*programmedFactorySessionEvents) MoveWorkForSession(context.Context, string, string, string, string) (work.OperatorMoveResult, error) {
	panic("unexpected WorkAPI.MoveWorkForSession call")
}

func newAPITestServer(workAPI apisurface.WorkAPI) *api.Server {
	logger, _ := zap.NewDevelopment()
	handler := factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{
		Work: workAPI, SessionRequests: sseRequestPreparation{},
	}, logger)
	return api.NewServer(handler, nil, nil, logger, nil)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}
