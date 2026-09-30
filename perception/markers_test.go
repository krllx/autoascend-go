package perception

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	. "aago/grid"
	. "aago/internal/pyrt"
)

func TestMarkerFindAllMatchesRegex(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pieces := []string{"--More--", "-More--", "(end)", "(end", "(1 of 2)", "(12 of 3)", "( of 2)", "(1 of )", "(1of 2)",
		"((3 of 4)", "(3 of 4))", " of ", "a", "(", ")", "-", "--", "5", " ", "(1 of 2(3 of 4)", "--More--(end)"}
	for n := 0; n < 200000; n++ {
		s := ""
		for k := rng.Intn(6); k >= 0; k-- {
			s += pieces[rng.Intn(len(pieces))]
		}
		want := markerRe.FindAllString(s, -1)
		got := markerFindAll(s)
		if len(want) == 0 && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("%q: regex %q, hand %q", s, want, got)
		}
	}
}

func TestAppendCleanRow(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	alphabet := []byte{0, '\n', ' ', 'a', '-', '('}
	for n := 0; n < 20000; n++ {
		row := make([]byte, rng.Intn(90))
		for i := range row {
			if rng.Intn(3) == 0 {
				row[i] = alphabet[rng.Intn(len(alphabet))]
			} else {
				row[i] = 'x'
			}
		}
		want := strings.ReplaceAll(strings.ReplaceAll(string(row), "\x00", " "), "\n", "")
		if got := string(AppendCleanRow([]byte("pre"), row)); got != "pre"+want {
			t.Fatalf("%q: %q, want %q", row, got, "pre"+want)
		}
	}
}

func TestCleanStripRow(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	pieces := []string{"\x00", "\n", " ", "\t", "\r", "\x0b", "\x0c", "\x1c", "\x1f", "\x1b", "a", "You see", "--More--",
		"\xc2\xa0", "\xe3\x80\x80", "\xc2\x85", "\xff", "\xc2", "é", "\x00\x00\x00\x00"}
	for n := 0; n < 100000; n++ {
		var b []byte
		for k := rng.Intn(8); k >= 0; k-- {
			b = append(b, pieces[rng.Intn(len(pieces))]...)
		}
		if want, got := PyStrip(CleanRow(b)), CleanStripRow(b); want != got {
			t.Fatalf("%q: %q, want %q", b, got, want)
		}
	}
}

// findMarkerAllLines is FindMarker without the early exit: every line is searched.
func findMarkerAllLines(joined string, lines []string) (pos Pos, ok bool, marker string) {
	if len(markerFindAll(joined)) > 1 {
		return Pos{-1, -1}, false, "raise"
	}
	for i, line := range lines {
		if res := markerFindAll(line); len(res) > 0 {
			pos, ok, marker = Pos{i, strings.Index(line, res[0])}, true, res[0]
			break
		}
	}
	if ok && pos.X == 1 {
		pos.X = 0
	}
	return
}

func TestFindMarkerEarlyExit(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	pieces := []string{"--More--", "--Mo", "re--", "(end)", "(en", "d)", "(1 of 2)", "(1", "of 2)", "(1 of", "2)", "a", "(", ")",
		"HP:14(14)", " ", "-", "3"}
	for n := 0; n < 50000; n++ {
		lines := make([]string, 1+rng.Intn(4))
		for i := range lines {
			for k := rng.Intn(4); k >= 0; k-- {
				lines[i] += pieces[rng.Intn(len(pieces))]
			}
		}
		joined := strings.Join(lines, " ")
		wp, wok, wm := findMarkerAllLines(joined, lines)
		var gp Pos
		var gok bool
		var gm string
		func() {
			defer func() {
				if recover() != nil {
					gp, gok, gm = Pos{-1, -1}, false, "raise"
				}
			}()
			gp, gok, gm = FindMarker(joined, lines)
		}()
		if wp != gp || wok != gok || wm != gm {
			t.Fatalf("%q: got %v %v %q, want %v %v %q", lines, gp, gok, gm, wp, wok, wm)
		}
	}
}
