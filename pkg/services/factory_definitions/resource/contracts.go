// Package resource owns authored Factory resource capacity contracts.
package resource

type Config struct {
	ID         string `json:"id,omitempty" yaml:"id,omitempty"`
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	Capacity   int    `json:"capacity"`
	Model      string `json:"model,omitempty"`
	Backend    string `json:"backend,omitempty"`
	LoadPolicy string `json:"loadPolicy,omitempty"`
	Provider   string `json:"provider,omitempty"`
}

const (
	TypeModel          = "MODEL"
	TypeProviderQuota  = "PROVIDER_QUOTA"
	TypeInvocationSlot = "INVOCATION_SLOT"
)
