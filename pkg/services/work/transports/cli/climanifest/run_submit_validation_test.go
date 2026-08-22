package climanifest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
)

func TestValidateRunSubmitFamily_AcceptsProductionContract(t *testing.T) {
	manifest := loadRunSubmitFixture(t)
	if err := climanifest.ValidateRunSubmitFamily(manifest); err != nil {
		t.Fatalf("ValidateRunSubmitFamily() error = %v", err)
	}
}

func TestValidateRunSubmitFamily_RejectsIncompleteAndContradictoryContracts(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(climanifest.Manifest)
		wantError string
	}{
		{
			name: "missing run flag",
			mutate: func(manifest climanifest.Manifest) {
				delete(manifest.Commands["you.run"].Flags, "you.run.flag.factory")
			},
			wantError: "flags, want",
		},
		{
			name: "missing server command",
			mutate: func(manifest climanifest.Manifest) {
				delete(manifest.Commands, "you.server")
			},
			wantError: `missing command "you.server"`,
		},
		{
			name: "missing run manifest help",
			mutate: func(manifest climanifest.Manifest) {
				flag := manifest.Commands["you.run"].Flags["you.run.flag.with-server"]
				flag.Usage = ""
				manifest.Commands["you.run"].Flags[flag.ID] = flag
			},
			wantError: "missing manifest-owned help",
		},
		{
			name: "missing run symbolic errors",
			mutate: func(manifest climanifest.Manifest) {
				record := manifest.Commands["you.run"]
				record.Errors = nil
				manifest.Commands[record.ID] = record
			},
			wantError: "incomplete symbolic error metadata",
		},
		{
			name: "missing run selector relationship",
			mutate: func(manifest climanifest.Manifest) {
				delete(manifest.Commands["you.run"].Relationships, "you.run.rel.selectors")
			},
			wantError: "missing relationship",
		},
		{
			name: "dangling relationship participant",
			mutate: func(manifest climanifest.Manifest) {
				relationship := manifest.Commands["you.run"].Relationships["you.run.rel.selectors"]
				relationship.Participants[0].ID = "you.run.flag.missing"
				manifest.Commands["you.run"].Relationships[relationship.ID] = relationship
			},
			wantError: "references missing flag",
		},
		{
			name: "unary input not required",
			mutate: func(manifest climanifest.Manifest) {
				flag := manifest.Commands["you.submit"].Flags["you.submit.flag.payload"]
				flag.Required = false
				manifest.Commands["you.submit"].Flags[flag.ID] = flag
			},
			wantError: "must declare --payload as required",
		},
		{
			name: "batch stdin outranks explicit input",
			mutate: func(manifest climanifest.Manifest) {
				record := manifest.Commands["you.submit.batch"]
				record.Precedence.Order = []string{"stdin", "cli", "environment", "operator-config", "manifest-default", "factory-signature-default"}
				manifest.Commands[record.ID] = record
			},
			wantError: "contradictory input precedence",
		},
		{
			name: "missing no-option mock worker behavior",
			mutate: func(manifest climanifest.Manifest) {
				flag := manifest.Commands["you.run"].Flags["you.run.flag.with-mock-workers"]
				flag.NoOptionDefault = ""
				manifest.Commands["you.run"].Flags[flag.ID] = flag
			},
			wantError: "contradictory --with-mock-workers",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := loadRunSubmitFixture(t)
			test.mutate(manifest)
			err := climanifest.ValidateRunSubmitFamily(manifest)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ValidateRunSubmitFamily() error = %v, want containing %q", err, test.wantError)
			}
		})
	}
}

func TestValidateRunSubmitFamily_DocumentsRunParityInputs(t *testing.T) {
	manifest := loadRunSubmitFixture(t)
	run := manifest.Commands["you.run"]

	for _, test := range []struct {
		name string
		flag string
		want []string
	}{
		{
			name: "reasoning effort",
			flag: "worker-reasoning-effort",
			want: []string{"minimal", "low", "medium", "high", "xhigh", "max", "normalized", "before dispatch"},
		},
		{
			name: "file prompt",
			flag: "to-file",
			want: []string{"exact", "UTF-8", "one-shot", "--to", "positional", "stdin"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			flag, ok := run.FlagByLong(test.flag)
			if !ok {
				t.Fatalf("run flag --%s is missing", test.flag)
			}
			for _, marker := range test.want {
				if !strings.Contains(flag.Usage, marker) {
					t.Fatalf("run flag --%s usage = %q, want marker %q", test.flag, flag.Usage, marker)
				}
			}
		})
	}

	for _, relationshipID := range []string{
		"you.run.rel.to-file-invocation-input",
		"you.run.rel.to-file-work",
		"you.run.rel.to-file-continuously",
		"you.run.rel.to-file-replay",
		"you.run.rel.remote-with-server",
		"you.run.rel.remote-with-site",
	} {
		if _, ok := run.Relationships[relationshipID]; !ok {
			t.Fatalf("run manifest missing documented --to-file relationship %q", relationshipID)
		}
	}

	for _, test := range []struct {
		name string
		flag string
		want []string
	}{
		{name: "listen", flag: "listen", want: []string{"exact local listener", "--with-server", "--with-site"}},
		{name: "with-server", flag: "with-server", want: []string{"locally", "conflicts with --remote"}},
		{name: "with-site", flag: "with-site", want: []string{"locally", "conflicts with --remote"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			flag, ok := run.FlagByLong(test.flag)
			if !ok {
				t.Fatalf("run flag --%s is missing", test.flag)
			}
			for _, marker := range test.want {
				if !strings.Contains(flag.Usage, marker) {
					t.Fatalf("run flag --%s usage = %q, want marker %q", test.flag, flag.Usage, marker)
				}
			}
		})
	}

	root := manifest.Commands["you"]
	for _, test := range []struct {
		name string
		flag string
		want []string
	}{
		{name: "remote", flag: "remote", want: []string{"running You server", "does not wait", "listener"}},
		{name: "server", flag: "server", want: []string{"remote placement", "--listen", "warned compatibility"}},
	} {
		t.Run("root-"+test.name, func(t *testing.T) {
			flag, ok := root.FlagByLong(test.flag)
			if !ok {
				t.Fatalf("root flag --%s is missing", test.flag)
			}
			for _, marker := range test.want {
				if !strings.Contains(flag.Usage, marker) {
					t.Fatalf("root flag --%s usage = %q, want marker %q", test.flag, flag.Usage, marker)
				}
			}
		})
	}
}

func loadRunSubmitFixture(t *testing.T) climanifest.Manifest {
	t.Helper()
	path := testutil.MustRepoPath(t, climanifest.ProductionManifestPath)
	manifest, err := climanifest.LoadProduction(sourceStore(), path)
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	var clone climanifest.Manifest
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatalf("clone fixture: %v", err)
	}
	return clone
}
