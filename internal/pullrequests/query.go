package pullrequests

import "strings"

// inboxSearch is one named search of the inbox document; a scope may
// merge several (Issues: assigned to the user, and opened by them).
type inboxSearch struct {
	alias string
	scope scope
	query string
}

// inboxSearches are the searches one inbox refresh runs, limited to only
// when it is a repo, across every repo the user can see otherwise.
func inboxSearches(only repo) []inboxSearch {
	out := []inboxSearch{
		{"review", reviewRequested, "is:pr is:open review-requested:@me"},
		{"mine", authored, "is:pr is:open author:@me"},
		{"involved", involved, "is:pr is:open involves:@me -author:@me"},
		{"assigned", issues, "is:issue is:open assignee:@me"},
		{"opened", issues, "is:issue is:open author:@me"},
	}
	for i := range out {
		out[i].query = finishQuery(out[i].query, only)
	}
	return out
}

// searchQuery is the GitHub search Enter runs for text: open pull requests
// and issues in only, or across repos those involving the user, holding
// text as one phrase so that qualifiers typed in it stay words.
func searchQuery(text string, only repo) string {
	phrase := quotePhrase(text)
	if phrase == "" {
		return ""
	}
	q := "is:open"
	if only.owner == "" {
		q += " involves:@me"
	}
	return finishQuery(q+" "+phrase, only)
}

func finishQuery(q string, only repo) string {
	q += " archived:false"
	if only.owner != "" {
		q += " repo:" + only.String()
	}
	return q + " sort:updated-desc"
}

// maxPhrase is GitHub's limit on a search's free text.
const maxPhrase = 256

// quotePhrase is text as one quoted search phrase. GitHub search has no
// escape inside quotes, so a quote or backslash in text becomes a space
// rather than ending the phrase early.
func quotePhrase(text string) string {
	text = strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	if r := []rune(text); len(r) > maxPhrase {
		text = strings.TrimSpace(string(r[:maxPhrase]))
	}
	return `"` + text + `"`
}
