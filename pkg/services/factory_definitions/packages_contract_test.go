package factorydefinitions

import "testing"

func TestCustomerVisibleFactoryName_PrefersPackagedPublicName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *FactoryConfig
		want string
	}{
		{
			name: "public packaged name",
			cfg:  &FactoryConfig{Name: "@you/fusion"},
			want: "@you/fusion",
		},
		{
			name: "generated runtime name",
			cfg:  &FactoryConfig{Name: "fusion", Project: "builtin-fusion"},
			want: "@you/fusion",
		},
		{
			name: "customer local factory",
			cfg:  &FactoryConfig{Name: "customer-workshop"},
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
