package main

import (
	"testing"
)

func TestServiceFacadeOwnersTopLevelUnexpectedCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	for _, owner := range []string{"automations", "provider_sessions"} {
		spec := productOwnerTopLevelSpecs[owner]
		for _, child := range spec.unexpected {
			if child != "service" {
				t.Fatalf("owner %q unexpected inventory child %q is not the legacy service facade", owner, child)
			}
			got, ok := mapLegacyServiceImplementationPackage(owner, "pkg/services/"+owner+"/"+child, child)
			if !ok {
				t.Fatalf("mapLegacyServiceImplementationPackage(%q) ok = false", owner)
			}
			if got.Disposition != DispositionMove || got.Destination != owner+"/internal" {
				t.Fatalf("service move mapping for %q = %#v, want move→%s/internal", owner, got, owner)
			}
		}
	}
}

func TestAllProductOwnerUnexpectedSiblingsCoveredByMoveRules(t *testing.T) {
	t.Parallel()

	for _, spec := range productOwnerTopLevelSpecsList() {
		if len(spec.unexpected) == 0 {
			continue
		}
		for _, child := range spec.unexpected {
			switch spec.owner {
			case "workers":
				switch child {
				case "agypty", "cliprovider", "provider", "provider_test":
					_, ok := mapProvidersExtraction("pkg/services/workers/" + child)
					if !ok {
						t.Fatalf("mapProvidersExtraction(workers/%s) ok = false", child)
					}
					continue
				case "service":
					got, ok := mapLegacyServiceImplementationPackage(spec.owner, "pkg/services/workers/"+child, child)
					if !ok || got.Disposition != DispositionMove {
						t.Fatalf("workers/service legacy mapping = %#v ok=%v", got, ok)
					}
					continue
				}
			case "automations", "provider_sessions", "work", "factory_definitions", "factory_runtime", "recordings":
				if child == "service" {
					got, ok := mapLegacyServiceImplementationPackage(spec.owner, "pkg/services/"+spec.owner+"/"+child, child)
					if !ok || got.Disposition != DispositionMove {
						t.Fatalf("%s/service legacy mapping = %#v ok=%v", spec.owner, got, ok)
					}
					continue
				}
			}

			destination, ok := nestedOwnerMoveDestination(spec.owner, child)
			if !ok {
				t.Fatalf("nestedOwnerMoveDestination(%s, %q) ok = false", spec.owner, child)
			}
			if destination == spec.owner {
				t.Fatalf("owner %q unexpected child %q maps to owner root retain destination", spec.owner, child)
			}
		}
	}
}
