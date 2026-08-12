package runtimeopening

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestDefinitionHostHandoffRemainsLazyUntilBound(t *testing.T) {
	t.Parallel()

	handoff := &definitionHostHandoff{}
	if got := handoff.PersistRootDir(); got != "" {
		t.Fatalf("PersistRootDir() before bind = %q, want empty", got)
	}
	if _, err := handoff.RequireSession("session"); err == nil {
		t.Fatal("RequireSession() before bind returned nil error")
	}

	handoff.bind(definitionHostHandoffStub{root: "factory-root"})
	if got := handoff.PersistRootDir(); got != "factory-root" {
		t.Fatalf("PersistRootDir() after bind = %q, want factory-root", got)
	}
}

func TestDefinitionActivationHandoffDelegatesAfterBind(t *testing.T) {
	t.Parallel()

	handoff := &definitionActivationHandoff{}
	if err := handoff.WithActivationLock(func() error { return nil }); err == nil {
		t.Fatal("WithActivationLock() before bind returned nil error")
	}
	target := definitionActivationHandoffStub{id: "session"}
	handoff.bind(target)
	if got := handoff.RunSessionID(); got != target.id {
		t.Fatalf("RunSessionID() = %q, want %q", got, target.id)
	}
	called := false
	if err := handoff.WithActivationLock(func() error { called = true; return nil }); err != nil {
		t.Fatalf("WithActivationLock() error = %v", err)
	}
	if !called {
		t.Fatal("WithActivationLock() did not delegate callback")
	}
}

type definitionHostHandoffStub struct {
	root string
}

func (s definitionHostHandoffStub) PersistRootDir() string { return s.root }
func (definitionHostHandoffStub) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (definitionHostHandoffStub) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	return nil
}
func (definitionHostHandoffStub) WorkflowID() string { return "" }
func (definitionHostHandoffStub) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return nil, nil
}
func (definitionHostHandoffStub) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	return nil, nil
}
func (definitionHostHandoffStub) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}
func (definitionHostHandoffStub) ValidateEditableFactorySnapshot(context.Context, *factorydefinitions.FactorySnapshot) error {
	return nil
}
func (definitionHostHandoffStub) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return nil, nil
}
func (definitionHostHandoffStub) ReplaceFactoryLayoutAtDir(string, *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

type definitionActivationHandoffStub struct {
	id string
}

func (s definitionActivationHandoffStub) RunSessionID() string { return s.id }
func (definitionActivationHandoffStub) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return nil
}
func (definitionActivationHandoffStub) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return nil, nil
}
func (definitionActivationHandoffStub) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}
func (definitionActivationHandoffStub) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return "", ""
}
func (definitionActivationHandoffStub) SaveNow() time.Time                       { return time.Time{} }
func (definitionActivationHandoffStub) WithActivationLock(fn func() error) error { return fn() }
func (definitionActivationHandoffStub) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}
func (definitionActivationHandoffStub) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorydefinitions.DefinitionSession) error {
	return nil
}
func (definitionActivationHandoffStub) ActivateSessionEditableFactory(context.Context, *factorydefinitions.DefinitionSession, string, string, string, string, string) error {
	return nil
}
func (definitionActivationHandoffStub) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorydefinitions.DefinitionSession, string, string, string, string) error {
	return nil
}
