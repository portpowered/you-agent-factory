package packagedinstallation

import (
	"context"
	"crypto/sha256"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type managedPublicationIdentity struct {
	rootDir, name, rootFileName string
	payloadSHA256               [sha256.Size]byte
}

func managedPublication(rootDir, name, rootFileName string, payload []byte) managedPublicationIdentity {
	return managedPublicationIdentity{rootDir, name, rootFileName, sha256.Sum256(payload)}
}

// prepareManagedContentIdentity retains only the verified published hash.
// Customer files and management evidence are read again by reconciliation.
// Prepared layouts stay operation-owned because persistence may mutate them.
func (service *Service) prepareManagedContentIdentity(
	ctx context.Context,
	rootDir, name, rootFileName string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	key := managedPublication(rootDir, name, rootFileName, payload)
	if identity, ok := service.managedContentIDs.Load(key); ok {
		return nil, identity.(string), nil
	}
	prepared, err := service.prepareManagedLayout(ctx, name, payload, rootFileName)
	if err != nil {
		return nil, "", err
	}
	identity, err := service.expectedManagedContentID(ctx, rootDir, name, prepared)
	if err != nil {
		return nil, "", err
	}
	service.managedContentIDs.Store(key, identity)
	return prepared, identity, nil
}
