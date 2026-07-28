package providersessions_test

import (
	"reflect"
	"strings"
	"testing"

	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const providerSessionsRootPackage = "github.com/portpowered/infinite-you/pkg/services/provider_sessions"

// Compile-time proofs that Provider Sessions root identity contracts are aliases
// of or carry Providers-owned SessionRef values rather than parallel vocabulary.
var (
	_ providers.SessionRef = providersessions.SessionRef{}
	_ providers.SessionRef = providersessions.InspectRequest{}.Session
	_ providers.SessionRef = providersessions.InspectResult{}.Session
	_ providers.SessionRef = providersessions.ProjectRequest{}.Session
	_ providers.SessionRef = providersessions.ProjectResult{}.Session
)

var providersRootIdentityContracts = []reflect.Type{
	reflect.TypeOf(providersessions.SessionRef{}),
	reflect.TypeOf(providersessions.InspectRequest{}),
	reflect.TypeOf(providersessions.InspectResult{}),
	reflect.TypeOf(providersessions.ProjectRequest{}),
	reflect.TypeOf(providersessions.ProjectResult{}),
}

var forbiddenProvidersPeerSurfaceRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/providers/internal",
	"github.com/portpowered/infinite-you/pkg/services/providers/wire",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider",
}

func TestRootPackage_ImportsProvidersOnlyThroughServiceRoot(t *testing.T) {
	t.Parallel()

	assertPackageDepsForbidden(t, providerSessionsRootPackage, forbiddenProvidersConsumerRoots)
	for _, importPath := range listDirectImports(t, providerSessionsRootPackage) {
		if !strings.HasPrefix(importPath, "github.com/portpowered/infinite-you/pkg/services/providers") {
			continue
		}
		if importPath != providersServiceRoot {
			t.Fatalf(
				"%s must import Providers only through %s; found direct import %s",
				providerSessionsRootPackage,
				providersServiceRoot,
				importPath,
			)
		}
	}
}

func TestRootContracts_ProvidersRootBoundary_PeerSurfaceUsesProvidersIdentityOnly(t *testing.T) {
	t.Parallel()

	for _, typ := range providersRootIdentityContracts {
		typ := typ
		t.Run(typ.String(), func(t *testing.T) {
			t.Parallel()
			assertContractTypeUsesOnlyProvidersRootIdentity(t, typ, map[reflect.Type]bool{})
		})
	}
}

func assertContractTypeUsesOnlyProvidersRootIdentity(
	t *testing.T,
	typ reflect.Type,
	visiting map[reflect.Type]bool,
) {
	t.Helper()

	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice ||
		typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if visiting[typ] {
		return
	}
	visiting[typ] = true
	defer delete(visiting, typ)

	switch typ.Kind() {
	case reflect.Interface:
		if typ.PkgPath() == "context" {
			return
		}
		t.Fatalf("root contract %s must not expose non-context interface dependency", typ)
	case reflect.Func, reflect.Chan:
		t.Fatalf("root contract %s must not expose non-value dependency %s", typ, typ.Kind())
	case reflect.Struct:
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			if field.PkgPath != "" && !field.IsExported() {
				continue
			}
			assertContractTypeUsesOnlyProvidersRootIdentity(t, field.Type, visiting)
		}
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return
	default:
		if typ.PkgPath() == "" {
			return
		}
	}

	pkgPath := typ.PkgPath()
	if pkgPath == "" || pkgPath == "context" || pkgPath == "time" {
		return
	}
	if pkgPath == providerSessionsRootPackage || pkgPath == providersServiceRoot {
		return
	}
	for _, forbidden := range forbiddenProvidersPeerSurfaceRoots {
		if pkgPath == forbidden || strings.HasPrefix(pkgPath, forbidden+"/") {
			t.Fatalf("root contract type %s depends on forbidden Providers consumer path %s", typ, pkgPath)
		}
	}
	t.Fatalf(
		"root contract type %s depends on unexpected package %s; peer surface must use only Provider Sessions root and Providers service-root identity types",
		typ,
		pkgPath,
	)
}
