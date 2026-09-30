package strategy

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	. "aago/grid"
	"aago/internal/botsim"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// desLevel is one Sokoban level of NetHack 3.6.6 (testdata/sokoban.des, NetHack's own level file).
type desLevel struct {
	Name     string
	Map      []string
	Boulders []Pos // (y, x) in the map
	Pits     []Pos
	Holes    []Pos
	Objects  []Pos // scrolls of earth
	Doors    []Pos
	Locked   []Pos // the locked doors among them
	Start    Pos   // where the hero arrives: the branch stairs (bottom level) or the down stairs
	Up       []Pos // the up stairs (none on the top level)
}

var (
	desXY    = regexp.MustCompile(`\((\d+),(\d+)`)
	desLevNo = regexp.MustCompile(`^MAZE:"soko(\d)-(\d)"`)
)

func readSokobanDes(t *testing.T) []*desLevel {
	f, err := os.Open("testdata/sokoban.des")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var levels []*desLevel
	var l *desLevel
	inMap := false
	pos := func(line string) Pos {
		m := desXY.FindStringSubmatch(line)
		x, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		return Pos{y, x}
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MAZE:"):
			l = &desLevel{Name: strings.Split(line, `"`)[1]}
			levels = append(levels, l)
		case line == "MAP":
			inMap = true
		case line == "ENDMAP":
			inMap = false
		case inMap:
			l.Map = append(l.Map, line)
		case strings.HasPrefix(line, "BRANCH:"), strings.HasPrefix(line, "STAIR:") && strings.HasSuffix(line, "down"):
			l.Start = pos(line)
		case strings.HasPrefix(line, "STAIR:"):
			l.Up = append(l.Up, pos(line))
		case strings.HasPrefix(line, "OBJECT:('`',\"boulder\")"):
			l.Boulders = append(l.Boulders, pos(line))
		case strings.HasPrefix(line, "OBJECT:('?',\"earth\")"):
			l.Objects = append(l.Objects, pos(line))
		case strings.HasPrefix(line, `TRAP:"pit"`):
			l.Pits = append(l.Pits, pos(line))
		case strings.HasPrefix(line, `TRAP:"hole"`):
			l.Holes = append(l.Holes, pos(line))
		case strings.HasPrefix(line, "DOOR:"):
			l.Doors = append(l.Doors, pos(line))
			if strings.HasPrefix(line, "DOOR:locked") {
				l.Locked = append(l.Locked, pos(line))
			}
		}
	}
	if len(levels) != 8 {
		t.Fatalf("%d levels in sokoban.des", len(levels))
	}
	return levels
}

// game builds the level in the scripted game, centered on the screen like GEOMETRY:center,center.
func (l *desLevel) game(t *testing.T) (*botsim.Game, int) {
	h, w := len(l.Map), 0
	for _, r := range l.Map {
		w = max(w, len(r))
	}
	rows := make([][]byte, h)
	for i, r := range l.Map {
		rows[i] = []byte(r + strings.Repeat(" ", w-len(r)))
	}
	set := func(ps []Pos, ch byte) {
		for _, p := range ps {
			if rows[p.Y][p.X] != '.' && rows[p.Y][p.X] != '+' {
				t.Fatalf("%s: %c on %q at %v", l.Name, ch, rows[p.Y][p.X], p)
			}
			rows[p.Y][p.X] = ch
		}
	}
	set(l.Boulders, '0')
	set(l.Pits, '_')
	set(l.Holes, '^')
	set(l.Objects, '?')
	set(l.Up, '<')
	set([]Pos{l.Start}, '@')
	set(l.Doors, '+')
	strs := make([]string, h)
	for i := range rows {
		strs[i] = string(rows[i])
	}
	y0, x0 := (SY-h)/2, (SX-w)/2
	g := botsim.Parse(y0, x0, strs...)
	g.Sokoban = true
	for _, p := range l.Locked {
		g.Locked[y0+p.Y][x0+p.X] = true
	}
	m := desLevNo.FindStringSubmatch("MAZE:\"" + l.Name + "\"")
	n, _ := strconv.Atoi(m[1])
	return g, n
}

// The solver plays every Sokoban level of NetHack to the end: it finds its map, pushes by the solution, and
// every pit and hole gets plugged; the hero never walks into a trap. Keys go to a scripted game with the real
// level (boulders, traps, scrolls, doors), no monsters.
func TestSokobanSolvesLevels(t *testing.T) {
	used := map[int]string{}
	for _, l := range readSokobanDes(t) {
		t.Run(l.Name, func(t *testing.T) {
			g, n := l.game(t)
			traps := g.Traps()
			a := g.NewAgent(world.Level_SOKOBAN, n)
			f := &solveSokobanF{}
			_, exc := g.Run(a, f, 20000)
			if exc != nil {
				t.Fatalf("%v; %d pushes, %d of %d traps plugged; %s", exc, g.Pushes, g.Filled, traps, g.Report(a))
			}
			if g.Traps() != 0 || g.Filled != traps {
				t.Errorf("%d of %d traps plugged", g.Filled, traps)
			}
			if len(g.Fell) > 0 {
				t.Errorf("the hero walked into traps at %v", g.Fell)
			}
			for mi := range SokoMapsData {
				if f.Answer != nil && &SokoMapsData[mi].Answer[0] == &f.Answer[0] {
					if prev, ok := used[mi]; ok {
						t.Errorf("solver map %d also matched %s", mi, prev)
					}
					used[mi] = l.Name
				}
			}
			t.Logf("%d keys, %d pushes, %d traps plugged, %d boulders left", len(g.Keys), g.Pushes, g.Filled,
				len(g.Boulder.Nonzero()))
		})
	}
}

