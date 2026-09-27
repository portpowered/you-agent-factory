// Package content provides embedded MCP skills and resources.
package content

import (
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed subagent-configuration/SKILL.md
var skill []byte

//go:embed operator-config.schema.json
var schema []byte

// SkillBytes returns the subagent configuration skill document.
func SkillBytes() []byte { return append([]byte(nil), skill...) }

// SkillFrontmatter returns the metadata authored at the top of SKILL.md.
func SkillFrontmatter() (map[string]any, error) {
	parts := strings.SplitN(string(skill), "---", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
		return nil, fmt.Errorf("subagent skill frontmatter is missing")
	}
	var metadata map[string]any
	if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil {
		return nil, fmt.Errorf("parse subagent skill frontmatter: %w", err)
	}
	return metadata, nil
}

// SchemaBytes returns the operator configuration JSON Schema resource.
func SchemaBytes() []byte { return append([]byte(nil), schema...) }
