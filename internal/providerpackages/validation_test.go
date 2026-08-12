package providerpackages

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestValidateAcceptsSelectableAndCatalogOnlyACPPackages(t *testing.T) {
	source := packageFixture()
	packages, err := Validate(source, []RuntimeProfile{{ID: "cursor-acp"}})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(packages) != 2 {
		t.Fatalf("package count = %d, want 2", len(packages))
	}
	if packages[0].ID != "catalog-acp" || packages[0].Selectable() {
		t.Fatalf("catalog package = %#v, want first and non-selectable", packages[0])
	}
	if packages[1].ID != "cursor-acp" || !packages[1].Selectable() {
		t.Fatalf("selectable package = %#v, want second and selectable", packages[1])
	}
}

func TestValidateRejectsRegisteredRuntimeProfileWithoutPackageOwner(t *testing.T) {
	_, err := Validate(packageFixture(), []RuntimeProfile{
		{ID: "cursor-acp"},
		{ID: "missing-profile"},
	})
	if err == nil || !strings.Contains(err.Error(), `registered runtime profile "missing-profile" has no owning provider package`) {
		t.Fatalf("Validate() error = %v, want missing profile owner", err)
	}
}

func TestValidateRejectsMultiplePackagesClaimingOneRuntimeProfile(t *testing.T) {
	source := packageFixture()
	addFile(source, "packages/model-providers/providers/other-acp/provider.yaml", []byte(providerManifest("other-acp", "externally-supplied", "other-docs", "supported")))
	addFile(source, "packages/model-providers/providers/other-acp/harness.yaml", selectableHarness("cursor-acp", "other-agent"))

	_, err := Validate(source, []RuntimeProfile{{ID: "cursor-acp"}})
	if err == nil || !strings.Contains(err.Error(), `runtime profile collision: "cursor-acp"`) {
		t.Fatalf("Validate() error = %v, want runtime profile collision", err)
	}
}

func TestRuntimeProjectionIncludesSelectablePackageOnceAndOmitsCatalogOnly(t *testing.T) {
	packages, err := Validate(packageFixture(), []RuntimeProfile{{ID: "cursor-acp"}})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	projection := RuntimeProjection(packages)
	if len(projection.ACP) != 1 {
		t.Fatalf("runtime projection count = %d, want one selectable package", len(projection.ACP))
	}
	entry := projection.ACP[0]
	if entry.Name != "cursor-acp" || !sameStrings(entry.Aliases, nil) || entry.Transport != TransportStdio || entry.Executable != "cursor-agent" || entry.Command != "cursor-agent acp" || !sameStrings(entry.Arguments, []string{"acp"}) || entry.Posture != LaunchPostureInstalledExecutable || entry.Implementation.Kind != ImplementationKindACPAgent || entry.Implementation.Profile != "cursor-acp" {
		t.Fatalf("runtime projection = %#v, want package-owned cursor launch", entry)
	}
}

