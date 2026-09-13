package omni_media_probe

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestManifestFixtureConformance(t *testing.T) {
	manifestPath := checkedInManifestPath(t)
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load checked-in OMNI manifest: %v", err)
	}
	if manifest.SchemaVersion != ManifestSchemaV1 || len(manifest.Artifacts) != 2 {
		t.Fatalf("manifest identity = schema %q artifacts=%d, want schema %q and two artifacts", manifest.SchemaVersion, len(manifest.Artifacts), ManifestSchemaV1)
	}

	want := []struct {
		id        string
		path      string
		bytes     int64
		sha256    string
		source    string
		resolved  string
		mediaType string
	}{
		{id: wantImageID, path: wantImagePath, bytes: wantImageBytes, sha256: wantImageSHA256, source: filepath.Join(repositoryRoot(t), wantImageSource), resolved: filepath.Join(manifest.Root, wantImagePath), mediaType: wantImageMediaType},
		{id: wantVideoID, path: wantVideoPath, bytes: wantVideoBytes, sha256: wantVideoSHA256, source: filepath.Join(repositoryRoot(t), wantVideoSource), resolved: filepath.Join(manifest.Root, wantVideoPath), mediaType: wantVideoMediaType},
	}
	for index, expected := range want {
		artifact := manifest.Artifacts[index]
		if artifact.ID != expected.id || artifact.Path != expected.path || artifact.Bytes != expected.bytes || artifact.SHA256 != expected.sha256 || artifact.MediaType != expected.mediaType {
			t.Errorf("artifact[%d] identity = %#v, want id=%q path=%q bytes=%d sha256=%s mediaType=%s", index, artifact, expected.id, expected.path, expected.bytes, expected.sha256, expected.mediaType)
		}
		if filepath.Clean(artifact.ResolvedPath) != filepath.Clean(expected.resolved) {
			t.Errorf("artifact[%d] resolved path = %q, want %q", index, artifact.ResolvedPath, expected.resolved)
		}
		promoted, err := os.ReadFile(artifact.ResolvedPath)
		if err != nil {
			t.Fatalf("read promoted artifact[%d]: %v", index, err)
		}
		source, err := os.ReadFile(expected.source)
		if err != nil {
			t.Fatalf("read source artifact[%d]: %v", index, err)
		}
		if !bytes.Equal(promoted, source) {
			t.Errorf("artifact[%d] promoted bytes differ from declared source", index)
		}
		t.Logf("artifact[%d] id=%s bytes=%d sha256=%s source=%s promoted=%s", index, artifact.ID, artifact.Bytes, artifact.SHA256, expected.source, artifact.ResolvedPath)
	}
}

func TestManifestRubricConformance(t *testing.T) {
	manifest, err := LoadManifest(checkedInManifestPath(t))
	if err != nil {
		t.Fatalf("load checked-in OMNI manifest: %v", err)
	}

	imageRubric := manifest.Artifacts[0].SemanticRubric.Image
	if imageRubric == nil || manifest.Artifacts[0].SemanticRubric.Video != nil || len(imageRubric.AllOf) != 3 {
		t.Fatalf("image rubric = %#v, want one three-predicate image rubric", manifest.Artifacts[0].SemanticRubric)
	}
	if got := imageRubric.AllOf; got[0].Kind != "symbol" || string(got[0].Exact) != `"infinity"` || got[1].Kind != "text" || string(got[1].Exact) != `"INFINITE YOU"` || got[2].Kind != "accentColors" || string(got[2].Exact) != `["blue", "gold"]` {
		t.Fatalf("image rubric predicates = %#v, want exact symbol/text/accent predicates", got)
	}

	videoRubric := manifest.Artifacts[1].SemanticRubric.Video
	if videoRubric == nil || manifest.Artifacts[1].SemanticRubric.Image != nil || len(videoRubric.OrderedPhases) != 2 || videoRubric.Transition == nil {
		t.Fatalf("video rubric = %#v, want two phases and one transition", manifest.Artifacts[1].SemanticRubric)
	}
	wantPhases := []VideoPhase{
		{Label: "PHASE 1", Background: "red", TextColor: "white", StartMillis: 0, EndMillisExclusive: 2000},
		{Label: "PHASE 2", Background: "blue", TextColor: "yellow", StartMillis: 2000, EndMillisExclusive: 4000},
	}
	for index, phase := range wantPhases {
		if videoRubric.OrderedPhases[index] != phase {
			t.Errorf("video phase[%d] = %#v, want %#v", index, videoRubric.OrderedPhases[index], phase)
		}
	}
	if *videoRubric.Transition != (VideoTransition{Kind: "hardCut", AtMillis: 2000, ToleranceMillis: 40}) {
		t.Fatalf("video transition = %#v, want hard cut at 2000ms with 40ms tolerance", videoRubric.Transition)
	}
}

