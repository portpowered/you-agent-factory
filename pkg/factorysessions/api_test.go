package factorysessions

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestListSummaries_OrdersDefaultSessionFirst(t *testing.T) {
	registry := NewRegistry()
	registry.Upsert(NewLiveSession(
		"session-b",
		"/factories/b",
		"/workspace",
		TargetRef{Kind: TargetKindNamed, Name: "b"},
		nil,
		false,
		"b",
	), true)
	registry.Upsert(NewLiveSession(
		DefaultSessionID,
		"/factories/default",
		"/workspace",
		TargetRef{Kind: TargetKindDefault},
		nil,
		true,
		"default",
	), false)

	summaries := ListSummaries(registry)
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}
	if !summaries[0].IsDefault || summaries[0].Id != DefaultSessionID {
		t.Fatalf("first summary = %#v, want default session first", summaries[0])
	}
}

func TestSummaryResponse_MapsLiveSessionFields(t *testing.T) {
	name := "beta"
	summary := SummaryResponse(&LiveSession{
		ID:         "session-1",
		FactoryDir: "/factories/beta",
		FolderPath: "/workspace",
		IsDefault:  false,
		Project:    "beta-project",
		Target:     TargetRef{Kind: TargetKindNamed, Name: name},
	})
	if summary.Id != "session-1" || summary.Project != "beta-project" {
		t.Fatalf("summary = %#v, want mapped session fields", summary)
	}
	if summary.Target.Kind != factoryapi.FactorySessionTargetRefKindNamed || summary.Target.Name == nil || *summary.Target.Name != name {
		t.Fatalf("summary target = %#v, want named beta target", summary.Target)
	}
}
