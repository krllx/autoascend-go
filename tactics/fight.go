package tactics

import (
	"aago/assess"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/skills"
	"aago/world"
)

// Tactics of agent.py (fight, emergency, eating, curing) as nodes.

func fight2Check(a *world.Agent) (exit bool, dis I32Grid, allowAttackAll bool) {
	monsters := a.GetVisibleMonsters()
	allowAttackAll = float64(a.LastTurn)-a.AllowAttackAllTurn < 3
	onlyRangedSlow := true
	for _, m := range monsters {
		if !(Contains(ONLY_RANGED_SLOW_MONSTERS, m.Mon.Mname) && !assess.ConsiderMeleeOnlyRangedIfHpFull(a, m)) {
			onlyRangedSlow = false
			break
		}
	}
	dis = a.Bfs()
	allFar := true
	for _, m := range monsters {
		if !(m.Dis > 7) {
			allFar = false
			break
		}
	}
	exit = len(monsters) == 0 || allFar
	if !exit && onlyRangedSlow && len(assess.GetRangedCombinations(a.Inventory, nil, true, false, false, false, nil)) == 0 {
		n := 0
		for y := 0; y < SY; y++ {
			for x := 0; x < SX; x++ {
				if dis[y][x] != -1 {
					n++
				}
			}
		}
		exit = n > 1 && !allowAttackAll
	}
	return
}

// ---- engulfed_fight: hit the engulfer until out

type EngulfedFight struct{}

func (EngulfedFight) Check(a *world.Agent) (bool, Frame) {
	if !AnyIn(a.Glyphs(), G_SWALLOW) {
		return false, nil
	}
	return true, &engulfedFightF{}
}

type engulfedFightF struct{ PC int }

func (s *engulfedFightF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 1 {
		Assert(c.Ret.(bool))
	}
	mask := Isin(a.Glyphs(), G_SWALLOW)
	nz := mask.Nonzero()
	if len(nz) == 0 {
		return ret(nil)
	}
	s.PC = 1
	return call(skills.MeleeAttack(nz[0].Y, nz[0].X))
}

// ---- fight2

// encumbranceOvertaxed is EXT_ENCUMBER: from this blstats encumbrance on, check_capacity refuses throw and zap.
const encumbranceOvertaxed = 4

type Fight2 struct{}

func (Fight2) Check(a *world.Agent) (bool, Frame) {
	exit, dis, allowAttackAll := fight2Check(a)
	if exit {
		return false, nil
	}
	return true, &fight2F{Dis: dis, AllowAttackAll: allowAttackAll}
}

// fight2F is the body of fight2: pick the best action by priority, do it, repeat while there is something
// to fight (after a move, wait up to 5 turns for the monsters to come).
type fight2F struct {
	Dis            I32Grid
	AllowAttackAll bool
	WaitCounter    int
	PC             int // 0: start, 1: choose, 2: action done, 3: searched
}

func (s *fight2F) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(sense.ParseEnhanceView())
	case 2:
		s.WaitCounter = c.Ret.(int)
	case 3:
		s.WaitCounter--
	}
	if s.PC >= 2 {
		exit, dis, allowAttackAll := fight2Check(a)
		s.Dis, s.AllowAttackAll = dis, allowAttackAll
		if exit {
			if s.WaitCounter != 0 {
				s.PC = 3
				return call(skills.SearchTimes(1))
			}
			return ret(nil)
		}
	}
	heat, actions := assess.FightGetPriorities(a)
	actions = append(actions, assess.FightGetMoveActions(a, &s.Dis, &heat)...)
	if a.Character.Prop.Polymorph() {
		var f []assess.FightAction
		for _, x := range actions {
			if x.Kind != "ranged" {
				f = append(f, x)
			}
		}
		actions = f
	}
	if a.Blstats.CarryingCapacity >= encumbranceOvertaxed {
		// Not in the Python bot: Overtaxed or worse, the game refuses to throw or zap ("You can't do that while
		// carrying so much stuff.") without the turn passing, and the item letter typed next ran as a command
		// (seed 3166: 'w' wielded a potion, then the fight repeated until "turn inactivity").
		var f []assess.FightAction
		for _, x := range actions {
			if x.Kind != "ranged" && x.Kind != "zap" {
				f = append(f, x)
			}
		}
		actions = f
	}
	if s.AllowAttackAll {
		var att []assess.FightAction
		for _, x := range actions {
			if x.Kind == "melee" || x.Kind == "ranged" || x.Kind == "zap" {
				att = append(att, x)
			}
		}
		if len(att) > 0 {
			actions = att
		}
	}
	Assert(len(actions) > 0, "No possible action available during fight2")
	best := actions[0]
	for _, x := range actions[1:] {
		if x.Priority > best.Priority {
			best = x
		}
	}
	s.PC = 2
	return call(&fightActionF{B: best, Wait: s.WaitCounter})
}

