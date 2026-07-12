package defaultpaths

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// RuntimeArtifactKind names a runtime file artifact family placed under a dated
// root directory.
type RuntimeArtifactKind string

const (
	RuntimeArtifactKindLog     RuntimeArtifactKind = "runtime-log"
	RuntimeArtifactKindMetrics RuntimeArtifactKind = "runtime-metrics"

	RuntimeArtifactTimeLayout = "150405.000000000"
	RuntimeArtifactExtension  = ".log"
)

// RuntimeArtifactFilename builds <time>-<kind>[ -<suffix>].<ext> using the
// supplied UTC-normalized start time.
func RuntimeArtifactFilename(at time.Time, kind RuntimeArtifactKind, suffix string) string {
	at = at.UTC()
	filename := fmt.Sprintf("%s-%s", at.Format(RuntimeArtifactTimeLayout), kind)
	if trimmed := strings.TrimSpace(suffix); trimmed != "" {
		filename += "-" + trimmed
	}
	return filename + RuntimeArtifactExtension
}

// RuntimeArtifactPath builds root/YYYY/MM/DD/<time>-<kind>[ -<suffix>].<ext>
// using the supplied UTC-normalized start time.
func RuntimeArtifactPath(rootDir string, at time.Time, kind RuntimeArtifactKind, suffix string) string {
	return filepath.Join(calendarDatedDir(rootDir, at), RuntimeArtifactFilename(at, kind, suffix))
}

// RuntimeArtifactPathComponents joins sanitized path components for use as the
// optional suffix segment after the artifact kind.
func RuntimeArtifactPathComponents(components ...string) string {
	parts := make([]string, 0, len(components))
	for _, component := range components {
		if sanitized := sanitizeRuntimeArtifactPathComponent(component); sanitized != "" {
			parts = append(parts, sanitized)
		}
	}
	return strings.Join(parts, "-")
}

func sanitizeRuntimeArtifactPathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(value))
	lastUnderscore := false
	for _, r := range value {
		if r == '-' || r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	trimmed := strings.Trim(b.String(), "_.-")
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}
