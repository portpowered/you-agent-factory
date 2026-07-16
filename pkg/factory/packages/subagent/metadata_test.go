package subagent

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestIsPackagedFactory_MatchesNameOrProject(t *testing.T) {
	if !IsPackagedFactory(&interfaces.FactoryConfig{Name: PackagedFactoryName}) {
		t.Fatal("expected factory name match")
	}
	if !IsPackagedFactory(&interfaces.FactoryConfig{Project: PackagedFactoryProject}) {
		t.Fatal("expected factory project match")
	}
	if IsPackagedFactory(&interfaces.FactoryConfig{Name: "@you/other"}) {
		t.Fatal("expected unrelated factory name to be rejected")
	}
	if IsPackagedFactory(nil) {
		t.Fatal("expected nil factory config to be rejected")
	}
}
