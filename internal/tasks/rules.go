package tasks

import (
	"strings"
	"unicode"

	"github.com/sviniabanditka/notch-lab/internal/session"
)

// claudeKind is herdr's name for Claude Code, in agent.get and agent.start.
const claudeKind = "claude"

// claudeArgs is what follows claude on the new agent's command line. A
// fork resumes a copy of the source's conversation under a new session
// id; the prompt goes later by agent.prompt, as herdr refuses newlines
// in arguments.
func claudeArgs(fork bool, sessionID string, skipPermissions bool) []string {
	var args []string
	if fork {
		args = append(args, "--resume", sessionID, "--fork-session")
	}
	if skipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
}

// forkBlocked is why the source agent cannot be forked now, or "". A
// working or blocked agent is mid-turn: its copy would end in a
// half-finished tool call.
func forkBlocked(kind, sessionID string, status session.AgentStatus) string {
	switch {
	case kind != claudeKind:
		return "Fork works with Claude agents only"
	case sessionID == "":
		return "herdr doesn't know this agent's session; install herdr's Claude integration"
	case status == session.AgentWorking:
		return "The agent is working; fork it once its turn ends"
	case status == session.AgentBlocked:
		return "The agent waits for you; fork it once you've answered"
	}
	return ""
}

const (
	labelMax  = 30
	branchMax = 40
)

// tabLabel is the prompt's first labelMax characters, its spaces and
// lines collapsed, cut at a word when one ends in the second half.
func tabLabel(prompt string) string {
	r := []rune(strings.Join(strings.Fields(prompt), " "))
	if len(r) <= labelMax {
		return string(r)
	}
	cut := r[:labelMax]
	for i := len(cut) - 1; i >= labelMax/2; i-- {
		if cut[i] == ' ' {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRight(string(cut), " ,.;:-") + "…"
}

// branchName is task/ and the prompt as a slug: lower-case ASCII words,
// Cyrillic transliterated, joined by dashes, cut at a word within
// branchMax. herdr names the worktree's folder after the branch in ASCII
// only, so a slug of other letters would collide; a prompt without
// ASCII-able words gets fallback.
func branchName(prompt, fallback string) string {
	var b strings.Builder
	dash := false
	for _, ch := range strings.ToLower(prompt) {
		s, ok := cyrillic[ch]
		if ch < unicode.MaxASCII && (unicode.IsLetter(ch) || unicode.IsDigit(ch)) {
			s, ok = string(ch), true
		}
		if !ok {
			dash = b.Len() > 0
			continue
		}
		if s == "" {
			continue // a sign or an apostrophe inside a word
		}
		if dash {
			b.WriteByte('-')
			dash = false
		}
		b.WriteString(s)
	}
	slug := b.String()
	if len(slug) > branchMax {
		slug = slug[:branchMax]
		if i := strings.LastIndexByte(slug, '-'); i >= branchMax/2 {
			slug = slug[:i]
		}
		slug = strings.TrimRight(slug, "-")
	}
	if slug == "" {
		slug = fallback
	}
	return "task/" + slug
}

// cyrillic transliterates Russian and Ukrainian letters, lower case;
// the signs and apostrophes, which stay inside words, go silent.
var cyrillic = map[rune]string{
	'\'': "", '’': "",
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'ґ': "g", 'д': "d", 'е': "e", 'є': "ye", 'ё': "yo",
	'ж': "zh", 'з': "z", 'и': "i", 'і': "i", 'ї': "yi", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh",
	'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu",
	'я': "ya",
}
