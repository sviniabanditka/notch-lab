//go:build darwin

package webkit

import (
	"encoding/json"
	"testing"
)

// A socket frame's control characters (SignalR's record separator 0x1e)
// survive the trip through NSJSONSerialization to a port and back.
func TestJSONKeepsRecordSeparator(t *testing.T) {
	load()
	in := map[string]any{"text": "{}\x1e"}
	b, _ := json.Marshal(in)
	var out map[string]any
	if err := json.Unmarshal(toJSON(fromJSON(b)), &out); err != nil {
		t.Fatalf("%s %v", toJSON(fromJSON(b)), err)
	}
	if got := out["text"]; got != "{}\x1e" {
		t.Fatalf("got %q", got)
	}
}
