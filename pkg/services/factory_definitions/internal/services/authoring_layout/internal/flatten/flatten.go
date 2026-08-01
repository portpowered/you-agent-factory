// Package flatten owns portable flatten orchestration for one on-disk Factory
// layout aggregate.
package flatten

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// FactoryLayout renders one on-disk or logical Factory path into detached
// canonical Factory bytes.
func FactoryLayout(
	ctx context.Context,
	path string,
	flatten factorydefinitions.FactoryLayoutFlattener,
) ([]byte, error) {
	if flatten == nil {
		return nil, fmt.Errorf("Factory Definitions layout flattener is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("factory path is required")
	}
	return flatten(path)
}
