package workask_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workask"
)

func TestIsEmptyCustomerAsk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content []interfaces.WorkContentPart
		payload []byte
		want    bool
	}{
		{
			name: "missing payload and content",
			want: true,
		},
		{
			name:    "whitespace only payload string",
			payload: []byte(`"   \t\n"`),
			want:    true,
		},
		{
			name:    "empty json string payload",
			payload: []byte(`""`),
			want:    true,
		},
		{
			name:    "whitespace only raw payload",
			payload: []byte("  \t  "),
			want:    true,
		},
		{
			name:    "non empty text payload",
			payload: []byte("add search to docs"),
			want:    false,
		},
		{
			name:    "structured json payload",
			payload: []byte(`{"title":"search bar on docs"}`),
			want:    false,
		},
		{
			name: "whitespace only text content",
			content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "  \n\t ",
			}},
			want: true,
		},
		{
			name: "non empty text content",
			content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "Improve onboarding",
			}},
			want: false,
		},
		{
			name: "image only content without text",
			content: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeImage,
				URL:  "file://fixtures/sketch.png",
			}},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := workask.IsEmptyCustomerAsk(tc.content, tc.payload); got != tc.want {
				t.Fatalf("IsEmptyCustomerAsk() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateIdeaCustomerAsk_ReturnsStableMessage(t *testing.T) {
	t.Parallel()

	err := workask.ValidateIdeaCustomerAsk(0, "fd", nil, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), workask.CustomerAskRequiredMessage) {
		t.Fatalf("error = %q, want customer ask required message", err.Error())
	}
	if !strings.Contains(err.Error(), `works[0] ("fd")`) {
		t.Fatalf("error = %q, want indexed work name", err.Error())
	}
}

func TestValidateIdeaWorkInBatch_SkipsNonIdeaWork(t *testing.T) {
	t.Parallel()

	if err := workask.ValidateIdeaWorkInBatch(0, interfaces.Work{
		Name:       "task-item",
		WorkTypeID: "task",
	}); err != nil {
		t.Fatalf("ValidateIdeaWorkInBatch() = %v, want nil", err)
	}
}
