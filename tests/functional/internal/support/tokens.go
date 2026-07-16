package support

import (
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
)

func FirstInputToken(rawTokens any) factorytoken.Token {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return factorytoken.Token{}
		}
		tok, ok := tokens[0].(factorytoken.Token)
		if !ok {
			return factorytoken.Token{}
		}
		return tok
	case []factorytoken.Token:
		if len(tokens) == 0 {
			return factorytoken.Token{}
		}
		return tokens[0]
	default:
		return factorytoken.Token{}
	}
}
