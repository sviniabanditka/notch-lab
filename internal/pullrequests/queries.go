package pullrequests

import (
	"sort"
	"strings"
	"time"
)

type login struct {
	Login    string
	Typename string `json:"__typename"`
}

type rollup struct {
	State    string
	Contexts struct {
		Nodes []struct {
			Typename   string `json:"__typename"`
			Name       string
			Status     string
			Conclusion string
			Context    string
			State      string
		}
	}
}

type lastCommit struct {
	Nodes []struct {
		Commit struct{ StatusCheckRollup *rollup }
	}
}

func (l lastCommit) rollup() *rollup {
	if len(l.Nodes) == 0 {
		return nil
	}
	return l.Nodes[0].Commit.StatusCheckRollup
}

type commentNode struct {
	Author      *login
	Body        string
	CreatedAt   time.Time
	SubmittedAt time.Time
}

func (n commentNode) comment() comment {
	c := comment{Body: strings.TrimSpace(n.Body), At: n.CreatedAt}
	if n.Author != nil {
		c.Author, c.Bot = n.Author.Login, n.Author.Typename == "Bot"
	}
	if c.At.IsZero() {
		c.At = n.SubmittedAt
	}
	return c
}

// statusState maps a roll-up's or a status context's state.
func statusState(s string) checkState {
	switch s {
	case "SUCCESS":
		return checkPassed
	case "FAILURE", "ERROR":
		return checkFailed
	case "PENDING", "EXPECTED":
		return checkPending
	}
	return checkNone
}

func runState(status, conclusion string) checkState {
	if status != "COMPLETED" {
		return checkPending
	}
	switch conclusion {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return checkPassed
	}
	return checkFailed
}

func checks(ru *rollup) []check {
	if ru == nil {
		return nil
	}
	var out []check
	for _, n := range ru.Contexts.Nodes {
		if n.Typename == "CheckRun" {
			status := strings.ToLower(n.Conclusion)
			if n.Status != "COMPLETED" {
				status = strings.ToLower(n.Status)
			}
			out = append(out, check{Name: n.Name, State: runState(n.Status, n.Conclusion), Status: status})
		} else {
			out = append(out, check{Name: n.Context, State: statusState(n.State), Status: strings.ToLower(n.State)})
		}
	}
	// Failed checks first, then those still running.
	urgency := map[checkState]int{checkFailed: 0, checkPending: 1, checkNone: 2, checkPassed: 3}
	sort.SliceStable(out, func(i, j int) bool { return urgency[out[i].State] < urgency[out[j].State] })
	return out
}
