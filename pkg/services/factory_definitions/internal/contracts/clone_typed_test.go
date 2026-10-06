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

func TestWorkstationClonePreservesTextAndDetachesMutableFields(t *testing.T) {
	t.Parallel()
	cloners := map[string]func(*testing.T, FactoryWorkstationConfig) FactoryWorkstationConfig{
		"workstation": func(_ *testing.T, source FactoryWorkstationConfig) FactoryWorkstationConfig {
			return CloneWorkstationConfig(source)
		},
		"factory": func(t *testing.T, source FactoryWorkstationConfig) FactoryWorkstationConfig {
			cloned, err := CloneFactoryConfig(&FactoryConfig{Workstations: []FactoryWorkstationConfig{source}})
			if err != nil {
				t.Fatal(err)
			}
			return cloned.Workstations[0]
		},
	}
	for name, clone := range cloners {
		t.Run(name, func(t *testing.T) {
			for _, text := range []struct{ source, expected string }{
				{"prompt <tag>\n漢字\x00", "prompt <tag>\n漢字\x00"},
				{"prompt \xff", "prompt \ufffd"},
			} {
				source := FactoryWorkstationConfig{
					Body: text.source, PromptTemplate: text.source,
					PromptSourcePath: "workstations/run/AGENTS.md", PromptSourceIsTemplate: true,
					Description: &NameValueConfig{Values: map[string]string{"en": "original"}},
					Env:         map[string]string{"CONFIG": "original"}, StopWords: []string{"original"},
					Inputs: []IOConfig{{Guard: &InputGuardConfig{MatchInput: "original"}}},
				}
				cloned := clone(t, source)
				if cloned.Body != text.expected || cloned.PromptTemplate != text.expected ||
					cloned.PromptSourcePath != source.PromptSourcePath || !cloned.PromptSourceIsTemplate {
					t.Fatal("clone changed prompt text normalization or runtime source metadata")
				}
				cloned.Description.Values["en"] = "changed"
				cloned.Env["CONFIG"] = "changed"
				cloned.StopWords[0] = "changed"
				cloned.Inputs[0].Guard.MatchInput = "changed"
				if source.Description.Values["en"] != "original" || source.Env["CONFIG"] != "original" ||
					source.StopWords[0] != "original" || source.Inputs[0].Guard.MatchInput != "original" {
					t.Fatal("clone shares mutable workstation fields")
				}
			}
		})
	}
}
