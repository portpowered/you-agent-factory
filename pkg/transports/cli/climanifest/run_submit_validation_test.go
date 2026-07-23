package climanifest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
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
