package assess

import (
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

type wandState struct {
	y, x, dy, dx int
	nextProb     float64
	rangePenalty int
}

func getNextStates(a *world.Agent, wand *world.Item, y, x, dy, dx int) []wandState {
	walkable := &a.CurrentLevel().Walkable
	if !combatInside(y, x) || !walkable[y][x] {
		canBounce := wand.IsRayWand()
		if !canBounce {
			return nil
		}
		if dy == 0 || dx == 0 {
			return []wandState{{y - dy, x - dx, -dy, -dx, 1.0, 1}}
		}
		// TODO: diagonal
		s1y, s1x := y, x-dx
		s2y, s2x := y-dy, x
		side1Wall := !combatInside(s1y, s1x) || !a.CurrentLevel().Walkable[s1y][s1x]
		side2Wall := !combatInside(s2y, s2x) || !a.CurrentLevel().Walkable[s2y][s2x]
		dy1, dx1 := s2y-s1y, s2x-s1x
		dy2, dx2 := s1y-s2y, s1x-s2x
		if side1Wall && side2Wall {
			return []wandState{{y - dy, x - dx, -dy, -dx, 1.0, 1}}
		} else if !side1Wall && !side2Wall {
			return []wandState{
				{y - dy, x - dx, -dy, -dx, 1.0 / 20, 1},
				{y + dy1, x + dx1, dy1, dx1, 19.0 / 40, 1},
				{y + dy2, x + dx2, dy2, dx2, 19.0 / 40, 1},
			}
		} else if side1Wall {
			return []wandState{{y + dy1, x + dx1, dy1, dx1, 1.0, 1}}
		} else if side2Wall {
			return []wandState{{y + dy2, x + dx2, dy2, dx2, 1.0, 1}}
		} else {
			Assert(false)
		}
	}
	return []wandState{{y + dy, x + dx, dy, dx, 1.0, 0}}
}

// wandHitKind: 0 None, 1 monster, 2 'pet', 3 'self'.
type wandHit struct {
	kind    int
	monster world.Monster
	p       float64
}

func simulateWandPathRec(a *world.Agent, wand *world.Item, monsters []world.Monster, y, x, dy, dx, rangeLeft int,
	hitTargets *OMap[Pos, *wandHit], probability float64) {
	if rangeLeft < 0 {
		return
	}

	for _, st := range getNextStates(a, wand, y, x, dy, dx) {
		y, x, dy, dx := st.y, st.x, st.dy, st.dx
		rangeLeft -= st.rangePenalty
		monster := monstersAt(monsters, y, x)
		h := &wandHit{}
		if len(monster) > 0 {
			Assert(len(monster) == 1)
			h.kind, h.monster = 1, monster[0]
			// For each monster hit, range decreases by 2.
			rangeLeft -= 2
		} else if combatInside(y, x) && G_PETS.Has(int(a.Glyphs()[y][x])) {
			h.kind = 2
			// For each monster hit, range decreases by 2.
			rangeLeft -= 2
		} else if a.Blstats.Y == y && a.Blstats.X == x {
			h.kind = 3
			rangeLeft -= 2
		} else {
			h.kind = 0
		}

		// hit_targets[(y, x, monster)] += probability * next_prob; the monster part of the key is a
		// function of (y, x), so (y, x) identifies the key.
		cur := hitTargets.GetOrInsert(Pos{y, x}, func() *wandHit { return h })
		cur.p += float64(probability * st.nextProb)

		simulateWandPathRec(a, wand, monsters, y, x, dy, dx, rangeLeft-1, hitTargets, 1.0)
	}
}

// simulateWandPath mirrors simulate_wand_path: (y, x, hit_object, expected_hit_count) in dict order.
func simulateWandPath(a *world.Agent, wand *world.Item, monsters []world.Monster, dy, dx int) ([]Pos, []*wandHit) {
	y, x := a.Blstats.Y, a.Blstats.X

	// TODO: random range left from 6 or 7 to 13
	hitTargets := NewOMap[Pos, *wandHit]()
	simulateWandPathRec(a, wand, monsters, y, x, dy, dx, 13, hitTargets, 1.0)
	return hitTargets.Keys(), hitTargets.Values()
}

func getPotentialWandUsages(a *world.Agent, monsters []world.Monster, dy, dx int) []FightAction {
	var ret []FightAction
	_ = Div(float64(a.Blstats.Hitpoints), float64(a.Blstats.MaxHitpoints)) // player_hp_ratio
	// TODO: also get items recursively from bags
	for _, item := range a.Inventory.Items.AllItems {
		targetedMonsters := 0
		if !item.IsOffensiveUsableWand() {
			continue
		}
		priority := 0.0
		_, hits := simulateWandPath(a, item, monsters, dy, dx)
		for _, h := range hits {
			p := h.p
			if h.kind == 2 {
				priority -= float64(p * 20)
			} else if h.kind == 3 {
				priority -= float64(p * 30)
			} else if h.kind == 1 {
				mon := h.monster.Mon
				if Contains(WEAK_MONSTERS, mon.Mname) {
					priority += float64(PyMinF(p, 1) * 1)
				} else if IsDangerousMonster(h.monster) {
					priority += float64(p * 25)
				} else {
					priority += float64(PyMinF(p, 1) * 10)
				}
				targetedMonsters++
			}
		}
		if targetedMonsters > 0 {
			// priority = priority * (1 - player_hp_ratio) - 10
			priority = priority - 15
			if combatElbereth(a) {
				priority -= 100
			}
			ret = append(ret, FightAction{Priority: priority, Kind: "zap", Dy: dy, Dx: dx, Wand: item, HasWand: true})
		}
	}
	return ret
}
