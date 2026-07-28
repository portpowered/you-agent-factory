package subsystems_test

import (
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func firstInputToken(rawTokens []any) factorytoken.Token {
	if len(rawTokens) == 0 {
		return factorytoken.Token{}
	}
	tok, ok := rawTokens[0].(factorytoken.Token)
	if !ok {
		return factorytoken.Token{}
	}
	return tok
}