func TestManifestFixtureNegativeCases(t *testing.T) {
	cases := []struct {
		name   string
		code   ValidationCode
		mutate func(t *testing.T, root, manifestPath string)
	}{
		{
			name: "missing fixture",
			code: CodeMissingFile,
			mutate: func(t *testing.T, root, _ string) {
				if err := os.Remove(filepath.Join(root, wantImagePath)); err != nil {
					t.Fatalf("remove image fixture: %v", err)
				}
			},
		},
		{
			name: "one byte mutation",
			code: CodeHashMismatch,
			mutate: func(t *testing.T, root, _ string) {
				path := filepath.Join(root, wantImagePath)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read image mutation fixture: %v", err)
				}
				data[len(data)-1] ^= 1
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatalf("write image mutation fixture: %v", err)
				}
			},
		},
		{
			name: "metadata drift",
			code: CodeMetadataMismatch,
			mutate: func(t *testing.T, _, manifestPath string) {
				replaceManifestText(t, manifestPath, `"width": 897`, `"width": 898`)
			},
		},
		{
			name: "ambiguous image rubric",
			code: CodeRubricAmbiguous,
			mutate: func(t *testing.T, _, manifestPath string) {
				const textPredicate = `{"kind": "text", "exact": "INFINITE YOU"}`
				replaceManifestText(t, manifestPath, textPredicate, textPredicate+",\n          "+textPredicate)
			},
		},
		{
			name: "missing rubric predicate",
			code: CodeRubricAmbiguous,
			mutate: func(t *testing.T, _, manifestPath string) {
				const textPredicate = `{"kind": "text", "exact": "INFINITE YOU"}`
				replaceManifestText(t, manifestPath, ",\n          "+textPredicate, "")
			},
		},
		{
			name: "missing rubric object",
			code: CodeRubricAmbiguous,
			mutate: func(t *testing.T, _, manifestPath string) {
				removeImageRubric(t, manifestPath)
			},
		},
		{
			name: "invalid phase range",
			code: CodeInvalidPhaseRange,
			mutate: func(t *testing.T, _, manifestPath string) {
				replaceManifestText(t, manifestPath, `"endMillisExclusive": 2000`, `"endMillisExclusive": 1999`)
			},
		},
		{
			name: "path escape",
			code: CodePathEscape,
			mutate: func(t *testing.T, _, manifestPath string) {
				replaceManifestText(t, manifestPath, `"path": "infinite-you.png"`, `"path": "../infinite-you.png"`)
			},
		},
		{
			name: "provenance drift",
			code: CodeProvenanceMismatch,
			mutate: func(t *testing.T, _, manifestPath string) {
				replaceManifestTextAll(t, manifestPath, wantRevision, "0000000000000000000000000000000000000000")
			},
		},
		{
			name: "unknown field",
			code: CodeUnknownField,
			mutate: func(t *testing.T, _, manifestPath string) {
				replaceManifestText(t, manifestPath, `"schemaVersion": "`+ManifestSchemaV1+`",`, "\"schemaVersion\": \""+ManifestSchemaV1+"\",\n  \"unexpected\": true,")
			},
		},
		{
			name: "duplicate JSON key",
			code: CodeDuplicateJSONKey,
			mutate: func(t *testing.T, _, manifestPath string) {
				replaceManifestText(t, manifestPath, `"schemaVersion": "`+ManifestSchemaV1+`",`, "\"schemaVersion\": \""+ManifestSchemaV1+"\",\n  \"schemaVersion\": \""+ManifestSchemaV1+"\",")
			},
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			root, manifestPath := copyBundle(t)
			testCase.mutate(t, root, manifestPath)
			_, err := LoadManifest(manifestPath)
			if err == nil {
				t.Fatal("invalid fixture bundle was accepted")
			}
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want typed ValidationError", err)
			}
			if validation.Code != testCase.code {
				t.Fatalf("validation code = %q, want %q (error=%v)", validation.Code, testCase.code, err)
			}
			t.Logf("negative case=%s code=%s field=%s", testCase.name, validation.Code, validation.Field)
		})
	}
}

func checkedInManifestPath(t testing.TB) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), "tests", "integration", "models", "testdata", "omni_media", "manifest.json")
}

func repositoryRoot(t testing.TB) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate manifest test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", ".."))
}

func copyBundle(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sourceRoot := filepath.Dir(checkedInManifestPath(t))
	for _, name := range []string{"manifest.json", wantImagePath, wantVideoPath} {
		data, err := os.ReadFile(filepath.Join(sourceRoot, name))
		if err != nil {
			t.Fatalf("read checked-in bundle file %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			t.Fatalf("write copied bundle file %s: %v", name, err)
		}
	}
	return root, filepath.Join(root, "manifest.json")
}

func replaceManifestText(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest variant: %v", err)
	}
	if bytes.Count(data, []byte(old)) != 1 {
		t.Fatalf("manifest replacement target %q occurred %d times, want once", old, bytes.Count(data, []byte(old)))
	}
	data = bytes.Replace(data, []byte(old), []byte(replacement), 1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write manifest variant: %v", err)
	}
}

func replaceManifestTextAll(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest variant: %v", err)
	}
	if bytes.Count(data, []byte(old)) == 0 {
		t.Fatalf("manifest replacement target %q was absent", old)
	}
	data = bytes.ReplaceAll(data, []byte(old), []byte(replacement))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write manifest variant: %v", err)
	}
}

func removeImageRubric(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest variant: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode manifest variant: %v", err)
	}
	artifacts, ok := document["artifacts"].([]any)
	if !ok || len(artifacts) == 0 {
		t.Fatal("manifest variant has no artifact array")
	}
	first, ok := artifacts[0].(map[string]any)
	if !ok {
		t.Fatal("manifest variant first artifact is not an object")
	}
	delete(first, "semanticRubric")
	data, err = json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest variant: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write manifest variant: %v", err)
	}
}
