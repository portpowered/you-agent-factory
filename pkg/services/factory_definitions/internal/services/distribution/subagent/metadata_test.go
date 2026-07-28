package subagent

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestIsPackagedFactory_MatchesNameOrProject(t *testing.T) {
	if !IsPackagedFactory(&factorydefinitions.FactoryConfig{Name: PackagedFactoryName}) {
		t.Fatal("expected factory name match")
	}
	if !IsPackagedFactory(&factorydefinitions.FactoryConfig{Project: PackagedFactoryProject}) {
		t.Fatal("expected factory project match")
	}
	if IsPackagedFactory(&factorydefinitions.FactoryConfig{Name: "@you/other"}) {
		t.Fatal("expected unrelated factory name to be rejected")
	}
	if IsPackagedFactory(nil) {
		t.Fatal("expected nil factory config to be rejected")
	}
}
