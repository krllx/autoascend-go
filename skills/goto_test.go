package skills

import (
	"strings"
	"testing"

	. "aago/grid"
	"aago/internal/botsim"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// room is a lit room with a pillar and a pit; the hero at its top-left corner (level coordinates (3, 11)).
func room() *botsim.Game {
	return botsim.Parse(2, 10,
		"------------",
		"|@.........|",
		"|..........|",
		"|...|......|",
		"|...|...^..|",
		"|..........|",
		"------------")
}

func run(t *testing.T, g *botsim.Game, a *world.Agent, f Frame) any {
	t.Helper()
	r, exc := g.Run(a, f, 500)
	if exc != nil {
		t.Fatalf("%v; %s", exc, g.Report(a))
	}
	return r
}

func dirKeys(g *botsim.Game) (n int) {
	for _, k := range g.Keys {
		for _, d := range []string{"n", "s", "e", "w", "ne", "se", "nw", "sw"} {
			if k == world.DirActions[d] {
				n++
			}
		}
	}
	return n
}

// go_to walks the shortest path, one move per key, around the pit.
func TestGoToWalks(t *testing.T) {
	g := room()
	a := g.NewAgent(0, 1)
	run(t, g, a, GoTo(6, 19, GoToOpts{}))
	if g.Y != 6 || g.X != 19 || len(g.Fell) > 0 {
		t.Fatalf("hero at (%d, %d), fell %v\n%s", g.Y, g.X, g.Fell, g)
	}
	if len(g.Keys) != 8 || dirKeys(g) != 8 {
		t.Fatalf("%d keys %v, want 8 moves", len(g.Keys), g.Keys)
	}
}

// go_to(fast=True) uses the travel command when the path is longer than 2: the cursor goes from where the
// game puts it (the hero, later the last destination) to the target, then '.'.
func TestGoToFastTravel(t *testing.T) {
	g := room()
	a := g.NewAgent(0, 1)
	run(t, g, a, GoTo(6, 19, GoToOpts{Fast: true}))
	if g.Y != 6 || g.X != 19 || g.Travelled != 1 {
		t.Fatalf("hero at (%d, %d), %d travels\n%s", g.Y, g.X, g.Travelled, g)
	}
	// TRAVEL, cursor (3,11) -> (6,19): 3 x se, 5 x e, then '.'
	want := []Action{A_Command_TRAVEL}
	for range 3 {
		want = append(want, A_CompassDirection_SE)
	}
	for range 5 {
		want = append(want, A_CompassDirection_E)
	}
	want = append(want, A_MiscDirection_WAIT)
	if !equalKeys(g.Keys, want) {
		t.Fatalf("keys %v, want %v", g.Keys, want)
	}

	// the next travel: the cursor starts at the last destination
	g.Keys = nil
	run(t, g, a, GoTo(3, 12, GoToOpts{Fast: true}))
	if g.Y != 3 || g.X != 12 || g.Travelled != 2 {
		t.Fatalf("hero at (%d, %d)", g.Y, g.X)
	}
	want = []Action{A_Command_TRAVEL, A_CompassDirection_NW, A_CompassDirection_NW, A_CompassDirection_NW}
	for range 4 {
		want = append(want, A_CompassDirection_W)
	}
	want = append(want, A_MiscDirection_WAIT)
	if !equalKeys(g.Keys, want) {
		t.Fatalf("keys %v, want %v", g.Keys, want)
	}

	// a short path is walked
	g.Keys = nil
	run(t, g, a, GoTo(4, 13, GoToOpts{Fast: true}))
	if g.Y != 4 || g.X != 13 || g.Travelled != 2 || dirKeys(g) != 1 {
		t.Fatalf("hero at (%d, %d), keys %v", g.Y, g.X, g.Keys)
	}
}

func equalKeys(a, b []Action) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// When travel is interrupted (the hero did not reach the end), go_to recomputes the path and goes on.
func TestGoToFastTravelInterrupted(t *testing.T) {
	g := room()
	a := g.NewAgent(0, 1)
	g.StopTravel = 1 // the first travel stops after one step
	run(t, g, a, GoTo(6, 19, GoToOpts{Fast: true}))
	if g.Y != 6 || g.X != 19 || g.Travelled != 2 {
		t.Fatalf("hero at (%d, %d), %d travels", g.Y, g.X, g.Travelled)
	}
}

// stop_one_before stops next to the target; an unreachable target is replaced by its nearest reachable
// neighbour.
func TestGoToStopOneBefore(t *testing.T) {
	g := room()
	a := g.NewAgent(0, 1)
	run(t, g, a, GoTo(3, 16, GoToOpts{StopOneBefore: true}))
	if g.Y != 3 || g.X != 15 {
		t.Fatalf("hero at (%d, %d), want (3, 15)", g.Y, g.X)
	}
	// the pillar (5, 14) is a wall: go to its nearest reachable neighbour, (4, 14)... whichever is nearest
	run(t, g, a, GoTo(5, 14, GoToOpts{StopOneBefore: true}))
	if Abs(g.Y-5) > 1 || Abs(g.X-14) > 1 || (g.Y == 5 && g.X == 14) {
		t.Fatalf("hero at (%d, %d), not next to the pillar", g.Y, g.X)
	}
	// no reachable neighbour at all
	_, exc := g.Run(a, GoTo(0, 0, GoToOpts{StopOneBefore: true}), 100)
	if !IsPyExc(exc, "AssertionError") || !strings.Contains(exc.(*PyExc).Msg, "no reachable neighbor") {
		t.Fatalf("got %v", exc)
	}
}

// max_steps stops the walk early; the callback stops it when it says so.
func TestGoToMaxStepsAndCallback(t *testing.T) {
	g := room()
	a := g.NewAgent(0, 1)
	run(t, g, a, GoTo(3, 20, GoToOpts{MaxSteps: 3}))
	if g.X != 14 || len(g.Keys) != 3 {
		t.Fatalf("hero at (%d, %d) after %d keys, want column 14 after 3", g.Y, g.X, len(g.Keys))
	}
	run(t, g, a, GoTo(3, 20, GoToOpts{Callback: atColumn{17}}))
	if g.X != 17 {
		t.Fatalf("hero at (%d, %d), want column 17", g.Y, g.X)
	}
	n := len(g.Keys)
	run(t, g, a, GoTo(3, 20, GoToOpts{Callback: atColumn{17}})) // true before the walk: no keys
	if len(g.Keys) != n {
		t.Fatalf("keys after a callback that is already true: %v", g.Keys[n:])
	}
}

type atColumn struct{ X int }

func (c atColumn) Check(a *world.Agent) bool { return a.Blstats.X == c.X }

// blockAhead puts peaceful monsters on column 14 above the pillar once the hero reaches column 12: every
// shortest path to (3, 16) went through one of them.
type blockAhead struct{}

func (blockAhead) Check(a *world.Agent) bool {
	if a.Blstats.X == 12 {
		a.MonsterTracker.PeacefulMonsterMask[3][14] = true
		a.MonsterTracker.PeacefulMonsterMask[4][14] = true
	}
	return false
}

// A peaceful monster on the path stops the walk before it. The next round does not go around: the Python
// loop `for y, x in path` rebinds the target to the blocked tile, which is unreachable, so go_to panics
// (AutoAscend does the same; the strategy above starts over).
func TestGoToBlockedPath(t *testing.T) {
	g := room()
	a := g.NewAgent(0, 1)
	_, exc := g.Run(a, GoTo(3, 16, GoToOpts{Callback: blockAhead{}}), 100)
	if p, ok := exc.(*AgentPanic); !ok || !strings.Contains(p.Msg, "end point is no longer accessible") {
		t.Fatalf("got %v; %s", exc, g.Report(a))
	}
	if g.X != 13 || a.MonsterTracker.PeacefulMonsterMask[g.Y][g.X] {
		t.Fatalf("hero at (%d, %d)", g.Y, g.X)
	}
}

// Moves check the position after the step, and refuse to step onto a monster.
func TestMovePanics(t *testing.T) {
	g := room()
	a := g.NewAgent(0, 1)
	_, exc := g.Run(a, Move("n"), 10) // into the wall: the hero stays
	if p, ok := exc.(*AgentPanic); !ok || !strings.Contains(p.Msg, "agent position do not match") {
		t.Fatalf("got %v", exc)
	}
	a.MonsterTracker.MonsterMask[3][12] = true
	_, exc = g.Run(a, Move("e"), 10)
	if p, ok := exc.(*AgentPanic); !ok || !strings.Contains(p.Msg, "Monster on a next tile") {
		t.Fatalf("got %v", exc)
	}
	_, exc = g.Run(a, Direction("up"), 10)
	if !IsPyExc(exc, "KeyError") {
		t.Fatalf("got %v", exc)
	}
}

// An unreachable target fails the assert before the first key.
func TestGoToUnreachable(t *testing.T) {
	g := room()
	a := g.NewAgent(0, 1)
	for _, p := range []Pos{{3, 15}, {4, 15}, {4, 16}, {4, 17}, {3, 17}} { // peaceful monsters around (3, 16)
		a.MonsterTracker.PeacefulMonsterMask[p.Y][p.X] = true
	}
	_, exc := g.Run(a, GoTo(3, 16, GoToOpts{}), 100)
	if !IsPyExc(exc, "AssertionError") || len(g.Keys) != 0 {
		t.Fatalf("got %v after %d keys", exc, len(g.Keys))
	}
}
