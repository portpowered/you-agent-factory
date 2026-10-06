package factorycontracts

import (
	"slices"
	"testing"
)

func TestCloneFactoryConfigDetachesWorkersAndPortableFiles(t *testing.T) {
	source := &FactoryConfig{
		Name: "typed-clone",
		Workers: []FactoryWorkerConfig{{
			Name: "worker", Body: "worker prompt", Args: []string{"original"},
			Description:      &NameValueConfig{Values: map[string]string{"en": "original"}},
			Operations:       []ModelOperation{{Inputs: []ModelOperationSlot{{ContentTypes: []string{"TEXT"}}}}},
			PromptSourcePath: "worker/AGENTS.md", SessionID: "runtime-session", Concurrency: 2,
			RuntimeDefaultModelProvider: "codex", RuntimeDefaultModel: "runtime-model",
		}},
		ResourceManifest: &PortableResourceManifestConfig{
			RequiredTools: []RequiredToolConfig{{Name: "git", VersionArgs: []string{"--version"}}},
			BundledFiles:  []BundledFileConfig{{TargetPath: "guide.md", Content: BundledFileContentConfig{Inline: "guide body"}}},
		},
	}
	source.SetIgnoredJSONPaths([]string{"factory.future"})
	cloned, err := CloneFactoryConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	worker := &cloned.Workers[0]
	if worker.PromptSourcePath != source.Workers[0].PromptSourcePath || worker.SessionID != "" || worker.Concurrency != 0 ||
		worker.RuntimeDefaultModelProvider != "" || worker.RuntimeDefaultModel != "" {
		t.Fatalf("Factory clone changed its runtime metadata boundary: %#v", worker)
	}
	worker.Args[0] = "changed"
	worker.Description.Values["en"] = "changed"
	worker.Operations[0].Inputs[0].ContentTypes[0] = "IMAGE"
	worker.Body = "changed"
	cloned.ResourceManifest.RequiredTools[0].VersionArgs[0] = "changed"
	cloned.ResourceManifest.BundledFiles[0].Content.Inline = "changed"
	cloned.SetIgnoredJSONPaths([]string{"factory.changed"})
	got := []string{source.Workers[0].Args[0], source.Workers[0].Description.Values["en"],
		source.Workers[0].Operations[0].Inputs[0].ContentTypes[0], source.Workers[0].Body,
		source.ResourceManifest.RequiredTools[0].VersionArgs[0], source.ResourceManifest.BundledFiles[0].Content.Inline,
		source.IgnoredJSONPaths()[0]}
	want := []string{"original", "original", "TEXT", "worker prompt", "--version", "guide body", "factory.future"}
	if !slices.Equal(got, want) {
		t.Fatal("Factory clone shares mutable Worker, manifest or diagnostic storage")
	}
}
