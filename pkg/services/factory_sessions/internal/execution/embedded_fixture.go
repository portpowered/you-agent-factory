package factorysessionexecution

import (
	_ "embed"
	"fmt"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// embeddedContractFixtureCatalog is the immutable deterministic catalog used by
// the default standalone Factory Session execution path. It is owned here so
// the installed binary does not need a repository-relative file at runtime.
//
//go:embed testdata/durable-session-contract-fixtures.json
var embeddedContractFixtureCatalog []byte

// NewFakeServiceFromEmbeddedContractFixtures constructs the default fixture
// service without consulting the host filesystem.
func NewFakeServiceFromEmbeddedContractFixtures(clock factory.Clock) (*FakeService, error) {
	if clock == nil {
		return nil, fmt.Errorf("Factory Session execution clock is required")
	}
	scenarios, err := loadFakeScenariosFromContractFixtureData(embeddedContractFixtureCatalog)
	if err != nil {
		return nil, err
	}
	return NewFakeService(clock, scenarios...)
}
