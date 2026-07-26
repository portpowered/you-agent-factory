package climanifest

import (
	"fmt"
	"slices"
)

var runSubmitFamilySpecs = []struct {
	id          string
	path        string
	operationID string
	flags       []string
}{
	{
		id: "you.run", path: "you run",
		flags: []string{
			"continuously", "debug", "default-worker-model", "default-worker-model-provider", "dir",
			"factory", "json", "named", "no-record", "output", "port", "quiet", "record", "replay",
			"runtime-log-compress", "runtime-log-dir", "runtime-log-max-age-days", "runtime-log-max-backups",
			"runtime-log-max-size-mb", "runtime-metrics-compress", "runtime-metrics-dir",
			"runtime-metrics-max-age-days", "runtime-metrics-max-backups", "runtime-metrics-max-size-mb",
			"server", "skip-permissions", "verbose", "with-mock-workers", "with-server", "with-site", "work",
		},
	},
	{
		id: "you.server", path: "you server",
		flags: []string{"debug", "default-worker-model", "default-worker-model-provider", "json", "server", "verbose"},
	},
	{
		id: "you.submit", path: "you submit", operationID: "submitWorkBySessionId",
		flags: []string{"debug", "default-worker-model", "default-worker-model-provider", "json", "name", "payload", "port", "server", "session", "verbose", "work-type-name"},
	},
	{
		id: "you.submit.batch", path: "you submit batch", operationID: "upsertWorkRequestBySessionId",
		flags: []string{"debug", "default-worker-model", "default-worker-model-provider", "dry-run", "file", "json", "port", "server", "session", "verbose"},
	},
}

// ValidateRunSubmitFamily rejects incomplete or contradictory canonical records
// before family generation can consume them.
func ValidateRunSubmitFamily(manifest Manifest) error {
	for _, spec := range runSubmitFamilySpecs {
		record, ok := manifest.Commands[spec.id]
		if !ok {
			return fmt.Errorf("missing command %q", spec.id)
		}
		if err := validateRunSubmitCommand(record, spec.id, spec.path, spec.operationID, spec.flags); err != nil {
			return err
		}
	}
	return validateRunSubmitInputPolicy(manifest)
}

func validateRunSubmitCommand(record Command, id, path, operationID string, flagNames []string) error {
	if record.ID != id || record.Path != path || !record.Runnable {
		return fmt.Errorf("command %q has contradictory identity, path, or runnable metadata", id)
	}
	if err := validateRunServerAuthority(record); err != nil {
		return err
	}
	if record.Handler == nil || record.Handler.ID != id+".handler" || record.Handler.OperationID != operationID {
		return fmt.Errorf("command %q has incomplete handler binding", id)
	}
	if err := validateRunSubmitFlags(record, flagNames); err != nil {
		return err
	}
	return validateRunSubmitExecutionMetadata(record)
}

func validateRunServerAuthority(record Command) error {
	if record.ID != "you.run" && record.ID != "you.server" {
		return nil
	}
	if record.Completeness != "authoritative" ||
		record.Documentation.Documentation.Title.CanonicalEnglish == "" ||
		record.Documentation.Documentation.Description.CanonicalEnglish == "" ||
		record.Lifecycle.State != "active" {
		return fmt.Errorf("command %q has incomplete authoritative lifecycle metadata", record.ID)
	}
	return nil
}

func validateRunSubmitExecutionMetadata(record Command) error {
	if len(record.Channels.Input) == 0 || len(record.Channels.Output) == 0 ||
		len(record.Outputs) == 0 ||
		len(record.Exits) == 0 || len(record.Constraints.Runtime) == 0 {
		return fmt.Errorf("command %q has incomplete execution metadata", record.ID)
	}
	if (record.ID == "you.run" || record.ID == "you.server") && len(record.Errors) == 0 {
		return fmt.Errorf("command %q has incomplete symbolic error metadata", record.ID)
	}
	return validateRelationshipReferences(record)
}

func validateRunSubmitFlags(record Command, flagNames []string) error {
	if len(record.Flags) != len(flagNames) {
		return fmt.Errorf("command %q has %d flags, want %d", record.ID, len(record.Flags), len(flagNames))
	}
	for _, name := range flagNames {
		flag, ok := record.FlagByLong(name)
		if !ok || flag.ID != record.ID+".flag."+name {
			return fmt.Errorf("command %q missing canonical --%s flag", record.ID, name)
		}
	}
	return nil
}

