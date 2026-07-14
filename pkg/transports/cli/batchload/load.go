// Package batchload reads canonical FACTORY_REQUEST_BATCH JSON for CLI ingress.
package batchload

import (
	"fmt"
	"os"

	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// LoadFromFile reads and validates a canonical FACTORY_REQUEST_BATCH from path.
func LoadFromFile(path string) (interfaces.WorkRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return interfaces.WorkRequest{}, fmt.Errorf("read %s: %w", path, err)
	}
	req, err := requests.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return interfaces.WorkRequest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return req, nil
}
