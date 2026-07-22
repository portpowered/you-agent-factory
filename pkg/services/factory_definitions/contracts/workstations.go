package factorycontracts

// WorkstationLoader loads one authored workstation definition by name.
type WorkstationLoader interface {
	Load(name string) (*FactoryWorkstationConfig, error)
}
