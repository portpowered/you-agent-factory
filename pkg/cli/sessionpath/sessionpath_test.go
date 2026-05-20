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
			name:   "default legacy path without explicit session",
			legacy: "/work",
			want:   "/work",
		},
		{
			name:      "default legacy path with explicit default session",
			legacy:    "/work",
			sessionID: DefaultFactorySessionID,
			want:      "/work",
		},
		{
			name:      "non default session scopes path",
			legacy:    "/work",
			sessionID: "session-beta",
			want:      "/factories/session-beta/work",
		},
		{
			name:      "session id is path escaped",
			legacy:    "/work",
			sessionID: "session/beta",
			want:      "/factories/session%2Fbeta/work",
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
