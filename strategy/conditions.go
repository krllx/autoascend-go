package strategy

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// The top-level plan of global_logic.py as data: milestones and preempting layers.

// ---- conditions

type xpAtLeast struct{ N int }

func (p xpAtLeast) Eval(a *world.Agent) bool { return a.Blstats.ExperienceLevel >= p.N }

type onAltar struct{}

func (onAltar) Eval(a *world.Agent) bool {
	return G_ALTAR.Has(int(a.CurrentLevel().Objects[a.Blstats.Y][a.Blstats.X]))
}

type atPos struct{ Y, X int }

func (p atPos) Eval(a *world.Agent) bool { return a.Blstats.Y == p.Y && a.Blstats.X == p.X }

// hpRecovered is hp >= 0.8 max hp.
type hpRecovered struct{}

func (hpRecovered) Eval(a *world.Agent) bool {
	return float64(a.Blstats.Hitpoints) >= float64(0.8*float64(a.Blstats.MaxHitpoints))
}

type notHungry struct{}

func (notHungry) Eval(a *world.Agent) bool { return a.Blstats.HungerState >= Hunger_NOT_HUNGRY }

// noCoalignedAltarOrArtifact: go_to_strategy may walk only when there is no altar to sacrifice at.
type noCoalignedAltarOrArtifact struct{}

func (noCoalignedAltarOrArtifact) Eval(a *world.Agent) bool {
	if a.GlobalLogic.St.GotArtifact {
		return true
	}
	lvl := a.CurrentLevel()
	return !Any(lvl.Altars.Keys(), func(p Pos) bool { return lvl.Altars.MustGet(p) == a.Character.Alignment })
}

type sokobanActive struct{}

func (sokobanActive) Eval(a *world.Agent) bool {
	return a.GlobalLogic.Milestone == world.Milestone_SOLVE_SOKOBAN && a.CurrentLevel().DungeonNumber == world.Level_SOKOBAN
}

type never struct{}

func (never) Eval(*world.Agent) bool { return false }

type inDungeon struct{ D int }

func (p inDungeon) Eval(a *world.Agent) bool { return a.CurrentLevel().DungeonNumber == p.D }

type atLevel struct{ L world.LevelKey }

func (p atLevel) Eval(a *world.Agent) bool { return a.CurrentLevel().Key() == p.L }

type minetownFound struct{}

func (minetownFound) Eval(a *world.Agent) bool { return a.GlobalLogic.St.MinetownLevel != nil }

type sokobanTopSolved struct{}

func (sokobanTopSolved) Eval(a *world.Agent) bool {
	return a.CurrentLevel().Key() == world.LevelKey{world.Level_SOKOBAN, 1} && !AnyIn(&a.CurrentLevel().Objects, G_DOOR_CLOSED)
}
