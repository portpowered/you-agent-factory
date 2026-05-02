package support

import "github.com/portpowered/infinite-you/pkg/interfaces"

func FirstInputToken(rawTokens any) interfaces.Token {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return interfaces.Token{}
		}
		tok, ok := tokens[0].(interfaces.Token)
		if !ok {
			return interfaces.Token{}
		}
		return tok
	case []interfaces.Token:
		if len(tokens) == 0 {
			return interfaces.Token{}
		}
		return tokens[0]
	default:
		return interfaces.Token{}
	}
}
