package climanifestgen

import (
	"fmt"
	"slices"
)

// MetricsFamilyCommandIDs are the stable command IDs emitted for the local
// runtime-metrics inspection family.
var MetricsFamilyCommandIDs = []string{
	"you.metrics",
}

// IsMetricsFamilyCommandID reports whether id belongs to the metrics family.
func IsMetricsFamilyCommandID(id string) bool {
	return slices.Contains(MetricsFamilyCommandIDs, id)
}

// AssertMetricsFamilyCommandID rejects command IDs outside the metrics family.
func AssertMetricsFamilyCommandID(id string) error {
	if IsMetricsFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf("command id %q is outside the metrics family %v", id, MetricsFamilyCommandIDs)
}
