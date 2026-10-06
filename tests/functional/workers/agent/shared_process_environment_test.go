package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func agentOwnedEnvironment(home string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") || strings.EqualFold(name, runcli.ModelCacheDirEnvironment) {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+home, "USERPROFILE="+home, runcli.ModelCacheDirEnvironment+"="+filepath.Join(home, "models"))
}

func waitForAgentProcessingWork(t testing.TB, baseURL, sessionID, workID string) {
	t.Helper()
	listed, err := support.WaitForObservation(agentSharedProcessTimeout,
		func() (factoryapi.ListWorkResponse, error) {
			return listAgentSessionWork(t, baseURL, sessionID), nil
		},
		func(listed factoryapi.ListWorkResponse) bool {
			if len(listed.Results) != 1 {
				return false
			}
			item := listed.Results[0]
			return support.StringPointerValue(item.WorkId) == workID && item.State != nil && item.State.Name == "init" && item.State.Type == factoryapi.WorkStateTypePROCESSING
		})
	if err != nil {
		t.Fatalf("observe processing Work %q before cancellation: %v; listed=%#v", workID, err, listed)
	}
}
