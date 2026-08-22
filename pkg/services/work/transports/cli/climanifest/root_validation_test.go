package climanifest

import (
	"strings"
	"testing"
)

func TestValidateRootContractAcceptsCompleteManifestOwnedGlobal(t *testing.T) {
	manifest := rootContractTestManifest()
	if err := ValidateRootContract(manifest); err != nil {
		t.Fatalf("ValidateRootContract() error = %v", err)
	}
}

func TestValidateRootContractRejectsIncompleteOrAmbiguousGlobals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{name: "duplicate public spelling", mutate: func(manifest *Manifest) {
			root := manifest.Commands["you"]
			copy := root.Flags["you.flag.server"]
			copy.ID = "you.flag.copy"
			copy.HandlerBindingID = "you.binding.copy"
			root.Flags[copy.ID] = copy
			root.HandlerBindings[copy.HandlerBindingID] = HandlerBinding{ID: copy.HandlerBindingID, InputID: copy.ID}
			manifest.Commands["you"] = root
		}, want: "duplicate public spelling"},
		{name: "missing root record", mutate: func(manifest *Manifest) {
			delete(manifest.Commands, "you")
		}, want: "no matching root command"},
		{name: "invalid root scope", mutate: func(manifest *Manifest) {
			updateRootTestFlag(manifest, func(flag *Flag) { flag.Scope = "inherited" })
		}, want: "persistent scope"},
		{name: "missing stable binding", mutate: func(manifest *Manifest) {
			updateRootTestFlag(manifest, func(flag *Flag) { flag.HandlerBindingID = "you.binding.missing" })
		}, want: "missing stable handler binding"},
		{name: "invalid typed default", mutate: func(manifest *Manifest) {
			updateRootTestFlag(manifest, func(flag *Flag) {
				value := false
				flag.DefaultValue = &InputValue{Boolean: &value}
			})
		}, want: "typed default does not match"},
		{name: "unsupported alias", mutate: func(manifest *Manifest) {
			updateRootTestFlag(manifest, func(flag *Flag) { flag.Aliases = []string{"--diagnostics"} })
		}, want: "unsupported public spelling"},
		{name: "unsupported shorthand", mutate: func(manifest *Manifest) {
			updateRootTestFlag(manifest, func(flag *Flag) { flag.Shorthand = "h" })
		}, want: "unsupported shorthand"},
		{name: "missing usage", mutate: func(manifest *Manifest) {
			updateRootTestFlag(manifest, func(flag *Flag) { flag.Usage = "" })
		}, want: "manifest-owned usage"},
		{name: "missing sensitivity", mutate: func(manifest *Manifest) {
			updateRootTestFlag(manifest, func(flag *Flag) { flag.Sensitivity = "" })
		}, want: "unsupported sensitivity"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := rootContractTestManifest()
			test.mutate(&manifest)
			err := ValidateRootContract(manifest)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateRootContract() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidatePlacementContractRequiresAuthoritativeRunnableCapability(t *testing.T) {
	tests := []struct {
		name      string
		placement PlacementCapability
		wantError bool
	}{
		{name: "local-only", placement: PlacementLocalOnly},
		{name: "remote-only", placement: PlacementRemoteOnly},
		{name: "dual", placement: PlacementDual},
		{name: "missing", wantError: true},
		{name: "invalid", placement: PlacementCapability("unsupported"), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := Manifest{Commands: map[string]Command{
				"you.example": {
					ID:           "you.example",
					Runnable:     true,
					Completeness: "authoritative",
					Placement:    test.placement,
				},
			}}
			err := ValidatePlacementContract(manifest)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "placement capability") {
					t.Fatalf("ValidatePlacementContract() error = %v, want placement failure", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePlacementContract() error = %v", err)
			}
		})
	}
}

func TestCommandResolvePlacementUsesExplicitRootSelection(t *testing.T) {
	tests := []struct {
		name    string
		command Command
		remote  bool
		want    ExecutionPlacement
		wantErr bool
	}{
		{name: "dual defaults local", command: Command{Path: "you example", Placement: PlacementDual}, want: ExecutionPlacementLocal},
		{name: "dual remote", command: Command{Path: "you example", Placement: PlacementDual}, remote: true, want: ExecutionPlacementRemote},
		{name: "remote-only defaults remote", command: Command{Path: "you example", Placement: PlacementRemoteOnly}, want: ExecutionPlacementRemote},
		{name: "local-only rejects remote", command: Command{Path: "you example", Placement: PlacementLocalOnly}, remote: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.command.ResolvePlacement(test.remote)
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "local placement only") {
					t.Fatalf("ResolvePlacement() error = %v, want local-only rejection", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("ResolvePlacement() = (%q, %v), want (%q, nil)", got, err, test.want)
			}
		})
	}
}

func rootContractTestManifest() Manifest {
	defaultServer := "http://localhost:7437"
	defaultRemote := false
	remoteNoOption := true
	flag := Flag{
		ID: "you.flag.server", Long: "server", Aliases: []string{}, Scope: "persistent",
		ValueType: "string", Kind: "named", MaxCardinality: 1,
		DefaultValue:     &InputValue{String: &defaultServer},
		AcceptedSources:  []string{"cli", "manifest-default"},
		HandlerBindingID: "you.binding.server", Usage: "factory API base URI",
		Sensitivity: "public", Completion: "none", Visibility: "visible",
		Lifecycle: Lifecycle{FormatVersion: "1.0.0", ItemID: "you.flag.server", State: "active", Since: "1.0.0"},
	}
	remote := Flag{
		ID: "you.flag.remote", Long: "remote", Aliases: []string{}, Scope: "persistent",
		ValueType: "bool", Kind: "named", MaxCardinality: 1,
		DefaultValue:     &InputValue{Boolean: &defaultRemote},
		NoOptionValue:    &InputValue{Boolean: &remoteNoOption},
		AcceptedSources:  []string{"cli", "manifest-default"},
		HandlerBindingID: "you.binding.remote", Usage: "execute remotely",
		Sensitivity: "public", Completion: "none", Visibility: "visible",
		Lifecycle: Lifecycle{FormatVersion: "1.0.0", ItemID: "you.flag.remote", State: "active", Since: "1.0.0"},
	}
	root := Command{
		ID: "you", Name: "you", Path: "you",
		Flags: map[string]Flag{flag.ID: flag, remote.ID: remote},
		HandlerBindings: map[string]HandlerBinding{
			flag.HandlerBindingID:   {ID: flag.HandlerBindingID, InputID: flag.ID},
			remote.HandlerBindingID: {ID: remote.HandlerBindingID, InputID: remote.ID},
		},
		RootLifecycle: &RootLifecycle{
			NoArguments: "help", HelpOutput: "stdout", ExitCode: 0, SideEffects: "none",
			Ownership: RootOwnership{Help: "you", Init: "you.init", Run: "you.run", Server: flag.ID},
		},
	}
	return Manifest{FormatVersion: "1.0.0", RootPath: "you", Commands: map[string]Command{"you": root}}
}

func updateRootTestFlag(manifest *Manifest, mutate func(*Flag)) {
	root := manifest.Commands["you"]
	flag := root.Flags["you.flag.server"]
	mutate(&flag)
	root.Flags["you.flag.server"] = flag
	manifest.Commands["you"] = root
}
