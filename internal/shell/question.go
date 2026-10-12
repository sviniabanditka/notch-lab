package shell

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/kit"
)

// Questions are the alerts a window draws over a dimmed backdrop
// (R-SHELL-157), not system alerts: they look like the app and show in the
// notch panel too. They wait in a queue; one shows at a time.
type Questions struct {
	// Win is the window that draws them; nil in tests, which answer with
	// the first button.
	Win  *mygo.Window
	asks []question
}

type question struct {
	message, detail string
	// buttons has the action first and Cancel last.
	buttons []string
	danger  bool
	then    func(button int)
}

// destructive are the answers the Shell draws in the danger color.
var destructive = map[string]bool{"Delete": true, "Remove": true, "Discard": true, "Reset": true, "Clear": true, "Move to Trash": true}

// Ask shows a question with buttons, the last of which cancels. then gets
// the index of the button chosen; the last one when the user dismisses
// the question. A message's first blank line starts its detail.
func (s *Questions) Ask(message string, buttons []string, then func(button int)) {
	if s.Win == nil {
		then(0)
		return
	}
	q := question{message: message, buttons: buttons, then: then, danger: destructive[buttons[0]]}
	if m, d, ok := strings.Cut(message, "\n\n"); ok {
		q.message, q.detail = m, d
	}
	s.asks = append(s.asks, q)
	s.Win.Invalidate()
}

// Confirm asks a yes-or-no question; then gets the answer.
func (s *Questions) Confirm(question, yes string, then func(ok bool)) {
	s.Ask(question, []string{yes, "Cancel"}, func(i int) { then(i == 0) })
}

// Yes runs then when the answer is yes.
func (s *Questions) Yes(question, yes string, then func()) {
	s.Confirm(question, yes, func(ok bool) {
		if ok {
			then()
		}
	})
}

// View draws the first question.
func (s *Questions) View(c *ui.Context) {
	if len(s.asks) == 0 {
		return
	}
	q := s.asks[0]
	t := c.Theme()
	answer := -1
	open := true
	kit.Modal(c, fmt.Sprint("ask", len(s.asks)), &open, func() {
		ui.Column(c).Width(360).Padding(6, 4, 2, 4).Gap(10).AlignItems(ui.Stretch).Children(func() {
			col := t.Accent
			if q.danger {
				col = t.Danger
			}
			ui.Row(c).Justify(ui.Center).Children(func() {
				icon := kit.Info
				if q.danger {
					icon = kit.Warning
				}
				kit.Tile(c, icon, col, 44)
			})
			ui.Text(c, q.message).FontWeight(600).FontSize(14).TextAlign(ui.Center)
			if q.detail != "" {
				ui.Text(c, q.detail).FontSize(12).TextColor(faint(t)).TextAlign(ui.Center)
			}
			ui.Row(c.Key("answers")).Gap(8).Padding(6, 0, 0, 0).Children(func() {
				last := len(q.buttons) - 1
				for i := last; i >= 0; i-- { // Cancel left, the action right, as macOS
					b := ui.Button(c.Key(i), q.buttons[i]).Grow(1).Basis(0).Height(30).Radius(radiusMd)
					if i == 0 {
						b.Background(col).TextColor(ui.RGB(255, 255, 255)).Border(1, ui.Transparent)
						if !q.danger {
							b.AutoFocus()
						}
					} else if i == last && q.danger {
						b.AutoFocus()
					}
					if b.Clicked() {
						answer = i
					}
				}
			})
		})
	})
	if !open && answer < 0 {
		answer = len(q.buttons) - 1
	}
	if answer >= 0 {
		s.asks = s.asks[1:]
		q.then(answer)
	}
}

// Questions are the questions of the Shell's window.
func (s *Shell) Questions() *Questions {
	s.q.Win = s.Win
	return &s.q
}

// Ask shows a question in the Shell's window; see Questions.Ask.
func (s *Shell) Ask(message string, buttons []string, then func(button int)) {
	s.q.Win = s.Win
	s.q.Ask(message, buttons, then)
}

// Confirm asks a yes-or-no question in the Shell's window.
func (s *Shell) Confirm(question, yes string, then func(ok bool)) {
	s.q.Win = s.Win
	s.q.Confirm(question, yes, then)
}

func (s *Shell) confirm(question, yes string, then func()) {
	s.q.Win = s.Win
	s.q.Yes(question, yes, then)
}
