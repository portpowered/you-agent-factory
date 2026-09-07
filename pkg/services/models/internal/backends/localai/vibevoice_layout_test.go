package localai

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	modelartifacts "github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
)

func TestConfinedVibeVoiceRolePathsAcceptsExactManifestLayout(t *testing.T) {
	t.Parallel()

	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	definition, ok := manifest.Model("tts")
	if !ok {
		t.Fatal("TTS role definition is missing")
	}
	root := t.TempDir()
	modelFile := filepath.Join(root, definition.Artifacts[0].Path)
	absoluteFiles := make([]string, 0, len(definition.Artifacts))
	relativeFiles := make([]string, 0, len(definition.Artifacts))
	for _, artifact := range definition.Artifacts {
		path := filepath.Join(root, artifact.Path)
		if err := os.WriteFile(path, []byte(artifact.Role), 0o600); err != nil {
			t.Fatalf("write %s fixture: %v", artifact.Role, err)
		}
		absoluteFiles = append(absoluteFiles, path)
		relativeFiles = append(relativeFiles, artifact.Path)
	}

	for _, testCase := range []struct {
		name  string
		files []string
	}{
		{name: "absolute entries", files: absoluteFiles},
		{name: "relative entries", files: relativeFiles},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			paths, err := confinedVibeVoiceRolePaths(modelFile, testCase.files, definition, filepath.EvalSymlinks)
			if err != nil {
				t.Fatalf("confinedVibeVoiceRolePaths: %v", err)
			}
			want := map[string]string{}
			for _, artifact := range definition.Artifacts {
				want[artifact.Role] = filepath.Join(root, artifact.Path)
			}
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("role paths = %#v, want %#v", paths, want)
			}
		})
	}
}

func TestConfinedVibeVoiceRolePathsRejectsMissingDuplicateMismatchAndTraversal(t *testing.T) {
	t.Parallel()

	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	definition, ok := manifest.Model("tts")
	if !ok {
		t.Fatal("TTS role definition is missing")
	}
	root := t.TempDir()
	modelFile := filepath.Join(root, definition.Artifacts[0].Path)
	validFiles := []string{
		modelFile,
		filepath.Join(root, definition.Artifacts[1].Path),
		filepath.Join(root, definition.Artifacts[2].Path),
	}
	tests := []struct {
		name  string
		files []string
	}{
		{name: "missing", files: validFiles[:2]},
		{name: "duplicate", files: []string{validFiles[0], validFiles[1], validFiles[1]}},
		{name: "mismatched role", files: []string{validFiles[0], validFiles[1], filepath.Join(root, "voice.bin")}},
		{name: "traversal", files: []string{validFiles[0], validFiles[1], filepath.Join(root, "..", definition.Artifacts[2].Path)}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			_, err := confinedVibeVoiceRolePaths(modelFile, testCase.files, definition, nil)
			if !errors.Is(err, errVibeVoiceLayout) {
				t.Fatalf("confinedVibeVoiceRolePaths error = %v, want typed layout error", err)
			}
		})
	}
}

func TestConfinedVibeVoiceRolePathsRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	definition, ok := manifest.Model("tts")
	if !ok {
		t.Fatal("TTS role definition is missing")
	}
	root := t.TempDir()
	outside := t.TempDir()
	modelFile := filepath.Join(root, definition.Artifacts[0].Path)
	tokenizerFile := filepath.Join(root, definition.Artifacts[1].Path)
	voiceFile := filepath.Join(root, definition.Artifacts[2].Path)
	if err := os.WriteFile(modelFile, []byte("model"), 0o600); err != nil {
		t.Fatalf("write model fixture: %v", err)
	}
	if err := os.WriteFile(tokenizerFile, []byte("tokenizer"), 0o600); err != nil {
		t.Fatalf("write tokenizer fixture: %v", err)
	}
	outsideVoice := filepath.Join(outside, definition.Artifacts[2].Path)
	if err := os.WriteFile(outsideVoice, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write outside voice fixture: %v", err)
	}
	if err := os.Symlink(outsideVoice, voiceFile); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	_, err = confinedVibeVoiceRolePaths(modelFile, []string{modelFile, tokenizerFile, voiceFile}, definition, filepath.EvalSymlinks)
	if !errors.Is(err, errVibeVoiceLayout) {
		t.Fatalf("confinedVibeVoiceRolePaths error = %v, want symlink escape rejection", err)
	}
}

func TestConfinedVibeVoiceRolePathsRejectsInjectedResolvedEscape(t *testing.T) {
	t.Parallel()

	manifest, err := modelartifacts.DefaultModelRoleManifest()
	if err != nil {
		t.Fatalf("DefaultModelRoleManifest: %v", err)
	}
	definition, ok := manifest.Model("tts")
	if !ok {
		t.Fatal("TTS role definition is missing")
	}
	root := t.TempDir()
	modelFile := filepath.Join(root, definition.Artifacts[0].Path)
	tokenizerFile := filepath.Join(root, definition.Artifacts[1].Path)
	voiceFile := filepath.Join(root, definition.Artifacts[2].Path)
	for _, path := range []string{modelFile, tokenizerFile, voiceFile} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
	outside := filepath.Join(filepath.Dir(root), "outside", definition.Artifacts[1].Path)
	resolve := func(path string) (string, error) {
		if filepath.Clean(path) == filepath.Clean(tokenizerFile) {
			return outside, nil
		}
		return path, nil
	}

	_, err = confinedVibeVoiceRolePaths(modelFile, []string{modelFile, tokenizerFile, voiceFile}, definition, resolve)
	if !errors.Is(err, errVibeVoiceLayout) {
		t.Fatalf("confinedVibeVoiceRolePaths error = %v, want injected resolved escape rejection", err)
	}
}
