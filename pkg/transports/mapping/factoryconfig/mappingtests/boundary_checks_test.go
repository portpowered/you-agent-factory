package mappingtests

import (
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
