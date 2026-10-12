package chat

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

var sendBase = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

// prompts is a conversation of typed prompts, one per id at sendBase plus
// its offset.
func prompts(at map[string]time.Duration) *turnList {
	var items []transcript.Item
	for _, id := range []string{"terminal", "r1", "r2", "r3"} {
		if d, ok := at[id]; ok {
			items = append(items, transcript.Item{ID: id, Kind: transcript.KindPrompt, At: sendBase.Add(d), Text: "hi"})
		}
	}
	return group(transcript.Conversation{Items: items})
}

func sending(p *paneChat, after time.Duration) *pending {
	p.sent++
	pd := &pending{it: &item{kind: itemPending, key: "pending/" + strconv.Itoa(p.sent)}, sentAt: sendBase.Add(after),
		timer: time.NewTimer(time.Hour)}
	p.pending = append(p.pending, pd)
	return pd
}

func pendingKeys(p *paneChat) []string {
	out := []string{}
	for _, pd := range p.pending {
		out = append(out, pd.it.key)
	}
	return out
}

func TestPendingPairing(t *testing.T) {
	t.Run("oldest prompt takes the oldest record", func(t *testing.T) {
		p := &paneChat{claimed: map[string]bool{}}
		sending(p, 0)
		sending(p, 5*time.Second)
		// Paired newest-first, the 5.5 s record would take the first
		// prompt and leave the 0.4 s one too old for the second.
		p.turns = prompts(map[string]time.Duration{"r1": 400 * time.Millisecond, "r2": 5500 * time.Millisecond})
		p.settle()
		if got := pendingKeys(p); len(got) != 0 {
			t.Fatalf("still pending %q, want none", got)
		}
	})

	t.Run("a record older than the send claims nothing", func(t *testing.T) {
		p := &paneChat{claimed: map[string]bool{}}
		sending(p, 10*time.Second)
		p.turns = prompts(map[string]time.Duration{"terminal": 8 * time.Second})
		p.settle()
		if got, want := pendingKeys(p), []string{"pending/1"}; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("pending %q, want %q: a prompt typed in the terminal took the app's", got, want)
		}
	})

	t.Run("a record claims one prompt only", func(t *testing.T) {
		p := &paneChat{claimed: map[string]bool{}}
		sending(p, 0)
		p.turns = prompts(map[string]time.Duration{"r1": 300 * time.Millisecond})
		p.settle()
		// Sent within the second after r1, which is already claimed.
		sending(p, 900*time.Millisecond)
		p.settle()
		if got, want := pendingKeys(p), []string{"pending/2"}; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("pending %q, want %q", got, want)
		}
		p.turns = prompts(map[string]time.Duration{"r1": 300 * time.Millisecond, "r2": 1200 * time.Millisecond})
		p.settle()
		if got := pendingKeys(p); len(got) != 0 {
			t.Fatalf("still pending %q after its record came, want none", got)
		}
	})
}
