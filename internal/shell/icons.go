package shell

import (
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
)

// Icons are Lucide shapes (ISC license, see NOTICE), drawn in the text color.
func lucide(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var (
	iconTerminal = lucide(`<polyline points="4 17 10 11 4 5"/><line x1="12" x2="20" y1="19" y2="19"/>`)
	iconBrowser  = lucide(`<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>`)
	iconEditor   = lucide(`<polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/>`)
	iconFiles    = lucide(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
	iconNotes    = lucide(`<path d="M15.5 3H5a2 2 0 0 0-2 2v14c0 1.1.9 2 2 2h14a2 2 0 0 0 2-2V8.5L15.5 3Z"/><path d="M15 3v6h6"/>`)
	iconHTTP     = lucide(`<path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/>`)
	iconDB       = lucide(`<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5V19A9 3 0 0 0 21 19V5"/><path d="M3 12A9 3 0 0 0 21 12"/>`)
	iconSSH      = lucide(`<rect width="20" height="8" x="2" y="2" rx="2"/><rect width="20" height="8" x="2" y="14" rx="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/>`)
	iconPR       = lucide(`<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3a2 2 0 0 1 2 2v7"/><line x1="6" x2="6" y1="9" y2="21"/>`)
	iconAgents   = lucide(`<path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/>`)
	iconPlus     = lucide(`<path d="M5 12h14"/><path d="M12 5v14"/>`)
	iconSidebar  = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>`)
	iconSplitR   = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M12 3v18"/>`)
	iconSplitD   = lucide(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 12h18"/>`)
	iconZoomIn   = lucide(`<polyline points="15 3 21 3 21 9"/><polyline points="9 21 3 21 3 15"/><line x1="21" x2="14" y1="3" y2="10"/><line x1="3" x2="10" y1="21" y2="14"/>`)
	iconZoomOut  = lucide(`<polyline points="4 14 10 14 10 20"/><polyline points="20 10 14 10 14 4"/><line x1="14" x2="21" y1="10" y2="3"/><line x1="3" x2="10" y1="21" y2="14"/>`)
	iconGear     = lucide(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`)
	iconContrast = lucide(`<circle cx="12" cy="12" r="10"/><path d="M12 18a6 6 0 0 0 0-12v12z"/>`)
	iconKeyboard = lucide(`<rect width="20" height="16" x="2" y="4" rx="2"/><path d="M6 8h.01"/><path d="M10 8h.01"/><path d="M14 8h.01"/><path d="M18 8h.01"/><path d="M8 12h.01"/><path d="M12 12h.01"/><path d="M16 12h.01"/><path d="M7 16h10"/>`)
	iconPuzzle   = lucide(`<path d="M15.39 4.39a1 1 0 0 0 1.68-.474 2.5 2.5 0 1 1 3.014 3.015 1 1 0 0 0-.474 1.68l1.683 1.682a2.414 2.414 0 0 1 0 3.414L19.61 15.39a1 1 0 0 1-1.68-.474 2.5 2.5 0 1 0-3.014 3.015 1 1 0 0 1 .474 1.68l-1.683 1.682a2.414 2.414 0 0 1-3.414 0L8.61 19.61a1 1 0 0 0-1.68.474 2.5 2.5 0 1 1-3.014-3.015 1 1 0 0 0 .474-1.68l-1.683-1.682a2.414 2.414 0 0 1 0-3.414L4.39 8.61a1 1 0 0 1 1.68.474 2.5 2.5 0 1 0 3.014-3.015 1 1 0 0 1-.474-1.68l1.683-1.682a2.414 2.414 0 0 1 3.414 0z"/>`)
	iconSearch   = lucide(`<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>`)
	iconSettings = lucide(`<path d="M20 7h-9"/><path d="M14 17H5"/><circle cx="17" cy="17" r="3"/><circle cx="7" cy="7" r="3"/>`)
	iconClose    = lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconPin      = lucide(`<line x1="12" x2="12" y1="17" y2="22"/><path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z"/>`)
)

// TypeIcon is the default icon of a Bench type (R-SHELL-043).
// TypeColor is a Bench type's own color, for its icon tile.
func TypeColor(t model.BenchType) ui.Color {
	switch t {
	case model.Terminal:
		return ui.Hex("#3a3a3c")
	case model.Browser:
		return ui.Hex("#0a84ff")
	case model.Editor:
		return ui.Hex("#5e5ce6")
	case model.Files:
		return ui.Hex("#64a8f0")
	case model.Notes:
		return ui.Hex("#ff9f0a")
	case model.HTTP:
		return ui.Hex("#30b0c7")
	case model.DB:
		return ui.Hex("#34c759")
	case model.SSH:
		return ui.Hex("#8e8e93")
	case model.PullRequests:
		return ui.Hex("#bf5af2")
	case model.Agents:
		return ui.Hex("#ff6482")
	}
	return ui.Hex("#8e8e93")
}

func TypeIcon(t model.BenchType) *ui.SVG {
	switch t {
	case model.Terminal:
		return iconTerminal
	case model.Browser:
		return iconBrowser
	case model.Editor:
		return iconEditor
	case model.Files:
		return iconFiles
	case model.Notes:
		return iconNotes
	case model.HTTP:
		return iconHTTP
	case model.DB:
		return iconDB
	case model.SSH:
		return iconSSH
	case model.PullRequests:
		return iconPR
	}
	return iconAgents
}

// arrives names the milestone that brings a Bench type (docs/milestones.md).
func arrives(t model.BenchType) string {
	switch t {
	case model.Terminal:
		return "M2"
	case model.Editor, model.Files:
		return "M3"
	case model.Browser:
		return "M4"
	case model.HTTP:
		return "M7"
	case model.DB:
		return "M8"
	case model.SSH:
		return "M9"
	case model.Notes:
		return "M10"
	}
	return "M11"
}
