package factorycontracts

const DefaultFactoryInputType = "task"

// ScaffoldConfig identifies the directory where the supported default Factory
// scaffold is materialized.
type ScaffoldConfig struct {
	Dir string
}

type ScaffoldInitializer func(ScaffoldConfig) error
