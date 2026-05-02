package subsystems

import "github.com/portpowered/infinite-you/pkg/interfaces"

func firstInputToken(rawTokens []any) interfaces.Token {
	if len(rawTokens) == 0 {
		return interfaces.Token{}
	}
	tok, ok := rawTokens[0].(interfaces.Token)
	if !ok {
		return interfaces.Token{}
	}
	return tok
}
