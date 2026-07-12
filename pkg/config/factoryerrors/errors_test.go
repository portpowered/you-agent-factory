package factoryerrors

import (
	"errors"
	"testing"
)

func TestErrInvalidNamedFactory_IsStableSentinel(t *testing.T) {
	if ErrInvalidNamedFactory.Error() != "invalid named factory" {
		t.Fatalf("error = %q, want invalid named factory", ErrInvalidNamedFactory.Error())
	}
	if !errors.Is(ErrInvalidNamedFactory, ErrInvalidNamedFactory) {
		t.Fatal("expected stable ErrInvalidNamedFactory sentinel")
	}
	if !Is(ErrInvalidNamedFactory) {
		t.Fatal("Is should match ErrInvalidNamedFactory")
	}
	if Is(errors.New("other")) {
		t.Fatal("Is should not match unrelated errors")
	}
}
