package factorysessions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestSessionErrorsMatchStableBoundarySentinels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		domain error
		legacy error
	}{
		{name: "not found", domain: ErrNotFound, legacy: errors.New("factory session not found")},
		{name: "result unavailable", domain: ErrResultUnavailable, legacy: errors.New("factory session result unavailable")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if !errors.Is(fmt.Errorf("read session: %w", test.domain), test.legacy) {
				t.Fatalf("wrapped %v did not match stable boundary sentinel %v", test.domain, test.legacy)
			}
		})
	}
}

func TestValidateInitNewFactoryNestedDir_AllowsMissingNestedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := ValidateInitNewFactoryNestedDir(root); err != nil {
		t.Fatalf("ValidateInitNewFactoryNestedDir(missing) = %v, want nil", err)
	}
}

func TestValidateInitNewFactoryNestedDir_AllowsEmptyNestedDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, interfaces.FactoryDir)
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("Mkdir(nested): %v", err)
	}
	if err := ValidateInitNewFactoryNestedDir(root); err != nil {
		t.Fatalf("ValidateInitNewFactoryNestedDir(empty nested) = %v, want nil", err)
	}
}

func TestValidateInitNewFactoryNestedDir_RejectsNestedFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, interfaces.FactoryDir)
	if err := os.WriteFile(nested, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(nested): %v", err)
	}
	err := ValidateInitNewFactoryNestedDir(root)
	if err == nil {
		t.Fatal("ValidateInitNewFactoryNestedDir(file) = nil, want conflict")
	}
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonConflict || field != "folderPath" {
		t.Fatalf("ValidationReasonFromError = (%q, %q, %v), want conflict on folderPath", reason, field, ok)
	}
}

func TestValidateInitNewFactoryNestedDir_RejectsPopulatedNestedDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, interfaces.FactoryDir)
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll(nested): %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "notes.txt"), []byte("existing notes\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes): %v", err)
	}
	err := ValidateInitNewFactoryNestedDir(root)
	if err == nil {
		t.Fatal("ValidateInitNewFactoryNestedDir(populated) = nil, want conflict")
	}
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonConflict || field != "folderPath" {
		t.Fatalf("ValidationReasonFromError = (%q, %q, %v), want conflict on folderPath", reason, field, ok)
	}
}

func TestNewSessionResponseEventStoreAlias(t *testing.T) {
	t.Parallel()

	store := NewSessionResponseEventStore("session-alias")
	if store == nil {
		t.Fatal("NewSessionResponseEventStore returned nil")
	}
	if got := store.FactorySessionID(); got != "session-alias" {
		t.Fatalf("FactorySessionID() = %q, want session-alias", got)
	}
}

func TestNewLiveSessionOwnsCanonicalResponseEventStore(t *testing.T) {
	t.Parallel()

	session := NewLiveSession(
		DefaultSessionID,
		"/factories/default",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
	)
	if session.ResponseEvents == nil {
		t.Fatal("ResponseEvents = nil, want session-owned store")
	}
	if got := session.ResponseEvents.FactorySessionID(); got != CanonicalFactorySessionID(session) {
		t.Fatalf("response event store session ID = %q, want %q", got, CanonicalFactorySessionID(session))
	}
}

func TestNewLiveSessionDefaultUUIDKeepsRegistryIdentity(t *testing.T) {
	t.Parallel()

	sessionID := NewSessionID()
	session := NewLiveSession(
		sessionID,
		"/factories/default",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
	)
	if got := CanonicalFactorySessionID(session); got != sessionID {
		t.Fatalf("canonical session ID = %q, want registry UUID %q", got, sessionID)
	}
	if got := session.ResponseEvents.FactorySessionID(); got != sessionID {
		t.Fatalf("response event store session ID = %q, want registry UUID %q", got, sessionID)
	}
}

func TestBindResponseEventCompletion_UsesCanonicalFactoryEventTypes(t *testing.T) {
	t.Parallel()

	session := NewLiveSession(
		"session-completion",
		"/factories/default",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
	)
	var recorder func(interfaces.FactoryEventType)
	BindResponseEventCompletion(session, func(bound func(interfaces.FactoryEventType)) {
		recorder = bound
	})
	if recorder == nil {
		t.Fatal("completion recorder = nil, want canonical Factory event callback")
	}

	recorder(interfaces.FactoryEventTypeSessionResultUpdated)
	if session.ResponseEvents.Completed() {
		t.Fatal("response events completed for non-terminal Factory event")
	}
	recorder(interfaces.FactoryEventTypeSessionCompleted)
	if !session.ResponseEvents.Completed() {
		t.Fatal("response events remain live after SESSION_COMPLETED")
	}
}
