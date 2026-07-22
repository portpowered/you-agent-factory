package factorycontracts

// PackagedDefinition is one Factory Definition shipped with the executable.
type PackagedDefinition struct {
	Name    string
	Project string
	JSON    []byte
}
