// Package responseevents defines the provider-neutral FactoryResponseEvent
// vocabulary for transient agent activity observed during a Factory Session.
//
// These records are intentionally separate from factoryapi.FactoryEvent and must
// not be projected into canonical factory replay. Later transports, adapters,
// and consumers share this envelope, kind, phase, provenance, typed payload,
// capability, and validation contract without depending on provider-native
// schemas. Use ValidateEvent to reject invalid kind/phase/payload combinations.
package responseevents
