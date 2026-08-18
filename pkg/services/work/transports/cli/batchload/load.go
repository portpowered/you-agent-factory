// Package batchload reads canonical FACTORY_REQUEST_BATCH JSON for CLI ingress.
package batchload

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// LoadFromFile reads and validates a canonical FACTORY_REQUEST_BATCH from path.
func LoadFromFile(load work.RequestFileLoader, path string) (work.WorkRequest, error) {
	if load == nil {
		return work.WorkRequest{}, fmt.Errorf("Work request file loader is required")
	}
	return load(path)
}
