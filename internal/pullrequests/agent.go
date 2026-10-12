package pullrequests

import (
	"fmt"
	"strconv"
	"strings"
)

// forAgent writes the unresolved review threads of d as a prompt for an
// agent, each with its file and line and its comments in order; "" when
// every thread is resolved.
func forAgent(d *detail) string {
	var b strings.Builder
	n := 0
	for _, t := range d.Threads {
		if t.Resolved {
			continue
		}
		n++
		if n == 1 {
			fmt.Fprintf(&b, "Address the unresolved review comments on pull request #%d (%s):\n", d.Number, d.Title)
		}
		where := t.Path
		if t.Line > 0 {
			where += ":" + strconv.Itoa(t.Line)
		}
		if t.Outdated {
			where += " (outdated)"
		}
		fmt.Fprintf(&b, "\n%d. %s\n", n, where)
		for _, c := range t.Comments {
			fmt.Fprintf(&b, "%s: %s\n", c.Author, strings.TrimSpace(c.Body))
		}
	}
	return b.String()
}
