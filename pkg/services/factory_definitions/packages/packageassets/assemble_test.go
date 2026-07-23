package packageassets

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

func TestAssemble_DiscoversFlatAndNestedScriptsInTargetOrder(t *testing.T) {
	definition := Definition{
		Package:     "@you/example",
		FactoryJSON: []byte(`{"name":"@you/example"}`),
		Assets: fstest.MapFS{
			"assets/scripts/z-last.sh":       {Data: []byte("#!/bin/sh\necho last\n")},
			"assets/scripts/nested/first.py": {Data: []byte("print('first')\n")},
			"assets/scripts/middle":          {Data: []byte("exact content without newline")},
		},
		AssetRoot: "assets",
	}

	assembled, err := Assemble(definition)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	files := assembledBundledFiles(t, assembled)
	want := []scriptAsset{
		{targetPath: "factory/scripts/middle", content: "exact content without newline"},
		{targetPath: "factory/scripts/nested/first.py", content: "print('first')\n"},
		{targetPath: "factory/scripts/z-last.sh", content: "#!/bin/sh\necho last\n"},
	}
	if len(files) != len(want) {
		t.Fatalf("bundled files = %#v, want %d scripts", files, len(want))
	}
	for i, expected := range want {
		assertScriptEntry(t, files[i], expected)
	}
}

func TestAssemble_DiscoversPortableDocumentsAndInputsWithExactContent(t *testing.T) {
	definition := Definition{
		Package:     "@you/example",
		FactoryJSON: []byte(`{"name":"@you/example"}`),
		Assets: fstest.MapFS{
			"docs/guide.md":                    {Data: []byte("# Guide\n\nExact.\n")},
			"inputs/task/default/request.json": {Data: []byte("{\n  \"input\": \"keep spaces  \"\n}\n")},
		},
	}

	assembled, err := Assemble(definition)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	files := assembledBundledFiles(t, assembled)
	if len(files) != 2 {
		t.Fatalf("bundled files = %#v, want document and input", files)
	}
	assertBundledEntry(
		t, files[0], "factory/docs/guide.md", interfaces.BundledFileTypeDoc, "# Guide\n\nExact.\n",
	)
	assertBundledEntry(
		t, files[1], "factory/inputs/task/default/request.json", interfaces.BundledFileTypeInput,
		"{\n  \"input\": \"keep spaces  \"\n}\n",
	)
}

func TestAssemble_AbsentOrEmptyScriptsContributeNoEntries(t *testing.T) {
	for _, tt := range []struct {
		name   string
		assets fstest.MapFS
	}{
		{name: "absent", assets: fstest.MapFS{}},
		{name: "empty", assets: fstest.MapFS{"scripts": {Mode: fs.ModeDir | 0o755}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assembled, err := Assemble(Definition{
				Package:     "@you/example",
				FactoryJSON: []byte(`{"name":"@you/example"}`),
				Assets:      tt.assets,
			})
			if err != nil {
				t.Fatalf("Assemble: %v", err)
			}
			if files := assembledBundledFiles(t, assembled); len(files) != 0 {
				t.Fatalf("bundled files = %#v, want none", files)
			}
		})
	}
}

func TestAssemble_RejectsMissingAssetFilesystemWithoutPanic(t *testing.T) {
	definition := Definition{
		Package:     "@you/example",
		FactoryJSON: []byte(`{"name":"@you/example"}`),
	}

	_, err := Assemble(definition)
	assertAssetError(t, err, definition.Package, "scripts")
}

func TestAssemble_ScriptsAreRepeatableAndDoNotMutateDefinition(t *testing.T) {
	original := []byte(`{"name":"@you/example","supportingFiles":{"requiredTools":[]}}`)
	definition := Definition{
		Package:     "@you/example",
		FactoryJSON: append([]byte(nil), original...),
		Assets: fstest.MapFS{
			"scripts/setup.sh": {Data: []byte("#!/bin/sh\necho setup\n")},
		},
	}

	first, err := Assemble(definition)
	if err != nil {
		t.Fatalf("first Assemble: %v", err)
	}
	second, err := Assemble(definition)
	if err != nil {
		t.Fatalf("second Assemble: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeated assembly differs:\nfirst:  %s\nsecond: %s", first, second)
	}
	if !reflect.DeepEqual(assembledBundledFiles(t, first), assembledBundledFiles(t, second)) {
		t.Fatal("repeated assembly produced semantically different script entries")
	}
	if string(definition.FactoryJSON) != string(original) {
		t.Fatal("assembly mutated the authored factory definition")
	}
}

