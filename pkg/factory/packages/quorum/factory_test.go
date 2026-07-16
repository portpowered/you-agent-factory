package quorum

import (
	"reflect"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestBuiltInFactoryJSON_LoadsRunnablePackagedQuorumFactory(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName || cfg.Project != PackagedFactoryProject {
		t.Fatalf("packaged identity = %q/%q, want %q/%q", cfg.Name, cfg.Project, PackagedFactoryName, PackagedFactoryProject)
	}
	if cfg.InvocationSignature == nil || len(cfg.Workers) != 3 || len(cfg.Workstations) != 4 {
		t.Fatalf("quorum config is not runnable: %#v", cfg)
	}
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if strings.HasPrefix(target.Code, "factory.invocationSignature.") {
			t.Fatalf("validation target = %#v, want valid quorum signature", target)
		}
	}
}

func TestBuiltInQuorumFactory_DefaultNamedInvocationAcceptsInput(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	got, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Compare two release plans."},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}
	input, ok := got.Arguments["input"]
	if !ok || !reflect.DeepEqual(input.Values, []string{"Compare two release plans."}) {
		t.Fatalf("input = %#v, want accepted default named invocation input", input)
	}
}

func TestIsPackagedFactory_MatchesBuiltInQuorumIdentity(t *testing.T) {
	if !IsPackagedFactory(&interfaces.FactoryConfig{Name: PackagedFactoryName}) || !IsPackagedFactory(&interfaces.FactoryConfig{Project: PackagedFactoryProject}) {
		t.Fatal("expected packaged quorum identity match")
	}
	if IsPackagedFactory(&interfaces.FactoryConfig{Name: "customer-quorum"}) || IsPackagedFactory(nil) {
		t.Fatal("unexpected packaged quorum identity match")
	}
}
