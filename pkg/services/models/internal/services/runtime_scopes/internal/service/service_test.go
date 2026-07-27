package service_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestOpenReturnsOpaqueReferenceThatResolvesDetachedBinding(t *testing.T) {
	t.Parallel()

	config := &models.RuntimeConfig{
		FactoryDirectory: "factory",
		BaseDirectory:    "base",
		Workers: []models.RuntimeWorker{{
			Name:      "speaker",
			Args:      []string{"--voice", "original"},
			Resources: []models.RuntimeResource{{ID: "worker-resource", Capacity: 1}},
			Operations: []models.RuntimeOperation{{
				Name: "speak",
				Inputs: []models.RuntimeOperationSlot{{
					Name: "text", ContentTypes: []string{models.RuntimeContentTypeText},
				}},
			}},
		}},
		Resources: []models.RuntimeResource{{ID: "model-resource", Capacity: 2}},
	}
	service := newService(t, "open-resolve")
	ref, err := service.Open(models.RuntimeBinding{
		CacheDirectory: "cache",
		RuntimeConfig:  func() *models.RuntimeConfig { return config },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if ref == "" {
		t.Fatal("Open reference is empty")
	}

	config.FactoryDirectory = "mutated"
	config.Workers[0].Args[1] = "mutated"
	config.Workers[0].Resources[0].Capacity = 99
	config.Workers[0].Operations[0].Inputs[0].ContentTypes[0] = models.RuntimeContentTypeAudio
	config.Resources[0].Capacity = 99

	resolved, err := service.Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertOriginalBinding(t, resolved)

	first := resolved.RuntimeConfig()
	first.Workers[0].Args[1] = "changed-after-resolve"
	first.Resources[0].Capacity = 100
	resolvedAgain, err := service.Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve again: %v", err)
	}
	assertOriginalBinding(t, resolvedAgain)
}

func TestOpenRejectsInvalidBindingWithoutIssuingReference(t *testing.T) {
	t.Parallel()

	service := newService(t, "invalid-binding")
	ref, err := service.Open(models.RuntimeBinding{CacheDirectory: "cache"})
	if !errors.Is(err, models.ErrInvalidRuntimeBinding) {
		t.Fatalf("Open error = %v, want ErrInvalidRuntimeBinding", err)
	}
	if ref != "" {
		t.Fatalf("Open reference = %q, want empty", ref)
	}

	if _, err := service.Resolve(ref); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
		t.Fatalf("Resolve invalid-open reference error = %v, want ErrScopeUnknown", err)
	}
}

func TestConstructionAndOpenOnlySnapshotTheAcceptedBinding(t *testing.T) {
	t.Parallel()

	loaderCalls := 0
	service := newService(t, "inert-construction")
	if service == nil {
		t.Fatal("NewService returned nil")
	}
	if loaderCalls != 0 {
		t.Fatalf("construction called runtime loader %d times", loaderCalls)
	}

	ref, err := service.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			loaderCalls++
			return &models.RuntimeConfig{FactoryDirectory: "factory"}
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if loaderCalls != 1 {
		t.Fatalf("Open called runtime loader %d times, want one snapshot", loaderCalls)
	}
	if _, err := service.Resolve(ref); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if loaderCalls != 1 {
		t.Fatalf("Resolve called caller runtime loader; calls = %d", loaderCalls)
	}
}

func TestCloseInvalidatesOnlyTheSelectedScope(t *testing.T) {
	t.Parallel()

	service := newService(t, "close-isolation")
	firstRef := openBinding(t, service, "first")
	secondRef := openBinding(t, service, "second")

	if err := service.Close(firstRef); err != nil {
		t.Fatalf("Close first scope: %v", err)
	}
	if _, err := service.Resolve(firstRef); !errors.Is(err, runtimescopes.ErrScopeClosed) {
		t.Fatalf("Resolve closed scope error = %v, want ErrScopeClosed", err)
	}
	if err := service.Close(firstRef); !errors.Is(err, runtimescopes.ErrScopeClosed) {
		t.Fatalf("Close closed scope error = %v, want ErrScopeClosed", err)
	}

	resolved, err := service.Resolve(secondRef)
	if err != nil {
		t.Fatalf("Resolve second scope: %v", err)
	}
	if got := resolved.RuntimeConfig().FactoryDirectory; got != "second" {
		t.Fatalf("second scope FactoryDirectory = %q, want second", got)
	}
}

func TestCloseRejectsMalformedAndUnknownReferencesWithoutChangingLiveScopes(t *testing.T) {
	t.Parallel()

	service := newService(t, "unknown-references")
	liveRef := openBinding(t, service, "live")

	for _, ref := range []runtimescopes.Reference{
		"",
		"malformed",
		"00000000000000000000000000000000.00000000000000000000000000000000",
	} {
		if _, err := service.Resolve(ref); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
			t.Errorf("Resolve(%q) error = %v, want ErrScopeUnknown", ref, err)
		}
		if err := service.Close(ref); !errors.Is(err, runtimescopes.ErrScopeUnknown) {
			t.Errorf("Close(%q) error = %v, want ErrScopeUnknown", ref, err)
		}
	}

	resolved, err := service.Resolve(liveRef)
	if err != nil {
		t.Fatalf("Resolve live scope: %v", err)
	}
	if got := resolved.RuntimeConfig().FactoryDirectory; got != "live" {
		t.Fatalf("live scope FactoryDirectory = %q, want live", got)
	}
}

