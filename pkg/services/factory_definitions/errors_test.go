package factorydefinitions

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
	if errors.Is(errors.New("other"), ErrInvalidNamedFactory) {
		t.Fatal("unrelated error should not match ErrInvalidNamedFactory")
	}
}