func validateRelationshipReferences(record Command) error {
	for _, relationship := range record.Relationships {
		for _, participant := range relationship.Participants {
			switch participant.Type {
			case "flag":
				if _, ok := record.Flags[participant.ID]; !ok {
					return fmt.Errorf("command %q relationship %q references missing flag %q", record.ID, relationship.ID, participant.ID)
				}
			case "argument":
				if _, ok := record.Arguments[participant.ID]; !ok {
					return fmt.Errorf("command %q relationship %q references missing argument %q", record.ID, relationship.ID, participant.ID)
				}
			default:
				return fmt.Errorf("command %q relationship %q has unsupported participant type %q", record.ID, relationship.ID, participant.Type)
			}
		}
	}
	return nil
}

func validateRunSubmitInputPolicy(manifest Manifest) error {
	run := manifest.Commands["you.run"]
	if err := validateRunInputPolicy(run); err != nil {
		return err
	}
	if manifest.Commands["you.server"].Exits["you.server.exit.cancel"].Code != 130 {
		return fmt.Errorf("command %q must declare cancellation exit 130", "you.server")
	}
	if err := validateSubmitInputPolicy(manifest.Commands["you.submit"]); err != nil {
		return err
	}
	if err := validateBatchInputPolicy(manifest.Commands["you.submit.batch"]); err != nil {
		return err
	}
	for _, record := range []Command{run, manifest.Commands["you.submit"], manifest.Commands["you.submit.batch"]} {
		if !record.Precedence.IsCanonical() {
			return fmt.Errorf("command %q has contradictory input precedence", record.ID)
		}
	}
	return nil
}

func validateRunInputPolicy(run Command) error {
	runArg, ok := run.ArgumentAt(0)
	if !ok || !runArg.Variadic || runArg.MaxCardinality != -1 || runArg.DoubleDash != "terminates-flags" || !slices.Contains(runArg.Channels, "stdin") {
		return fmt.Errorf("command %q has incomplete invocation input metadata", run.ID)
	}
	mockWorkers, _ := run.FlagByLong("with-mock-workers")
	if mockWorkers.ValueType != "string" || mockWorkers.NoOptionDefault == "" {
		return fmt.Errorf("command %q has contradictory --with-mock-workers no-option metadata", run.ID)
	}
	if err := validateRunStaticServerSurface(run); err != nil {
		return err
	}
	for _, relationshipID := range []string{
		"you.run.rel.selectors",
		"you.run.rel.work-invocation-input",
		"you.run.rel.recording",
		"you.run.rel.quiet-json",
		"you.run.rel.quiet-output",
	} {
		if _, ok := run.Relationships[relationshipID]; !ok {
			return fmt.Errorf("command %q missing relationship %q", run.ID, relationshipID)
		}
	}
	return nil
}

func validateRunStaticServerSurface(run Command) error {
	named, _ := run.FlagByLong("named")
	if named.Shorthand != "a" || slices.Contains(named.Aliases, "a") {
		return fmt.Errorf("command %q must declare canonical --named shorthand -a", run.ID)
	}
	withServer, _ := run.FlagByLong("with-server")
	withSite, _ := run.FlagByLong("with-site")
	if withServer.ValueType != "bool" || withSite.ValueType != "bool" {
		return fmt.Errorf("command %q has contradictory server/site metadata", run.ID)
	}
	if run.Exits["you.run.exit.cancel"].Code != 130 {
		return fmt.Errorf("command %q must declare cancellation exit 130", run.ID)
	}
	for _, flag := range run.Flags {
		if flag.Scope == "local" && flag.Usage == "" {
			return fmt.Errorf("command %q flag --%s is missing manifest-owned help", run.ID, flag.Long)
		}
	}
	return nil
}

func validateSubmitInputPolicy(submit Command) error {
	for _, name := range []string{"name", "work-type-name", "payload"} {
		flag, _ := submit.FlagByLong(name)
		if !flag.Required {
			return fmt.Errorf("command %q must declare --%s as required", submit.ID, name)
		}
	}
	return nil
}

func validateBatchInputPolicy(batch Command) error {
	batchArg, ok := batch.ArgumentAt(0)
	if !ok || batchArg.MaxCardinality != 1 || batchArg.Variadic || !slices.Contains(batchArg.Channels, "stdin") {
		return fmt.Errorf("command %q has incomplete batch input metadata", batch.ID)
	}
	return nil
}
