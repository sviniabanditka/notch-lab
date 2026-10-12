package shell

import "github.com/sviniabanditka/notch-lab/internal/model"

// history is a window's History (R-SHELL-100 … R-SHELL-102) and its most
// recently shown Tabs for the Switcher (R-SHELL-082).
type history struct {
	entries []*model.Tab
	at      int
	mru     []*model.Tab
}

const (
	maxHistory = 200
	maxMRU     = 50
)

// push records a Tab shown in the focused Leaf and drops entries ahead.
func (h *history) push(t *model.Tab) {
	h.touch(t)
	if len(h.entries) > 0 && h.entries[h.at] == t {
		return
	}
	if len(h.entries) > 0 {
		h.entries = h.entries[:h.at+1]
	}
	h.entries = append(h.entries, t)
	if len(h.entries) > maxHistory {
		h.entries = h.entries[len(h.entries)-maxHistory:]
	}
	h.at = len(h.entries) - 1
}

func (h *history) touch(t *model.Tab) {
	out := []*model.Tab{t}
	for _, x := range h.mru {
		if x != t && x.Bench != nil && len(out) < maxMRU {
			out = append(out, x)
		}
	}
	h.mru = out
}

// step moves by -1 (Back) or +1 (Forward) to the next open Tab, skipping
// closed ones, and returns it or nil.
func (h *history) step(d int) *model.Tab {
	for i := h.at + d; i >= 0 && i < len(h.entries); i += d {
		if h.entries[i].Bench != nil {
			h.at = i
			h.touch(h.entries[i])
			return h.entries[i]
		}
	}
	return nil
}
