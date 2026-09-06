package cli

import (
	"math/big"
	"strings"

	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

type metricsSessionDecimalValue struct {
	coefficient *big.Int
	scale       int
}

func metricsSessionExactCost(items []generatedclient.CostsLineItem) *string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		if strings.ToUpper(strings.TrimSpace(string(item.Status))) != "PRICED" || item.PricedAmount == nil {
			continue
		}
		values = append(values, *item.PricedAmount)
	}
	return sumMetricsSessionDecimalStrings(values)
}

func sumMetricsSessionDecimalStrings(values []string) *string {
	if len(values) == 0 {
		return nil
	}
	parsed, maxScale, ok := parseMetricsSessionDecimals(values)
	if !ok {
		return nil
	}
	total := new(big.Int)
	for _, value := range parsed {
		coefficient := new(big.Int).Set(value.coefficient)
		if power := maxScale - value.scale; power > 0 {
			coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(power)), nil))
		}
		total.Add(total, coefficient)
	}
	return formatMetricsSessionDecimal(total, maxScale)
}

func parseMetricsSessionDecimals(values []string) ([]metricsSessionDecimalValue, int, bool) {
	parsed := make([]metricsSessionDecimalValue, 0, len(values))
	maxScale := 0
	for _, raw := range values {
		value, ok := parseMetricsSessionDecimal(raw)
		if !ok {
			return nil, 0, false
		}
		parsed = append(parsed, value)
		if value.scale > maxScale {
			maxScale = value.scale
		}
	}
	return parsed, maxScale, true
}

func parseMetricsSessionDecimal(raw string) (metricsSessionDecimalValue, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return metricsSessionDecimalValue{}, false
	}
	sign := ""
	if value[0] == '-' || value[0] == '+' {
		sign, value = value[:1], value[1:]
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || !metricsSessionDecimalDigits(parts[0]) {
		return metricsSessionDecimalValue{}, false
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if !metricsSessionDecimalDigits(fraction) {
		return metricsSessionDecimalValue{}, false
	}
	coefficient, ok := new(big.Int).SetString(sign+parts[0]+fraction, 10)
	if !ok {
		return metricsSessionDecimalValue{}, false
	}
	return metricsSessionDecimalValue{coefficient: coefficient, scale: len(fraction)}, true
}

func metricsSessionDecimalDigits(value string) bool {
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func formatMetricsSessionDecimal(total *big.Int, scale int) *string {
	negative := total.Sign() < 0
	if negative {
		total.Abs(total)
	}
	digits := total.String()
	if scale > 0 {
		if len(digits) <= scale {
			digits = strings.Repeat("0", scale-len(digits)+1) + digits
		}
		position := len(digits) - scale
		digits = digits[:position] + "." + digits[position:]
		digits = strings.TrimRight(digits, "0")
		digits = strings.TrimRight(digits, ".")
	}
	if digits == "" {
		digits = "0"
	}
	if negative && digits != "0" {
		digits = "-" + digits
	}
	return &digits
}
