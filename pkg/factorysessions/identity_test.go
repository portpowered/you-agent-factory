package factorysessions

import "testing"

func TestLogicalSessionKeyID_DefaultTargetUsesStableKey(t *testing.T) {
	session := &LiveSession{
		SessionState: SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{
			Kind: TargetKindDefault,
		},
	}
	if got := LogicalSessionKeyID(session); got != "/workspace/root::default::" {
		t.Fatalf("LogicalSessionKeyID(default) = %q, want /workspace/root::default::", got)
	}
}

func TestLogicalSessionKeyID_NamedTargetIncludesFactoryName(t *testing.T) {
	session := &LiveSession{
		SessionState: SessionState{
			FolderPath: "/workspace/root",
		},
		Target: TargetRef{
			Kind: TargetKindNamed,
			Name: "beta",
		},
	}
	if got := LogicalSessionKeyID(session); got != "/workspace/root::named::beta" {
		t.Fatalf("LogicalSessionKeyID(named) = %q, want /workspace/root::named::beta", got)
	}
}
