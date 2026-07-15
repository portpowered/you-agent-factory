package packageassets

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
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
	file, ok := entry.(map[string]any)
	if !ok {
		t.Fatalf("bundled file = %#v, want object", entry)
	}
	if got := file["id"]; got != want.targetPath {
		t.Errorf("id = %#v, want %q", got, want.targetPath)
	}
	if got := file["type"]; got != "SCRIPT" {
		t.Errorf("type = %#v, want SCRIPT", got)
	}
	if got := file["targetPath"]; got != want.targetPath {
		t.Errorf("targetPath = %#v, want %q", got, want.targetPath)
	}
	content, ok := file["content"].(map[string]any)
	if !ok {
		t.Fatalf("content = %#v, want object", file["content"])
	}
	if got := content["encoding"]; got != "utf-8" {
		t.Errorf("encoding = %#v, want utf-8", got)
	}
	if got := content["inline"]; got != want.content {
		t.Errorf("inline = %#v, want exact content %q", got, want.content)
	}
}
