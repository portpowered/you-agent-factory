package factory

// FactoryStatus is the detached Factory Runtime status read model. It contains
// no marking, topology, token, or implementation references.
type FactoryStatus struct {
	Categories             FactoryStatusCategories
	FactoryState           string
	LifecycleControlStatus string
	Resources              []FactoryResourceUsage
	RuntimeStatus          string
	TotalTokens            int
}

// FactoryStatusCategories counts public Work by runtime state category.
type FactoryStatusCategories struct {
	Failed     int
	Initial    int
	Processing int
	Terminal   int
}

// FactoryResourceUsage is the detached availability projection for one
// runtime resource.
type FactoryResourceUsage struct {
	Available int
	Name      string
	Total     int
}

// FactoryStatusProjector owns the exact status projection operation injected
// into consumers. Transport packages receive this role through composition and
// never categorize runtime tokens themselves.
type FactoryStatusProjector interface {
	ProjectFactoryStatusFromObservation(Observation) FactoryStatus
}

type factoryStatusProjector struct{}

// NewFactoryStatusProjector constructs the stateless Factory Runtime status
// projection operation. Application composition owns this constructor.
func NewFactoryStatusProjector() FactoryStatusProjector {
	return factoryStatusProjector{}
}

func (factoryStatusProjector) ProjectFactoryStatusFromObservation(observation Observation) FactoryStatus {
	return FactoryStatusFromObservation(observation)
}
