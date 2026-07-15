package factorysessions

import (
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestListSummaries_OrdersDefaultSessionFirst(t *testing.T) {
	registry := NewRegistry()
	registry.Upsert(NewLiveSession(
		"session-b",
		"/factories/b",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindNamed, Name: "b"},
		nil,
		false,
		"b",
	), true)
	registry.Upsert(NewLiveSession(
		DefaultSessionID,
		"/factories/default",
		"/workspace",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
	), false)

	summaries := ListSummaries(registry)
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}
	if !summaries[0].IsDefault || summaries[0].Id != CanonicalFactorySessionID(registry.DefaultSession()) {
		t.Fatalf("first summary = %#v, want default session first", summaries[0])
	}
}

func TestSummaryResponse_MapsLiveSessionFields(t *testing.T) {
	name := "beta"
	summary := SummaryResponse(&LiveSession{
		ID: "session-1",
		SessionState: SessionState{
			FactoryDir: "/factories/beta",
			FolderPath: "/workspace",
		},
		IsDefault: false,
		Project:   "beta-project",
		Target:    TargetRef{Kind: TargetKindNamed, Name: name},
	})
	if summary.Id != "session-1" || summary.Project != "beta-project" {
		t.Fatalf("summary = %#v, want mapped session fields", summary)
	}
	if summary.Target.Kind != factoryapi.FactorySessionTargetRefKindNamed || summary.Target.Name == nil || *summary.Target.Name != name {
		t.Fatalf("summary target = %#v, want named beta target", summary.Target)
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
