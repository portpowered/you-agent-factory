package bootstrap_portability

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func assertListedWorkPayload(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workType string,
	state string,
	want string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType ||
			item.State == nil || item.State.Name != state {
			continue
		}
		if item.Content == nil || len(*item.Content) == 0 {
			t.Fatalf("%s:%s Work has no public content", workType, state)
		}
		part, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			t.Fatalf("decode %s:%s Work text content: %v", workType, state, err)
		}
		if part.Text != want {
			t.Fatalf("expected payload %q, got %q", want, part.Text)
		}
		return
	}
	t.Fatalf("no listed Work found in %s:%s", workType, state)
}
