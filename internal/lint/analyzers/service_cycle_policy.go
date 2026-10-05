package analyzers

import (
	"fmt"
	"strconv"
	"strings"
)

const serviceCycleRule = "service-cycle-weight"

func serviceCycleCeiling(text string) (int, bool, error) {
	value, present := 0, false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if parts[0] != serviceCycleRule {
			continue
		}
		if present || len(parts) != 3 || parts[1] != "internal/lint/analyzers" {
			return 0, false, fmt.Errorf("malformed/duplicate service-cycle-weight singleton: %s", line)
		}
		decimal := parts[2]
		if decimal != "0" && !positiveDecimal(decimal) {
			return 0, false, fmt.Errorf("invalid service-cycle-weight ceiling: %s", line)
		}
		var err error
		value, err = strconv.Atoi(decimal)
		if err != nil {
			return 0, false, fmt.Errorf("invalid service-cycle-weight ceiling: %w", err)
		}
		present = true
	}
	return value, present, nil
}

func compareServiceCycleCeilings(base, head string) error {
	old, existed, err := serviceCycleCeiling(base)
	if err != nil {
		return err
	}
	current, present, err := serviceCycleCeiling(head)
	if err != nil {
		return err
	}
	if existed && (!present || current > old) {
		return fmt.Errorf("service-cycle-weight ceiling may only decrease; base %d, current %d, present %t", old, current, present)
	}
	return nil
}
