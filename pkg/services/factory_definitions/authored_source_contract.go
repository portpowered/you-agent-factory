package factorydefinitions

type AuthoredFactoryFormat string

const (
	AuthoredFactoryFormatJSON AuthoredFactoryFormat = "JSON"
	AuthoredFactoryFormatYAML AuthoredFactoryFormat = "YAML"
)

const SupportedAuthoredFactoryExtensions = ".json, .yaml, and .yml"
const SupportedAuthoredFactoryRootFiles = "factory.json, factory.yaml, and factory.yml"

// AuthoredFactorySource retains the selected source identity while carrying
// JSON-compatible bytes into representation mapping and validation.
type AuthoredFactorySource struct {
	Path   string
	Format AuthoredFactoryFormat
	Data   []byte
}

// AuthoredFactorySourceLoader resolves an authored Factory Definition path and
// returns its selected source identity and JSON-compatible representation.
type AuthoredFactorySourceLoader func(path string) (AuthoredFactorySource, error)
