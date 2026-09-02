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

func TestCurrentFactoryPath(t *testing.T) {
	t.Parallel()

	if got := CurrentFactoryPath(""); got != "/factory-sessions/~default/factory" {
		t.Fatalf("CurrentFactoryPath(\"\") = %q, want /factory-sessions/~default/factory", got)
	}

	if got := CurrentFactoryPath("session/beta"); got != "/factory-sessions/session%2Fbeta/factory" {
		t.Fatalf("CurrentFactoryPath(\"session/beta\") = %q, want /factory-sessions/session%%2Fbeta/factory", got)
	}
}

func TestFactoryInvocationsPath(t *testing.T) {
	t.Parallel()

	if got := FactoryInvocationsPath("session/beta"); got != "/factory-sessions/session%2Fbeta/invocations" {
		t.Fatalf("FactoryInvocationsPath() = %q, want escaped session invocation route", got)
	}
}

func TestWorkPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sessionID  string
		workID     string
		collection string
		item       string
		move       string
	}{
		{
			name:       "omitted session uses default",
			workID:     "work-1",
			collection: "/factory-sessions/~default/work",
			item:       "/factory-sessions/~default/work/work-1",
			move:       "/factory-sessions/~default/work/work-1/move",
		},
		{
			name:       "explicit session",
			sessionID:  "session-beta",
			workID:     "work-2",
			collection: "/factory-sessions/session-beta/work",
			item:       "/factory-sessions/session-beta/work/work-2",
			move:       "/factory-sessions/session-beta/work/work-2/move",
		},
		{
			name:       "identifiers are independently escaped",
			sessionID:  "session/beta",
			workID:     "work/review",
			collection: "/factory-sessions/session%2Fbeta/work",
			item:       "/factory-sessions/session%2Fbeta/work/work%2Freview",
			move:       "/factory-sessions/session%2Fbeta/work/work%2Freview/move",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := WorkCollectionPath(tt.sessionID); got != tt.collection {
				t.Fatalf("WorkCollectionPath(%q) = %q, want %q", tt.sessionID, got, tt.collection)
			}
			if got := WorkItemPath(tt.sessionID, tt.workID); got != tt.item {
				t.Fatalf("WorkItemPath(%q, %q) = %q, want %q", tt.sessionID, tt.workID, got, tt.item)
			}
			if got := WorkMovePath(tt.sessionID, tt.workID); got != tt.move {
				t.Fatalf("WorkMovePath(%q, %q) = %q, want %q", tt.sessionID, tt.workID, got, tt.move)
			}
		})
	}
}
