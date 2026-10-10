package mappingtests

import (
	"reflect"
	"strings"
	"testing"

	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestFactoryBoundaryRejectsUnsupportedFieldsWithCaseInsensitiveContainers(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, payload, diagnostic string }{
		{"join", `{"name":"invalid","Workstations":[{"name":"task","join":null}]}`, "workstations[0].join"},
		{"cron interval", `{"name":"invalid","workStations":[{"name":"task","Cron":{"interval":null}}]}`, "workstations[0].cron.interval"},
		{"mixed case cron", `{"name":"invalid","workstations":[{"name":"task","cRoN":{"interval":"1s"}}]}`, "workstations[0].cron.interval"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config, err := factorymapping.NewFactoryConfigMapper().Expand([]byte(test.payload))
			if config != nil || err == nil || !strings.Contains(err.Error(), test.diagnostic) {
				t.Fatalf("config = %v, error = %v, want %q", config, err, test.diagnostic)
			}
		})
	}
}

func TestFactoryBoundaryDoesNotTreatUnknownEmptyKeyAsWorkstations(t *testing.T) {
	t.Parallel()
	config, err := factorymapping.NewFactoryConfigMapper().Expand([]byte(`{"name":"valid","": [{"join":null}]}`))
	if err != nil || config == nil || len(config.Workstations) != 0 {
		t.Fatalf("config = %v, error = %v, want an independent additive field", config, err)
	}
}

func TestNormalizeCanonicalFactoryPreservesRepeatedExampleArguments(t *testing.T) {
	t.Parallel()
	mapper := factorymapping.NewFactoryConfigMapper()
	payload := []byte(`{"name":"examples","examples":[{"name":"tags","description":{"type":"LOCALIZABLE_ASSET","value":"Tagged work"},"args":{"tag":["alpha","beta"],"input":"hello"}}],"customerExtension":{"enabled":true}}`)
	config, err := mapper.Expand(payload)
	if err != nil {
		t.Fatalf("load canonical Factory: %v", err)
	}
	if len(config.Examples) != 1 {
		t.Fatalf("examples = %v, want one example", config.Examples)
	}
	if got := config.Examples[0].Args["tag"]; !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("repeated arguments = %#v", got)
	}
	if got := config.IgnoredJSONPaths(); !reflect.DeepEqual(got, []string{"$.customerExtension"}) {
		t.Fatalf("ignored fields = %v", got)
	}
	config.Examples[0].Args["tag"].([]string)[0] = "changed"
	again, err := mapper.Expand(payload)
	if err != nil || !reflect.DeepEqual(again.Examples[0].Args["tag"], []string{"alpha", "beta"}) {
		t.Fatalf("independent load = %#v, error = %v", again, err)
	}
}

func TestNormalizeCanonicalFactoryRejectsInvalidPublicInputs(t *testing.T) {
	t.Parallel()
	mapper := factorymapping.NewFactoryConfigMapper()
	cases := []struct{ name, payload, diagnostic string }{
		{"missing name", `{"name":" "}`, "factory.name is required"},
		{"invalid description", `{"name":"invalid","description":{"type":"TEXT","value":"wrong"}}`, "factory.description.type"},
		{"invalid arguments", `{"name":"invalid","examples":[{"name":"example","description":{"type":"LOCALIZABLE_ASSET","value":"Example"},"args":{"tag":42}}]}`, "args"},
		{"trailing value", `{"name":"invalid"} {}`, "after top-level value"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config, err := mapper.Expand([]byte(test.payload))
			if config != nil || err == nil || !strings.Contains(err.Error(), test.diagnostic) {
				t.Fatalf("config = %v, error = %v, want %q", config, err, test.diagnostic)
			}
		})
	}
}
