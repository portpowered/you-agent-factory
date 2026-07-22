package random

import "testing"

func TestCryptoSourceReturnsValueWithinBound(t *testing.T) {
	t.Parallel()

	value, err := (CryptoSource{}).Int63n(7)
	if err != nil {
		t.Fatalf("Int63n() error = %v", err)
	}
	if value < 0 || value >= 7 {
		t.Fatalf("Int63n() = %d, want [0, 7)", value)
	}
}

func TestCryptoSourceRejectsNonPositiveBound(t *testing.T) {
	t.Parallel()

	if _, err := (CryptoSource{}).Int63n(0); err == nil {
		t.Fatal("Int63n() error = nil, want invalid-bound error")
	}
}

func TestSourceFuncImplementsExactPort(t *testing.T) {
	t.Parallel()

	var source Source = SourceFunc(func(bound int64) (int64, error) { return bound - 1, nil })
	value, err := source.Int63n(11)
	if err != nil || value != 10 {
		t.Fatalf("Int63n() = (%d, %v), want (10, nil)", value, err)
	}
}
