package factorycontracts

// These aliases keep the authored worker vocabulary explicit at the
// Definition contract boundary while the implementation remains local to this
// contract package.
type FactoryWorkerConfig = Config

var CloneWorkerConfig = Clone
