package shell

import (
	"strings"
)

// match is a fuzzy match of a query in a text (R-SHELL-083): every query
// rune in order, case ignored. prefix tells an exact prefix; gaps counts
// runes skipped between matched ones; pos are the matched rune indexes.
type match struct {
	ok     bool
	prefix bool
	gaps   int
	pos    []int
}

func fuzzy(query, text string) match {
	q := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(q) == 0 {
		return match{ok: true}
	}
	lt := strings.ToLower(text)
	if strings.HasPrefix(lt, string(q)) {
		pos := make([]int, len(q))
		for i := range pos {
			pos[i] = i
		}
		return match{ok: true, prefix: true, pos: pos}
	}
	m := match{pos: make([]int, 0, len(q))}
	qi, last := 0, -1
	for i, r := range []rune(lt) {
		if qi < len(q) && r == q[qi] {
			if last >= 0 {
				m.gaps += i - last - 1
			}
			m.pos = append(m.pos, i)
			last = i
			qi++
		}
	}
	m.ok = qi == len(q)
	return m
}

// better orders matches: prefix first, then fewer gaps.
func (m match) better(o match) bool {
	if m.prefix != o.prefix {
		return m.prefix
	}
	return m.gaps < o.gaps
}
