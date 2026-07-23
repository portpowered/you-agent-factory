package clicontract

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
)

func TestCheckProductionAcceptsCompleteApprovedTree(t *testing.T) {
	production := productionCLIInventory(t)
	productionInputs := productionCLIInputs(t)
	before := append([]commandidentity.CommandRecord(nil), production.Commands...)

	findings, err := CheckProduction(
		testSourceStore(),
		production,
		productionInputs,
		repositoryRoot(t),
	)
	if err != nil {
		t.Fatalf("CheckProduction() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("CheckProduction() findings =\n%s", formatFindings(findings))
	}

	if !reflect.DeepEqual(before, production.Commands) {
		t.Fatal("CheckProduction mutated the detached production inventory")
	}
}

func TestCheckProductionViolationUsesProductionDiagnosticsWithoutMutatingTree(t *testing.T) {
	tests := []struct {
		violation DeliberateViolation
		kind      string
		stableID  string
		path      string
		field     string
	}{
		{ViolationUncontractedCommand, KindUncontractedCommand, "you.experimental", "you experimental", ""},
		{ViolationStaleMetadata, KindStaleMetadata, "you", "you", "name"},
		{ViolationMissingHandler, KindMissingHandler, "you.run", "you run", "handler"},
		{ViolationAliasAsCanonical, KindAliasAsCanonical, "you.compatibility-preview", "you compatibility-preview", "classification"},
		{ViolationUncontractedGlobal, KindUncontractedGlobal, "you.flag.extra-global", "you", "long"},
	}

	for _, tc := range tests {
		t.Run(string(tc.violation), func(t *testing.T) {
			production := productionCLIInventory(t)
			productionInputs := productionCLIInputs(t)
			before := append([]commandidentity.CommandRecord(nil), production.Commands...)

			findings, err := CheckProductionViolation(
				testSourceStore(),
				production,
				productionInputs,
				repositoryRoot(t),
				tc.violation,
			)
			if err != nil {
				t.Fatalf("CheckProductionViolation() error = %v", err)
			}
			assertFinding(t, findings, tc.kind, tc.stableID, tc.path, tc.field)

			if !reflect.DeepEqual(before, production.Commands) {
				t.Fatal("CheckProductionViolation mutated the detached production inventory")
			}
		})
	}
}

func TestCheckProductionViolationRejectsUnknownFixture(t *testing.T) {
	findings, err := CheckProductionViolation(
		testSourceStore(),
		productionCLIInventory(t),
		productionCLIInputs(t),
		repositoryRoot(t),
		DeliberateViolation("unknown"),
	)
	if err == nil || err.Error() != `unknown deliberate CLI contract violation "unknown"` {
		t.Fatalf("CheckProductionViolation() findings = %#v, error = %v", findings, err)
	}
}

func TestValidateRejectsMissingAndUncontractedProductionCommands(t *testing.T) {
	input := productionInput(t)
	commands := append([]commandidentity.CommandRecord(nil), input.Production.Commands...)
	commands = removeProductionCommand(commands, "you run")
	commands = append(commands, commandidentity.CommandRecord{
		IDCandidate: "you.experimental", Name: "experimental", Path: "you experimental",
		Visibility: "visible", Runnable: true, HandlerPresent: true,
	})
	input.Production.Commands = commands

	findings := Validate(input)
	assertFinding(t, findings, KindMissingCommand, "you.run", "you run", "")
	assertFinding(t, findings, KindUncontractedCommand, "you.experimental", "you experimental", "")
}

func TestValidateRejectsRootGlobalSetAndMetadataDrift(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Input)
		kind     string
		stableID string
		field    string
	}{
		{
			name: "missing executable global",
			mutate: func(input *Input) {
				input.ProductionInputs.Flags = removeProductionFlag(
					input.ProductionInputs.Flags,
					"you.flag.server",
				)
			},
			kind:     KindMissingGlobal,
			stableID: "you.flag.server",
			field:    "long",
		},
		{
			name: "changed executable metadata",
			mutate: func(input *Input) {
				for index := range input.ProductionInputs.Flags {
					flag := &input.ProductionInputs.Flags[index]
					if flag.CommandPath == "you" && flag.IDCandidate == "you.flag.server" {
						flag.Default = "http://example.invalid"
						return
					}
				}
				t.Fatal("production server global is missing")
			},
			kind:     KindRootGlobalDrift,
			stableID: "you.flag.server",
			field:    "default",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := productionInput(t)
			test.mutate(&input)
			findings := Validate(input)
			assertFinding(t, findings, test.kind, test.stableID, "you", test.field)
		})
	}
}

func TestValidateKeepsCompatibilityOutOfCanonicalContracts(t *testing.T) {
	input := productionInput(t)
	compatibility := addSyntheticCompatibility(&input)
	input.Canonical.Commands[compatibility.ID] = compatibility
	input.GeneratedCanonical[0].Commands[compatibility.ID] = compatibility

	findings := Validate(input)
	assertFinding(t, findings, KindAliasAsCanonical, compatibility.ID, compatibility.Path, "classification")
}

