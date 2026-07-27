package factorycontracts

type PackagedFactoryFormat string

const (
	PackagedFactoryFormatJSON PackagedFactoryFormat = "JSON"
	PackagedFactoryFormatYAML PackagedFactoryFormat = "YAML"
)

// PackagedDefinition is one Factory Definition shipped with the executable.
type PackagedDefinition struct {
	Name    string
	Project string
	JSON    []byte
	YAML    []byte
	Formats []PackagedFactoryFormat
}
