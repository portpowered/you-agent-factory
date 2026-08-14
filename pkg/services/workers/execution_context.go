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

// Clone returns a detached copy of the execution context. Context values are
// request data at the Workers boundary; callers must not be able to mutate a
// running attempt or another attempt through the EnvVars map.
func (context *Context) Clone() *Context {
	if context == nil {
		return nil
	}
	clone := *context
	if len(context.EnvVars) > 0 {
		clone.EnvVars = make(map[string]string, len(context.EnvVars))
		for key, value := range context.EnvVars {
			clone.EnvVars[key] = value
		}
	} else {
		clone.EnvVars = nil
	}
	return &clone
}

// ResolveProjectID normalizes an explicit project or returns the neutral
// Worker execution default.
func ResolveProjectID(explicit string) string {
	if project := strings.TrimSpace(explicit); project != "" {
		return project
	}
	return DefaultProjectID
}
