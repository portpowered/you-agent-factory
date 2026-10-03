package factorydefinitions

import (
	"errors"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
)

func TestErrInvalidNamedFactory_IsStableSentinel(t *testing.T) {
	if ErrInvalidNamedFactory.Error() != "invalid named factory" {
		t.Fatalf("error = %q, want invalid named factory", ErrInvalidNamedFactory.Error())
	}
	if !errors.Is(ErrInvalidNamedFactory, ErrInvalidNamedFactory) {
		t.Fatal("expected stable ErrInvalidNamedFactory sentinel")
	}
	if errors.Is(errors.New("other"), ErrInvalidNamedFactory) {
		t.Fatal("unrelated error should not match ErrInvalidNamedFactory")
	}
}

func TestMissingFactoryLayoutMatchesCanonicalSentinel(t *testing.T) {
	t.Parallel()

	resolver, err := catalognamedpaths.New(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("New named paths: %v", err)
	}
	_, err = resolver.ResolveCurrentDir(t.TempDir())
	if !errors.Is(err, ErrLayoutNotFound) || !errors.Is(err, ErrFactoryLayoutNotFound) {
		t.Fatalf("ResolveCurrentDir error = %v, want both public layout sentinels", err)
	}
	if errors.Is(errors.New("factory layout not found"), ErrFactoryLayoutNotFound) {
		t.Fatal("same-text error must not match the canonical layout sentinel")
	}
}
