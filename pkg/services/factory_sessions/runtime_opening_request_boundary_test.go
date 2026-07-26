package factorysessions_test

import (
	"errors"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"reflect"
	"testing"
	"time"
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

// --- merged from opening_contract_characterization_test.go ---

// peerOpeningBindingFake exercises the published opening/binding root slice
// through singular Service.ForRuntime. It compiles against only the Sessions
// root package (plus approved peer roots already in root signatures) and never
// imports factory_sessions/internal or nested opening interfaces.
type peerOpeningBindingFake struct {
	*peerRootServiceFake
	bound        bool
	clockReads   int
	requireClock bool
	construction error
}

func newPeerOpeningBindingFake() *peerOpeningBindingFake {
	return &peerOpeningBindingFake{
		peerRootServiceFake: newPeerRootServiceFake(),
		requireClock:        true,
	}
}

var _ factorysessions.Service = (*peerOpeningBindingFake)(nil)

func (fake *peerOpeningBindingFake) ForRuntime(
	binding factorysessions.OpeningBindingRequest,
) (factorysessions.Service, error) {
	if fake.construction != nil {
		return nil, fake.construction
	}
	if fake.requireClock && binding.Clock == nil {
		return nil, &factorysessions.OpeningBindingError{
			Field:   "clock",
			Message: "clock is required",
		}
	}
	fake.bound = true
	return fake, nil
}

type openingProbeClock struct {
	reads *int
}

func (c openingProbeClock) Now() time.Time {
	if c.reads != nil {
		*c.reads++
	}
	return time.Time{}
}

func TestOpeningBindingRootContract_PeerFakeSuccessWithoutRuntimeActivity(t *testing.T) {
	t.Parallel()

	fake := newPeerOpeningBindingFake()
	var service factorysessions.Service = fake
	clock := openingProbeClock{reads: &fake.clockReads}

	bound, err := service.ForRuntime(factorysessions.OpeningBindingRequest{Clock: clock})
	if err != nil {
		t.Fatalf("ForRuntime() error = %v, want nil", err)
	}
	if bound == nil {
		t.Fatal("ForRuntime() returned nil Service view")
	}
	if !fake.bound {
		t.Fatal("ForRuntime() did not record a successful binding")
	}
	if fake.clockReads != 0 {
		t.Fatalf("binding read clock %d times, want no runtime activity during construction", fake.clockReads)
	}

	var _ factoryruntime.Clock = clock
	result := factorysessions.OpeningBindingResult{Service: bound}
	if result.Service == nil {
		t.Fatal("OpeningBindingResult must carry the usable root Service view")
	}
}

func TestOpeningBindingRootContract_PeerFakeMissingClockTypedFailure(t *testing.T) {
	t.Parallel()

	fake := newPeerOpeningBindingFake()
	var service factorysessions.Service = fake

	bound, err := service.ForRuntime(factorysessions.OpeningBindingRequest{})
	if bound != nil {
		t.Fatalf("ForRuntime() = %#v, want nil Service on missing binding input", bound)
	}
	var openingErr *factorysessions.OpeningBindingError
	if !errors.As(err, &openingErr) {
		t.Fatalf("ForRuntime() error = %v, want *OpeningBindingError", err)
	}
	if openingErr.Field != "clock" {
		t.Fatalf("OpeningBindingError.Field = %q, want clock", openingErr.Field)
	}
	if !errors.Is(err, factorysessions.ErrOpeningBindingInvalid) {
		t.Fatalf("ForRuntime() error = %v, want errors.Is ErrOpeningBindingInvalid", err)
	}
}

func TestOpeningBindingRootContract_PeerFakeMissingServiceTypedFailure(t *testing.T) {
	t.Parallel()

	fake := newPeerOpeningBindingFake()
	fake.construction = &factorysessions.OpeningBindingError{
		Field:   "service",
		Message: "service is required",
	}
	var service factorysessions.Service = fake

	bound, err := service.ForRuntime(factorysessions.OpeningBindingRequest{
		Clock: openingProbeClock{},
	})
	if bound != nil {
		t.Fatalf("ForRuntime() = %#v, want nil Service on missing service", bound)
	}
	var openingErr *factorysessions.OpeningBindingError
	if !errors.As(err, &openingErr) {
		t.Fatalf("ForRuntime() error = %v, want *OpeningBindingError", err)
	}
	if openingErr.Field != "service" {
		t.Fatalf("OpeningBindingError.Field = %q, want service", openingErr.Field)
	}
}
