package editor

import (
	"encoding/json"

	"github.com/sviniabanditka/notch-lab/internal/model"
)

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

// setBenchKey sets one key of a Bench's settings JSON.
func setBenchKey(bench *model.Bench, key string, v any) {
	var s map[string]any
	_ = json.Unmarshal([]byte(bench.Settings), &s)
	if s == nil {
		s = map[string]any{}
	}
	s[key] = v
	raw, _ := json.Marshal(s)
	bench.Settings = string(raw)
}
