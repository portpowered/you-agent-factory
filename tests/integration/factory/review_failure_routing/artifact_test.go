package review_failure_routing

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	prebuiltArtifactEnv        = "INFINITE_YOU_PREBUILT_ARTIFACT"
	requirePrebuiltArtifactEnv = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	factoryMainPackage         = "github.com/portpowered/infinite-you/cmd/factory"
)

type prebuiltArtifact struct {
	path   string
	size   int64
	sha256 string
	build  *buildinfo.BuildInfo
}

var routingArtifact prebuiltArtifact

// TestMain validates the externally supplied CLI once. The integration test
// never builds, copies, or replaces the executable; an ordinary local package
// run skips until its caller supplies the same artifact that CI prebuilds.
func TestMain(m *testing.M) {
	path := strings.TrimSpace(os.Getenv(prebuiltArtifactEnv))
	required := prebuiltArtifactRequired(os.Getenv(requirePrebuiltArtifactEnv))
	if path == "" {
		if required {
			fmt.Fprintf(os.Stderr, "ROUTING-ARTIFACT failed: %s is required\n", prebuiltArtifactEnv)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "ROUTING-ARTIFACT status=optional-skip env=%s\n", prebuiltArtifactEnv)
		os.Exit(m.Run())
	}
	artifact, err := loadPrebuiltArtifact(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ROUTING-ARTIFACT failed: %v\n", err)
		os.Exit(1)
	}
	routingArtifact = artifact
	fmt.Fprintf(os.Stderr, "ROUTING-ARTIFACT status=validated path=%q size=%d sha256=%s source=%q revision=%q platform=%s/%s\n",
		artifact.path, artifact.size, artifact.sha256, artifact.build.Path,
		buildSetting(artifact.build, "vcs.revision"), buildSetting(artifact.build, "GOOS"), buildSetting(artifact.build, "GOARCH"))
	os.Exit(m.Run())
}

func prebuiltArtifactRequired(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func loadPrebuiltArtifact(path string) (prebuiltArtifact, error) {
	if !filepath.IsAbs(path) {
		return prebuiltArtifact{}, fmt.Errorf("%s must be absolute, got %q", prebuiltArtifactEnv, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return prebuiltArtifact{}, fmt.Errorf("stat %s: %w", prebuiltArtifactEnv, err)
	}
	if !info.Mode().IsRegular() {
		return prebuiltArtifact{}, fmt.Errorf("%s must name a regular file, got %q", prebuiltArtifactEnv, info.Mode())
	}
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return prebuiltArtifact{}, fmt.Errorf("%s must use .exe on Windows, got %q", prebuiltArtifactEnv, path)
		}
	} else if info.Mode().Perm()&0o111 == 0 {
		return prebuiltArtifact{}, fmt.Errorf("%s must be executable, mode=%#o", prebuiltArtifactEnv, info.Mode().Perm())
	}

	file, err := os.Open(path)
	if err != nil {
		return prebuiltArtifact{}, fmt.Errorf("open %s: %w", prebuiltArtifactEnv, err)
	}
	hasher := sha256.New()
	_, copyErr := io.Copy(hasher, file)
	closeErr := file.Close()
	if copyErr != nil {
		return prebuiltArtifact{}, fmt.Errorf("hash %s: %w", prebuiltArtifactEnv, copyErr)
	}
	if closeErr != nil {
		return prebuiltArtifact{}, fmt.Errorf("close %s: %w", prebuiltArtifactEnv, closeErr)
	}
	if observed := hex.EncodeToString(hasher.Sum(nil)); observed == "" {
		return prebuiltArtifact{}, fmt.Errorf("hash %s was empty", prebuiltArtifactEnv)
	} else {
		build, buildErr := buildinfo.ReadFile(path)
		if buildErr != nil {
			return prebuiltArtifact{}, fmt.Errorf("read Go build identity for %s: %w", prebuiltArtifactEnv, buildErr)
		}
		if build.Path != factoryMainPackage {
			return prebuiltArtifact{}, fmt.Errorf("Go main package = %q, want %q", build.Path, factoryMainPackage)
		}
		return prebuiltArtifact{path: filepath.Clean(path), size: info.Size(), sha256: observed, build: build}, nil
	}
}

func buildSetting(build *buildinfo.BuildInfo, key string) string {
	if build == nil {
		return ""
	}
	for _, setting := range build.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

func requireRoutingArtifact(t *testing.T) prebuiltArtifact {
	t.Helper()
	if routingArtifact.path == "" {
		t.Skipf("compiled child routing requires %s; use the externally prebuilt integration artifact", prebuiltArtifactEnv)
	}
	return routingArtifact
}
