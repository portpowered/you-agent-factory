package wire

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestCustomerVisibleFactoryName_PrefersPackagedPublicName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *factorydefinitions.FactoryConfig
		want string
	}{
		{
			name: "public packaged name",
			cfg:  &factorydefinitions.FactoryConfig{Name: "@you/fusion"},
			want: "@you/fusion",
		},
		{
			name: "generated runtime name",
			cfg:  &factorydefinitions.FactoryConfig{Name: "fusion", Project: "builtin-fusion"},
			want: "@you/fusion",
		},
		{
			name: "customer local factory",
			cfg:  &factorydefinitions.FactoryConfig{Name: "customer-workshop"},
			want: "customer-workshop",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := CustomerVisibleFactoryName(test.cfg); got != test.want {
				t.Fatalf("CustomerVisibleFactoryName() = %q, want %q", got, test.want)
			}
		})
	}
}
