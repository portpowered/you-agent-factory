package retiredsurfaceguard

import "strings"

// CLICommandRecord captures one reachable production CLI command identity.
type CLICommandRecord struct {
	Path              string
	Aliases           []string
	Visibility        string
	Lifecycle         string
	DeprecatedMessage string
}

// CLIInventory is the production CLI command tree snapshot scanned by guards.
type CLIInventory struct {
	Commands []CLICommandRecord
}

// ScanCLIReintroductionViolations reports retired CLI paths re-registered directly,
// via aliases, or through hidden or deprecated wrappers.
func ScanCLIReintroductionViolations(inventory CLIInventory) []Violation {
	retired := retiredCLIPathSet()
	registered := make(map[string]struct{}, len(inventory.Commands))
	for _, record := range inventory.Commands {
		registered[record.Path] = struct{}{}
	}

	var violations []Violation
	for path := range retired {
		if _, stillRegistered := registered[path]; stillRegistered {
			violations = append(violations, Violation{
				Family:  "cli",
				Surface: path,
				Detail:  "retired CLI path is still registered in the production command tree",
			})
		}
	}

	for _, record := range inventory.Commands {
		if _, stillRegistered := retired[record.Path]; stillRegistered {
			continue
		}
		for _, alias := range record.Aliases {
			for retiredPath := range retired {
				if cliAliasReintroducesRetiredPath(record.Path, alias, retiredPath) {
					violations = append(violations, Violation{
						Family:  "cli",
						Surface: retiredPath,
						Detail:  "alias " + alias + " on " + record.Path + " would reintroduce retired CLI path",
					})
				}
			}
		}
	}

	return violations
}

func retiredCLIPathSet() map[string]struct{} {
	retired := make(map[string]struct{}, len(settledRetiredCLIPaths))
	for _, path := range settledRetiredCLIPaths {
		retired[path] = struct{}{}
	}
	return retired
}

func cliAliasReintroducesRetiredPath(recordPath, alias, retiredPath string) bool {
	retiredSegments := strings.Fields(strings.TrimPrefix(retiredPath, "you "))
	if len(retiredSegments) < 2 {
		return false
	}
	retiredLeaf := retiredSegments[len(retiredSegments)-1]
	recordSegments := strings.Fields(strings.TrimPrefix(recordPath, "you "))
	if len(recordSegments) < 2 {
		return false
	}
	return alias == retiredLeaf &&
		strings.Join(recordSegments[:len(recordSegments)-1], " ") ==
			strings.Join(retiredSegments[:len(retiredSegments)-1], " ")
}
