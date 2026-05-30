package initcmd

import _ "embed"

//go:embed default_factory.json
var embeddedDefaultFactoryJSON string

// DefaultFactoryJSON returns the canonical default init factory.json document
// shared by CLI init and server-side init scaffold writers.
func DefaultFactoryJSON() string {
	return embeddedDefaultFactoryJSON
}
