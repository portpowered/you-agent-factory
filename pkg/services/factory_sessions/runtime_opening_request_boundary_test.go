package factorysessions_test

import (
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func TestRuntimeOpeningRequestContainsOnlyImmutableValueSelections(t *testing.T) {
	t.Parallel()
	assertValueOnlyRuntimeRequest(t, reflect.TypeOf(factorysessions.RuntimeOpeningRequest{}), map[reflect.Type]bool{})
}

func assertValueOnlyRuntimeRequest(t *testing.T, typ reflect.Type, visiting map[reflect.Type]bool) {
	t.Helper()
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if visiting[typ] {
		return
	}
	visiting[typ] = true
	defer delete(visiting, typ)

	switch typ.Kind() {
	case reflect.Func, reflect.Interface, reflect.Chan:
		t.Fatalf("RuntimeOpeningRequest contains non-value dependency %s", typ)
	case reflect.Struct:
		for index := 0; index < typ.NumField(); index++ {
			assertValueOnlyRuntimeRequest(t, typ.Field(index).Type, visiting)
		}
	}
}