func TestAssemble_RejectsUnsafeAuthoredBundledTargets(t *testing.T) {
	for _, target := range []string{
		"/tmp/setup.sh",
		"C:/temp/setup.sh",
		"factory/scripts/../../outside.sh",
		"factory/scripts/../outside.sh",
	} {
		t.Run(strings.ReplaceAll(target, "/", "_"), func(t *testing.T) {
			definition := Definition{
				Package: "@you/example",
				FactoryJSON: []byte(`{"name":"@you/example","supportingFiles":{"bundledFiles":[{` +
					`"id":"unsafe","type":"SCRIPT","targetPath":` + quotedJSON(t, target) + `,` +
					`"content":{"encoding":"utf-8","inline":"echo unsafe"}}]}}`),
				Assets: fstest.MapFS{},
			}

			_, err := Assemble(definition)
			assertAssetError(t, err, definition.Package, target)
		})
	}
}

func TestAssemble_RejectsScriptTargetOutsideCanonicalScriptRoot(t *testing.T) {
	definition := Definition{
		Package: "@you/example",
		FactoryJSON: []byte(`{"name":"@you/example","supportingFiles":{"bundledFiles":[{` +
			`"id":"outside","type":"SCRIPT","targetPath":"factory/docs/setup.sh",` +
			`"content":{"encoding":"utf-8","inline":"echo unsafe"}}]}}`),
		Assets: fstest.MapFS{},
	}

	_, err := Assemble(definition)
	assertAssetError(t, err, definition.Package, "factory/docs/setup.sh")
}

func TestAssemble_RejectsPortableAssetTargetsOutsideCanonicalTypeRoots(t *testing.T) {
	tests := []struct {
		name       string
		fileType   string
		targetPath string
		wantRoot   string
	}{
		{
			name:       "document",
			fileType:   interfaces.BundledFileTypeDoc,
			targetPath: "factory/inputs/guide.md",
			wantRoot:   "factory/docs/",
		},
		{
			name:       "input",
			fileType:   interfaces.BundledFileTypeInput,
			targetPath: "factory/docs/request.json",
			wantRoot:   "factory/inputs/",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			definition := Definition{
				Package: "@you/example",
				FactoryJSON: []byte(`{"name":"@you/example","supportingFiles":{"bundledFiles":[{` +
					`"id":"outside","type":"` + test.fileType + `","targetPath":"` + test.targetPath + `",` +
					`"content":{"encoding":"utf-8","inline":"content"}}]}}`),
				Assets: fstest.MapFS{},
			}

			_, err := Assemble(definition)
			assertAssetError(t, err, definition.Package, test.targetPath)
			if !strings.Contains(err.Error(), test.wantRoot) {
				t.Fatalf("error = %q, want canonical root %q", err, test.wantRoot)
			}
		})
	}
}

func TestAssemble_RejectsDuplicateDiscoveredAndAuthoredTarget(t *testing.T) {
	const target = "factory/scripts/setup.sh"
	definition := Definition{
		Package: "@you/example",
		FactoryJSON: []byte(`{"name":"@you/example","supportingFiles":{"bundledFiles":[{` +
			`"id":"authored","type":"SCRIPT","targetPath":"` + target + `",` +
			`"content":{"encoding":"utf-8","inline":"echo authored"}}]}}`),
		Assets: fstest.MapFS{
			"scripts/setup.sh": {Data: []byte("echo discovered\n")},
		},
	}

	_, err := Assemble(definition)
	assertAssetError(t, err, definition.Package, target)
	if !strings.Contains(err.Error(), "duplicate canonical target") {
		t.Fatalf("error = %q, want duplicate target detail", err)
	}
}

func TestAssemble_RejectsUnsupportedScriptAssetKinds(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode fs.FileMode
	}{
		{name: "symlink", mode: fs.ModeSymlink | 0o777},
		{name: "named pipe", mode: fs.ModeNamedPipe | 0o644},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const assetPath = "scripts/setup.sh"
			definition := Definition{
				Package:     "@you/example",
				FactoryJSON: []byte(`{"name":"@you/example"}`),
				Assets: fstest.MapFS{
					assetPath: {Mode: tt.mode},
				},
			}

			_, err := Assemble(definition)
			assertAssetError(t, err, definition.Package, assetPath)
			if !strings.Contains(err.Error(), "unsupported non-regular") {
				t.Fatalf("error = %q, want unsupported file-kind detail", err)
			}
		})
	}
}

