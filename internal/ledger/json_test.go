package ledger

import "testing"

func TestAmbiguousJSONRejected(t *testing.T) {
	for _, raw := range []string{`{"supply":"1","supply":"2"}`, `{"a":{"balance":"1","\u0062alance":"2"}}`, `{"a":[{"x":1,"x":2}]}`} {
		var value any
		if Decode([]byte(raw), &value) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{`{"a":{"balance":"1"},"b":{"balance":"2"}}`, `{"a":[1,{"b":2}],"c":null}`} {
		var value any
		if err := Decode([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
	}
}