// fightActionF performs one fight action; returns the new wait counter.
type fightActionF struct {
	B      assess.FightAction
	Wait   int
	TY, TX int
	Ammo   *world.Item
	PC     int
}

func (s *fightActionF) Resume(c *Ctx) Cmd {
	a := c.W
	b := s.B
	if s.PC == 0 {
		switch b.Kind {
		case "move":
			s.Wait = 5
			s.PC = 9
			return call(skills.MoveTo(a, a.Blstats.Y+b.Dy, a.Blstats.X+b.Dx))
		case "melee":
			s.TY, s.TX = a.Blstats.Y+b.Dy, a.Blstats.X+b.Dx
			s.PC = 1
			return call(skills.WieldBest())
		case "ranged":
			s.TY, s.TX = a.Blstats.Y+b.Dy, a.Blstats.X+b.Dx
			launcher, ammo := assess.GetBestRangedSet(a.Inventory, nil, true, false, false, false, nil)
			Assert(ammo != nil)
			s.Ammo = ammo
			if launcher != nil && !launcher.Equipped {
				s.PC = 3
				return call(skills.Wield(launcher, true))
			}
			return s.fire(a)
		case "elbereth":
			Assert(a.EngravingLower() != "elbereth")
			s.PC = 9
			return call(skills.Engrave("Elbereth"))
		case "wait":
			Assert(a.EngravingLower() == "elbereth")
			s.PC = 9
			return call(skills.SearchTimes(1))
		case "zap":
			var wand *world.Item
			if b.HasWand {
				wand = b.Wand
			} else {
				for _, it := range a.Inventory.Items.AllItems {
					if it.IsOffensiveUsableWand() {
						wand = it
						break
					}
				}
				Assert(wand != nil)
			}
			dir := world.CalcDirection(a.Blstats.Y, a.Blstats.X, a.Blstats.Y+b.Dy, a.Blstats.X+b.Dx, true)
			s.PC = 9
			return call(skills.Zap(wand, dir))
		case "pickup":
			var items []*world.Item
			if b.HasItems {
				items = b.Items
			} else {
				items = assess.FightDecideWhatToPickup(a)
			}
			s.PC = 9
			return call(skills.Pickup(items, nil))
		case "go_to":
			s.PC = 9
			return call(skills.GoTo(b.Dy, b.Dx, skills.GoToOpts{StopOneBefore: true, MaxSteps: 1}))
		}
		Raise("NotImplementedError", "%v", b.Kind)
	}
	switch s.PC {
	case 1: // melee: wielded the best weapon?
		if c.Ret.(bool) {
			return ret(s.Wait)
		}
		s.PC = 2
		return call(skills.MeleeAttack(s.TY, s.TX))
	case 2:
		return ret(0)
	case 3: // ranged: wielded the launcher?
		if c.Ret.(bool) {
			return ret(s.Wait)
		}
		return s.fire(a)
	case 4:
		Assert(c.Ret.(bool)) // fired
	}
	return ret(s.Wait) // PC 9
}

func (s *fightActionF) fire(a *world.Agent) Cmd {
	dir := world.CalcDirection(a.Blstats.Y, a.Blstats.X, s.TY, s.TX, true)
	s.PC = 4
	return call(skills.Fire(s.Ammo, dir))
}
