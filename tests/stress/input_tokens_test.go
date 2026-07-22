package stress_test

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func firstInputToken(rawTokens any) factoryruntime.RuntimeToken {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return factoryruntime.RuntimeToken{}
		}
		tok, ok := tokens[0].(factoryruntime.RuntimeToken)
		if !ok {
			return factoryruntime.RuntimeToken{}
		}
		return tok
	case []factoryruntime.RuntimeToken:
		if len(tokens) == 0 {
			return factoryruntime.RuntimeToken{}
		}
		return tokens[0]
	default:
		return factoryruntime.RuntimeToken{}
	}
}
