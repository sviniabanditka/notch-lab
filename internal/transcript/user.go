package transcript

import (
	"encoding/base64"
	"strings"
)

// decodeUser reads a user record: its tool results, or else what the user
// or the system said. A meta record and a compaction summary are read for
// their branch only.
func decodeUser(r *rawRecord, rc *rec) {
	if r.Message == nil || r.IsMeta || r.IsCompactSummary {
		return
	}
	var text strings.Builder
	var images []Image
	for _, b := range blocks(r.Message.Content) {
		switch b.Type {
		case "tool_result":
			rc.results = append(rc.results, decodeResult(b, r.ToolUseResult))
		case "text":
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(b.Text)
		case "image":
			if img, ok := decodeImage(b); ok {
				images = append(images, img)
			}
		}
	}
	if len(rc.results) > 0 {
		return
	}
	t := strings.TrimSpace(text.String())
	if t == "" && len(images) == 0 {
		return
	}
	classifyUser(r.PromptSource, t, images, rc)
}

// classifyUser sorts a user row that is no tool result: an interrupt, a
// command or its output, a notice, or a prompt.
func classifyUser(source, t string, images []Image, rc *rec) {
	id := rc.uuid
	switch {
	case strings.Contains(t, "[Request interrupted by user"):
		rc.items = append(rc.items, Item{ID: id, Kind: KindInterrupted})
		rc.interrupt = true
	case source == "system" || source == "sdk":
		rc.task = tag(t, "task-id")
		rc.items = append(rc.items, Item{ID: id, Kind: KindNotice, Text: noticeLine(t)})
	case source != "" && source != "typed" && source != "suggestion_accepted" && source != "queued":
	case strings.HasPrefix(t, "<command-name>") || strings.HasPrefix(t, "<command-message>"):
		rc.rewinds = true
		rc.items = append(rc.items, Item{ID: id, Kind: KindSlashCommand, Command: commandOf(t)})
	case strings.HasPrefix(t, "<bash-input>"):
		rc.rewinds = true
		rc.items = append(rc.items, Item{ID: id, Kind: KindSlashCommand,
			Command: &SlashCommand{Name: tag(t, "bash-input"), IsShell: true}})
	case strings.HasPrefix(t, "<local-command-stdout>"):
		rc.isOutput, rc.output = true, tag(t, "local-command-stdout")
	case strings.HasPrefix(t, "<bash-stdout>"):
		rc.isOutput, rc.output = true, strings.TrimSpace(tag(t, "bash-stdout")+"\n"+tag(t, "bash-stderr"))
	case strings.HasPrefix(t, "<"):
	default:
		rc.rewinds = true
		rc.items = append(rc.items, Item{ID: id, Kind: KindPrompt, Text: t, Images: images, Queued: source == "queued"})
	}
}

// commandOf reads a slash command row: its name without the slash, and its
// arguments.
func commandOf(t string) *SlashCommand {
	name := tag(t, "command-name")
	if name == "" {
		name = tag(t, "command-message")
	}
	return &SlashCommand{Name: strings.TrimPrefix(name, "/"), Args: tag(t, "command-args")}
}

// noticeLine is a notice's summary, or its first line.
func noticeLine(t string) string {
	if s := tag(t, "summary"); s != "" {
		return s
	}
	if strings.HasPrefix(t, "<") {
		return "Task notification"
	}
	line, _, _ := strings.Cut(t, "\n")
	return line
}

// recapText strips the recap prefix and the setting hint Claude appends.
func recapText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "※ recap:")
	s = strings.TrimSuffix(strings.TrimSpace(s), "(disable recaps in /config)")
	return strings.TrimSpace(s)
}

// decodeImage reads an image block that carries its bytes in base64.
func decodeImage(b rawBlock) (Image, bool) {
	if b.Source == nil || b.Source.Type != "base64" {
		return Image{}, false
	}
	data, err := base64.StdEncoding.DecodeString(b.Source.Data)
	if err != nil {
		return Image{}, false
	}
	return Image{MediaType: b.Source.MediaType, Data: data}, true
}
