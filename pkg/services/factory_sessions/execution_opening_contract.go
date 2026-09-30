package factorysessions

// ProviderIdentityResolver resolves one authored provider selection through
// the immutable process registry without exposing a second service interface.
type ProviderIdentityResolver func(string) (string, error)

// DirectJavaScriptRunRequest carries customer-edge values for one raw
// JavaScript workflow invocation. Source resolution and execution policy stay
// behind DirectJavaScriptRunOperation. Protocol output and host observation are
// owner-private state selected by ScopeID.
type DirectJavaScriptRunRequest struct {
	SourcePath         string
	MockWorkersEnabled bool
	JSONOutput         bool
	Host               *RuntimeHostRequest
	ScopeID            OpeningScopeID
}
