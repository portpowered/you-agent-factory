package workers

import "testing"

func TestContainsStopToken_CompleteMarkerMustBeFinalNonEmptyLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "final marker", output: "finished\n<COMPLETE>\n", want: true},
		{name: "continue wins", output: "completion uses <COMPLETE>\n<CONTINUE>"},
		{name: "inline mention", output: "finished with <COMPLETE> in prose"},
		{name: "trailing prose", output: "<COMPLETE>\nadditional caveat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ContainsStopToken(tc.output, "<COMPLETE>"); got != tc.want {
				t.Fatalf("ContainsStopToken() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestContainsStopToken_LegacyTokensRetainSubstringSemantics(t *testing.T) {
	t.Parallel()
	if !ContainsStopToken("Work done. COMPLETE", "COMPLETE") {
		t.Fatal("plain legacy stop token did not match inline output")
	}
	if !ContainsStopToken("prefix <result>ACCEPTED</result> suffix", "<result>ACCEPTED</result>") {
		t.Fatal("structured legacy stop token did not match inline output")
	}
}
