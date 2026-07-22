package factorydefinition_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestValidateEditableFactoryTopology_RoutelessCronAndLogicalMove_InvalidFactoryTargets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		decode      func() (factoryapi.Factory, error)
		workstation string
	}{
		{
			name:        "routeless_cron",
			decode:      factoryfixtures.DecodeRoutelessCronFactory,
			workstation: "cron",
		},
		{
			name:        "routeless_logical_move",
			decode:      factoryfixtures.DecodeRoutelessLogicalMoveFactory,
			workstation: "router",
		},
		{
			name:        "routeless_logical_move_cron",
			decode:      factoryfixtures.DecodeRoutelessLogicalMoveCronFactory,
			workstation: "trigger-monkey",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			factory, err := tc.decode()
			if err != nil {
				t.Fatalf("decode factory: %v", err)
			}

			saveErr := validateEditableFactoryTopology(factory, nil)
			var topologyErr *interfaces.ValidationTopologyError
			if !errors.As(saveErr, &topologyErr) {
				t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
			}

			validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeWorkstationMissingOutputRoutes)
			validationassert.HasDomainTargetSubject(t, topologyErr.Targets, interfaces.ValidationSubject{
				Type: interfaces.ValidationSubjectTypeWorkstation, ID: tc.workstation,
				Location: interfaces.ValidationSubjectLocationOutputs,
			})
			for _, target := range topologyErr.Targets {
				if target.Code == factoryvalidation.CodeWorkstationMissingFailureRoute &&
					target.Subject.Type == interfaces.ValidationSubjectTypeWorkstation &&
					target.Subject.ID == tc.workstation &&
					target.Subject.Location == interfaces.ValidationSubjectLocationOnFailure {
					t.Fatalf("targets = %#v, want no missingFailureRoute at ON_FAILURE for routeless workstation", topologyErr.Targets)
				}
			}
		})
	}
}

func TestValidateUpsertNamedFactoryRequest_RoutelessLogicalMove_InvalidFactoryTargets(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeRoutelessLogicalMoveFactory()
	if err != nil {
		t.Fatalf("DecodeRoutelessLogicalMoveFactory: %v", err)
	}

	saveErr := validateUpsertNamedFactoryRequest(factory, nil)
	var topologyErr *interfaces.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateUpsertNamedFactoryRequest error = %v, want topology validation error", saveErr)
	}

	validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeWorkstationMissingOutputRoutes)
	validationassert.HasDomainTargetSubject(t, topologyErr.Targets, interfaces.ValidationSubject{
		Type: interfaces.ValidationSubjectTypeWorkstation, ID: "router",
		Location: interfaces.ValidationSubjectLocationOutputs,
	})
}
