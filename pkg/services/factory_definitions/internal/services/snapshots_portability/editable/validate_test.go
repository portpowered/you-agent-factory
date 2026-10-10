package editable_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	fd "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/editable"
)

type validationOperation func(context.Context, fd.DefinitionValidationRequest) (fd.ValidationResult, error)

func (operation validationOperation) ValidateDefinition(ctx context.Context, request fd.DefinitionValidationRequest) (fd.ValidationResult, error) {
	return operation(ctx, request)
}

type workstationLoader struct{}

func (*workstationLoader) Load(string) (*fd.FactoryWorkstationConfig, error) {
	panic("editable validation must forward, not invoke, the workstation loader")
}

func TestValidateSnapshotRequiresFactory(t *testing.T) {
	t.Parallel()
	err := editable.ValidateSnapshot(t.Context(), nil, nil, nil, nil)
	if !errors.Is(err, fd.ErrInvalidNamedFactory) {
		t.Fatalf("ValidateSnapshot = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestValidateSnapshotForcesPrePersistOwnerProfile(t *testing.T) {
	t.Parallel()
	snapshot := &fd.FactorySnapshot{}
	loader := &workstationLoader{}
	config := &fd.FactoryConfig{Name: "selected"}
	payload := []byte(`{"name":"selected"}`)
	request := fd.DefinitionValidationRequest{Profile: fd.ValidationProfileTopology, Config: config, CanonicalPayload: payload, WorkstationLoader: loader}
	mapped, validated := 0, 0
	ctx := t.Context()
	err := editable.ValidateSnapshot(ctx, snapshot, loader,
		func(got *fd.FactorySnapshot, gotLoader fd.WorkstationLoader) (fd.DefinitionValidationRequest, error) {
			mapped++
			if got != snapshot || gotLoader != loader {
				t.Fatal("mapper lost selected snapshot/loader identity")
			}
			return request, nil
		}, validationOperation(func(gotCtx context.Context, got fd.DefinitionValidationRequest) (fd.ValidationResult, error) {
			validated++
			want := request
			want.Profile = fd.ValidationProfilePrePersist
			if gotCtx != ctx || !reflect.DeepEqual(got, want) || got.Config != config || &got.CanonicalPayload[0] != &payload[0] {
				t.Fatalf("validation changed more than profile: %#v", got)
			}
			return fd.ValidationResult{}, nil
		}))
	if err != nil || mapped != 1 || validated != 1 || request.Profile != fd.ValidationProfileTopology {
		t.Fatalf("ValidateSnapshot = %v, calls = %d/%d, input profile = %s", err, mapped, validated, request.Profile)
	}
}

func TestValidateSnapshotReturnsDomainTopologyError(t *testing.T) {
	t.Parallel()
	blocking := fd.ValidationTarget{Code: "workstation.worker.missing", Severity: fd.ValidationSeverityError, Message: "selected worker is absent"}
	warning := fd.ValidationTarget{Code: "workstation.warning", Severity: fd.ValidationSeverityWarning}
	for _, scenario := range []struct {
		name         string
		targets      []fd.ValidationTarget
		wantBlocking bool
	}{
		{"blocking", []fd.ValidationTarget{warning, blocking}, true},
		{"warning", []fd.ValidationTarget{warning}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			err := editable.ValidateSnapshot(t.Context(), &fd.FactorySnapshot{}, nil,
				func(*fd.FactorySnapshot, fd.WorkstationLoader) (fd.DefinitionValidationRequest, error) {
					return fd.DefinitionValidationRequest{}, nil
				},
				validationOperation(func(context.Context, fd.DefinitionValidationRequest) (fd.ValidationResult, error) {
					return fd.ValidationResult{Targets: scenario.targets}, nil
				}))
			if !scenario.wantBlocking {
				if err != nil {
					t.Fatalf("nonblocking result rejected: %v", err)
				}
				return
			}
			var topologyErr *fd.ValidationTopologyError
			if !errors.As(err, &topologyErr) || !reflect.DeepEqual(topologyErr.Targets, []fd.ValidationTarget{blocking}) {
				t.Fatalf("ValidateSnapshot = %v, want selected blocking target in TopologyError", err)
			}
		})
	}
}

func TestValidateSnapshotRejectsMissingAdaptersAndMappingFailure(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"mapper absent", "validator absent", "mapping failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			mapped := 0
			mapper := fd.EditableFactoryValidationRequestMapper(func(*fd.FactorySnapshot, fd.WorkstationLoader) (fd.DefinitionValidationRequest, error) {
				mapped++
				return fd.DefinitionValidationRequest{}, errors.New("selected mapping failure")
			})
			var validator fd.DefinitionValidationOperation = validationOperation(func(context.Context, fd.DefinitionValidationRequest) (fd.ValidationResult, error) {
				t.Fatal("validator called after absent adapter or mapping failure")
				return fd.ValidationResult{}, nil
			})
			if scenario == "mapper absent" {
				mapper = nil
			}
			if scenario == "validator absent" {
				validator = nil
			}
			err := editable.ValidateSnapshot(t.Context(), &fd.FactorySnapshot{}, nil, mapper, validator)
			wantCalls := 0
			if scenario == "mapping failure" {
				wantCalls = 1
			}
			if !errors.Is(err, fd.ErrInvalidNamedFactory) || mapped != wantCalls {
				t.Fatalf("ValidateSnapshot = %v, mapper calls = %d, want invalid factory and %d", err, mapped, wantCalls)
			}
			if scenario == "mapping failure" && !strings.Contains(err.Error(), "selected mapping failure") {
				t.Fatalf("mapping diagnostic lost: %v", err)
			}
		})
	}
}

func TestValidateSnapshotPreservesDomainErrorsAndWrapsOtherFailures(t *testing.T) {
	t.Parallel()
	domainErr := errors.Join(fd.ErrInvalidNamedFactory, errors.New("selected domain failure"))
	for _, failure := range []error{domainErr, context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			t.Parallel()
			err := editable.ValidateSnapshot(t.Context(), &fd.FactorySnapshot{}, nil,
				func(*fd.FactorySnapshot, fd.WorkstationLoader) (fd.DefinitionValidationRequest, error) {
					return fd.DefinitionValidationRequest{}, nil
				},
				validationOperation(func(context.Context, fd.DefinitionValidationRequest) (fd.ValidationResult, error) {
					return fd.ValidationResult{}, failure
				}))
			if !errors.Is(err, fd.ErrInvalidNamedFactory) || !strings.Contains(err.Error(), failure.Error()) {
				t.Fatalf("ValidateSnapshot = %v, want domain classification and %v diagnostic", err, failure)
			}
			if errors.Is(failure, domainErr) && err != domainErr { //nolint:errorlint // Existing domain errors must be returned unchanged, including their identity.
				t.Fatal("domain error identity lost")
			}
		})
	}
}
