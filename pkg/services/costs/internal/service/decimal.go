package service

import (
	"errors"
	"math/big"
	"strings"
)

var errNonTerminatingDecimal = errors.New("decimal amount has a non-terminating representation")

const millionTokenCount int64 = 1_000_000

func parseDecimal(value string) (*big.Rat, error) {
	parsed, ok := new(big.Rat).SetString(value)
	if !ok || parsed.Sign() < 0 {
		return nil, errors.New("rate is not a non-negative decimal")
	}
	return parsed, nil
}

// formatExactDecimal emits the shortest exact base-10 representation. Rate
// validation permits only finite decimals, and division by one million keeps
// the denominator composed solely of factors 2 and 5.
func formatExactDecimal(value *big.Rat) (string, error) {
	if value == nil {
		return "", errors.New("decimal value is required")
	}
	if value.Sign() == 0 {
		return "0", nil
	}

	numerator := new(big.Int).Set(value.Num())
	negative := numerator.Sign() < 0
	numerator.Abs(numerator)
	denominator := new(big.Int).Set(value.Denom())
	two := big.NewInt(2)
	five := big.NewInt(5)
	countTwo := factorCount(denominator, two)
	countFive := factorCount(denominator, five)
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return "", errNonTerminatingDecimal
	}
	scale := countTwo
	if countFive > scale {
		scale = countFive
	}
	if missing := scale - countTwo; missing > 0 {
		numerator.Mul(numerator, new(big.Int).Exp(two, big.NewInt(int64(missing)), nil))
	}
	if missing := scale - countFive; missing > 0 {
		numerator.Mul(numerator, new(big.Int).Exp(five, big.NewInt(int64(missing)), nil))
	}

	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	whole := new(big.Int)
	fraction := new(big.Int)
	whole.QuoRem(numerator, divisor, fraction)
	result := whole.String()
	if scale > 0 && fraction.Sign() != 0 {
		fractionText := fraction.String()
		if padding := scale - len(fractionText); padding > 0 {
			fractionText = strings.Repeat("0", padding) + fractionText
		}
		fractionText = strings.TrimRight(fractionText, "0")
		if fractionText != "" {
			result += "." + fractionText
		}
	}
	if negative {
		result = "-" + result
	}
	return result, nil
}

func factorCount(value, factor *big.Int) int {
	count := 0
	quotient := new(big.Int)
	remainder := new(big.Int)
	for value.Sign() != 0 {
		quotient.QuoRem(value, factor, remainder)
		if remainder.Sign() != 0 {
			break
		}
		value.Set(quotient)
		count++
	}
	return count
}
