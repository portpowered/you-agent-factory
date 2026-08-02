package http

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type defaultWorkTypeRole struct {
	id  string
	err error
}

func (role defaultWorkTypeRole) DefaultWorkType(*factorydefinitions.FactoryConfig) (string, error) {
	return role.id, role.err
}

type factoryDefinitionRole struct {
	apisurface.FactorySaveAPI
	factoryapi.Factory
	err error
}

func (role factoryDefinitionRole) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return role.Factory, role.err
}

func TestDefaultWorkTypeResolverPreservesSessionAdmissionPolicy(t *testing.T) {
	t.Parallel()

	checks := []struct {
		name        string
		definitions apisurface.FactorySaveAPI
		invocation  factorydefinitions.InvocationWorkTypeService
		want        string
		wantErr     string
	}{
		{name: "missing dependencies"},
		{name: "session not found", definitions: factoryDefinitionRole{err: apisurface.ErrFactorySessionNotFound}},
		{name: "current factory not found", definitions: factoryDefinitionRole{err: apisurface.ErrCurrentFactoryNotFound}},
		{name: "opaque definition error", definitions: factoryDefinitionRole{err: errors.New("definition failed")}, invocation: defaultWorkTypeRole{}, wantErr: "definition failed"},
		{name: "invocation policy error", definitions: factoryDefinitionRole{}, invocation: defaultWorkTypeRole{err: errors.New("policy failed")}},
		{name: "default type", definitions: factoryDefinitionRole{}, invocation: defaultWorkTypeRole{id: "default-task"}, want: "default-task"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			resolver := NewDefaultWorkTypeResolver(check.definitions, check.invocation)
			got, err := resolver(context.Background(), "session-alpha")
			if check.wantErr != "" {
				if err == nil || err.Error() != check.wantErr {
					t.Fatalf("error = %v, want %q", err, check.wantErr)
				}
				return
			}
			if err != nil || got != check.want {
				t.Fatalf("resolver = (%q, %v), want (%q, nil)", got, err, check.want)
			}
		})
	}
}
