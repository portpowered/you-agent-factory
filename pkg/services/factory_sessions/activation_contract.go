package factorysessions

// DefinitionActivationGatewayProvider preserves the existing Sessions-owned
// construction edge while Factory Definitions owns the canonical gateway
// contract. The provider is intentionally not part of the Definitions root.
// The return value stays opaque here because no Sessions peer consumes this
// transitional provider; the concrete activation edge is private to the
// Sessions implementation.
type DefinitionActivationGatewayProvider interface {
	DefinitionActivationGateway() any
}
