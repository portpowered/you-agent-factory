package canonical_test

import (
	"testing"

	compilationcanonical "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/canonical"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestMarshalFactoryConfig_MatchesTransportCanonicalEncoder(t *testing.T) {
	t.Parallel()

	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON([]byte(`{
  "name": "factory",
  "workTypes": [{
    "name": "story",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"}
    ]
  }],
  "workers": [{
    "name": "executor",
    "type": "SCRIPT_WORKER",
    "command": "go",
    "args": ["test", "./..."]
  }],
  "workstations": [{
    "name": "execute-story",
    "type": "MODEL_WORKSTATION",
    "worker": "executor"
  }]
}`))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	ownerLocal, err := compilationcanonical.MarshalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalFactoryConfig: %v", err)
	}
	transport, err := factorymapping.MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if string(ownerLocal) != string(transport) {
		t.Fatalf("owner-local canonical bytes = %q, want transport encoder %q", ownerLocal, transport)
	}
}
