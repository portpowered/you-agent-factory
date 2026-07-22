// Package random supplies policy-free process entropy adapters. Wire selects
// these adapters; consuming services own the policy that uses the entropy.
package random

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Source is the exact bounded-entropy effect consumed by retry policy. The
// contract lives beside its lowest platform adapter and is aggregated by Wire
// without service-owned aliases.
type Source interface {
	Int63n(upperBound int64) (int64, error)
}

// SourceFunc adapts a function to Source for deterministic edge replacements.
type SourceFunc func(int64) (int64, error)

func (source SourceFunc) Int63n(upperBound int64) (int64, error) {
	return source(upperBound)
}

// CryptoSource returns unbiased process entropy within a caller-owned bound.
type CryptoSource struct{}

var _ Source = CryptoSource{}

func (CryptoSource) Int63n(upperBound int64) (int64, error) {
	if upperBound <= 0 {
		return 0, fmt.Errorf("random upper bound must be positive")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(upperBound))
	if err != nil {
		return 0, fmt.Errorf("read process entropy: %w", err)
	}
	return value.Int64(), nil
}
