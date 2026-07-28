package orchestratorcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Hash returns a stable digest for one effective policy independent of map ordering.
func Hash(policy EffectivePolicy) string {
	encoded, err := json.Marshal(normalizePolicy(policy))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// HashDocument returns a stable digest for one raw policy document by resolving
// it through the effective policy contract.
func HashDocument(raw json.RawMessage) string {
	return ResolveFromFactoryDefault(raw).Hash
}
