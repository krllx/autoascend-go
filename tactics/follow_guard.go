package tactics

import (
	"aago/flow"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/skills"
	"aago/world"
)

// ---- follow_guard: a vault guard asks to drop the gold and follow him out

type FollowGuard struct{}

func (FollowGuard) Check(a *world.Agent) (bool, Frame) {
	g := a.GlobalLogic
	if !AnyIn(a.Glyphs(), G_GUARD) {
		return false, nil
	}
	if Any(world.FlattenItems(a.Inventory.Items.AllItems), func(item *world.Item) bool { return item.Category == NH_COIN_CLASS }) {
		return true, &dropGoldF{G: g}
	}
	guards := Isin(a.Glyphs(), G_GUARD)
	guard := guards.First()
	if Adjacent(guard, Pos{a.Blstats.Y, a.Blstats.X}) {
		return false, nil
	}
	// Not in the Python bot: go_to asserted "no reachable neighbor" when no cell next to the guard can be reached,
	// e.g. the guard's corridor leaves the vault diagonally between two walls and the bot carries more than 600
	// (the game refuses that squeeze too; seed 3031). Then the guard is not followed. The check must not change
	// the state go_to would see: no BFS cached for the step, and the neighbours without the shuffle (it draws from
	// the random state).
	dis := a.BfsPeek()
	if dis[guard.Y][guard.X] == -1 && !Any(a.Neighbors(guard.Y, guard.X, false, true), func(p Pos) bool { return dis[p.Y][p.X] != -1 }) {
		return false, nil
	}
	return true, flow.ThenNil(skills.GoTo(guard.Y, guard.X, skills.GoToOpts{StopOneBefore: true}))
}

type dropGoldF struct {
	G  *world.GlobalLogic
	PC int
}

func (s *dropGoldF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		// if 'Please drop that gold and follow me.' in self.message:
		a.StatsLogger.LogEvent("drop_gold")
		s.G.ItemPriority.DropGoldTillTurn = float64(a.Blstats.Time + 100)
		s.PC = 1
		return call(flow.RunNode(arrangeItemsN{}))
	}
	return ret(nil)
}
