package initializer

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
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
		durable apisurface.DurableSessionAPI,
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
		return apisurface.NewSessionAPISurface(session, model, factoryDefinition, invocation, durable)
	})
	if err != nil {
		t.Fatalf("composeSessionAPISurfaceWithConstructor: %v", err)
	}
	if constructorCalls != 1 {
		t.Fatalf("constructor calls = %d, want 1", constructorCalls)
	}
}
