package providers_test

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// TestRootContractInvariants_AllSlicesThroughSingularService seals the
// Providers root-contract packet: every published slice (catalog list/get and
// one-attempt Execute) is reachable through one named providers.Service, a
// peer-shaped fake can exercise success and typed-failure paths using only the
// root package, and no second peer-facing Providers authority is required.
func TestRootContractInvariants_AllSlicesThroughSingularService(t *testing.T) {
	t.Parallel()

	codex := providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}
	service := newExecutePeerFake("cancel-attempt", codex)
	var root providers.Service = service

	assertSealCatalogSuccess(t, root)
	assertSealCatalogFailures(t, root, codex)
	assertSealExecuteSuccess(t, root)
	assertSealExecuteFailures(t, root)
}

func assertSealCatalogSuccess(t *testing.T, service providers.Service) {
	t.Helper()

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) != 1 || list.Providers[0].ID != providers.IDCodex {
		t.Fatalf("ListProviders() = %#v, want one codex descriptor", list.Providers)
	}

	got, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.DisplayName != "Codex" ||
		got.Provider.Availability != providers.AvailabilitySelectable {
		t.Fatalf("GetProvider(codex) = %#v", got.Provider)
	}
}

func assertSealCatalogFailures(t *testing.T, service providers.Service, codex providers.Descriptor) {
	t.Helper()

	assertGetErrorIs(t, service, providers.GetProviderRequest{}, providers.ErrInvalidID)
	assertGetErrorIs(t, service, providers.GetProviderRequest{ID: providers.IDClaude}, providers.ErrUnknownProvider)

	unavailable := providers.Descriptor{
		ID:           providers.IDCursor,
		DisplayName:  "Cursor",
		Availability: providers.AvailabilitySupportedButUnavailable,
		Readiness:    providers.ReadinessUnavailable,
		Prerequisites: []providers.Prerequisite{{
			Kind:   providers.PrerequisiteDependency,
			Name:   "cursor-agent",
			Status: providers.PrerequisiteMissing,
		}},
	}
	blocked := newExecutePeerFake("cancel-attempt", codex, unavailable)
	assertGetErrorIs(
		t,
		blocked,
		providers.GetProviderRequest{ID: providers.IDCursor},
		providers.ErrProviderUnavailable,
	)
}

func assertSealExecuteSuccess(t *testing.T, service providers.Service) {
	t.Helper()

	result, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "seal-attempt",
		UserMessage: "hello seal",
	})
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if result.Content != "hello seal-result" {
		t.Fatalf("Execute() content = %q, want hello seal-result", result.Content)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "session-seal-attempt" {
		t.Fatalf("Execute() SessionRef = %#v", result.SessionRef)
	}
}

func assertSealExecuteFailures(t *testing.T, service providers.Service) {
	t.Helper()

	_, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "cancel-attempt",
	})
	if !errors.Is(err, providers.ErrExecuteCancelled) {
		t.Fatalf("cancelled Execute() error = %v, want ErrExecuteCancelled", err)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("cancelled Execute() error = %T(%v), want ExecuteFailure", err, err)
	}

	_, err = service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDClaude,
		AttemptID: "seal-unknown",
	})
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("unknown provider Execute() error = %v, want ErrUnknownProvider", err)
	}
}

func TestRootContract_ContractValuesStayInertWhenHeld(t *testing.T) {
	t.Parallel()

	descriptor := providers.Descriptor{
		ID:          providers.IDCodex,
		Aliases:     []string{"openai-codex"},
		DisplayName: "Codex",
	}
	clonedDescriptor := descriptor.Clone()
	descriptor.DisplayName = "mutated"
	if clonedDescriptor.DisplayName == "mutated" {
		t.Fatal("Descriptor.Clone() shares mutable display name state")
	}

	request := providers.ExecuteRequest{
		Provider:           providers.IDCodex,
		AttemptID:          "inert-attempt",
		UserMessage:        "hello",
		EnvVars:            map[string]string{"FIXTURE": "original"},
		ProcessEnvironment: []string{"FIXTURE=original"},
	}
	clonedRequest := request.Clone()
	request.UserMessage = "mutated"
	request.EnvVars["FIXTURE"] = "mutated"
	request.ProcessEnvironment[0] = "FIXTURE=mutated"
	if clonedRequest.UserMessage == "mutated" {
		t.Fatal("ExecuteRequest.Clone() shares mutable user message state")
	}
	if clonedRequest.EnvVars["FIXTURE"] == "mutated" {
		t.Fatal("ExecuteRequest.Clone() shares mutable env vars state")
	}
	if clonedRequest.ProcessEnvironment[0] == "FIXTURE=mutated" {
		t.Fatal("ExecuteRequest.Clone() shares mutable process environment state")
	}

	session := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "session-inert",
	}
	if err := session.Validate(); err != nil {
		t.Fatalf("SessionRef.Validate() = %v", err)
	}
	if cloned := session.Clone(); cloned != session {
		t.Fatalf("SessionRef.Clone() = %#v, want %#v", cloned, session)
	}

	// Holding contract values must not require a Service implementation or
	// perform adapter/registry/conductor/process work.
	var (
		_ providers.Descriptor     = descriptor
		_ providers.ExecuteRequest = request
		_ providers.SessionRef     = session
		_ providers.ListProvidersRequest
		_ providers.GetProviderRequest
		_ providers.ExecuteResult
	)
}

