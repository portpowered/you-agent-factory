package factorydefinition_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
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
			var topologyErr *factorydefinitions.ValidationTopologyError
			if !errors.As(saveErr, &topologyErr) {
				t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
			}

			validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeWorkstationMissingOutputRoutes)
			validationassert.HasDomainTargetSubject(t, topologyErr.Targets, factorydefinitions.ValidationSubject{
				Type: factorydefinitions.ValidationSubjectTypeWorkstation, ID: tc.workstation,
				Location: factorydefinitions.ValidationSubjectLocationOutputs,
			})
			for _, target := range topologyErr.Targets {
				if target.Code == factoryvalidation.CodeWorkstationMissingFailureRoute &&
					target.Subject.Type == factorydefinitions.ValidationSubjectTypeWorkstation &&
					target.Subject.ID == tc.workstation &&
					target.Subject.Location == factorydefinitions.ValidationSubjectLocationOnFailure {
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
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateUpsertNamedFactoryRequest error = %v, want topology validation error", saveErr)
	}

	validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeWorkstationMissingOutputRoutes)
	validationassert.HasDomainTargetSubject(t, topologyErr.Targets, factorydefinitions.ValidationSubject{
		Type: factorydefinitions.ValidationSubjectTypeWorkstation, ID: "router",
		Location: factorydefinitions.ValidationSubjectLocationOutputs,
	})
}