func TestForeignReferenceIsRejectedWithoutAffectingEitherService(t *testing.T) {
	t.Parallel()

	issuer := newService(t, "foreign-issuer")
	receiver := newService(t, "foreign-receiver")
	issuerRef := openBinding(t, issuer, "issuer")
	receiverRef := openBinding(t, receiver, "receiver")

	if _, err := receiver.Resolve(issuerRef); !errors.Is(err, runtimescopes.ErrScopeForeign) {
		t.Fatalf("Resolve foreign scope error = %v, want ErrScopeForeign", err)
	}
	if err := receiver.Close(issuerRef); !errors.Is(err, runtimescopes.ErrScopeForeign) {
		t.Fatalf("Close foreign scope error = %v, want ErrScopeForeign", err)
	}

	assertFactoryDirectory(t, issuer, issuerRef, "issuer")
	assertFactoryDirectory(t, receiver, receiverRef, "receiver")
}

func TestEqualBindingsReceiveDistinctInstanceBoundReferences(t *testing.T) {
	t.Parallel()

	first := newService(t, "equal-first")
	second := newService(t, "equal-second")
	firstRef := openBinding(t, first, "equal")
	secondRef := openBinding(t, second, "equal")

	if firstRef == secondRef {
		t.Fatalf("independent services issued equal references %q", firstRef)
	}
	if _, err := first.Resolve(secondRef); !errors.Is(err, runtimescopes.ErrScopeForeign) {
		t.Fatalf("first Resolve(second reference) error = %v, want ErrScopeForeign", err)
	}
	if _, err := second.Resolve(firstRef); !errors.Is(err, runtimescopes.ErrScopeForeign) {
		t.Fatalf("second Resolve(first reference) error = %v, want ErrScopeForeign", err)
	}

	assertFactoryDirectory(t, first, firstRef, "equal")
	assertFactoryDirectory(t, second, secondRef, "equal")
}

func TestReferenceFromIndependentProcessCannotResolveOrCloseLocalScope(t *testing.T) {
	local := newService(t, "subprocess-parent")
	localRef := openBinding(t, local, "local")

	command := exec.Command(os.Args[0], "-test.run=^TestRuntimeScopesSubprocessHelper$")
	command.Env = append(os.Environ(), "RUNTIME_SCOPES_SUBPROCESS=1")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run Runtime Scopes subprocess helper: %v", err)
	}
	foreignRef := runtimescopes.Reference(strings.TrimSpace(string(output)))
	if foreignRef == "" {
		t.Fatal("subprocess returned an empty reference")
	}
	if foreignRef == localRef {
		t.Fatalf("independent processes issued equal references %q", foreignRef)
	}
	if _, err := local.Resolve(foreignRef); !errors.Is(err, runtimescopes.ErrScopeForeign) {
		t.Fatalf("Resolve subprocess reference error = %v, want ErrScopeForeign", err)
	}
	if err := local.Close(foreignRef); !errors.Is(err, runtimescopes.ErrScopeForeign) {
		t.Fatalf("Close subprocess reference error = %v, want ErrScopeForeign", err)
	}
	assertFactoryDirectory(t, local, localRef, "local")
}

func TestRuntimeScopesSubprocessHelper(t *testing.T) {
	if os.Getenv("RUNTIME_SCOPES_SUBPROCESS") != "1" {
		t.Skip("subprocess helper")
	}
	service := newService(t, "subprocess-child")
	fmt.Print(openBinding(t, service, "foreign"))
	os.Exit(0)
}

func TestConstructionRejectsMissingIssuerIdentity(t *testing.T) {
	if service, err := runtimescopeswire.NewService(nil); err == nil || service != nil {
		t.Fatalf("NewService(nil) = (%v, %v), want nil service and error", service, err)
	}
	if service, err := runtimescopeswire.NewService(func() string { return "  " }); err == nil || service != nil {
		t.Fatalf("NewService(empty identity) = (%v, %v), want nil service and error", service, err)
	}
}

func newService(t *testing.T, issuerID string) runtimescopes.Service {
	t.Helper()
	service, err := runtimescopeswire.NewService(func() string { return issuerID })
	if err != nil {
		t.Fatalf("NewService(%q): %v", issuerID, err)
	}
	return service
}

func openBinding(t *testing.T, service runtimescopes.Service, factoryDirectory string) runtimescopes.Reference {
	t.Helper()
	ref, err := service.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{FactoryDirectory: factoryDirectory}
		},
	})
	if err != nil {
		t.Fatalf("Open %q scope: %v", factoryDirectory, err)
	}
	return ref
}

func assertFactoryDirectory(
	t *testing.T,
	service runtimescopes.Service,
	ref runtimescopes.Reference,
	want string,
) {
	t.Helper()
	binding, err := service.Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve %q scope: %v", want, err)
	}
	if got := binding.RuntimeConfig().FactoryDirectory; got != want {
		t.Fatalf("FactoryDirectory = %q, want %q", got, want)
	}
}

func assertOriginalBinding(t *testing.T, binding models.RuntimeBinding) {
	t.Helper()
	if binding.CacheDirectory != "cache" {
		t.Fatalf("CacheDirectory = %q, want cache", binding.CacheDirectory)
	}
	config := binding.RuntimeConfig()
	if config.FactoryDirectory != "factory" ||
		config.Workers[0].Args[1] != "original" ||
		config.Workers[0].Resources[0].Capacity != 1 ||
		config.Workers[0].Operations[0].Inputs[0].ContentTypes[0] != models.RuntimeContentTypeText ||
		config.Resources[0].Capacity != 2 {
		t.Fatalf("resolved binding was not detached: %#v", config)
	}
}