func TestRootContract_FakePeerConstructionIsInert(t *testing.T) {
	t.Parallel()

	fake := newExecutePeerFake("cancel-attempt")
	if fake.providers == nil {
		t.Fatal("fake peer construction returned nil catalog map")
	}
	if len(fake.providers) != 0 {
		t.Fatalf("fake peer construction initialized catalog entries = %d, want 0", len(fake.providers))
	}

	var service providers.Service = fake
	if service == nil {
		t.Fatal("constructed Service is nil")
	}
}

func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	serviceRoot := filepath.Join(providersRepositoryRoot(t), "pkg", "services", "providers")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}
	var got []string
	for _, entry := range entries {
		if entry.IsDir() {
			got = append(got, entry.Name())
		}
	}
	slices.Sort(got)
	if want := []string{"internal", "transports", "wire"}; !slices.Equal(got, want) {
		t.Fatalf("Providers root directories = %v, want %v", got, want)
	}

	for _, forbidden := range []string{"catalog", "execution", "inference", "service", "services"} {
		path := filepath.Join(serviceRoot, forbidden)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("pkg/services/providers/%s must not exist as a public sibling", forbidden)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s/ = %v", forbidden, err)
		}
	}
}

func TestProvidersRootContractInventorySeal(t *testing.T) {
	t.Parallel()

	if err := ownershipinventory.VerifyProvidersRootContractInventory(providersRepositoryRoot(t)); err != nil {
		t.Fatalf("VerifyProvidersRootContractInventory() error = %v", err)
	}
}

func TestProvidersRootServiceInterfaceCountAndSelectionMethods(t *testing.T) {
	t.Parallel()

	serviceRoot := filepath.Join(providersRepositoryRoot(t), "pkg", "services", "providers")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}
	var serviceInterfaces []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(serviceRoot, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%q) = %v", path, err)
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != "Service" {
					continue
				}
				if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
					serviceInterfaces = append(serviceInterfaces, entry.Name()+":Service")
				}
			}
		}
	}
	slices.Sort(serviceInterfaces)
	if want := []string{"service_contract.go:Service"}; !slices.Equal(serviceInterfaces, want) {
		t.Fatalf("Providers root Service interfaces = %v, want %v", serviceInterfaces, want)
	}

	rootType := reflect.TypeOf((*providers.Service)(nil)).Elem()
	wantMethods := []string{
		"Execute",
		"GetProvider",
		"ListProviders",
		"ResolveIdentity",
		"ResolveSelection",
		"ValidatePrerequisites",
	}
	gotMethods := make([]string, rootType.NumMethod())
	for index := range gotMethods {
		gotMethods[index] = rootType.Method(index).Name
	}
	if !slices.Equal(gotMethods, wantMethods) {
		t.Fatalf("Providers Service methods = %v, want %v", gotMethods, wantMethods)
	}
}

func TestSelectionContractDeletesFloatingOperationsAndBaselines(t *testing.T) {
	t.Parallel()

	selectionPath := filepath.Join(
		providersRepositoryRoot(t),
		"pkg",
		"services",
		"providers",
		"selection_contract.go",
	)
	file, err := parser.ParseFile(token.NewFileSet(), selectionPath, nil, 0)
	if err != nil {
		t.Fatalf("ParseFile(%q) = %v", selectionPath, err)
	}
	forbidden := map[string]struct{}{
		"ResolveIdentity":       {},
		"ResolveSelection":      {},
		"ValidatePrerequisites": {},
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil {
			continue
		}
		if _, forbidden := forbidden[function.Name.Name]; forbidden {
			t.Fatalf("selection_contract.go still declares floating operation %q", function.Name.Name)
		}
	}

	baselinePath := filepath.Join(
		providersRepositoryRoot(t),
		"docs",
		"internal",
		"baselines",
		"package-structure-baseline.json",
	)
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", baselinePath, err)
	}
	var baseline struct {
		Entries []struct {
			Rule     string `json:"rule"`
			FilePath string `json:"filePath"`
			Target   string `json:"target"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatalf("Unmarshal(%q) = %v", baselinePath, err)
	}
	for _, entry := range baseline.Entries {
		if entry.Rule != "service-root-exported-function" ||
			entry.FilePath != "pkg/services/providers/selection_contract.go" {
			continue
		}
		if _, forbidden := forbidden[entry.Target]; forbidden {
			t.Fatalf("baseline still records deleted floating operation %q", entry.Target)
		}
	}
}

func providersRepositoryRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
