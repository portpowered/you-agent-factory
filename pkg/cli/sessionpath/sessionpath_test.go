package sessionpath

import "testing"

func TestScopedPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		legacy    string
		sessionID string
		want      string
	}{
		{
			name:   "default path is always explicitly scoped",
			legacy: "/work",
			want:   "/factory-sessions/~default/work",
		},
		{
			name:      "explicit default session stays scoped",
			legacy:    "/work",
			sessionID: DefaultFactorySessionID,
			want:      "/factory-sessions/~default/work",
		},
		{
			name:      "non default session scopes path",
			legacy:    "/work",
			sessionID: "session-beta",
			want:      "/factory-sessions/session-beta/work",
		},
		{
			name:      "session id is path escaped",
			legacy:    "/work",
			sessionID: "session/beta",
			want:      "/factory-sessions/session%2Fbeta/work",
		},
		{
			name:      "current factory path maps to canonical session resource",
			legacy:    "/factory/~current",
			sessionID: "session-beta",
			want:      "/factory-sessions/session-beta/factory",
		},
		{
			name:      "editable factory path keeps session root and editing suffix",
			legacy:    "/factory/~current/editable-definition",
			sessionID: "session-beta",
			want:      "/factory-sessions/session-beta/factory/editable-definition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ScopedPath(tt.legacy, tt.sessionID); got != tt.want {
				t.Fatalf("ScopedPath(%q, %q) = %q, want %q", tt.legacy, tt.sessionID, got, tt.want)
			}
		})
	}
}
