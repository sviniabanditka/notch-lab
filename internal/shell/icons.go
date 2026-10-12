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
	iconSettings = lucide(`<path d="M20 7h-9"/><path d="M14 17H5"/><circle cx="17" cy="17" r="3"/><circle cx="7" cy="7" r="3"/>`)
	iconClose    = lucide(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconPin      = lucide(`<line x1="12" x2="12" y1="17" y2="22"/><path d="M5 17h14v-1.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V6h1a2 2 0 0 0 0-4H8a2 2 0 0 0 0 4h1v4.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24Z"/>`)
)

// TypeIcon is the default icon of a Bench type (R-SHELL-043).
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
