package perception

import (
	"bytes"
	"regexp"
	"strings"
	"unicode"
	"unsafe"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
)

var markerRe = regexp.MustCompile(`(--More--|\(end\)|\(\d+ of \d+\))`)

// markerFindAll mirrors markerRe.findall(s): non-overlapping leftmost matches of
// (--More--|\(end\)|\(\d+ of \d+\)), scanned by hand (hot path).
func markerFindAll(s string) []string {
	var r []string
	for _, sp := range markerSpans(s, nil) {
		r = append(r, s[sp[0]:sp[1]])
	}
	return r
}

// markerSpans appends the [start, end) of each markerFindAll match to dst (s is not kept).
func markerSpans(s string, dst [][2]int) [][2]int {
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	more := -2 // absolute position of the next "--More--" at or after i (-1: none left, -2: not searched yet)
	for i := 0; i < len(s); {
		// jump to the next candidate start: '(' or the literal "--More--"
		p := strings.IndexByte(s[i:], '(')
		if more != -1 && more < i {
			if more = strings.Index(s[i:], "--More--"); more >= 0 {
				more += i
			}
		}
		q := more
		if q >= 0 {
			q -= i
		}
		if p < 0 && q < 0 {
			break
		}
		if p < 0 || (q >= 0 && q < p) {
			dst = append(dst, [2]int{i + q, i + q + 8})
			i += q + 8
			continue
		}
		i += p
		if strings.HasPrefix(s[i:], "(end)") {
			dst = append(dst, [2]int{i, i + 5})
			i += 5
			continue
		}
		j := i + 1
		for j < len(s) && isDigit(s[j]) {
			j++
		}
		if j > i+1 && strings.HasPrefix(s[j:], " of ") {
			k := j + 4
			k0 := k
			for k < len(s) && isDigit(s[k]) {
				k++
			}
			if k > k0 && k < len(s) && s[k] == ')' {
				dst = append(dst, [2]int{i, k + 1})
				i = k + 1
				continue
			}
		}
		i++
	}
	return dst
}

// NoMarker reports whether text has no --More--, (end) or (n of m) marker (without copying it to a string).
func NoMarker(text []byte) bool {
	var buf [4][2]int
	return len(text) == 0 || len(markerSpans(unsafe.String(&text[0], len(text)), buf[:0])) == 0
}

func FindMarker(joined string, lines []string) (pos Pos, ok bool, marker string) {
	switch len(markerFindAll(joined)) {
	case 0:
		return // a marker inside one line would be found in the joined text as well
	case 1:
	default:
		Raise("ValueError", "Too many markers")
	}
	for i, line := range lines {
		res := markerFindAll(line)
		if len(res) > 0 {
			Assert(len(res) == 1)
			j := strings.Index(line, res[0])
			pos, ok, marker = Pos{i, j}, true, res[0]
			break
		}
	}
	if ok && pos.X == 1 {
		pos.X = 0
	}
	return
}

func AlnumOnly(s string) string {
	var b strings.Builder
	for _, c := range s {
		if unicode.IsLetter(c) || unicode.IsDigit(c) || unicode.IsNumber(c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func TtyContains(obs *Obs, sub string) bool {
	flat := unsafe.Slice(&obs.TtyChars[0][0], 24*80)
	return bytes.Contains(flat, []byte(sub))
}

// EditDistance mirrors nltk.edit_distance (Levenshtein, substitution cost 1).
func EditDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	prev := make([]int, len(r2)+1)
	cur := make([]int, len(r2)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(r1); i++ {
		cur[0] = i
		for j := 1; j <= len(r2); j++ {
			c := 1
			if r1[i-1] == r2[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev, cur = cur, prev
	}
	return prev[len(r2)]
}

// CleanStripRow is PyStrip(CleanRow(row)) (the message line: text, then NULs). The blank ends are skipped
// on the bytes; when an end is not ASCII (it could be a Unicode space), it takes the general way.
func CleanStripRow(row []uint8) string {
	i, j := 0, len(row)
	for i < j && asciiBlank(row[i]) {
		i++
	}
	for j > i && asciiBlank(row[j-1]) {
		j--
	}
	if i < j && (row[i] >= 0x80 || row[j-1] >= 0x80) {
		return PyStrip(CleanRow(row))
	}
	var buf [256]byte
	return string(AppendCleanRow(buf[:0], row[i:j]))
}

// asciiBlank: NUL (a space after cleaning), '\n' (dropped) or an ASCII character str.strip() removes.
func asciiBlank(c byte) bool {
	return c == 0 || c == ' ' || c >= '\t' && c <= '\r' || c >= 0x1c && c <= 0x1f
}

// cleanRow mirrors bytes(row).decode().replace('\0', ' ').replace('\n', ”).
func CleanRow(row []uint8) string {
	var buf [256]byte
	return string(AppendCleanRow(buf[:0], row))
}

func AppendCleanRow(dst []byte, row []uint8) []byte {
	if bytes.IndexByte(row, 0) < 0 && bytes.IndexByte(row, '\n') < 0 {
		return append(dst, row...) // the usual tty row: nothing to replace
	}
	for _, c := range row {
		switch c {
		case 0:
			dst = append(dst, ' ')
		case '\n':
		default:
			dst = append(dst, c)
		}
	}
	return dst
}
