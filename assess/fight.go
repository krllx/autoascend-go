package assess

import (
	"math"
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// Port of combat/fight_heur.py (get_corridors_priority_map is unused in Python and not ported).
//
// How a fight turn is decided: FightGetPriorities scores every candidate action (melee, ranged,
// zap a wand, engrave Elbereth, wait, pick up ammo, walk towards a monster) and builds a heatmap of
// how good each cell is to step on; FightGetMoveActions turns the heatmap into move actions. The
// agent (Fight2 in agent.go) takes the action with the highest priority.
//
// float64(x * y) conversions around products are deliberate: they stop the compiler from fusing
// x*y+z into one FMA instruction, which rounds differently from Python.

func meleeMonsterPriority(a *world.Agent, monsters []world.Monster, m world.Monster) float64 {
	y, x, mon := m.Y, m.X, m.Mon
	ret := 1.0
	if a.Blstats.Hitpoints > 8 || IsMonsterFaster(a, m) {
		ret += 15
	}
	if wieldingRangedWeapon(a) && !IsMonsterFaster(a, m) {
		ret -= 6
	}
	if Contains(EXPLODING_MONSTERS, mon.Mname) {
		ret -= 17
	}
	if strings.Contains(mon.Mname, "were") {
		ret += 1
	}
	// if not wielding_melee_weapon(agent):
	//     ret -= 5
	if Contains(ONLY_RANGED_SLOW_MONSTERS, mon.Mname) {
		if !ConsiderMeleeOnlyRangedIfHpFull(a, m) {
			ret -= 100
			if mon.Mname == "floating eye" {
				ret -= 10
			}
			if mon.Mname == "gas spore" {
				ret -= 5
			}
		}
	}

	if mon.Mname == "gas spore" {
		// handle a specific case when you are trapped by a gas spore
		if len(a.GetVisibleMonsters()) == 1 &&
			Div(float64(a.Blstats.Hitpoints), float64(a.Blstats.MaxHitpoints)) != 0 {
			dis := a.Bfs()
			reachable := dis.Ne(-1)
			for _, p := range reachable.Nonzero() {
				if !Adjacent(Pos{y, x}, p) {
					return ret
				}
			}
			a.StatsLogger.LogEvent("melee_gas_spore")
			return 1 // a priority higher than random moving around
		}
	}

	return ret
}

// monstersAt is [m for m in monsters if (m.y, m.x) == (y, x)].
func monstersAt(monsters []world.Monster, y, x int) []world.Monster {
	return Filter(monsters, func(m world.Monster) bool { return m.Y == y && m.X == x })
}

type rangedPr struct {
	pri     float64
	y, x    int
	monster world.Monster
}

// rangedPriority mirrors ranged_priority (nil = None).
func rangedPriority(a *world.Agent, dy, dx int, monsters []world.Monster) *rangedPr {
	ret := 11.0

	closestMonDis := math.Inf(1)
	for _, m := range monsters {
		Assert(m.Y != a.Blstats.Y || m.X != a.Blstats.X)
		if !Contains(WEAK_MONSTERS, m.Mon.Mname) && !Contains(ONLY_RANGED_SLOW_MONSTERS, m.Mon.Mname) {
			closestMonDis = PyMinF(closestMonDis, float64(lineDisFrom(a, m.Y, m.X)))
		}
	}

	if closestMonDis == 1 {
		ret -= 11
	}

	launcher, ammo := GetBestRangedSet(a.Inventory, nil, true, false, false, false, []*world.Item{})
	if ammo == nil {
		return nil
	}

	if launcher != nil && !launcher.Equipped {
		ret -= 5
	}

	y, x := a.Blstats.Y, a.Blstats.X
	for {
		y += dy
		x += dx
		if !(0 <= y && y < SY) || !(0 <= x && x < SX) {
			return nil
		}

		if G_PETS.Has(int(a.Glyphs()[y][x])) || !a.CurrentLevel().Walkable[y][x] {
			return nil
		}

		if G_MONS.Has(int(a.Glyphs()[y][x])) {
			monster := monstersAt(monsters, y, x)
			if len(monster) == 0 {
				// there is a monster that shouldn't be attacked
				return nil
			}
			Assert(len(monster) == 1)
			mon := monster[0].Mon
			dis := lineDisFrom(a, y, x)
			if dis > a.Character.GetRange(launcher, ammo) {
				return nil
			}
			if dis == 1 || dis == 2 {
				ret -= 5
			}
			if dis == 1 {
				ret -= 6
				if mon.Mname == "gas spore" { // only gas spore ?
					ret -= 100
				}
			}
			return &rangedPr{ret, y, x, monster[0]}
		}
	}
}

func elberethAction(a *world.Agent, monsters []world.Monster) []FightAction {
	if combatElbereth(a) {
		return nil
	}
	if !a.CanEngrave() {
		return nil
	}
	adjMonstersCount := 0.0
	for _, m := range monsters {
		mon := m.Mon
		if Contains(ONLY_RANGED_SLOW_MONSTERS, mon.Mname) {
			continue
		}
		if !Adjacent(Pos{m.Y, m.X}, Pos{a.Blstats.Y, a.Blstats.X}) {
			continue
		}
		multiplier := math.Min(math.Max(Div(20, float64(a.Blstats.Hitpoints)), 1.0), 1.5)
		if IsMonsterFaster(a, m) {
			multiplier *= 2
		}
		// `mon in WEAK_MONSTERS` compares a permonst to strings: always False
		adjMonstersCount += float64(1 * multiplier)
		if IsDangerousMonster(m) {
			adjMonstersCount += float64(2 * multiplier)
		}
	}

	playerHpRatio := math.Sqrt(Div(float64(a.Blstats.Hitpoints), float64(a.Blstats.MaxHitpoints)))
	if a.Blstats.Hitpoints < 30 && adjMonstersCount > 0 {
		return []FightAction{{Priority: -15 + float64(20*adjMonstersCount*(1-playerHpRatio)), Kind: "elbereth"}}
	}
	return nil
}

func waitAction(a *world.Agent, monsters []world.Monster) []FightAction {
	if combatElbereth(a) {
		playerHpRatio := Div(float64(a.Blstats.Hitpoints), float64(a.Blstats.MaxHitpoints))
		priority := 30 - float64(playerHpRatio*40)
		return []FightAction{{Priority: priority, Kind: "wait"}}
	}
	return nil
}

func getAvailableActions(a *world.Agent, monsters []world.Monster) []FightAction {
	var actions []FightAction

	// melee attack actions
	for _, m := range monsters {
		y, x := m.Y, m.X
		if Adjacent(Pos{y, x}, Pos{a.Blstats.Y, a.Blstats.X}) {
			priority := meleeMonsterPriority(a, monsters, m)
			if combatElbereth(a) {
				priority -= 100
			}
			dy := y - a.Blstats.Y
			dx := x - a.Blstats.X
			actions = append(actions, FightAction{Priority: priority, Kind: "melee", Dy: dy, Dx: dx})
		}
	}

	// ranged attack actions
	for _, dy := range []int{-1, 0, 1} {
		for _, dx := range []int{-1, 0, 1} {
			if dy != 0 || dx != 0 {
				rp := rangedPriority(a, dy, dx, monsters)
				if rp != nil {
					pri := rp.pri
					if combatElbereth(a) {
						pri -= 100
					}
					if All(monsters, func(m world.Monster) bool { return Contains(ONLY_RANGED_SLOW_MONSTERS, m.Mon.Mname) }) {
						pri += 10
					}
					actions = append(actions, FightAction{Priority: pri, Kind: "ranged", Dy: dy, Dx: dx})
				}

				actions = append(actions, getPotentialWandUsages(a, monsters, dy, dx)...)
			}
		}
	}

	toPickup := FightDecideWhatToPickup(a)
	if len(toPickup) > 0 {
		actions = append(actions, FightAction{Priority: 15, Kind: "pickup", Items: toPickup, HasItems: true})
	}

	actions = append(actions, elberethAction(a, monsters)...)
	actions = append(actions, waitAction(a, monsters)...)

	return actions
}

// FightDecideWhatToPickup mirrors decide_what_to_pickup.
func FightDecideWhatToPickup(a *world.Agent) []*world.Item {
	if a.Inventory.ItemsBelowMeNone {
		Raise("TypeError", "'NoneType' object is not iterable")
	}
	projectilesBelowMe := Filter(a.Inventory.ItemsBelowMe, func(i *world.Item) bool {
		return i.IsThrownProjectile() || i.IsFiredProjectile(nil)
	})
	myLauncher, _ := GetBestRangedSet(a.Inventory, nil, true, false, false, false, projectilesBelowMe)
	return Filter(a.Inventory.ItemsBelowMe, func(item *world.Item) bool {
		return item.IsThrownProjectile() || (myLauncher != nil && item.IsFiredProjectile(myLauncher))
	})
}

func gotoAction(a *world.Agent, priority *F64Grid, monsters []world.Monster) []FightAction {
	values := map[float64]bool{}
	for _, d := range [][2]int{{-1, -1}, {-1, 0}, {-1, 1}, {0, 1}, {1, 1}, {1, 0}, {1, -1}, {0, -1}} {
		y, x := a.Blstats.Y-d[0], a.Blstats.X-d[1]
		if !(0 <= y && y < SY) || !(0 <= x && x < SX) {
			continue
		}
		if !math.IsNaN(priority[y][x]) {
			values[priority[y][x]] = true
		}
	}
	if len(values) > 1 {
		return nil
	}

	Assert(len(monsters) > 0)
	for _, m := range monsters {
		if !Adjacent(Pos{a.Blstats.Y, a.Blstats.X}, Pos{m.Y, m.X}) {
			// and not mon.mname in ONLY_RANGED_SLOW_MONSTERS:
			return []FightAction{{Priority: 1, Kind: "go_to", Dy: m.Y, Dx: m.X}}
		}
	}
	Assert(false, monsters)
	return nil
}

// FightGetPriorities mirrors get_priorities: (move priority heatmap, other actions list).
func FightGetPriorities(a *world.Agent) (F64Grid, []FightAction) {
	walkable := a.CurrentLevel().Walkable
	var priority F64Grid
	monsters := a.GetVisibleMonsters()
	for _, m := range monsters {
		drawMonsterPriorityPositive(a, m, &priority, &walkable)
	}
	for _, m := range monsters {
		drawMonsterPriorityNegative(a, m, &priority, &walkable)
	}
	for y := 0; y < SY; y++ { // priority[~walkable] = nan
		for x := 0; x < SX; x++ {
			if !walkable[y][x] {
				priority[y][x] = math.NaN()
			}
		}
	}

	// use relative priority to te current position
	cur := priority[a.Blstats.Y][a.Blstats.X]
	for y := 0; y < SY; y++ {
		for x := 0; x < SX; x++ {
			priority[y][x] -= cur
		}
	}

	actions := getAvailableActions(a, monsters)
	if !Any(actions, func(ac FightAction) bool { return ac.Kind == "melee" || ac.Kind == "ranged" }) {
		actions = append(actions, gotoAction(a, &priority, monsters)...)
	}
	return priority, actions
}

// FightGetMoveActions mirrors get_move_actions (keeps the duplicated (-1, -1) direction).
func FightGetMoveActions(a *world.Agent, dis *I32Grid, heat *F64Grid) []FightAction {
	var ret []FightAction
	for _, d := range [][2]int{{-1, -1}, {-1, 0}, {-1, 1}, {0, 1}, {1, 1}, {1, 0}, {1, -1}, {0, -1}, {-1, -1}} {
		dy, dx := d[0], d[1]
		y, x := a.Blstats.Y+dy, a.Blstats.X+dx
		if !(0 <= y && y < SY) || !(0 <= x && x < SX) {
			continue
		}
		if !(dis[y][x] == 1) {
			continue
		}

		if !math.IsNaN(heat[y][x]) {
			ret = append(ret, FightAction{Priority: heat[y][x], Kind: "move", Dy: dy, Dx: dx})
		}
	}
	return ret
}

// ---- combat

type FightAction struct {
	Priority float64
	Kind     string      // "move", "melee", "ranged", "zap", "elbereth", "wait", "pickup", "go_to"
	Dy, Dx   int         // for go_to: target y, x
	Wand     *world.Item // zap with 5-tuple
	HasWand  bool
	Items    []*world.Item // pickup with 2-tuple
	HasItems bool
}