func TestAssemble_RejectsScriptsRootThatIsAFile(t *testing.T) {
	definition := Definition{
		Package:     "@you/example",
		FactoryJSON: []byte(`{"name":"@you/example"}`),
		Assets: fstest.MapFS{
			"scripts": {Data: []byte("not a directory\n")},
		},
	}

	_, err := Assemble(definition)
	assertAssetError(t, err, definition.Package, "scripts")
	if !strings.Contains(err.Error(), "must be a directory") {
		t.Fatalf("error = %q, want scripts-root file rejection", err)
	}
}

func TestAssemble_RejectsUnreadableAndInvalidUTF8Scripts(t *testing.T) {
	const assetPath = "scripts/setup.sh"
	base := fstest.MapFS{assetPath: {Data: []byte("echo ok\n")}}
	tests := []struct {
		name   string
		assets fs.FS
		want   string
	}{
		{
			name:   "unreadable",
			assets: openErrorFS{FS: base, path: assetPath, err: fs.ErrPermission},
			want:   "permission denied",
		},
		{
			name:   "invalid UTF-8",
			assets: fstest.MapFS{assetPath: {Data: []byte{0xff, 0xfe}}},
			want:   "not valid UTF-8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := Definition{
				Package:     "@you/example",
				FactoryJSON: []byte(`{"name":"@you/example"}`),
				Assets:      tt.assets,
			}

			_, err := Assemble(definition)
			assertAssetError(t, err, definition.Package, assetPath)
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want detail %q", err, tt.want)
			}
		})
	}
}

func TestAssemble_RejectsPackageRootEscape(t *testing.T) {
	definition := Definition{
		Package:     "@you/example",
		FactoryJSON: []byte(`{"name":"@you/example"}`),
		Assets:      fstest.MapFS{},
		AssetRoot:   "../outside",
	}

	_, err := Assemble(definition)
	assertAssetError(t, err, definition.Package, definition.AssetRoot)
}

type openErrorFS struct {
	fs.FS
	path string
	err  error
}

func (f openErrorFS) Open(name string) (fs.File, error) {
	if name == f.path {
		return nil, f.err
	}
	return f.FS.Open(name)
}

func quotedJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON string: %v", err)
	}
	return string(encoded)
}

func assertAssetError(t *testing.T, err error, packageName, assetOrTarget string) {
	t.Helper()
	if err == nil {
		t.Fatal("Assemble succeeded, want error")
	}
	for _, want := range []string{packageName, assetOrTarget} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want context %q", err, want)
		}
	}
}

func assembledBundledFiles(t *testing.T, payload []byte) []any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatalf("decode assembled payload: %v", err)
	}
	supportingFiles, ok := root["supportingFiles"].(map[string]any)
	if !ok {
		return nil
	}
	files, _ := supportingFiles["bundledFiles"].([]any)
	return files
}

func assertScriptEntry(t *testing.T, entry any, want scriptAsset) {
	t.Helper()
	assertBundledEntry(t, entry, want.targetPath, "SCRIPT", want.content)
}

func assertBundledEntry(t *testing.T, entry any, targetPath, fileType, contentValue string) {
	t.Helper()
	file, ok := entry.(map[string]any)
	if !ok {
		t.Fatalf("bundled file = %#v, want object", entry)
	}
	if got := file["id"]; got != targetPath {
		t.Errorf("id = %#v, want %q", got, targetPath)
	}
	if got := file["type"]; got != fileType {
		t.Errorf("type = %#v, want %s", got, fileType)
	}
	if got := file["targetPath"]; got != targetPath {
		t.Errorf("targetPath = %#v, want %q", got, targetPath)
	}
	content, ok := file["content"].(map[string]any)
	if !ok {
		t.Fatalf("content = %#v, want object", file["content"])
	}
	if got := content["encoding"]; got != "utf-8" {
		t.Errorf("encoding = %#v, want utf-8", got)
	}
	if got := content["inline"]; got != contentValue {
		t.Errorf("inline = %#v, want exact content %q", got, contentValue)
	}
}