func TestValidateRejectsStaleGeneratedMetadataFields(t *testing.T) {
	tests := []struct {
		name      string
		wantField string
		mutate    func(*climanifest.Command)
	}{
		{name: "identity", wantField: "path", mutate: func(command *climanifest.Command) { command.Path = "you stale" }},
		{name: "help", wantField: "documentation", mutate: func(command *climanifest.Command) {
			command.Documentation.Documentation.Title.CanonicalEnglish = "stale title"
		}},
		{name: "input completion", wantField: "flags", mutate: func(command *climanifest.Command) {
			flags := cloneFlags(command.Flags)
			flag := flags["you.flag.debug"]
			flag.Completion = "stale"
			flags[flag.ID] = flag
			command.Flags = flags
		}},
		{name: "lifecycle", wantField: "lifecycle", mutate: func(command *climanifest.Command) {
			command.Lifecycle.State = "stale"
		}},
		{name: "handler ID", wantField: "handler", mutate: func(command *climanifest.Command) {
			handler := *command.Handler
			handler.ID = "you.stale.handler"
			command.Handler = &handler
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := productionInput(t)
			manifest := cloneManifest(input.GeneratedCanonical[0])
			command := manifest.Commands["you"]
			tc.mutate(&command)
			manifest.Commands["you"] = command
			input.GeneratedCanonical[0] = manifest

			findings := Validate(input)
			assertFinding(t, findings, KindStaleMetadata, "you", "you", tc.wantField)
		})
	}
}

func TestValidateRejectsMissingRunnableHandlerDeterministically(t *testing.T) {
	input := productionInput(t)
	for index := range input.Production.Commands {
		if input.Production.Commands[index].Path == "you run" {
			input.Production.Commands[index].HandlerPresent = false
		}
	}

	first := Validate(input)
	second := Validate(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated findings differ:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	assertFinding(t, first, KindMissingHandler, "you.run", "you run", "handler")
}

func productionInput(t *testing.T) Input {
	t.Helper()
	root := repositoryRoot(t)
	production := productionCLIInventory(t)
	productionInputs := productionCLIInputs(t)
	canonical, err := climanifest.LoadProduction(testSourceStore(), filepath.Join(root, filepath.FromSlash(climanifest.ProductionManifestPath)))
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	compatibility, err := climanifest.LoadCompatibility(testSourceStore(), filepath.Join(root, filepath.FromSlash(climanifest.CompatibilityManifestPath)))
	if err != nil {
		t.Fatalf("LoadCompatibility() error = %v", err)
	}
	approved, err := LoadApprovedCompatibility(testSourceStore(), filepath.Join(root, filepath.FromSlash(CompatibilityInventoryPath)))
	if err != nil {
		t.Fatalf("LoadApprovedCompatibility() error = %v", err)
	}
	canonicalGenerated, compatibilityGenerated, err := loadGeneratedManifests()
	if err != nil {
		t.Fatalf("loadGeneratedManifests() error = %v", err)
	}
	return Input{
		Production: production, ProductionInputs: productionInputs,
		Canonical: cloneManifest(canonical), Compatibility: cloneManifest(compatibility),
		ApprovedCompatibility: approved, GeneratedCanonical: cloneManifests(canonicalGenerated),
		GeneratedCompatibility: cloneManifests(compatibilityGenerated),
	}
}

func cloneManifests(source []climanifest.Manifest) []climanifest.Manifest {
	result := make([]climanifest.Manifest, len(source))
	for index, manifest := range source {
		result[index] = cloneManifest(manifest)
	}
	return result
}

func cloneManifest(source climanifest.Manifest) climanifest.Manifest {
	commands := make(map[string]climanifest.Command, len(source.Commands))
	for id, command := range source.Commands {
		commands[id] = command
	}
	source.Commands = commands
	return source
}

func cloneFlags(source map[string]climanifest.Flag) map[string]climanifest.Flag {
	result := make(map[string]climanifest.Flag, len(source))
	for id, flag := range source {
		result[id] = flag
	}
	return result
}

func removeProductionCommand(commands []commandidentity.CommandRecord, path string) []commandidentity.CommandRecord {
	result := commands[:0]
	for _, command := range commands {
		if command.Path != path {
			result = append(result, command)
		}
	}
	return result
}

func removeProductionFlag(
	flags []cliinputs.FlagRecord,
	inputID string,
) []cliinputs.FlagRecord {
	result := flags[:0]
	for _, flag := range flags {
		if flag.CommandPath != "you" || flag.IDCandidate != inputID {
			result = append(result, flag)
		}
	}
	return result
}

func assertFinding(t *testing.T, findings []Finding, kind, stableID, path, field string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Kind == kind && finding.StableID == stableID && finding.Path == path && finding.Field == field {
			return
		}
	}
	t.Fatalf("missing finding kind=%q stableID=%q path=%q field=%q in\n%s", kind, stableID, path, field, formatFindings(findings))
}

func formatFindings(findings []Finding) string {
	result := ""
	for _, finding := range findings {
		result += finding.Error() + "\n"
	}
	return result
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
