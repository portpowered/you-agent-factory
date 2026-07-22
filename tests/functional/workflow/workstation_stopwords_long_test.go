//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWorkstationStopWords_ThroughCustomerProcess(t *testing.T) {
	support.SkipLongFunctional(t, "slow workstation stop-word customer-boundary sweep")

	tests := []struct {
		name        string
		fixture     string
		title       string
		response    string
		wantPlace   string
		emptyPlaces []string
	}{
		{
			name: "FactoryJSON_Success", fixture: "workstation_stopwords_factory_dir",
			title: "factory stop word success", response: "Work completed successfully. COMPLETE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "FactoryJSON_SecondWord", fixture: "workstation_stopwords_factory_dir",
			title: "factory stop word second", response: "All tasks finished. DONE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "FactoryJSON_Failure", fixture: "workstation_stopwords_factory_dir",
			title: "factory stop word failure", response: "I tried but could not finish the work",
			wantPlace: "task:failed", emptyPlaces: []string{"task:init", "task:complete"},
		},
		{
			name: "Frontmatter_Success", fixture: "workstation_stopwords_frontmatter_dir",
			title: "frontmatter stop word success", response: "Work completed successfully. COMPLETE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "Frontmatter_SecondWord", fixture: "workstation_stopwords_frontmatter_dir",
			title: "frontmatter stop word second", response: "All tasks finished. DONE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "Frontmatter_Failure", fixture: "workstation_stopwords_frontmatter_dir",
			title: "frontmatter stop word failure", response: "I tried but could not finish the work",
			wantPlace: "task:failed", emptyPlaces: []string{"task:init", "task:complete"},
		},
		{
			name: "Override_StationAcceptsWorkerRejects", fixture: "workstation_stopwords_override_dir",
			title: "station overrides worker", response: "The work is finished. STATION_COMPLETE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:init", "task:failed"},
		},
		{
			name: "Override_StationRejectsWorkerAccepts", fixture: "workstation_stopwords_override_dir",
			title: "station rejects worker accepts", response: "The work is done. WORKER_COMPLETE",
			wantPlace: "task:failed", emptyPlaces: []string{"task:init", "task:complete"},
		},
		{
			name: "Override_BothMatch", fixture: "workstation_stopwords_override_dir",
			title: "both match", response: "WORKER_COMPLETE and STATION_COMPLETE",
			wantPlace: "task:complete", emptyPlaces: []string{"task:failed"},
		},
		{
			name: "Override_NeitherMatch", fixture: "workstation_stopwords_override_dir",
			title: "neither match", response: "I tried but could not finish the work",
			wantPlace: "task:failed", emptyPlaces: []string{"task:complete"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, test.fixture))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"`+test.title+`"}`))
			provider := testutil.NewMockProvider(
				workerexecution.InferenceResponse{Content: test.response},
			)

			session := support.RunFactoryToCompletion(t, dir, provider, 15*time.Second)
			if got := support.SessionPlaceTokenCount(session, test.wantPlace); got != 1 {
				t.Errorf("%s token count = %d, want 1", test.wantPlace, got)
			}
			for _, placeID := range test.emptyPlaces {
				if got := support.SessionPlaceTokenCount(session, placeID); got != 0 {
					t.Errorf("%s token count = %d, want 0", placeID, got)
				}
			}
		})
	}
}
