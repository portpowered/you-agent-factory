package wire

import (
	"context"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func TestNewServiceIsInert(t *testing.T) {
	t.Parallel()

	scopes := panicScopes{}
	readiness := func(context.Context, models.RuntimeScopeRef, models.RuntimeScopeConfig, models.Detail) (models.Runtime, error) {
		panic("readiness query called during catalog construction")
	}
	service, err := NewService(scopes, readiness)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if service == nil {
		t.Fatal("NewService returned nil service")
	}
}

type panicScopes struct{}

func (panicScopes) Open(models.RuntimeBinding) (runtimescopes.Reference, error) {
	panic("scope opened during catalog construction")
}

func (panicScopes) Resolve(runtimescopes.Reference) (models.RuntimeBinding, error) {
	panic("scope resolved during catalog construction")
}

func (panicScopes) Close(runtimescopes.Reference) error {
	panic("scope closed during catalog construction")
}
