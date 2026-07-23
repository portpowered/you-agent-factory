package factorydefinitions

import (
	"fmt"
	"path/filepath"
	"strings"
)

type AuthoredFactoryFormat string

const (
	AuthoredFactoryFormatJSON AuthoredFactoryFormat = "JSON"
	AuthoredFactoryFormatYAML AuthoredFactoryFormat = "YAML"
)

const SupportedAuthoredFactoryExtensions = ".json, .yaml, and .yml"

// AuthoredFactoryFormatForPath selects the authored representation solely from
// an explicit source filename. Directory root selection is handled separately.
func AuthoredFactoryFormatForPath(path string) (AuthoredFactoryFormat, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return AuthoredFactoryFormatJSON, nil
	case ".yaml", ".yml":
		return AuthoredFactoryFormatYAML, nil
	default:
		return "", fmt.Errorf(
			"unsupported Factory Definition extension %q; supported extensions are %s",
			filepath.Ext(path),
			SupportedAuthoredFactoryExtensions,
		)
	}
}

// AuthoredFactorySourceLoader resolves an authored Factory Definition path and
// returns JSON-compatible representation bytes. The Factory Definitions
// service owns path, filesystem, and authored-format policy; consumers map the
// bytes through the canonical representation boundary.
type AuthoredFactorySourceLoader func(path string) ([]byte, error)
