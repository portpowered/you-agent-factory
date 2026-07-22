package process

// CLIObservation is a detached, transport-neutral process-edge observation.
// Serialized inventories are immutable strings so callers cannot retain or
// mutate the private command representation.
type CLIObservation struct {
	CommandIdentityJSON string
	CommandInputsJSON   string
	CommandTree         string
	RunFlags            string
	Parse               CLIParseResult
}

type CLIParseResult struct {
	CommandPath string
	Positionals []string
	Flags       []CLIParsedFlag
}

type CLIParsedFlag struct {
	Name    string
	Changed bool
	Value   string
}

// CLIObserver is the optional outer process edge used by contract tooling.
// When configured, CLI parsing is observed without running the selected
// product handler.
type CLIObserver func(CLIObservation) error
