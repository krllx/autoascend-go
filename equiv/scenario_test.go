package equiv

import (
	"os"
	"runtime"
	"testing"

	"aago/brain"
	"aago/internal/corpus"
	. "aago/nle"
	"aago/world"
)

// Scenarios on recorded games: what a game of the corpus must show besides its exact keys, so that a failure
// says what broke rather than only the first step that differs.

// Seed 1139 reaches Sokoban: the bot solves the three lower levels (no pit or hole left in its memory of them)
// and gives the top level up, the one with the giant mimics (sokoban_dropped, the milestone moves on).
func TestScenarioSokoban1139(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("run with GOARCH=amd64")
	}
	path := corpus.Path(corpus.Dir(), 1139)
	if _, err := os.Stat(path); err != nil {
		t.Skip("no corpus game 1139")
	}
	var last *brain.Driver
	replayWith(t, path, func(step int, d *brain.Driver) *brain.Driver {
		last = d
		return d
	})
	a := last.Agent
	for lv := 2; lv <= 4; lv++ {
		l, ok := a.Levels.Get(world.LevelKey{world.Level_SOKOBAN, lv})
		if !ok {
			t.Fatalf("Sokoban level %d not visited", lv)
		}
		if AnyIn(&l.Objects, G_TRAPS) {
			t.Errorf("Sokoban level %d not solved: traps left", lv)
		}
	}
	top, ok := a.Levels.Get(world.LevelKey{world.Level_SOKOBAN, 1})
	if !ok || !AnyIn(&top.Objects, G_TRAPS) {
		t.Errorf("the top level: visited %v, expected to be given up with holes left", ok)
	}
	if a.GlobalLogic.Milestone <= world.Milestone_SOLVE_SOKOBAN {
		t.Errorf("milestone %d, want past SOLVE_SOKOBAN", a.GlobalLogic.Milestone)
	}
}
