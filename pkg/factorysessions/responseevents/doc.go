// Package responseevents defines the provider-neutral FactoryResponseEvent
// vocabulary for transient agent activity observed during a Factory Session.
//
// These records are intentionally separate from factoryapi.FactoryEvent and must
// not be projected into canonical factory replay. Later transports, adapters,
// and consumers share this envelope, kind, phase, provenance, and payload
// contract without depending on provider-native schemas.
package responseevents
