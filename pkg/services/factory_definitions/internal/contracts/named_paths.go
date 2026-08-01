package factorycontracts

// NamedFactoryCandidatePaths contains the detached, ordered paths used to
// diagnose a failed cross-root named Factory lookup.
type NamedFactoryCandidatePaths struct {
	Project string
	Global  string
}
