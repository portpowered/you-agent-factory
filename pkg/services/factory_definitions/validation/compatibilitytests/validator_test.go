package compatibilitytests

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
)

func testFactoryDefinitionValidator() factorydefinitions.SubmittedDefinitionValidationOperation {
	return factoryvalidation.New(nil)
}