func TestValidateRejectsFailClosedACPPackageShapes(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(fstest.MapFS)
		wantErr string
	}{
		{
			name: "missing runtime binding",
			mutate: func(source fstest.MapFS) {
				mutateFile(source, "packages/model-providers/providers/cursor-acp/harness.yaml", "  profile: cursor-acp\n", "")
			},
			wantErr: "selectable launch requires an implementation profile",
		},
		{
			name: "unknown runtime profile",
			mutate: func(source fstest.MapFS) {
				mutateFile(source, "packages/model-providers/providers/cursor-acp/harness.yaml", "profile: cursor-acp", "profile: missing-profile")
			},
			wantErr: "implementation profile \"missing-profile\" is not registered",
		},
		{
			name: "invalid launch command",
			mutate: func(source fstest.MapFS) {
				mutateFile(source, "packages/model-providers/providers/cursor-acp/harness.yaml", "command: cursor-agent", "command: cursor-agent --acp")
			},
			wantErr: "shell-free executable token",
		},
		{
			name: "incomplete capability evidence",
			mutate: func(source fstest.MapFS) {
				mutateFile(source, "packages/model-providers/providers/cursor-acp/provider.yaml", "    evidenceRefs: [cursor-docs]", "    evidenceRefs: []")
			},
			wantErr: "requires evidenceRefs",
		},
		{
			name: "mismatched capability evidence",
			mutate: func(source fstest.MapFS) {
				mutateFile(source, "packages/model-providers/providers/cursor-acp/provider.yaml", "    factRefs: [model_catalog, harness/acp, harness/input/text]", "    factRefs: [model_catalog]")
			},
			wantErr: "does not cite capability fact",
		},
		{
			name: "catalog-only known fact",
			mutate: func(source fstest.MapFS) {
				mutateFile(source, "packages/model-providers/providers/catalog-acp/provider.yaml", "support: unknown", "support: supported")
			},
			wantErr: "must remain unknown",
		},
		{
			name: "catalog-only missing prerequisite",
			mutate: func(source fstest.MapFS) {
				mutateFile(source, "packages/model-providers/providers/catalog-acp/provider.yaml", "  prerequisites:\n    - kind: executable\n      name: catalog-agent\n      description: Install the documented agent before selecting it.\n", "  prerequisites: []\n")
			},
			wantErr: "requires actionable prerequisites",
		},
		{
			name: "identity shadows canonical id",
			mutate: func(source fstest.MapFS) {
				addFile(source, "packages/model-providers/providers/other-acp/provider.yaml", []byte(providerManifest("other-acp", "externally-supplied", "shared-acp", "supported")))
				addFile(source, "packages/model-providers/providers/other-acp/harness.yaml", selectableHarness("cursor-acp", "other-agent"))
				mutateFile(source, "packages/model-providers/providers/other-acp/provider.yaml", "aliases: []", "aliases: [catalog-acp]")
			},
			wantErr: "provider package identity collision",
		},
		{
			name: "partial import scaffold",
			mutate: func(source fstest.MapFS) {
				addFile(source, "packages/model-providers/providers/import-scaffold/README.md", []byte("candidate only\n"))
			},
			wantErr: "provider.yaml",
		},
		{
			name: "missing ACP harness definition",
			mutate: func(source fstest.MapFS) {
				delete(source, "packages/model-providers/providers/cursor-acp/harness.yaml")
			},
			wantErr: "requires harness.yaml",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := packageFixture()
			test.mutate(source)
			_, err := Validate(source, []RuntimeProfile{{ID: "cursor-acp"}})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeProjectionQuotesUnsafeArgumentsLosslessly(t *testing.T) {
	source := packageFixture()
	mutateFile(source, "packages/model-providers/providers/cursor-acp/harness.yaml", "arguments: [acp]", `arguments: ["hello world", "semi;colon", "quote's"]`)

	packages, err := Validate(source, []RuntimeProfile{{ID: "cursor-acp"}})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	projection := RuntimeProjection(packages)
	if len(projection.ACP) != 1 {
		t.Fatalf("runtime projection count = %d, want one selectable package", len(projection.ACP))
	}
	entry := projection.ACP[0]
	if entry.Command != `cursor-agent 'hello world' 'semi;colon' 'quote'\''s'` {
		t.Fatalf("runtime command = %q, want lossless shell-word encoding", entry.Command)
	}
	if !sameStrings(entry.Arguments, []string{"hello world", "semi;colon", "quote's"}) {
		t.Fatalf("runtime arguments = %#v, want explicit argument vector", entry.Arguments)
	}
}

func TestRuntimeProjectionQuotesUnsafeExecutableLosslessly(t *testing.T) {
	source := packageFixture()
	mutateFile(source, "packages/model-providers/providers/cursor-acp/harness.yaml", "command: cursor-agent", `command: agent'\tool`)

	packages, err := Validate(source, []RuntimeProfile{{ID: "cursor-acp"}})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	projection := RuntimeProjection(packages)
	if len(projection.ACP) != 1 {
		t.Fatalf("runtime projection count = %d, want one selectable package", len(projection.ACP))
	}
	entry := projection.ACP[0]
	if entry.Executable != `agent'\tool` {
		t.Fatalf("runtime executable = %q, want exact package executable", entry.Executable)
	}
	if entry.Command != `'agent'\''\tool' acp` {
		t.Fatalf("runtime command = %q, want lossless executable and argument encoding", entry.Command)
	}
}

func TestValidateRejectsAliasCollisionsAcrossPackages(t *testing.T) {
	source := packageFixture()
	addFile(source, "packages/model-providers/providers/other-acp/provider.yaml", []byte(providerManifest("other-acp", "externally-supplied", "other-agent", "supported")))
	addFile(source, "packages/model-providers/providers/other-acp/harness.yaml", selectableHarness("cursor-acp", "other-agent"))
	mutateFile(source, "packages/model-providers/providers/other-acp/provider.yaml", "aliases: []", "aliases: [cursor-acp]")
	_, err := Validate(source, []RuntimeProfile{{ID: "cursor-acp"}})
	if err == nil || !strings.Contains(err.Error(), "provider package identity collision") {
		t.Fatalf("Validate() error = %v, want alias collision", err)
	}
}

func packageFixture() fstest.MapFS {
	return fstest.MapFS{
		"packages/model-providers/providers/cursor-acp/provider.yaml": &fstest.MapFile{
			Data: []byte(providerManifest("cursor-acp", "externally-supplied", "cursor-docs", "supported")),
		},
		"packages/model-providers/providers/cursor-acp/harness.yaml": &fstest.MapFile{
			Data: selectableHarness("cursor-acp", "cursor-agent"),
		},
		"packages/model-providers/providers/catalog-acp/provider.yaml": &fstest.MapFile{
			Data: []byte(catalogOnlyManifest()),
		},
		"packages/model-providers/providers/catalog-acp/harness.yaml": &fstest.MapFile{
			Data: []byte(catalogOnlyHarness()),
		},
	}
}

func providerManifest(id, availability, evidenceID, support string) string {
	return "id: " + id + "\n" +
		"aliases: []\n" +
		"implementationAvailability: " + availability + "\n" +
		"harness:\n" +
		"  kind: acp\n" +
		"  acpSupport:\n" +
		"    support: " + support + "\n" +
		"    evidenceRefs: [" + evidenceID + "]\n" +
		"modelCatalogPosture: exact\n" +
		"harnessRoutes:\n" +
		"  - direction: input\n" +
		"    modality: text\n" +
		"    support: " + support + "\n" +
		"    transport: inline\n" +
		"    evidenceRefs: [" + evidenceID + "]\n" +
		"evidence:\n" +
		"  - id: " + evidenceID + "\n" +
		"    kind: primary_documentation\n" +
		"    verifiedOn: \"2026-08-11\"\n" +
		"    factRefs: [model_catalog, harness/acp, harness/input/text]\n" +
		"models: []\n" +
		"tools: []\n" +
		"knownLimits: []\n" +
		"discovery:\n" +
		"  prerequisites:\n" +
		"    - kind: executable\n" +
		"      name: " + id + "\n" +
		"      description: Install the provider executable before execution.\n"
}

func catalogOnlyManifest() string {
	return "id: catalog-acp\n" +
		"aliases: []\n" +
		"implementationAvailability: catalog-only\n" +
		"harness:\n" +
		"  kind: acp\n" +
		"  acpSupport:\n" +
		"    support: unknown\n" +
		"modelCatalogPosture: unknown\n" +
		"harnessRoutes:\n" +
		"  - direction: input\n" +
		"    modality: text\n" +
		"    support: unknown\n" +
		"    transport: none\n" +
		"evidence: []\n" +
		"models: []\n" +
		"tools: []\n" +
		"knownLimits: []\n" +
		"discovery:\n" +
		"  prerequisites:\n" +
		"    - kind: executable\n" +
		"      name: catalog-agent\n" +
		"      description: Install the documented agent before selecting it.\n"
}

func selectableHarness(profile, command string) []byte {
	return []byte("implementation:\n  kind: acp_agent\n  profile: " + profile + "\nlaunch:\n  posture: installed_executable\n  transport: stdio\n  command: " + command + "\n  arguments: [acp]\n")
}

func catalogOnlyHarness() string {
	return "implementation:\n  kind: acp_agent\nlaunch:\n  posture: catalog_only\n  transport: stdio\n"
}

func addFile(source fstest.MapFS, path string, data []byte) {
	source[path] = &fstest.MapFile{Data: data}
}

func mutateFile(source fstest.MapFS, path, old, replacement string) {
	file, ok := source[path]
	if !ok {
		panic("missing fixture " + path)
	}
	updated := strings.Replace(string(file.Data), old, replacement, 1)
	if updated == string(file.Data) {
		panic("fixture does not contain " + old)
	}
	source[path] = &fstest.MapFile{Data: []byte(updated), Mode: file.Mode}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
