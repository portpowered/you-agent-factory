// Package expand owns portable expand orchestration for one flattened or split
// Factory layout aggregate.
package expand

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// FactoryLayout materializes one flattened or split Factory path into a usable
// authored layout aggregate and reports expansion success facts.
func FactoryLayout(
	ctx context.Context,
	path string,
	expand factorydefinitions.FactoryLayoutExpander,
) (string, factorydefinitions.LayoutExpansionReport, error) {
	if expand == nil {
		return "", factorydefinitions.LayoutExpansionReport{},
			fmt.Errorf("Factory Definitions layout expander is required")
	}
	if err := ctx.Err(); err != nil {
		return "", factorydefinitions.LayoutExpansionReport{}, err
	}
	if strings.TrimSpace(path) == "" {
		return "", factorydefinitions.LayoutExpansionReport{},
			fmt.Errorf("factory path is required")
	}
	return expand(path)
}
