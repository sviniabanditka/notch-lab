package filetree

import (
	"slices"
	"unicode"
	"unicode/utf8"
)

// sortEntries puts folders, links to folders included, before files, each
// in natural order ignoring case: "file2" before "file10".
func sortEntries(es []entry) {
	slices.SortFunc(es, func(a, b entry) int {
		if a.isDir != b.isDir {
			if a.isDir {
				return -1
			}
			return 1
		}
		if n := naturalCompare(a.name, b.name); n != 0 {
			return n
		}
		// Names that differ only in case keep one order between reads.
		switch {
		case a.name < b.name:
			return -1
		case a.name > b.name:
			return 1
		}
		return 0
	})
}

// naturalCompare compares runs of digits by their value and other runes
// by their lower case.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			da, db := digitRun(a), digitRun(b)
			if n := compareNumbers(a[:da], b[:db]); n != 0 {
				return n
			}
			a, b = a[da:], b[db:]
			continue
		}
		ra, sa := utf8.DecodeRuneInString(a)
		rb, sb := utf8.DecodeRuneInString(b)
		if la, lb := unicode.ToLower(ra), unicode.ToLower(rb); la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		a, b = a[sa:], b[sb:]
	}
	return len(a) - len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func digitRun(s string) int {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return i
}

// compareNumbers compares two runs of digits by value, of any length.
func compareNumbers(a, b string) int {
	ta, tb := trimZeros(a), trimZeros(b)
	if len(ta) != len(tb) {
		return len(ta) - len(tb)
	}
	switch {
	case ta < tb:
		return -1
	case ta > tb:
		return 1
	}
	return 0
}

func trimZeros(s string) string {
	for len(s) > 1 && s[0] == '0' {
		s = s[1:]
	}
	return s
}
