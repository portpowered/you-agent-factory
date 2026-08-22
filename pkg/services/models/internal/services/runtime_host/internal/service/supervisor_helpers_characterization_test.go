package service

import "testing"

func TestRequiresSupervisedBackend_CharacterizesCurrentMembership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		backend string
		want    bool
	}{
		{name: "canonical", backend: "LLAMACPP", want: true},
		{name: "case and surrounding whitespace", backend: "  llamaCpp \t", want: true},
		{name: "blank", backend: "", want: false},
		{name: "only whitespace", backend: " \t\n", want: false},
		{name: "unknown", backend: "GGUF", want: false},
		{name: "llamacpp artifact identifier", backend: "localai-llamacpp", want: false},
		{name: "whisper artifact identifier", backend: "localai-whisper", want: false},
		{name: "vibevoice artifact identifier", backend: "localai-vibevoice", want: false},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := requiresSupervisedBackend(testCase.backend)
			if got != testCase.want {
				t.Fatalf("requiresSupervisedBackend(%q) = %t, want %t", testCase.backend, got, testCase.want)
			}
		})
	}
}
