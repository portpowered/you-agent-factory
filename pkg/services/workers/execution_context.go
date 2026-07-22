package workers

import "strings"

const (
	ProjectTagKey    = "project"
	DefaultProjectID = "default-project"
	DefaultSessionID = "~default"
)

// Context is the Worker-facing execution environment selected by a Factory
// Runtime or another Worker caller.
type Context struct {
	FactoryDirectory string            `json:"workflow_id"`
	WorkDirectory    string            `json:"work_directory"`
	EnvVars          map[string]string `json:"env_vars"`
	ArtifactDir      string            `json:"artifact_directory"`
	ProjectID        string            `json:"project_id,omitempty"`
	SessionID        string            `json:"session_id,omitempty"`
}

// ResolveProjectID normalizes an explicit project or returns the neutral
// Worker execution default.
func ResolveProjectID(explicit string) string {
	if project := strings.TrimSpace(explicit); project != "" {
		return project
	}
	return DefaultProjectID
}