// solveWithRestarts runs the solver the way the global strategy does: a revealed mimic (the fight layer takes
// over) or an AgentPanic ends the solver frame, and it starts again while the level has traps. The outcome is
// "solved", "dropped" (the solver gave the level up and the milestone moved on), or what went wrong.
func solveWithRestarts(g *botsim.Game, a *world.Agent, maxKeys int) (outcome string, runs int) {
	g.OnReveal = &AgentChangeStrategy{}
	for ; g.Traps() > 0; runs++ {
		if runs > 50 {
			return "more than 50 runs", runs
		}
		if len(g.Keys) >= maxKeys {
			return fmt.Sprintf("no end after %d keys", maxKeys), runs
		}
		_, exc := g.Run(a, &solveSokobanF{}, maxKeys-len(g.Keys))
		switch e := exc.(type) {
		case nil:
			if g.Traps() > 0 {
				return "the solver returned with traps left", runs
			}
		case *AgentChangeStrategy:
		case *AgentPanic:
			if a.GlobalLogic.Milestone != world.Milestone_BE_ON_FIRST_LEVEL {
				if e.Msg != "sokomap unsolvable" {
					return "milestone moved on: " + e.Msg, runs
				}
				return "dropped", runs + 1
			}
		default:
			return fmt.Sprint(exc), runs
		}
	}
	if len(g.Fell) > 0 {
		return fmt.Sprintf("fell into %v", g.Fell), runs
	}
	return "solved", runs
}

// withMimics is the level with giant mimics that look like boulders at level positions ps.
func (l *desLevel) withMimics(t *testing.T, ps ...Pos) (*botsim.Game, *world.Agent) {
	g, n := l.game(t)
	for _, p := range ps {
		if g.T[p.Y][p.X] != botsim.Floor || g.Boulder[p.Y][p.X] {
			t.Fatalf("%s: no free floor for a mimic at %v", l.Name, p)
		}
		g.Mimic[p.Y][p.X] = true
	}
	return g, g.NewAgent(world.Level_SOKOBAN, n)
}

func desByName(t *testing.T, name string) *desLevel {
	for _, l := range readSokobanDes(t) {
		if l.Name == name {
			return l
		}
	}
	t.Fatalf("no level %s", name)
	return nil
}

// The top levels have two giant mimics that look like boulders. The solver searches next to a boulder the
// solution does not know and the fight takes over; a mimic it cannot get to, found by pushing a boulder at it
// ("There's a giant mimic on the other side."), makes it give the level up (the Python bot failed an assert
// there, or pushed and explored forever).
func TestSokobanTopLevelMimics(t *testing.T) {
	for _, c := range []struct {
		level  string
		mimics []Pos
		want   string
	}{
		{"soko1-1", []Pos{{15, 46}, {3, 31}}, "solved"},  // in the closet (never reached) and next to the start
		{"soko1-2", []Pos{{9, 29}, {10, 32}}, "solved"},  // among the boulders
		{"soko1-1", []Pos{{8, 29}, {16, 45}}, "dropped"}, // shut in a corner: the push at it fails
		{"soko1-2", []Pos{{8, 27}, {9, 35}}, "dropped"},
	} {
		g, a := desByName(t, c.level).withMimics(t, c.mimics...)
		got, runs := solveWithRestarts(g, a, 6000)
		t.Logf("%s, mimics %v: %s, %d runs, %d keys, %d revealed", c.level, c.mimics, got, runs, len(g.Keys),
			g.Revealed)
		if got != c.want {
			t.Errorf("%s, mimics %v: %s, want %s; %s", c.level, c.mimics, got, c.want, g.Report(a))
		}
	}
}

// Random places for the two mimics (GOBOT_SOKO_TRIALS per top level, about 5 s each): every game ends solved
// or dropped, never stuck.
func TestSokobanMimicSweep(t *testing.T) {
	trials, _ := strconv.Atoi(os.Getenv("GOBOT_SOKO_TRIALS"))
	if trials <= 0 {
		t.Skip("set GOBOT_SOKO_TRIALS")
	}
	rng := rand.New(rand.NewSource(1))
	for _, name := range []string{"soko1-1", "soko1-2"} {
		l := desByName(t, name)
		count := map[string]int{}
		for trial := range trials {
			g, _ := l.game(t)
			var free []Pos
			for y := range SY {
				for x := range SX {
					if g.T[y][x] == botsim.Floor && !g.Boulder[y][x] && g.Object[y][x] == 0 && (y != g.Y || x != g.X) {
						free = append(free, Pos{y, x})
					}
				}
			}
			mims := []Pos{free[rng.Intn(len(free))], free[rng.Intn(len(free))]}
			if mims[0] == mims[1] {
				mims = mims[:1]
			}
			g, a := l.withMimics(t, mims...)
			got, runs := solveWithRestarts(g, a, 12000)
			count[got]++
			t.Logf("%s trial %d, mimics %v: %s, %d runs, %d keys", name, trial, mims, got, runs, len(g.Keys))
			if got != "solved" && got != "dropped" {
				t.Errorf("%s trial %d, mimics %v: %s; %s", name, trial, mims, got, g.Report(a))
			}
		}
		t.Logf("%s: %v", name, count)
	}
}
