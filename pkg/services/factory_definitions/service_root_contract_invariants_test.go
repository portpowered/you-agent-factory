package factorydefinitions_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// peerOwnedDefinitionsConsumer depends only on the Factory Definitions service
// root. It must not import factory_definitions/contracts or owner-local
// implementation packages to exercise catalog and validate slices.
type peerOwnedDefinitionsConsumer struct {
	service factorydefinitions.Service
}

func (c peerOwnedDefinitionsConsumer) exerciseCatalogSuccess(
	ctx context.Context,
	rootDir string,
) (factorydefinitions.ListNamedFactoriesResult, factorydefinitions.GetNamedFactoryResult, error) {
	listed, err := c.service.ListNamedFactories(
		ctx,
		factorydefinitions.ListNamedFactoriesRequest{RootDir: rootDir},
	)
	if err != nil {
		return factorydefinitions.ListNamedFactoriesResult{}, factorydefinitions.GetNamedFactoryResult{}, err
	}
	if len(listed.Entries) != 1 {
		return listed, factorydefinitions.GetNamedFactoryResult{}, errors.New("expected one catalog entry")
	}
	got, err := c.service.GetNamedFactory(
		ctx,
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: listed.Entries[0].Name},
	)
	return listed, got, err
}

func (c peerOwnedDefinitionsConsumer) exerciseCatalogTypedFailures(
	ctx context.Context,
	rootDir string,
) error {
	_, invalidErr := c.service.GetNamedFactory(
		ctx,
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "../evil"},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		return invalidErr
	}
	_, missingErr := c.service.GetNamedFactory(
		ctx,
		factorydefinitions.GetNamedFactoryRequest{RootDir: rootDir, Name: "missing"},
	)
	if !errors.Is(missingErr, factorydefinitions.ErrNamedFactoryNotFound) {
		return missingErr
	}
	if errors.Is(missingErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		return errors.New("missing named factory must not match invalid-name sentinel")
	}
	return nil
}

func (c peerOwnedDefinitionsConsumer) exerciseValidateSuccess(
	ctx context.Context,
	payload []byte,
) error {
	structural, err := c.service.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: payload,
			Profile:   factorydefinitions.ValidationProfilePrePersist,
		},
	)
	if err != nil {
		return err
	}
	if structural.Validation.HasBlockingTargets() {
		return errors.New("structural validation reported blocking findings")
	}
	effective, err := c.service.ValidateEffectiveFactoryDefinition(
		ctx,
		factorydefinitions.ValidateEffectiveFactoryDefinitionRequest{
			Canonical: payload,
			Effective: factorydefinitions.EffectiveFactorySource{
				FactoryDir:      "/factories/alpha",
				RuntimeBaseDir:  "/factories/alpha",
				ContentIdentity: string(payload),
			},
		},
	)
	if err != nil {
		return err
	}
	if effective.Validation.HasBlockingTargets() {
		return errors.New("effective validation reported blocking findings")
	}
	return nil
}

func (c peerOwnedDefinitionsConsumer) exerciseValidateTypedFailures(
	ctx context.Context,
) error {
	_, invalidErr := c.service.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		return invalidErr
	}
	_, findingsErr := c.service.ValidateStructuralFactoryDefinition(
		ctx,
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{
			Canonical: []byte(`{"invalidLayout":true}`),
			Profile:   factorydefinitions.ValidationProfileTopology,
		},
	)
	var validationFailure *factorydefinitions.FactoryDefinitionValidationFailure
	if !errors.As(findingsErr, &validationFailure) {
		return findingsErr
	}
	if !errors.Is(findingsErr, factorydefinitions.ErrFactoryDefinitionValidationFailed) {
		return findingsErr
	}
	if errors.Is(findingsErr, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		return errors.New("validation findings must not match invalid-payload sentinel")
	}
	for _, target := range validationFailure.Validation.Targets {
		if strings.Contains(strings.ToLower(target.Code), "petri") ||
			strings.Contains(strings.ToLower(target.Message), "petri") {
			return errors.New("validation findings must not use Petri vocabulary")
		}
	}
	return nil
}

func newOwnedPeerService() factorydefinitions.Service {
	return fakeDefinitionsPeer{
		entries: []factorydefinitions.NamedFactoryListEntry{{
			Name:       "alpha",
			FactoryDir: "/factories/alpha",
			Current:    true,
		}},
	}
}

// TestRootContractInvariants_OwnedCatalogAndValidateThroughSingularService seals
// CLN-DEF-CONTRACTS story 001: a peer-shaped consumer exercises catalog and
// validate root operations using only the service-root import path.
func TestRootContractInvariants_OwnedCatalogAndValidateThroughSingularService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	consumer := peerOwnedDefinitionsConsumer{service: newOwnedPeerService()}
	validPayload := []byte(`{"name":"alpha"}`)

	listed, got, err := consumer.exerciseCatalogSuccess(ctx, "/factories")
	if err != nil {
		t.Fatalf("catalog success: %v", err)
	}
	if listed.Entries[0].Name != "alpha" || got.Entry.FactoryDir != "/factories/alpha" {
		t.Fatalf("catalog result = %#v / %#v, want alpha at /factories/alpha", listed, got)
	}
	if err := consumer.exerciseCatalogTypedFailures(ctx, "/factories"); err != nil {
		t.Fatalf("catalog typed failures: %v", err)
	}
	if err := consumer.exerciseValidateSuccess(ctx, validPayload); err != nil {
		t.Fatalf("validate success: %v", err)
	}
	if err := consumer.exerciseValidateTypedFailures(ctx); err != nil {
		t.Fatalf("validate typed failures: %v", err)
	}
}
