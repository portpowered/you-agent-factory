package acp_test

import (
	"os"
)

func operatorACPConfigPath(home string) string {
	return home + string(os.PathSeparator) + ".you-agent-factory" + string(os.PathSeparator) + "config.json"
}
