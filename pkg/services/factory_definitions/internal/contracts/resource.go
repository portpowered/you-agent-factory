// ResourceConfig is the Definition-owned authored resource value. Catalog
// owns persistence and lookup of resources; the value remains in the root
// contract vocabulary consumed by validation and compilation.
package factorycontracts

type ResourceConfig struct {
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
	ResourceTypeModel          = "MODEL"
	ResourceTypeProviderQuota  = "PROVIDER_QUOTA"
	ResourceTypeInvocationSlot = "INVOCATION_SLOT"
)
