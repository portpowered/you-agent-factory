// Package reference holds canonical CLI reference markdown and the embedded
// filesystem used by the packaged you docs surface.
package reference

import "embed"

// PackagedTopics is the embedded markdown for every topic registered in
// pkg/transports/cli/docs. Authoritative content lives only in this directory.
var (
	//go:embed agents.md authoring-factories.md batch-inputs.md config.md guards.md javascript-workflows.md loop.md mcp.md mock-workers.md models.md orchestrators.md record-replay.md relationships.md resources.md run.md sessions.md templates.md work.md workers.md workstations.md
	PackagedTopics embed.FS
)
