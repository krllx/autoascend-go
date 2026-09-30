package tactics

import (
	"testing"

	"aago/internal/botsim"
	. "aago/nle"
)

// patrol (unused by the strategy, like in AutoAscend): pick a reachable tile, farther ones more likely, and go
// there with the travel command.
func TestPatrol(t *testing.T) {
	g := botsim.Parse(2, 10,
		"----------",
		"|@.......|",
		"|........|",
		"|...0....|",
		"----------")
	a := g.NewAgent(0, 1)
	f := patrolStepper{}.Step(a)
	if f == nil {
		t.Fatal("no patrol step")
	}
	if _, exc := g.Run(a, f, 100); exc != nil {
		t.Fatalf("%v; %s", exc, g.Report(a))
	}
	// a long way is travelled (one travel command), a short one walked
	if (g.Travelled == 1) != (g.Keys[0] == A_Command_TRAVEL) || (g.Travelled == 0 && len(g.Keys) > 2) {
		t.Fatalf("keys %v, %d travels", g.Keys, g.Travelled)
	}
	if g.Boulder[g.Y][g.X] || g.Y < 3 || g.Y > 5 || g.X < 11 || g.X > 18 {
		t.Fatalf("hero at (%d, %d)", g.Y, g.X)
	}

	// shut in: nothing to patrol
	g = botsim.Parse(2, 10,
		"---",
		"|@|",
		"---")
	a = g.NewAgent(0, 1)
	if f := (patrolStepper{}).Step(a); f != nil {
		t.Fatalf("a patrol step with nowhere to go: %T", f)
	}
}
