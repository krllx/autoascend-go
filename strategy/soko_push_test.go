package strategy

import (
	"strings"
	"testing"

	"aago/internal/botsim"
	. "aago/nle"
	"aago/world"
)

// pushRoom: the hero at (3, 11), a boulder east of it, and what lies behind the boulder.
func pushRoom(behind string) (*botsim.Game, *world.Agent) {
	g := botsim.Parse(2, 10,
		"--------",
		"|@0"+behind+"..|",
		"--------")
	g.Sokoban = true
	return g, g.NewAgent(world.Level_SOKOBAN, 2)
}

func runPush(t *testing.T, g *botsim.Game, a *world.Agent) bool {
	t.Helper()
	r, exc := g.Run(a, &pushBolderF{TY: 3, TX: 11, DY: 0, DX: 1}, 2000)
	if exc != nil {
		t.Fatalf("%v; %s", exc, g.Report(a))
	}
	return r.(bool)
}

// push_bolder: the boulder moves; a boulder behind it ("in vain") gives the push up at once; a monster behind
// it is waited for, three pushes in all; a pick-axe digs through the boulder instead.
func TestPushBolder(t *testing.T) {
	g, a := pushRoom(".")
	if !runPush(t, g, a) || g.X != 12 || !g.Boulder[3][13] {
		t.Fatalf("no push: hero (%d, %d)\n%s", g.Y, g.X, g)
	}

	g, a = pushRoom("0")
	if runPush(t, g, a) || len(g.Keys) != 1 || g.X != 11 {
		t.Fatalf("a push in vain: %d keys, hero (%d, %d)", len(g.Keys), g.Y, g.X)
	}

	// a giant mimic behind the boulder: "There's a giant mimic on the other side."; the hero cannot get to it
	g, a = pushRoom(".")
	g.Mimic[3][13] = true
	if runPush(t, g, a) {
		t.Fatal("pushed into a mimic")
	}
	pushes := 0
	for _, k := range g.Keys {
		if k == world.DirActions["e"] {
			pushes++
		}
	}
	if pushes != sokoPushTries || g.Turn < (sokoPushTries-1)*sokoWaitTurns || g.Mimic[3][13] == false {
		t.Fatalf("%d pushes, turn %d; %s", pushes, g.Turn, g.Report(a))
	}

	// with a pick-axe: apply it in the push direction
	g, a = pushRoom("0")
	botsim.GiveItem(a, 'x', "a pick-axe", botsim.ObjectGlyph("pick-axe"))
	g.Respond = func(g *botsim.Game, act Action) bool {
		switch {
		case act == A_Command_APPLY:
			g.Message = "What do you want to use or apply? [x or ?*]"
		case act == Action('x') && strings.HasPrefix(g.Prompt, "What do you want to use"):
			g.Message = "In what direction do you want to dig? [yuhjklbn><]"
		case act == world.DirActions["e"] && strings.HasPrefix(g.Prompt, "In what direction"):
			g.Boulder[3][12] = false
			g.Turn += 5
			g.Message = "You hit the boulder with all your might.  The boulder falls apart."
		default:
			return false
		}
		return true
	}
	if !runPush(t, g, a) || g.Boulder[3][12] {
		t.Fatalf("no dig; %s", g.Report(a))
	}
	want := []Action{world.DirActions["e"], A_Command_APPLY, Action('x'), world.DirActions["e"]}
	if len(g.Keys) != len(want) {
		t.Fatalf("keys %v, want %v", g.Keys, want)
	}
	for i := range want {
		if g.Keys[i] != want[i] {
			t.Fatalf("keys %v, want %v", g.Keys, want)
		}
	}
}
