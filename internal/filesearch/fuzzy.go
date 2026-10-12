package filesearch

// The matcher scores a path the way fzf's v2 algorithm does: the query's
// bytes in order, case ignored, each match worth scoreMatch plus a bonus
// for where it lands (after a slash, after another separator, at a
// camelCase hump), runs carrying the bonus of their first byte, gaps
// costing gapStart then gapExtension a byte. Matches in the last path
// segment earn bonusBasename on top, so a file's name beats a folder
// deep up its path. Non-ASCII bytes match only themselves.

const (
	scoreMatch        = 16
	gapStart          = 3
	gapExtension      = 1
	bonusSlash        = 9
	bonusBoundary     = 8
	bonusCamel        = 7
	bonusConsecutive  = 4
	bonusFirstFactor  = 2
	bonusBasename     = 3
	minScore          = -1 << 30
	maxQuery          = 64
	maxScoredPathSize = 1024
)

func lowerByte(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + 'a' - 'A'
	}
	return b
}

func lowerQuery(q string) []byte {
	out := make([]byte, 0, len(q))
	for i := 0; i < len(q) && len(out) < maxQuery; i++ {
		if q[i] != ' ' {
			out = append(out, lowerByte(q[i]))
		}
	}
	return out
}

func isWordByte(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9' || b >= 0x80
}

// bonusAt is what a match at text[j] earns for where it lands.
func bonusAt(text []byte, j int) int {
	if j == 0 {
		return bonusSlash
	}
	prev, cur := text[j-1], text[j]
	switch {
	case prev == '/':
		return bonusSlash
	case !isWordByte(prev) && isWordByte(cur):
		return bonusBoundary
	case 'a' <= prev && prev <= 'z' && 'A' <= cur && cur <= 'Z',
		!('0' <= prev && prev <= '9') && '0' <= cur && cur <= '9':
		return bonusCamel
	}
	return 0
}

func baseStart(text []byte) int {
	for j := len(text) - 1; j >= 0; j-- {
		if text[j] == '/' {
			return j + 1
		}
	}
	return 0
}

// window is where in text the query can match: from the first byte that
// starts a match to the last that ends one; ok is false when it cannot.
func window(q, text []byte) (from, to int, ok bool) {
	k := 0
	from = -1
	for j := 0; j < len(text); j++ {
		if lowerByte(text[j]) == q[k] {
			if k == 0 {
				from = j
			}
			if k++; k == len(q) {
				break
			}
		}
	}
	if k < len(q) {
		return 0, 0, false
	}
	k = len(q) - 1
	for j := len(text) - 1; j >= from; j-- {
		if lowerByte(text[j]) == q[k] {
			if k == len(q)-1 {
				to = j + 1
			}
			if k--; k < 0 {
				return from, to, true
			}
		}
	}
	return 0, 0, false
}

// scorer holds the rows a score reuses; one per goroutine.
type scorer struct {
	h, run, prevH, prevRun []int32
	// trace, when set, keeps every row for positions.
	trace bool
	full  [][]int32
	diag  [][]bool
}

func (s *scorer) grow(n int) {
	if cap(s.h) < n {
		s.h, s.run = make([]int32, n), make([]int32, n)
		s.prevH, s.prevRun = make([]int32, n), make([]int32, n)
	}
	s.h, s.run = s.h[:n], s.run[:n]
	s.prevH, s.prevRun = s.prevH[:n], s.prevRun[:n]
}

// score is how well q, lowered, matches text, and false when it does not.
func (s *scorer) score(q, text []byte) (int, bool) {
	if len(q) == 0 {
		return 0, true
	}
	if len(text) > maxScoredPathSize {
		text = text[len(text)-maxScoredPathSize:]
	}
	from, to, ok := window(q, text)
	if !ok {
		return 0, false
	}
	best, _ := s.fill(q, text, from, to)
	return int(best), true
}

// fill runs the alignment over text[from:to]; h[i][j] is the best score
// with q[:i+1] matched and q[i] at from+j. With s.trace it keeps every
// row in s.full and returns where the best ends.
func (s *scorer) fill(q, text []byte, from, to int) (int32, int) {
	n := to - from
	s.grow(n)
	base := baseStart(text)
	if s.trace {
		s.full, s.diag = s.full[:0], s.diag[:0]
	}
	var diagRow []bool
	best, bestAt := int32(minScore), -1
	for i := range q {
		h, run := s.h, s.run
		gap := int32(minScore) // best of the previous row before j-1, less the gap
		if s.trace {
			diagRow = make([]bool, n)
		}
		for j := 0; j < n; j++ {
			at := from + j
			if i > 0 && j >= 2 {
				gap = max(gap-gapExtension, s.prevH[j-2]-gapStart)
			}
			h[j], run[j] = minScore, 0
			if lowerByte(text[at]) != q[i] {
				continue
			}
			b := int32(bonusAt(text, at))
			extra := int32(0)
			if at >= base {
				extra = bonusBasename
			}
			if i == 0 {
				h[j], run[j] = scoreMatch+b*bonusFirstFactor+extra, b
				continue
			}
			var diag int32 = minScore
			if j > 0 && s.prevH[j-1] > minScore/2 {
				carry := max(b, s.prevRun[j-1], bonusConsecutive)
				diag = s.prevH[j-1] + scoreMatch + carry + extra
				h[j], run[j] = diag, max(s.prevRun[j-1], bonusConsecutive)
				if diagRow != nil {
					diagRow[j] = true
				}
			}
			if gap > minScore/2 {
				if g := gap + scoreMatch + b + extra; g > h[j] {
					h[j], run[j] = g, b
					if diagRow != nil {
						diagRow[j] = false
					}
				}
			}
		}
		if s.trace {
			s.full = append(s.full, append([]int32(nil), h...))
			s.diag = append(s.diag, diagRow)
		}
		s.h, s.prevH = s.prevH, h
		s.run, s.prevRun = s.prevRun, run
	}
	for j := 0; j < n; j++ {
		if s.prevH[j] > best {
			best, bestAt = s.prevH[j], j
		}
	}
	return best, bestAt
}

// positions are the bytes of text the best match of q takes, ascending,
// or nil when q does not match.
func positions(query string, text string) []int {
	q := lowerQuery(query)
	t := []byte(text)
	off := 0
	if len(t) > maxScoredPathSize {
		off = len(t) - maxScoredPathSize
		t = t[off:]
	}
	if len(q) == 0 {
		return nil
	}
	from, to, ok := window(q, t)
	if !ok {
		return nil
	}
	s := scorer{trace: true}
	_, j := s.fill(q, t, from, to)
	out := make([]int, len(q))
	for i := len(q) - 1; i >= 0; i-- {
		out[i] = off + from + j
		if i == 0 {
			break
		}
		// Step to the cell of q[i-1] this one came from: the diagonal,
		// or the best one before it less the gap.
		if s.diag[i][j] {
			j--
			continue
		}
		prev := s.full[i-1]
		bestK, bestV := -1, int32(minScore)
		for k := j - 2; k >= 0; k-- {
			if v := prev[k] - gapStart - int32(j-k-2)*gapExtension; v > bestV {
				bestK, bestV = k, v
			}
		}
		if bestK < 0 {
			bestK = j - 1
		}
		j = bestK
	}
	return out
}
