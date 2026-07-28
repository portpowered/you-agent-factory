package providers

// ACPIntegration is one customer-configured stdio Agent Client Protocol peer.
type ACPIntegration struct {
	ID        string
	Name      ID
	Transport string
	Command   string
}

// Factory constructs the singular Providers root with invocation-scoped ACP
// configuration. Construction is inert; commands start only during Execute.
type Factory func([]ACPIntegration) (Service, error)
