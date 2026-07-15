package initializer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	transportmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/composition"
	"go.uber.org/zap"
)

func TestComposeSessionAPISurfaceRejectsUnavailableCollaborator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		services *Services
		host     *SessionRuntimeHost
		wantRole string
	}{
		{
			name:     "session host",
			services: &Services{},
			wantRole: "session collaborator is required",
		},
		{
			name:     "model service",
			services: &Services{},
			host:     &SessionRuntimeHost{host: &runtimehost.Host{}},
			wantRole: "model collaborator is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := composeSessionAPISurface(tt.services, tt.host)
			if err == nil || !strings.Contains(err.Error(), tt.wantRole) {
				t.Fatalf("composeSessionAPISurface() error = %v, want role %q", err, tt.wantRole)
			}
		})
	}
}

func TestComposeSessionAPISurfacePassesBoundedCollaboratorsToConstructor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	core, err := BuildCore(context.Background(), &Config{Dir: dir})
	if err != nil {
		t.Fatalf("BuildCore: %v", err)
	}
	host := NewSessionRuntimeHostFromCore(core, &Config{Dir: dir})
	services := servicesFromCoreWithModels(core, host.ModelService())

	constructorCalls := 0
	_, err = composeSessionAPISurfaceWithConstructor(services, host, func(
		session apisurface.SessionAPI,
		model apisurface.ModelAPI,
		factoryDefinition apisurface.FactorySaveAPI,
		invocation apisurface.InvocationAPI,
		durable transportmapping.DurableSessionAPI,
	) (apisurface.SessionAPISurface, error) {
		constructorCalls++
		for role, collaborator := range map[string]any{
			"session": session, "model": model, "factory-definition": factoryDefinition,
			"invocation": invocation, "durable-execution": durable,
		} {
			switch collaborator.(type) {
			case *runtimehost.Host:
				t.Fatalf("%s constructor input retained *runtimehost.Host", role)
			case *service.FactoryService:
				t.Fatalf("%s constructor input retained *service.FactoryService", role)
			}
		}
		return transportmapping.NewSessionAPISurface(session, model, factoryDefinition, invocation, durable)
	})
	if err != nil {
		t.Fatalf("composeSessionAPISurfaceWithConstructor: %v", err)
	}
	if constructorCalls != 1 {
		t.Fatalf("constructor calls = %d, want 1", constructorCalls)
	}
}

type expiredResponseEventClock struct {
	now time.Time
}

func (c *expiredResponseEventClock) Now() time.Time {
	return c.now
}

func TestComposedSessionAPISurfaceReturnsTypedExpiredResponseEventOutcome(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	cfg := &Config{Dir: dir}
	core, err := BuildCore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BuildCore: %v", err)
	}

	const sessionID = "expired-session"
	start := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	clock := &expiredResponseEventClock{now: start}
	session := factorysessions.NewLiveSession(
		sessionID,
		dir,
		dir,
		dir,
		factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: "expired"},
		nil,
		false,
		"test",
	)
	session.ResponseEvents = responseeventstore.NewSessionResponseEventStoreWithClock(sessionID, clock)
	session.ResponseEvents.Complete()
	clock.now = start.Add(responseeventstore.CompletedStreamRetentionWindow)
	core.Sessions().Upsert(session, true)

	host := NewSessionRuntimeHostFromCore(core, cfg)
	services := servicesFromCoreWithModels(core, host.ModelService())
	surface, err := composeSessionAPISurface(services, host)
	if err != nil {
		t.Fatalf("composeSessionAPISurface: %v", err)
	}
	server := api.NewServer(surface, 0, zap.NewNop())
	request := httptest.NewRequest(http.MethodGet, "/factory-sessions/"+sessionID+"/response-events", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusGone {
		t.Fatalf("expired response-event status = %d, want 410: %s", recorder.Code, recorder.Body.String())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode expired response-event error: %v", err)
	}
	if response.Code != factoryapi.ErrorResponseCodeRESPONSEEVENTSTREAMEXPIRED || response.Family != factoryapi.ErrorFamilyGone {
		t.Fatalf("expired response-event error = %#v, want RESPONSE_EVENT_STREAM_EXPIRED/GONE", response)
	}
	if contentType := recorder.Header().Get("Content-Type"); strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expired response-event Content-Type = %q, want typed JSON before SSE headers", contentType)
	}
}
