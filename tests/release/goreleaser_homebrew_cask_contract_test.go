package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/testutil"
	"gopkg.in/yaml.v3"
)

type goreleaserConfig struct {
	ProjectName   string               `yaml:"project_name"`
	HomebrewCasks []homebrewCaskConfig `yaml:"homebrew_casks"`
}

type homebrewCaskConfig struct {
	Name        string                  `yaml:"name"`
	IDs         []string                `yaml:"ids"`
	Binaries    []string                `yaml:"binaries"`
	Directory   string                  `yaml:"directory"`
	Caveats     string                  `yaml:"caveats"`
	Homepage    string                  `yaml:"homepage"`
	Description string                  `yaml:"description"`
	URL         homebrewCaskURLConfig   `yaml:"url"`
	Hooks       homebrewCaskHooksConfig `yaml:"hooks"`
	Repository  homebrewCaskRepoConfig  `yaml:"repository"`
}

type homebrewCaskURLConfig struct {
	Template string `yaml:"template"`
	Verified string `yaml:"verified"`
}

type homebrewCaskHooksConfig struct {
	Post homebrewCaskLifecycleConfig `yaml:"post"`
}

type homebrewCaskLifecycleConfig struct {
	Install string `yaml:"install"`
}

type homebrewCaskRepoConfig struct {
	Owner  string `yaml:"owner"`
	Name   string `yaml:"name"`
	Branch string `yaml:"branch"`
	Token  string `yaml:"token"`
}

func TestGoReleaserHomebrewCaskContract_UsesTaggedReleaseAssetsAndTapMetadata(t *testing.T) {
	t.Parallel()

	config := loadGoReleaserConfig(t)
	if config.ProjectName != "agent-factory" {
		t.Fatalf("project_name = %q, want agent-factory", config.ProjectName)
	}
	if len(config.HomebrewCasks) != 1 {
		t.Fatalf("homebrew_casks count = %d, want 1", len(config.HomebrewCasks))
	}

	cask := config.HomebrewCasks[0]
	if cask.Name != "agent-factory" {
		t.Fatalf("cask name = %q, want agent-factory", cask.Name)
	}
	if got := strings.Join(cask.IDs, ","); got != "cli-archives" {
		t.Fatalf("cask ids = %q, want cli-archives", got)
	}
	if got := strings.Join(cask.Binaries, ","); got != "agent-factory" {
		t.Fatalf("cask binaries = %q, want agent-factory", got)
	}
	if cask.Directory != "Casks" {
		t.Fatalf("cask directory = %q, want Casks", cask.Directory)
	}
	if cask.URL.Template != "https://github.com/portpowered/infinite-you/releases/download/{{ .Tag }}/{{ .ArtifactName }}" {
		t.Fatalf("cask URL template = %q, want tagged GitHub release asset template", cask.URL.Template)
	}
	if cask.URL.Verified != "github.com/portpowered/infinite-you/" {
		t.Fatalf("cask verified URL = %q, want github.com/portpowered/infinite-you/", cask.URL.Verified)
	}
	if cask.Repository.Owner != "portpowered" || cask.Repository.Name != "cask" || cask.Repository.Branch != "main" {
		t.Fatalf("cask repository = %#v, want portpowered/cask on main", cask.Repository)
	}
	if cask.Repository.Token != "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}" {
		t.Fatalf("cask repository token = %q, want HOMEBREW_TAP_GITHUB_TOKEN template", cask.Repository.Token)
	}
	if cask.Homepage != "https://github.com/portpowered/infinite-you" {
		t.Fatalf("cask homepage = %q, want repository homepage", cask.Homepage)
	}
	if strings.TrimSpace(cask.Description) == "" {
		t.Fatal("cask description is empty, want consumer-facing description")
	}
}

func TestGoReleaserHomebrewCaskContract_DocumentsUnsignedMacOSHandling(t *testing.T) {
	t.Parallel()

	cask := loadGoReleaserConfig(t).HomebrewCasks[0]

	if !strings.Contains(cask.Hooks.Post.Install, "/usr/bin/xattr") {
		t.Fatalf("post-install hook = %q, want xattr invocation", cask.Hooks.Post.Install)
	}
	if !strings.Contains(cask.Hooks.Post.Install, "com.apple.quarantine") {
		t.Fatalf("post-install hook = %q, want quarantine removal", cask.Hooks.Post.Install)
	}
	if !strings.Contains(cask.Hooks.Post.Install, "#{staged_path}/agent-factory") {
		t.Fatalf("post-install hook = %q, want staged agent-factory binary path", cask.Hooks.Post.Install)
	}
	if !strings.Contains(cask.Caveats, "without Apple code signing or notarization") {
		t.Fatalf("caveats = %q, want unsigned macOS guidance", cask.Caveats)
	}
	if !strings.Contains(cask.Caveats, "xattr -dr com.apple.quarantine") {
		t.Fatalf("caveats = %q, want manual xattr fallback", cask.Caveats)
	}
}

func loadGoReleaserConfig(t *testing.T) goreleaserConfig {
	t.Helper()

	configPath := filepath.Join(testutil.MustRepoRoot(t), ".goreleaser.yml")
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}

	var config goreleaserConfig
	if err := yaml.Unmarshal(contents, &config); err != nil {
		t.Fatalf("unmarshal .goreleaser.yml: %v", err)
	}
	return config
}
