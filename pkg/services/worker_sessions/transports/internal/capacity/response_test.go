package capacity

import (
	"strings"
	"testing"
)

const valid = `{"message":"ignored upstream text","family":"CONFLICT","code":"WORKER_SESSION_DIRECT_CAPACITY_EXCEEDED","details":{"host":"https://user:secret@example.com:443/path?secret=yes#fragment","capacity":2}}`

func TestDirectCapacityResponse(t *testing.T) {
	t.Parallel()
	failure, ok := Decode(strings.NewReader(valid))
	if !ok || failure.Host != "https://example.com:443" || failure.Capacity != 2 {
		t.Fatalf("safe failure=%+v valid=%t", failure, ok)
	}
	for _, raw := range []string{valid + "x", valid[:len(valid)-1], strings.Replace(valid, `"capacity":2`, `"capacity":2,"capacity":3`, 1), strings.Replace(valid, `"code":`, `"Code":`, 1), strings.Replace(valid, `"capacity":2`, `"capacity":null`, 1), strings.Replace(valid, `"family":"CONFLICT"`, `"family":"INVALID"`, 1), strings.Replace(valid, "WORKER_SESSION_DIRECT_CAPACITY_EXCEEDED", "UNKNOWN", 1), strings.Repeat(" ", 4097) + valid} {
		if _, ok := Decode(strings.NewReader(raw)); ok {
			t.Fatal("malformed capacity refusal classified")
		}
	}
}
